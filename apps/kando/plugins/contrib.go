package plugins

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/publicsuffix"
)

// The limits of a script plugin's contributions besides its commands, and of its credentials.
const (
	contribMax      = 8
	credentialsMax  = 4
	credHostsMax    = 8
	credPrefixesMax = 8
	credFormatMax   = 200
	extensionMax    = 8
)

// Lookup is a source of word lookups a script plugin adds (docs/plugin-system.md E10): its export, called as lookUp(query), answers with an entry or null, after the dictionary packs and before the AI service. Languages are the languages of the words it answers, as language tags; none answers words of every language.
type Lookup struct {
	ID        string   `json:"id"`
	Export    string   `json:"export"`
	Languages []string `json:"languages,omitempty"`
	Label     Text     `json:"label"`
}

// SubtitleExtensions are the extensions a subtitle format of a script plugin may write its file with, beside the film's video: subtitle and text formats, never one a system runs or opens as a program (bat, exe, js, ps1 and the like).
var SubtitleExtensions = []string{"srt", "vtt", "ass", "ssa", "sub", "lrc", "txt", "tsv", "csv", "json", "md", "ttml", "xml", "sbv", "smi"}

// SubtitleFormat is a format the subtitle export offers that a script plugin writes (E11): its export, called as exportSubtitles(formatID, cues), returns the file's text, which Kando writes with Extension, one of SubtitleExtensions, without its dot.
type SubtitleFormat struct {
	ID        string `json:"id"`
	Export    string `json:"export"`
	Extension string `json:"extension"`
	Label     Text   `json:"label"`
}

// What an exporter takes (Exporter.Takes): the saved lines, the words of the vocabulary.
const (
	TakesLines = "lines"
	TakesWords = "words"
)

// Exporter is a target the saved lines' export offers that a script plugin sends them to (E12), as to Anki through AnkiConnect: its export, called as exportNotes(items, settings), answers {sent, skipped}. Takes says whether it takes the saved lines, the words, or both.
type Exporter struct {
	ID     string   `json:"id"`
	Export string   `json:"export"`
	Takes  []string `json:"takes"`
	Label  Text     `json:"label"`
}

// Annotator marks the words of a film's lines as the word lists do (E13): its export, called as annotate(lang, lines) with at most 50 lines at a time, answers each line's marks. Languages are the languages of the films it marks; none marks every film.
type Annotator struct {
	ID        string   `json:"id"`
	Export    string   `json:"export"`
	Languages []string `json:"languages,omitempty"`
	Label     Text     `json:"label"`
}

// Panel is a panel a script plugin adds to the 扩展 tab of the player's side panel (E15): its code exports Export, a TSX component, which the view of the panel mounts (script.Runner.Panel); Title names it in the tab's picker, and Icon, one of PanelIcons, draws beside it, none when empty.
type Panel struct {
	ID     string `json:"id"`
	Export string `json:"export"`
	Title  Text   `json:"title"`
	Icon   string `json:"icon,omitempty"`
}

// PanelIcons are the Lucide icons a panel may name (Panel.Icon), by their Lucide names: a short list Kando draws itself, never an image of the plugin's, so a panel cannot dress as one of Kando's own.
var PanelIcons = []string{"book-a", "book-open", "brain", "captions", "chart-bar", "graduation-cap", "highlighter", "languages", "lightbulb", "list-checks", "message-square", "notebook-pen", "puzzle", "search", "sparkles", "star"}

// Credential is a secret of a third-party service a script plugin uses through Kando alone (kando.net.credentialFetch, docs/plugin-system.md 6.3): the user enters it in a field Kando draws, Kando keeps it where the plugin's code cannot reach it, and puts it in the request header Header, written as Format with {secret} where it goes, only for a request to one of Hosts (https) with one of Methods and a path under one of PathPrefixes.
type Credential struct {
	ID           string   `json:"id"`
	Title        Text     `json:"title"`
	Header       string   `json:"header"`
	Format       string   `json:"format"`
	Hosts        []string `json:"hosts"`
	Methods      []string `json:"methods"`
	PathPrefixes []string `json:"pathPrefixes"`
}

// SecretMark is where the secret goes in a credential's Format.
const SecretMark = "{secret}"

// Grant is the part of c a grant holds, everything but its title: what decides where the secret is sent and how. Two credentials with the same grant send it alike.
func (c Credential) Grant() GrantedCredential {
	return GrantedCredential{ID: c.ID, Header: c.Header, Format: c.Format, Hosts: slices.Clone(c.Hosts), Methods: slices.Clone(c.Methods), PathPrefixes: slices.Clone(c.PathPrefixes)}
}

