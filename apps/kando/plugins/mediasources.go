package plugins

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The functions of a script plugin that contributes media sources (docs/media-sources.md 11.2), which Kando calls for each source of theirs the user added: list(source, dir) lists a folder, stat(source, path) a file or folder, and open(source, path) answers where Kando reads a file from, which Kando then reads itself, so a video's bytes never pass through the plugin's code.
const (
	MediaSourceList = "list"
	MediaSourceStat = "stat"
	MediaSourceOpen = "open"
)

// The limits of a script plugin's media sources (docs/media-sources.md 3.2, 11.1, 11.4): the sources it contributes, the fields of each one's form and the secret ones among them, and the path prefixes it may POST to.
const (
	sourcesMax      = 4
	sourceFieldsMax = 12
	sourceSecretMax = 3
	postPathsMax    = 8
)

// The types of a media source's fields (MediaSourceField.Type), as mediasource.FieldType names them.
const (
	MediaFieldText   = "text"
	MediaFieldNumber = "number"
	MediaFieldSecret = "secret"
	MediaFieldBool   = "bool"
	MediaFieldURL    = "url"
)

// MediaSourceEndpoint is the key of the one url field of a media source: the server the user fills in, the only one Kando sends the plugin's requests and the source's secrets to.
const MediaSourceEndpoint = "endpoint"

// MediaSourceIcons are the Lucide icons a media source may name (MediaSource.Icon), which Kando draws itself, as of mediasource.Icons.
var MediaSourceIcons = []string{"server", "hard-drive", "cloud", "film"}

// MediaSource is a kind of media source a script plugin adds (docs/media-sources.md E17, 11.1): a server the user adds in Settings › 媒体来源 with the form its Fields describe, browses and imports films from. The plugin lists and finds its files through its list, stat and open exports, and reaches the server only through kando.source.fetch, which Kando sends to the address the user filled in (MediaSourceEndpoint) with the secrets put in as each field's Auth says. PostPaths are the path prefixes under that address the plugin may POST to, as a login or a listing of Alist's; it may POST nowhere else.
type MediaSource struct {
	ID        string             `json:"id"`
	Title     Text               `json:"title"`
	Icon      string             `json:"icon"`
	Fields    []MediaSourceField `json:"fields"`
	PostPaths []string           `json:"postPaths,omitempty"`
}

// MediaSourceField is a field of the form that adds a source of a plugin's kind, which Kando draws: its key, type (MediaFieldText and the others), title and the hint under it, if any; for a secret, Auth says how Kando puts it into the plugin's requests, as the plugin never sees it.
type MediaSourceField struct {
	Key   string           `json:"key"`
	Type  string           `json:"type"`
	Title Text             `json:"title"`
	Hint  Text             `json:"hint,omitempty"`
	Auth  *MediaSourceAuth `json:"auth,omitempty"`
}

// MediaSourceAuth is how Kando puts a secret field into a source's requests, one of: Basic, the keys of the user's text field and of the secret, for HTTP Basic; Header, a header Kando writes the secret into as Format with {secret} in its place ({secret} alone when empty); or Login, the secret written into the body of a login request the plugin marks, where {{secret:<key>}} stands for it.
type MediaSourceAuth struct {
	Basic  []string `json:"basic,omitempty"`
	Header string   `json:"header,omitempty"`
	Format string   `json:"format,omitempty"`
	Login  bool     `json:"login,omitempty"`
}

