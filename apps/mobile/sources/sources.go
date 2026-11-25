// Package sources is the NAS and servers the phone reaches itself, without the computer (docs/media-sources.md 4.2, 10): Settings › NAS & Servers, which lists them (SettingsPage) and adds, edits, tests and removes one (EditPage), with the question that trusts an SFTP server's key or a WebDAV server's certificate at the first connection; the Library's NAS group, one row per source (Group); and a source's folders, one page per folder, whose videos download to the phone (BrowsePage). The session holds the sources and their connections (session/sources.go), and package nas what each kind is.
package sources

import (
	"errors"
	"strconv"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/nas"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"

	"github.com/nsgo-dev/nsgo/core"
)

// KindName is the name of kind, a protocol's, as each language writes it.
func KindName(cx *core.Scope, kind string) string {
	switch kind {
	case nas.KindSMB:
		return locales.SourceKindSMB(cx)
	case nas.KindSFTP:
		return locales.SourceKindSFTP(cx)
	case nas.KindWebDAV:
		return locales.SourceKindWebDAV(cx)
	}
	return kind
}

// Summary is a source's row's subtitle: its kind's name and where it is, as "SMB · nas.local/films".
func Summary(cx *core.Scope, s store.Source) string {
	return locales.SourceSummary(cx, KindName(cx, s.Kind), nas.Place(s))
}

// fieldTitle is the title of the field key of kind's form.
func fieldTitle(cx *core.Scope, kind, key string) string {
	switch key {
	case nas.Host:
		return locales.SourceHost(cx)
	case nas.Share:
		return locales.SourceShare(cx)
	case nas.Root:
		return locales.SourceRoot(cx)
	case nas.User:
		return locales.SourceUser(cx)
	case nas.Password:
		return locales.SourcePassword(cx)
	case nas.PrivateKey:
		return locales.SourcePrivateKey(cx)
	case nas.Passphrase:
		return locales.SourcePassphrase(cx)
	case nas.Port:
		return locales.SourcePort(cx)
	case nas.Domain:
		return locales.SourceDomain(cx)
	case nas.URL:
		return locales.SourceURL(cx)
	case nas.NoSigning:
		return locales.SourceNoSigning(cx)
	case nas.HostKey:
		return locales.SourceHostKey(cx)
	case nas.CertKey:
		return locales.SourceCertificate(cx)
	}
	return key
}

// fieldHint is what is said under the field key of kind's form, "" for nothing.
func fieldHint(cx *core.Scope, kind, key string) string {
	switch key {
	case nas.Share:
		return locales.SourceShareHint(cx)
	case nas.Root:
		switch kind {
		case nas.KindSMB:
			return locales.SourceRootHintSMB(cx)
		case nas.KindSFTP:
			return locales.SourceRootHintSFTP(cx)
		}
		return locales.SourceRootHintWebDAV(cx)
	case nas.User:
		if kind == nas.KindSMB {
			return locales.SourceUserHintSMB(cx)
		}
	case nas.Domain:
		return locales.SourceDomainHint(cx)
	case nas.Port:
		return locales.SourcePortHint(cx, strconv.Itoa(nas.DefaultPort(kind)))
	case nas.PrivateKey:
		return locales.SourcePrivateKeyHint(cx)
	case nas.NoSigning:
		return locales.SourceNoSigningHint(cx)
	}
	return ""
}

// problemText is why a draft cannot be tried or saved.
func problemText(cx *core.Scope, p nas.Problem) string {
	switch p {
	case nas.NeedsHost:
		return locales.SourceNeedsHost(cx)
	case nas.NeedsPort:
		return locales.SourceNeedsPort(cx)
	case nas.NeedsUser:
		return locales.SourceNeedsUser(cx)
	case nas.NeedsSecret:
		return locales.SourceNeedsSecret(cx)
	case nas.NeedsShare:
		return locales.SourceNeedsShare(cx)
	case nas.NeedsShareForRoot:
		return locales.SourceNeedsShareForRoot(cx)
	case nas.NeedsURL:
		return locales.SourceNeedsURL(cx)
	}
	return ""
}

// Failure says why reaching src failed with err, in the language shown, as the computer's media sources say it (remotefs.Diagnose); the error itself is English and may hold the server's address, so it is not shown.
func Failure(cx *core.Scope, src store.Source, err error) string {
	host := nas.HostOf(src)
	switch {
	case errors.Is(err, session.ErrLocalNetwork):
		return locales.SourceFailLocalNetwork(cx)
	case errors.Is(err, session.ErrSourceGone):
		return locales.SourceFailGone(cx)
	}
	switch remotefs.Diagnose(err) {
	case remotefs.FailHostKey:
		return locales.SourceFailHostKey(cx)
	case remotefs.FailSignIn:
		return locales.SourceFailSignIn(cx)
	case remotefs.FailDenied:
		return locales.SourceFailDenied(cx)
	case remotefs.FailNoShare:
		return locales.SourceFailNoShare(cx, strings.Trim(src.Config[nas.Share], `/\`))
	case remotefs.FailNoRoot:
		return locales.SourceFailNoRoot(cx)
	case remotefs.FailBadRoot:
		return locales.SourceFailBadRoot(cx)
	case remotefs.FailUnreachable:
		return locales.SourceFailUnreachable(cx, host)
	case remotefs.FailTimeout:
		return locales.SourceFailTimeout(cx, host)
	case remotefs.FailCertificate:
		return locales.SourceFailCertificate(cx, host)
	case remotefs.FailPrivateKey:
		return locales.SourceFailPrivateKey(cx)
	case remotefs.FailSigning:
		return locales.SourceFailSigning(cx)
	}
	return locales.SourceFailOther(cx)
}
