package account

import (
	"net/url"
	"strconv"
	"strings"

	"kando/apps/kando/locales"
	"kando/packages/remotefs"
)

// Location is a folder on a NAS or a server that films can be kept in, as the 存储位置 card lists it: the name it goes by and how to reach it. ID stays the same as it is edited.
type Location struct {
	ID     string
	Name   string
	Remote remotefs.Remote
}

// Status is how the last Test of a location went: running, connected, or why not, in the interface's language. The zero Status is no test yet.
type Status struct {
	Busy bool
	OK   bool
	Err  string
}

// AlertKind is the question the page asks over itself about a location.
type AlertKind int

const (
	// AlertNone is no question.
	AlertNone AlertKind = iota
	// AlertRemove confirms that a location is removed.
	AlertRemove
	// AlertTrust asks whether to trust an SFTP server's key at the first connection.
	AlertTrust
	// AlertKeyChanged warns that an SFTP server offered another key than the trusted one, and offers only a destructive Replace Key besides Cancel.
	AlertKeyChanged
)

// Alert is the question open over the page: about the location Name; for AlertRemove, how many films' videos were moved to it (Films), which do not play while it is gone; and for the host-key questions, the server, the fingerprint of the key it offered and, for AlertKeyChanged, of the key trusted before.
type Alert struct {
	Kind        AlertKind
	Name        string
	Films       int
	Host        string
	Fingerprint string
	Trusted     string
	// Certificate makes the host-key questions about the server's TLS certificate, as a media source's NAS has its own, rather than its SSH key.
	Certificate bool
}

// Draft is a location as its dialog edits it, every field as typed: Kind is one of the remotefs.Kind values, Port is "" for the protocol's own.
type Draft struct {
	Name       string
	Kind       string
	Host       string
	Port       string
	Share      string
	URL        string
	Root       string
	User       string
	Password   string
	Domain     string
	PrivateKey string
	Passphrase string
	HostKey    string
}

// Kinds are the protocols the dialog's drop-down offers, in its order.
var Kinds = []remotefs.Kind{remotefs.KindSMB, remotefs.KindWebDAV, remotefs.KindSFTP}

// KindName is a protocol's name, which no language translates.
func KindName(kind remotefs.Kind) string {
	switch kind {
	case remotefs.KindWebDAV:
		return "WebDAV"
	case remotefs.KindSFTP:
		return "SFTP"
	default:
		return "SMB"
	}
}

// defaultPort is the port a protocol uses when Port is left empty, "" for WebDAV, whose address holds it.
func defaultPort(kind remotefs.Kind) string {
	switch kind {
	case remotefs.KindSMB:
		return "445"
	case remotefs.KindSFTP:
		return "22"
	}
	return ""
}

// DraftOf is l as its dialog starts editing it.
func DraftOf(l Location) Draft {
	r := l.Remote
	d := Draft{Name: l.Name, Kind: string(r.Kind), Host: r.Host, Share: r.Share, URL: r.URL, Root: r.Root, User: r.User,
		Password: r.Password, Domain: r.Domain, PrivateKey: r.PrivateKey, Passphrase: r.Passphrase, HostKey: r.HostKey}
	if r.Port != 0 {
		d.Port = strconv.Itoa(r.Port)
	}
	if d.Kind == "" {
		d.Kind = string(remotefs.KindSMB)
	}
	return d
}