var (
	mediaFieldKeyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,31}$`)
	// mediaSourceHeaders are the headers a secret may not be put into: those the request's framing or Kando decide.
	mediaSourceHeaders = []string{"host", "cookie", "content-length", "content-type", "range", "connection", "transfer-encoding", "te", "upgrade", "proxy-authorization", "expect", "origin", "referer", "user-agent"}
)

// mediaSourceExcludes are what a plugin that contributes media sources may not ask for (ErrSourceNetwork, docs/media-sources.md 11.1): every other way out of Kando, and the lines a film says, so what it reads of the user's server and the session tokens it logs in with go back only to that server.
var mediaSourceExcludes = []Perm{PermCredFetch, PermLLMChat, PermOpenURL, PermLinesSeen}

// checkMediaSources checks the media sources of the script plugin m (docs/media-sources.md 11.1): at most sourcesMax, each with an id unique among them, a title, one of MediaSourceIcons, the fields of its form (checkMediaSourceFields) and at most postPathsMax POST prefixes; that a plugin contributes sources if and only if it asks for kando.source.fetch, among the permissions it needs; and that one that does reaches no other network and no credential, nor asks for the AI service, a page in the browser or the lines seen (ErrSourceNetwork).
func checkMediaSources(m *Manifest) error {
	var sources []MediaSource
	if m.Contributes != nil {
		sources = m.Contributes.Sources
	}
	declared := m.Declared()
	asks := declared.Has(PermSourceFetch)
	if asks != (len(sources) > 0) {
		return fmt.Errorf("%w: sources go with %s, and it with them", ErrBadField, PermSourceFetch)
	}
	if len(sources) == 0 {
		return nil
	}
	if slices.Contains(m.Optional, PermSourceFetch) {
		return fmt.Errorf("%w: %s is what its sources need, not optional", ErrBadField, PermSourceFetch)
	}
	if len(declared.Network) > 0 || len(m.Credentials) > 0 {
		return fmt.Errorf("%w: network rules or credentials", ErrSourceNetwork)
	}
	for _, p := range mediaSourceExcludes {
		if declared.Has(p) {
			return fmt.Errorf("%w: %s", ErrSourceNetwork, p)
		}
	}
	if len(sources) > sourcesMax {
		return fmt.Errorf("%w: at most %d sources", ErrBadField, sourcesMax)
	}
	for i := range sources {
		s := &sources[i]
		bad := func(what string) error { return fmt.Errorf("%w: source %s: %s", ErrBadField, s.ID, what) }
		if !commandPattern.MatchString(s.ID) || slices.ContainsFunc(sources[:i], func(o MediaSource) bool { return o.ID == s.ID }) {
			return bad("id")
		}
		if err := checkText("source title", s.Title, nameMax*2, true, false); err != nil {
			return err
		}
		if !slices.Contains(MediaSourceIcons, s.Icon) {
			return bad("icon " + s.Icon)
		}
		if err := checkMediaSourceFields(s.Fields); err != nil {
			return bad(err.Error())
		}
		if len(s.PostPaths) > postPathsMax {
			return bad(fmt.Sprintf("at most %d postPaths", postPathsMax))
		}
		for j, p := range s.PostPaths {
			if !prefixPattern.MatchString(p) || !plainPath(p) || slices.Contains(s.PostPaths[:j], p) {
				return bad("post path " + p)
			}
		}
	}
	return nil
}

// checkMediaSourceFields checks the fields of a source's form (docs/media-sources.md 3.2): 1 to sourceFieldsMax, of keys unique and of the shape ^[a-z][A-Za-z0-9]{0,31}$, of known types, each with a title, exactly one url field keyed MediaSourceEndpoint, at most sourceSecretMax secrets, each saying one way it is sent (MediaSourceAuth), and no Auth on a field that is not secret. The fields' texts are cleaned in place.
func checkMediaSourceFields(fields []MediaSourceField) error {
	if len(fields) == 0 || len(fields) > sourceFieldsMax {
		return fmt.Errorf("1 to %d fields", sourceFieldsMax)
	}
	types := map[string]string{}
	secrets, endpoints := 0, 0
	for i := range fields {
		f := &fields[i]
		if !mediaFieldKeyPattern.MatchString(f.Key) {
			return fmt.Errorf("field key %q", f.Key)
		}
		if _, dup := types[f.Key]; dup {
			return fmt.Errorf("field %q twice", f.Key)
		}
		types[f.Key] = f.Type
		if err := checkText("source field title", f.Title, nameMax*2, true, false); err != nil {
			return err
		}
		if f.Hint != nil {
			if err := checkText("source field hint", f.Hint, nameMax*4, true, false); err != nil {
				return err
			}
		}
		switch f.Type {
		case MediaFieldText, MediaFieldNumber, MediaFieldBool:
		case MediaFieldSecret:
			secrets++
		case MediaFieldURL:
			if f.Key != MediaSourceEndpoint {
				return fmt.Errorf("a url field is keyed %q", MediaSourceEndpoint)
			}
			endpoints++
		default:
			return fmt.Errorf("field %q of type %q", f.Key, f.Type)
		}
		if f.Auth != nil && f.Type != MediaFieldSecret {
			return fmt.Errorf("field %q is not secret but says how it is sent", f.Key)
		}
	}
	if endpoints != 1 {
		return fmt.Errorf("one %q field", MediaSourceEndpoint)
	}
	if secrets > sourceSecretMax {
		return fmt.Errorf("%d secret fields, at most %d", secrets, sourceSecretMax)
	}
	for _, f := range fields {
		if f.Type != MediaFieldSecret {
			continue
		}
		if err := f.Auth.check(f.Key, types); err != nil {
			return fmt.Errorf("secret %q: %w", f.Key, err)
		}
	}
	return nil
}

// check is nil for an Auth that says one way the secret of key is sent, else why not; types are the source's fields' types by key.
func (a *MediaSourceAuth) check(key string, types map[string]string) error {
	if a == nil {
		return fmt.Errorf("says not how it is sent")
	}
	ways := 0
	if a.Basic != nil {
		ways++
		if len(a.Basic) != 2 || types[a.Basic[0]] != MediaFieldText || a.Basic[1] != key {
			return fmt.Errorf("basic is [<a text field>, %q]", key)
		}
	}
	if a.Header != "" {
		ways++
		lower := strings.ToLower(a.Header)
		if !headerPattern.MatchString(a.Header) || slices.Contains(mediaSourceHeaders, lower) || strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "proxy-") {
			return fmt.Errorf("header %q", a.Header)
		}
		if a.Format != "" && (strings.Count(a.Format, SecretMark) != 1 || len(a.Format) > credFormatMax || strings.ContainsFunc(a.Format, func(r rune) bool { return r < ' ' || r == 0x7f })) {
			return fmt.Errorf("format %q", a.Format)
		}
	} else if a.Format != "" {
		return fmt.Errorf("a format without a header")
	}
	if a.Login {
		ways++
	}
	if ways != 1 {
		return fmt.Errorf("%d ways to send it, want one", ways)
	}
	return nil
}

// SecretKeys are the keys of the source's secret fields, in the order of its form.
func (s MediaSource) SecretKeys() []string {
	var keys []string
	for _, f := range s.Fields {
		if f.Type == MediaFieldSecret {
			keys = append(keys, f.Key)
		}
	}
	return keys
}

// AllowsPost reports whether the source lets its plugin POST to path, a path relative to its endpoint's: under one of its PostPaths by whole segments, as a credential's prefix takes a path (GrantedCredential.Allows), and only a plain path (plainPath).
func (s MediaSource) AllowsPost(path string) bool {
	if !plainPath(path) {
		return false
	}
	return slices.ContainsFunc(s.PostPaths, func(p string) bool {
		if !strings.HasPrefix(path, p) {
			return false
		}
		return strings.HasSuffix(p, "/") || len(path) == len(p) || path[len(p)] == '/'
	})
}