// GrantedCredential is a credential as the user granted it (Grant.Credentials): the whole tuple, so an update that changes any part of it, as a host added, is a credential the user never granted.
type GrantedCredential struct {
	ID           string   `json:"id"`
	Header       string   `json:"header"`
	Format       string   `json:"format"`
	Hosts        []string `json:"hosts"`
	Methods      []string `json:"methods"`
	PathPrefixes []string `json:"pathPrefixes"`
}

// equal reports whether c and o are the same in every part.
func (c GrantedCredential) equal(o GrantedCredential) bool {
	return c.ID == o.ID && c.Header == o.Header && c.Format == o.Format && slices.Equal(c.Hosts, o.Hosts) && slices.Equal(c.Methods, o.Methods) && slices.Equal(c.PathPrefixes, o.PathPrefixes)
}

// Allows reports whether c lets its secret go with a request of method to host and path: host one of its hosts, as written, method one of its methods, and path under one of its prefixes, by whole segments when the prefix does not end in a slash, so /v2/translate takes /v2/translate and /v2/translate/x but not /v2/translated; and only a plain path (plainPath), so /v2/translate/../admin is under none.
func (c GrantedCredential) Allows(method, host, path string) bool {
	if !slices.Contains(c.Hosts, strings.ToLower(host)) || !slices.Contains(c.Methods, method) || !plainPath(path) {
		return false
	}
	return slices.ContainsFunc(c.PathPrefixes, func(p string) bool {
		if !strings.HasPrefix(path, p) {
			return false
		}
		return strings.HasSuffix(p, "/") || len(path) == len(p) || path[len(p)] == '/' || path[len(p)] == '?'
	})
}

// plainPath reports whether path, as a URL writes it, names one place a server cannot read as another: it starts with a slash and has no segment . or .., no empty segment, no escaped slash, backslash or dot, and no backslash, so a path under a prefix stays under it whatever the server makes of it.
func plainPath(path string) bool {
	lower := strings.ToLower(path)
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%2e") {
		return false
	}
	segments := strings.Split(path[1:], "/")
	for i, s := range segments {
		if s == "." || s == ".." || s == "" && i < len(segments)-1 {
			return false
		}
	}
	return true
}

// Value is the header's value with secret in its place.
func (c GrantedCredential) Value(secret string) string {
	return strings.Replace(c.Format, SecretMark, secret, 1)
}

// SecretKeys are the keys of what Kando keeps for the script plugin m in its host partition: its credentials' (SecretKey), and those of the secret settings it was installed with (Setting.LegacySecret), so what the user saved under them goes with the plugin.
func (m Manifest) SecretKeys() []string {
	var keys []string
	for _, st := range m.Settings {
		if st.LegacySecret() {
			keys = append(keys, st.Key)
		}
	}
	for _, c := range m.Credentials {
		keys = append(keys, SecretKey(c.ID))
	}
	return keys
}

// SecretKey is the key Kando keeps the secret of the credential id under, in the plugin's host partition.
func SecretKey(id string) string { return "credential." + id }

