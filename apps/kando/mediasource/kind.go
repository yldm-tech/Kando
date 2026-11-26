package mediasource

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Built-in kinds' IDs. Kando reads these sources itself; a script plugin's kind has an ID of PluginKindID instead.
const (
	KindSMB    = "smb"
	KindSFTP   = "sftp"
	KindWebDAV = "webdav"
)

// Kind is a kind of media source: one Kando reads itself (Origin.Plugin empty) or one a script plugin contributes. ID names it in the Registry and in a Source; Title is its name in a language, as the interface shows it; Icon is one of Icons; Fields are what the form that adds a source of it asks for (Field); Phone is whether a phone can reach such a source itself, without the computer, which only a built-in kind can, as a phone runs no plugin; Open connects to a source of the kind, with the values of its secret fields by key, and gives the Driver that reads it.
type Kind struct {
	ID     string
	Title  func(locale string) string
	Icon   string
	Fields []Field
	Phone  bool
	Origin Origin
	Open   func(ctx context.Context, s Source, secrets map[string]string) (Driver, error)
}

// Origin is where a Kind comes from: Kando itself when Plugin is empty, else the script plugin of that ID, which the list of sources and a plugin's details page say.
type Origin struct {
	Plugin string
}

// Builtin reports whether the kind is Kando's own.
func (o Origin) Builtin() bool { return o.Plugin == "" }

// Icons are the icons a kind may have, Lucide's names, which Kando draws itself rather than take a plugin's image.
var Icons = []string{"server", "hard-drive", "cloud", "film"}

// FieldType is what a field of a kind's form holds.
type FieldType string

const (
	FieldText   FieldType = "text"
	FieldNumber FieldType = "number"
	// FieldSecret is a password, private key, passphrase or token: kept in the system's secure storage, never in settings.json, and never given to a plugin's code.
	FieldSecret FieldType = "secret"
	FieldBool   FieldType = "bool"
	// FieldURL is the address of the server a plugin's source reaches, whose key is EndpointKey; no other field is one.
	FieldURL FieldType = "url"
	// FieldHostKey is the server's key a built-in kind trusts, as SFTP's: a line of authorized_keys, which the form shows as its fingerprint and which only the question asked at the first connection, or when the server offers another key, sets (remotefs.HostKeyError). It is not secret, so settings.json keeps it; a plugin's kind has none.
	FieldHostKey FieldType = "hostKey"
)

// EndpointKey is the key of the one FieldURL field a plugin's kind has: the server the user fills in, the only one Kando sends the plugin's requests and secrets to.
const EndpointKey = "endpoint"

// Field is a field of a kind's form, which Kando draws the same for every kind. Key names its value in Source.Config, or in the secrets; Title is its label in a language, and Hint, when set, the line under it, as what an empty field means; Default is the value it stands for while it is empty, as 445 for an SMB port. Multiline is a text or secret field of several lines, as a private key's PEM text, which shows what is typed in a small monospaced face rather than hiding it. Auth says, for a secret field of a plugin's kind, how Kando puts the secret into the plugin's requests, which never see its value; a built-in kind's secrets are used by Kando's own code and have none.
type Field struct {
	Key       string
	Type      FieldType
	Title     func(locale string) string
	Hint      func(locale string) string
	Default   string
	Multiline bool
	Auth      *Auth
}

// Auth is how Kando puts a secret into the requests a plugin's source makes, set as one of: Basic, the keys of the user's field and of the secret, for HTTP Basic; Header, a header Kando writes the secret into, as Format with {secret} replaced, as X-Emby-Token; or Login, for a secret Kando writes into the body of the plugin's login request alone. SignIn, beside Header, is the account the form signs in to for the person, which fills the secret and the endpoint: SignInPlex, a Plex account's (package plexlink).
type Auth struct {
	Basic  []string
	Header string
	Format string
	Login  bool
	SignIn string
}

// SignInPlex is the SignIn of a secret the form fills by signing in to Plex (package plexlink).
const SignInPlex = "plex"

// The limits of a kind's form.
const (
	fieldsMax  = 12
	secretsMax = 3
)

