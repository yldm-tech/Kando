// Package nas is what the phone knows of the NAS and servers it reaches itself, without the computer (docs/media-sources.md 10): the kinds it speaks (SMB, SFTP and WebDAV, the computer's built-in kinds of media source, by the same IDs and field keys), how a source of each is reached (Remote), what keeps a source from being tried (Check), a connection that reads its folders and files (Open), and which of the files listed the phone shows (Listing). The protocols are kando/packages/remotefs's, the computer's own. Nothing here touches the interface or keeps anything: the session holds the sources (kando/apps/mobile/session) and the store keeps them (store.Source).
package nas

import (
	"crypto/rand"
	"encoding/hex"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"kando/apps/mobile/store"
	"kando/packages/remotefs"
)

// The kinds of source the phone reaches, by the IDs the computer's media sources have.
const (
	KindSMB    = "smb"
	KindSFTP   = "sftp"
	KindWebDAV = "webdav"
)

// Kinds are the kinds the form offers, in its order.
var Kinds = []string{KindSMB, KindSFTP, KindWebDAV}

// The keys of a source's fields, the computer's (kando/apps/kando/mediasource): the server, its port, an SMB share and a folder, who signs in and the domain, the password, an SFTP private key and its passphrase, which are secret, SMB's switch that does without signing, the SFTP server's key trusted, a WebDAV server's address, and the fingerprint of the certificate trusted for it (CertKey), which only the phone's WebDAV has.
const (
	Host       = "host"
	Port       = "port"
	Share      = "share"
	Root       = "root"
	User       = "user"
	Domain     = "domain"
	Password   = "password"
	PrivateKey = "privateKey"
	Passphrase = "passphrase"
	NoSigning  = "noSigning"
	HostKey    = "hostKey"
	URL        = "url"
	CertKey    = "tls.fingerprint"
)

// FieldType is how a field is filled in.
type FieldType uint8

const (
	// Text is a line of text.
	Text FieldType = iota
	// Number is a port.
	Number
	// Secret is kept in the platform's secure store, never in sources.json.
	Secret
	// Switch is on ("true") or off.
	Switch
	// Trusted is a server's key or certificate, which the first connection asks to trust rather than the user typing it.
	Trusted
)

// Field is a field of a kind's form: its key, how it is filled in, and whether a Text or Secret holds lines, as a private key's PEM text.
type Field struct {
	Key       string
	Type      FieldType
	Multiline bool
}

// Fields are the fields of kind's form, in the computer's order; none for a kind the phone does not know.
func Fields(kind string) []Field {
	switch kind {
	case KindSMB:
		return []Field{{Key: Host}, {Key: Share}, {Key: Root}, {Key: User}, {Key: Password, Type: Secret}, {Key: NoSigning, Type: Switch}, {Key: Domain}, {Key: Port, Type: Number}}
	case KindSFTP:
		return []Field{{Key: Host}, {Key: Root}, {Key: User}, {Key: Password, Type: Secret}, {Key: PrivateKey, Type: Secret, Multiline: true}, {Key: Passphrase, Type: Secret}, {Key: Port, Type: Number}, {Key: HostKey, Type: Trusted}}
	case KindWebDAV:
		return []Field{{Key: URL}, {Key: Root}, {Key: User}, {Key: Password, Type: Secret}, {Key: CertKey, Type: Trusted}}
	}
	return nil
}

// SecretKeys are the keys of kind's secret fields.
func SecretKeys(kind string) []string {
	var out []string
	for _, f := range Fields(kind) {
		if f.Type == Secret {
			out = append(out, f.Key)
		}
	}
	return out
}

// DefaultPort is the port a source of kind is reached at when it names none, 0 for a kind without one.
func DefaultPort(kind string) int {
	switch kind {
	case KindSMB:
		return 445
	case KindSFTP:
		return 22
	}
	return 0
}

// Config is values kept to the fields of kind that are not secret, each trimmed, without the empty ones and a switch that is not on; nil for none. A trusted key or certificate is kept as it is.
func Config(kind string, values map[string]string) map[string]string {
	var out map[string]string
	for _, f := range Fields(kind) {
		v := strings.TrimSpace(values[f.Key])
		if f.Type == Secret || v == "" || f.Type == Switch && v != "true" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[f.Key] = v
	}
	return out
}

// NewID is a new source's ID: 16 random hexadecimal digits, as the computer's.
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// port is the port c names, 0 for none or one that is not a port.
func port(c map[string]string) int {
	n, err := strconv.Atoi(strings.TrimSpace(c[Port]))
	if err != nil || n < 1 || n > 65535 {
		return 0
	}
	return n
}