var (
	headerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)
	hostPattern   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
	prefixPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,127}$`)
)

// credMethods are the methods a credential may name; credHeaders the headers it may not set, which the request's own framing or Kando's own say.
var (
	credMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	credHeaders = []string{"host", "cookie", "content-length", "content-type", "transfer-encoding", "connection", "proxy-authorization", "te", "upgrade", "expect", "origin", "referer", "user-agent"}
)

// checkContributes checks what a script plugin adds besides its commands: at most contribMax of each kind, each with an id unique among its kind, an export, a label (a panel's title), and what its kind needs.
func checkContributes(c *Contributes) error {
	if len(c.Lookups) > contribMax || len(c.SubtitleFormats) > contribMax || len(c.Exporters) > contribMax || len(c.Annotators) > contribMax || len(c.Panels) > contribMax {
		return fmt.Errorf("%w: at most %d contributions of a kind", ErrBadField, contribMax)
	}
	check := func(what string, ids, exports []string, labels []Text) error {
		for i := range ids {
			if !commandPattern.MatchString(ids[i]) || slices.Contains(ids[:i], ids[i]) {
				return fmt.Errorf("%w: %s id %q", ErrBadField, what, ids[i])
			}
			if !exportPattern.MatchString(exports[i]) {
				return fmt.Errorf("%w: %s export %q", ErrBadField, what, exports[i])
			}
			if err := checkText(what+" label", labels[i], nameMax*2, true, false); err != nil {
				return err
			}
		}
		return nil
	}
	var ids, exports []string
	var labels []Text
	reset := func() { ids, exports, labels = nil, nil, nil }
	for _, l := range c.Lookups {
		ids, exports, labels = append(ids, l.ID), append(exports, l.Export), append(labels, l.Label)
		if err := checkLanguages("lookup languages", l.Languages); err != nil && len(l.Languages) > 0 {
			return err
		}
	}
	if err := check("lookup", ids, exports, labels); err != nil {
		return err
	}
	reset()
	for _, f := range c.SubtitleFormats {
		ids, exports, labels = append(ids, f.ID), append(exports, f.Export), append(labels, f.Label)
		if !slices.Contains(SubtitleExtensions, f.Extension) {
			return fmt.Errorf("%w: subtitle format extension %q", ErrBadField, f.Extension)
		}
	}
	if err := check("subtitle format", ids, exports, labels); err != nil {
		return err
	}
	reset()
	for _, e := range c.Exporters {
		ids, exports, labels = append(ids, e.ID), append(exports, e.Export), append(labels, e.Label)
		if len(e.Takes) == 0 || len(e.Takes) > 2 {
			return fmt.Errorf("%w: an exporter takes lines, words or both", ErrBadField)
		}
		for i, t := range e.Takes {
			if t != TakesLines && t != TakesWords || slices.Contains(e.Takes[:i], t) {
				return fmt.Errorf("%w: exporter takes %q", ErrBadField, t)
			}
		}
	}
	if err := check("exporter", ids, exports, labels); err != nil {
		return err
	}
	reset()
	for _, an := range c.Annotators {
		ids, exports, labels = append(ids, an.ID), append(exports, an.Export), append(labels, an.Label)
		if err := checkLanguages("annotator languages", an.Languages); err != nil && len(an.Languages) > 0 {
			return err
		}
	}
	if err := check("annotator", ids, exports, labels); err != nil {
		return err
	}
	reset()
	for _, pn := range c.Panels {
		ids, exports, labels = append(ids, pn.ID), append(exports, pn.Export), append(labels, pn.Title)
		if pn.Icon != "" && !slices.Contains(PanelIcons, pn.Icon) {
			return fmt.Errorf("%w: panel icon %q", ErrBadField, pn.Icon)
		}
	}
	return check("panel", ids, exports, labels)
}

// checkCredentials checks a script plugin's credentials: at most credentialsMax, each with an id unique among them, a title, a header Kando lets it set, a format with the secret once and no line break, 1 to credHostsMax hosts that are host names under a registrable domain (no IP, no wildcard, no public suffix), methods among credMethods, and 1 to credPrefixesMax path prefixes; and that a plugin has credentials if and only if it asks for kando.net.credentialFetch.
func checkCredentials(m *Manifest) error {
	if len(m.Credentials) > credentialsMax {
		return fmt.Errorf("%w: at most %d credentials", ErrBadField, credentialsMax)
	}
	asks := slices.Contains(m.Declared().Perms, PermCredFetch)
	if asks != (len(m.Credentials) > 0) {
		return fmt.Errorf("%w: credentials go with %s, and it with them", ErrBadField, PermCredFetch)
	}
	for i := range m.Credentials {
		c := &m.Credentials[i]
		bad := func(what string) error { return fmt.Errorf("%w: credential %s: %s", ErrBadField, c.ID, what) }
		if !commandPattern.MatchString(c.ID) || slices.ContainsFunc(m.Credentials[:i], func(o Credential) bool { return o.ID == c.ID }) {
			return bad("id")
		}
		if err := checkText("credential title", c.Title, nameMax*2, true, false); err != nil {
			return err
		}
		if !headerPattern.MatchString(c.Header) || slices.Contains(credHeaders, strings.ToLower(c.Header)) || strings.HasPrefix(strings.ToLower(c.Header), "sec-") || strings.HasPrefix(strings.ToLower(c.Header), "proxy-") {
			return bad("header")
		}
		if strings.Count(c.Format, SecretMark) != 1 || utf8.RuneCountInString(c.Format) > credFormatMax || strings.ContainsFunc(c.Format, func(r rune) bool { return r < ' ' || r == 0x7f }) {
			return bad("format")
		}
		if len(c.Hosts) == 0 || len(c.Hosts) > credHostsMax {
			return bad("hosts")
		}
		for j, h := range c.Hosts {
			if !hostPattern.MatchString(h) || net.ParseIP(h) != nil || slices.Contains(c.Hosts[:j], h) {
				return bad("host " + h)
			}
			if _, err := publicsuffix.EffectiveTLDPlusOne(h); err != nil {
				return bad("host " + h + " is a public suffix")
			}
		}
		if len(c.Methods) == 0 {
			return bad("methods")
		}
		for j, method := range c.Methods {
			if !slices.Contains(credMethods, method) || slices.Contains(c.Methods[:j], method) {
				return bad("method " + method)
			}
		}
		if len(c.PathPrefixes) == 0 || len(c.PathPrefixes) > credPrefixesMax {
			return bad("pathPrefixes")
		}
		for j, p := range c.PathPrefixes {
			if !prefixPattern.MatchString(p) || strings.Contains(p, "//") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") || strings.Contains(p, "/./") || slices.Contains(c.PathPrefixes[:j], p) {
				return bad("path prefix " + p)
			}
		}
	}
	return nil
}

// ExportsNeeded are the functions of the script plugin m its contributions call, each once, which its code must export.
func (m Manifest) ExportsNeeded() []string {
	c := m.Contributes
	if c == nil {
		return nil
	}
	var out []string
	add := func(e string) {
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	for _, x := range c.Commands {
		add(x.Export)
	}
	for _, x := range c.Lookups {
		add(x.Export)
	}
	for _, x := range c.SubtitleFormats {
		add(x.Export)
	}
	for _, x := range c.Exporters {
		add(x.Export)
	}
	for _, x := range c.Annotators {
		add(x.Export)
	}
	if len(c.Translators) > 0 {
		add(TranslateExport)
	}
	for _, x := range c.Panels {
		add(x.Export)
	}
	if len(c.Sources) > 0 {
		add(MediaSourceList)
		add(MediaSourceStat)
		add(MediaSourceOpen)
	}
	return out
}

// ReadsLines reports whether the plugin m is given the words of a film's lines or of the saved lines without asking a permission for them, which its contributions are: a lookup source hears each word looked up and its line, a subtitle format the cues it writes, an export target the saved lines and words, an annotator the lines it marks, a translator the lines it translates.
func (m Manifest) ReadsLines() bool {
	c := m.Contributes
	return c != nil && (len(c.Lookups) > 0 || len(c.SubtitleFormats) > 0 || len(c.Exporters) > 0 || len(c.Annotators) > 0 || len(c.Translators) > 0)
}

// KnownService is a well-known service: its name, in lower case, as a credential's id or title would say it, and the hosts of its API.
type KnownService struct {
	Name  string
	Hosts []string
}

// Services is a table of well-known services and the hosts their APIs are on, which a credential that names one of them is expected to be sent to (docs/plugin-system.md 6.3): a plugin whose credential's id or title names DeepL but sends it elsewhere is flagged.
var Services = []KnownService{
	{"deepl", []string{"api.deepl.com", "api-free.deepl.com"}},
	{"notion", []string{"api.notion.com"}},
	{"openai", []string{"api.openai.com"}},
	{"anthropic", []string{"api.anthropic.com"}},
	{"claude", []string{"api.anthropic.com"}},
	{"gemini", []string{"generativelanguage.googleapis.com"}},
	{"google", []string{"translation.googleapis.com", "generativelanguage.googleapis.com", "www.googleapis.com"}},
	{"azure", []string{"api.cognitive.microsofttranslator.com"}},
	{"microsoft", []string{"api.cognitive.microsofttranslator.com"}},
	{"openrouter", []string{"openrouter.ai"}},
	{"deepseek", []string{"api.deepseek.com"}},
	{"anki", []string{"ankiweb.net"}},
	{"github", []string{"api.github.com"}},
	{"airtable", []string{"api.airtable.com"}},
	{"readwise", []string{"readwise.io"}},
}

// Mismatched is the well-known service the credential c names, by a whole word of its id or English title, whose hosts c's hosts are not all among: "" when it names none, or sends its secret only where that service is. "banking" names no Anki.
func (c Credential) Mismatched() string {
	words := strings.FieldsFunc(strings.ToLower(c.ID+" "+c.Title["en"]), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, s := range Services {
		if !slices.Contains(words, s.Name) {
			continue
		}
		for _, h := range c.Hosts {
			if !slices.Contains(s.Hosts, h) {
				return s.Name
			}
		}
	}
	return ""
}

// PluginHeader reports whether a plugin may set the header name on a request it sends with a credential whose header is credHeader: a header's name, not the credential's own, and none the request's framing or Kando sets (Host, Cookie and the like), but Content-Type, which a body needs.
func PluginHeader(name, credHeader string) bool {
	lower := strings.ToLower(name)
	if !headerPattern.MatchString(name) || strings.EqualFold(name, credHeader) || strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "proxy-") {
		return false
	}
	return lower == "content-type" || !slices.Contains(credHeaders, lower)
}

// KnownHosts reports whether every one of hosts is a host of a service of Services.
func KnownHosts(hosts []string) bool {
	for _, h := range hosts {
		if !slices.ContainsFunc(Services, func(s KnownService) bool { return slices.Contains(s.Hosts, h) }) {
			return false
		}
	}
	return true
}