// Remote is how to reach the location d describes, with only the fields its kind uses. Call it once Problem is nil; a port that is not a number reads as the protocol's own.
func (d Draft) Remote() remotefs.Remote {
	kind := remotefs.Kind(d.Kind)
	port, _ := strconv.Atoi(strings.TrimSpace(d.Port))
	r := remotefs.Remote{Kind: kind, Root: strings.TrimSpace(d.Root), User: strings.TrimSpace(d.User), Password: d.Password}
	switch kind {
	case remotefs.KindWebDAV:
		r.URL = strings.TrimSpace(d.URL)
	case remotefs.KindSFTP:
		r.Host, r.Port = strings.TrimSpace(d.Host), port
		r.PrivateKey, r.Passphrase = strings.TrimSpace(d.PrivateKey), d.Passphrase
		r.HostKey = strings.TrimSpace(d.HostKey)
	default:
		r.Kind = remotefs.KindSMB
		r.Host, r.Port = strings.TrimSpace(d.Host), port
		r.Share = strings.Trim(strings.TrimSpace(d.Share), `/\`)
		r.Domain = strings.TrimSpace(d.Domain)
	}
	return r
}

// Problem is what keeps d from being tried or saved, as a message in the language it is read in, nil when nothing does. The name is not needed to try it; Save checks it (NameProblem).
func (d Draft) Problem() func(locales.Messages) string {
	kind := remotefs.Kind(d.Kind)
	if kind == remotefs.KindWebDAV {
		u, err := url.Parse(strings.TrimSpace(d.URL))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return locales.Messages.AccountLocationNeedsURL
		}
	} else if strings.TrimSpace(d.Host) == "" {
		return locales.Messages.AccountLocationNeedsHost
	}
	if port := strings.TrimSpace(d.Port); port != "" && kind != remotefs.KindWebDAV {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return locales.Messages.AccountLocationNeedsPort
		}
	}
	if kind == remotefs.KindSMB || kind == "" {
		if share := strings.Trim(strings.TrimSpace(d.Share), `/\`); share == "" || strings.ContainsAny(share, `/\`) {
			return locales.Messages.AccountLocationNeedsShare
		}
	}
	if kind != remotefs.KindWebDAV && strings.TrimSpace(d.User) == "" {
		return locales.Messages.AccountLocationNeedsUser
	}
	if kind == remotefs.KindSFTP && d.Password == "" && strings.TrimSpace(d.PrivateKey) == "" {
		return locales.Messages.AccountLocationNeedsSecret
	}
	return nil
}

// NameProblem is the message of a draft saved without a name, nil when it has one.
func (d Draft) NameProblem() func(locales.Messages) string {
	if strings.TrimSpace(d.Name) == "" {
		return locales.Messages.AccountLocationNeedsName
	}
	return nil
}

// Place is where r is, as a location's row says it: the server and the share or path, and the folder.
func Place(r remotefs.Remote) string {
	var at string
	switch r.Kind {
	case remotefs.KindWebDAV:
		at = r.URL
		if u, err := url.Parse(r.URL); err == nil && u.Host != "" {
			at = u.Host + strings.TrimSuffix(u.Path, "/")
		}
	case remotefs.KindSMB:
		at = r.Host + "/" + r.Share
	default:
		at = r.Host
	}
	if root := strings.Trim(r.Root, "/"); root != "" {
		at += "/" + root
	}
	return at
}

// host is the server r reaches, as the failure messages name it.
func host(r remotefs.Remote) string {
	if r.Kind == remotefs.KindWebDAV {
		if u, err := url.Parse(r.URL); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return r.Host
}

// Failure says why opening r failed with err, in m's language: never err's own words, which are English and may hold more of the server's address than the person typed. A host-key error is asked about instead (Alert), so it only reads here as the key not trusted.
func Failure(m locales.Messages, r remotefs.Remote, err error) string {
	switch remotefs.Diagnose(err) {
	case remotefs.FailHostKey:
		return m.AccountLocationNotTrusted()
	case remotefs.FailSignIn:
		return m.AccountLocationFailSignIn()
	case remotefs.FailDenied:
		return m.AccountLocationFailDenied()
	case remotefs.FailNoShare:
		return m.AccountLocationFailNoShare(r.Share)
	case remotefs.FailNoRoot:
		return m.AccountLocationFailNoRoot()
	case remotefs.FailBadRoot:
		return m.AccountLocationFailBadRoot()
	case remotefs.FailUnreachable:
		return m.AccountLocationFailUnreachable(host(r))
	case remotefs.FailTimeout:
		return m.AccountLocationFailTimeout(host(r))
	case remotefs.FailCertificate:
		return m.AccountLocationFailCertificate(host(r))
	case remotefs.FailPrivateKey:
		return m.AccountLocationFailPrivateKey()
	}
	return m.AccountLocationFailOther()
}