// Remote is how remotefs reaches s with its secrets by key: SMB requires signing unless its switch says the NAS does without, SFTP checks the server's key against the one trusted, and WebDAV takes the certificate trusted in place of the system's verification. The passphrase is kept as typed, as a space may be part of it.
func Remote(s store.Source, secrets map[string]string) remotefs.Remote {
	c := s.Config
	switch s.Kind {
	case KindSMB:
		return remotefs.Remote{Kind: remotefs.KindSMB, Host: strings.TrimSpace(c[Host]), Port: port(c), Share: strings.Trim(strings.TrimSpace(c[Share]), `/\`),
			Root: strings.TrimSpace(c[Root]), User: strings.TrimSpace(c[User]), Domain: strings.TrimSpace(c[Domain]), Password: secrets[Password], RequireSigning: c[NoSigning] != "true"}
	case KindSFTP:
		return remotefs.Remote{Kind: remotefs.KindSFTP, Host: strings.TrimSpace(c[Host]), Port: port(c), Root: strings.TrimSpace(c[Root]), User: strings.TrimSpace(c[User]),
			Password: secrets[Password], PrivateKey: strings.TrimSpace(secrets[PrivateKey]), Passphrase: secrets[Passphrase], HostKey: strings.TrimSpace(c[HostKey])}
	case KindWebDAV:
		return remotefs.Remote{Kind: remotefs.KindWebDAV, URL: strings.TrimSpace(c[URL]), Root: strings.TrimSpace(c[Root]), User: strings.TrimSpace(c[User]), Password: secrets[Password],
			CertFingerprint: strings.TrimSpace(c[CertKey])}
	}
	return remotefs.Remote{}
}

// Problem is what keeps a source from being tried or saved (Check).
type Problem uint8

const (
	NoProblem Problem = iota
	// NeedsHost is a server not filled in.
	NeedsHost
	// NeedsPort is a port that is not a number from 1 to 65535.
	NeedsPort
	// NeedsUser is a user not filled in.
	NeedsUser
	// NeedsSecret is a new SFTP source with neither a password nor a private key.
	NeedsSecret
	// NeedsShare is an SMB share that holds a slash.
	NeedsShare
	// NeedsShareForRoot is an SMB folder without a share.
	NeedsShareForRoot
	// NeedsURL is a WebDAV address that is not a plain https:// or http:// one.
	NeedsURL
)

// Check is what keeps s from being tried or saved with the secrets typed, as the computer's form checks it: a WebDAV source needs a plain https:// or http:// address; SMB and SFTP need a server, a port that is a number and a user; an SMB source needs a share of one name, and a share for a folder; a new SFTP source (saved false) needs a password, a private key or both, which one saved keeps in the secure store while its fields are left empty.
func Check(s store.Source, typed map[string]string, saved bool) Problem {
	c := s.Config
	if s.Kind == KindWebDAV {
		u, err := url.Parse(strings.TrimSpace(c[URL]))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return NeedsURL
		}
		return NoProblem
	}
	if strings.TrimSpace(c[Host]) == "" {
		return NeedsHost
	}
	if p := strings.TrimSpace(c[Port]); p != "" && port(c) == 0 {
		return NeedsPort
	}
	if s.Kind == KindSFTP {
		if strings.TrimSpace(c[User]) == "" {
			return NeedsUser
		}
		if !saved && typed[Password] == "" && strings.TrimSpace(typed[PrivateKey]) == "" {
			return NeedsSecret
		}
		return NoProblem
	}
	share := strings.Trim(strings.TrimSpace(c[Share]), `/\`)
	if strings.ContainsAny(share, `/\`) {
		return NeedsShare
	}
	if share == "" && strings.Trim(strings.TrimSpace(c[Root]), "/") != "" {
		return NeedsShareForRoot
	}
	if strings.TrimSpace(c[User]) == "" {
		return NeedsUser
	}
	return NoProblem
}

// Place is where s is, as its row says it: the server and the share and folder for SMB, the server and folder for SFTP, the address's server and path and the folder for WebDAV.
func Place(s store.Source) string {
	r := Remote(s, nil)
	parts := []string{r.Host}
	if r.Kind == remotefs.KindWebDAV {
		parts = []string{r.URL}
		if u, err := url.Parse(r.URL); err == nil && u.Host != "" {
			parts = []string{u.Host + strings.TrimSuffix(u.Path, "/")}
		}
	}
	for _, p := range []string{r.Share, strings.Trim(r.Root, "/")} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "/")
}

// HostOf is the server s reaches, as a question about its key or certificate names it.
func HostOf(s store.Source) string {
	r := Remote(s, nil)
	if r.Kind == remotefs.KindWebDAV {
		if u, err := url.Parse(r.URL); err == nil {
			return u.Hostname()
		}
	}
	return r.Host
}

// Local reports whether host is on the local network by its name or its address, as iOS asks the user before the app reaches one: an address that is loopback, private (RFC 1918, fc00::/7) or link-local, or a name of the local network, one without a dot or under .local, .lan or .home.arpa. A name that resolves there is not told, which iOS asks of as the connection is made.
func Local(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if a, err := netip.ParseAddr(host); err == nil {
		return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast()
	}
	host = strings.TrimSuffix(host, ".")
	return host != "" && (!strings.Contains(host, ".") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home.arpa"))
}