var (
	// builtinPattern is a built-in kind's ID.
	builtinPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)
	// pluginPattern is a plugin's ID, as apps/kando/plugins takes it: lowercase words of letters and digits joined by single dots or hyphens, at most 64 bytes.
	pluginPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
	// contribPattern is the ID of a source a plugin contributes, as of its commands.
	contribPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	keyPattern     = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,31}$`)
	headerPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)
)

// pluginPrefix starts the ID of a plugin's kind.
const pluginPrefix = "plugin:"

// PluginKindID is the ID of the kind of source the plugin of ID plugin contributes as source: plugin:<plugin>:<source>.
func PluginKindID(plugin, source string) string {
	return pluginPrefix + plugin + ":" + source
}

// headersRefused are the headers a secret may not be put into: those the request itself, or Kando, decides.
var headersRefused = []string{"host", "cookie", "content-length", "content-type", "range", "connection", "transfer-encoding", "te", "upgrade", "proxy-authorization"}

// ErrInvalidKind is the error of a Kind the Registry does not take, wrapped with why.
var ErrInvalidKind = errors.New("mediasource: invalid kind")

// Validate is nil for a kind the Registry can take, else ErrInvalidKind with why: an ID of the shape of its Origin (a built-in name, or PluginKindID of its plugin), a title, one of Icons, a way to open it, at most 12 fields, 3 of them secret, of known types and keys unique and of the shape ^[a-z][A-Za-z0-9]{0,31}$; a plugin's kind has exactly one FieldURL field, keyed EndpointKey, and says how each of its secrets is used (Auth); a built-in kind has neither, but may have one FieldHostKey field, which a plugin's kind may not, and only a text or secret field is of several lines; no phone can reach a plugin's kind.
func (k Kind) Validate() error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w %q: %s", ErrInvalidKind, k.ID, fmt.Sprintf(format, args...))
	}
	if k.Origin.Builtin() {
		if !builtinPattern.MatchString(k.ID) {
			return fail("not the ID of a built-in kind")
		}
	} else {
		if len(k.Origin.Plugin) > 64 || !pluginPattern.MatchString(k.Origin.Plugin) {
			return fail("not a plugin's ID: %q", k.Origin.Plugin)
		}
		contrib, ok := strings.CutPrefix(k.ID, pluginPrefix+k.Origin.Plugin+":")
		if !ok || !contribPattern.MatchString(contrib) {
			return fail("a plugin's kind is plugin:<plugin>:<source>")
		}
		if k.Phone {
			return fail("a phone runs no plugin")
		}
	}
	if k.Title == nil || k.Open == nil {
		return fail("no title or no way to open it")
	}
	known := false
	for _, icon := range Icons {
		known = known || icon == k.Icon
	}
	if !known {
		return fail("icon %q", k.Icon)
	}
	if len(k.Fields) > fieldsMax {
		return fail("%d fields, at most %d", len(k.Fields), fieldsMax)
	}
	types := map[string]FieldType{}
	secrets, endpoints, hostKeys := 0, 0, 0
	for _, f := range k.Fields {
		if !keyPattern.MatchString(f.Key) {
			return fail("field key %q", f.Key)
		}
		if _, dup := types[f.Key]; dup {
			return fail("field %q twice", f.Key)
		}
		types[f.Key] = f.Type
		if f.Title == nil {
			return fail("field %q has no title", f.Key)
		}
		switch f.Type {
		case FieldText, FieldNumber, FieldBool:
		case FieldSecret:
			secrets++
		case FieldURL:
			if f.Key != EndpointKey {
				return fail("a url field is keyed %q", EndpointKey)
			}
			endpoints++
		case FieldHostKey:
			hostKeys++
		default:
			return fail("field %q of type %q", f.Key, f.Type)
		}
		if f.Auth != nil && f.Type != FieldSecret {
			return fail("field %q is not secret but says how it is sent", f.Key)
		}
		if f.Multiline && f.Type != FieldText && f.Type != FieldSecret {
			return fail("field %q of type %q is not of several lines", f.Key, f.Type)
		}
	}
	if secrets > secretsMax {
		return fail("%d secret fields, at most %d", secrets, secretsMax)
	}
	if hostKeys > 1 || hostKeys == 1 && !k.Origin.Builtin() {
		return fail("%d host key fields, which only a built-in kind has, one at most", hostKeys)
	}
	if k.Origin.Builtin() {
		if endpoints != 0 {
			return fail("a built-in kind has no endpoint")
		}
		for _, f := range k.Fields {
			if f.Auth != nil {
				return fail("a built-in kind's secret %q needs no Auth", f.Key)
			}
		}
		return nil
	}
	if endpoints != 1 {
		return fail("a plugin's kind has one %q field", EndpointKey)
	}
	for _, f := range k.Fields {
		if f.Type != FieldSecret {
			continue
		}
		if err := f.Auth.check(f.Key, types); err != nil {
			return fail("secret %q: %v", f.Key, err)
		}
	}
	return nil
}

// check is nil for an Auth that says one way the secret of key is used, else why not; types are the kind's fields' types by key.
func (a *Auth) check(key string, types map[string]FieldType) error {
	if a == nil {
		return errors.New("says not how it is sent")
	}
	ways := 0
	if a.Basic != nil {
		ways++
		if len(a.Basic) != 2 || types[a.Basic[0]] != FieldText || a.Basic[1] != key {
			return errors.New(`basic is [<a text field>, <this field>]`)
		}
	}
	if a.Header != "" {
		ways++
		if !headerPattern.MatchString(a.Header) {
			return fmt.Errorf("header %q", a.Header)
		}
		for _, h := range headersRefused {
			if strings.EqualFold(a.Header, h) {
				return fmt.Errorf("header %q is not a plugin's", a.Header)
			}
		}
		if a.Format != "" && !strings.Contains(a.Format, "{secret}") {
			return errors.New("a format without {secret}")
		}
	} else if a.Format != "" {
		return errors.New("a format without a header")
	}
	if a.Login {
		ways++
	}
	if ways != 1 {
		return fmt.Errorf("%d ways to send it, want one", ways)
	}
	if a.SignIn != "" && (a.SignIn != SignInPlex || a.Header == "") {
		return fmt.Errorf("signIn %q", a.SignIn)
	}
	return nil
}

// SignIn is the account the kind's form signs in to for the person (Auth.SignIn), and the key of the secret it fills; "" for none.
func (k Kind) SignIn() (account, key string) {
	for _, f := range k.Fields {
		if f.Auth != nil && f.Auth.SignIn != "" {
			return f.Auth.SignIn, f.Key
		}
	}
	return "", ""
}

// SecretKeys are the keys of the kind's secret fields, in the order of its form.
func (k Kind) SecretKeys() []string {
	var keys []string
	for _, f := range k.Fields {
		if f.Type == FieldSecret {
			keys = append(keys, f.Key)
		}
	}
	return keys
}
