package mediasource

import (
	"context"
	"net/url"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/storage"
)

// The keys of a WebDAV source's fields (WebDAV): the server's address, a folder below it, who signs in and the password, which is secret.
const (
	WebDAVURL      = "url"
	WebDAVRoot     = "root"
	WebDAVUser     = "user"
	WebDAVPassword = "password"
)

// WebDAV is the built-in kind of a folder of a WebDAV server, as many NAS systems and cloud drives offer, reached over HTTPS or HTTP with basic authentication (storage's WebDAV). It is built in, beside the official WebDAV plugin of P4, so a phone can reach it too (docs/media-sources.md 17.3). An http:// address sends the password unencrypted, which the form says (PlainHTTP).
func WebDAV() Kind {
	return Kind{
		ID:    KindWebDAV,
		Title: func(locale string) string { return locales.In(locale).SettingsSourcesKindWebDAV() },
		Icon:  "cloud",
		Phone: true,
		Fields: []Field{
			{Key: WebDAVURL, Type: FieldText, Title: msg(locales.Messages.AccountLocationURL)},
			{Key: WebDAVRoot, Type: FieldText, Title: msg(locales.Messages.AccountLocationRoot), Hint: msg(locales.Messages.AccountLocationRootHintWebDAV)},
			{Key: WebDAVUser, Type: FieldText, Title: msg(locales.Messages.AccountLocationUser)},
			{Key: WebDAVPassword, Type: FieldSecret, Title: msg(locales.Messages.AccountLocationPassword)},
		},
		Open: func(ctx context.Context, s Source, secrets map[string]string) (Driver, error) {
			p, c, err := storage.OpenRemote(ctx, WebDAVRemote(s, secrets[WebDAVPassword]))
			if err != nil {
				return nil, err
			}
			return &providerDriver{p: p, c: c}, nil
		},
	}
}

// WebDAVRemote is how storage reaches the WebDAV source s with password: its fields as a storage.Remote.
func WebDAVRemote(s Source, password string) storage.Remote {
	c := s.Config
	return storage.Remote{
		Kind:     storage.KindWebDAV,
		URL:      strings.TrimSpace(c[WebDAVURL]),
		Root:     strings.TrimSpace(c[WebDAVRoot]),
		User:     strings.TrimSpace(c[WebDAVUser]),
		Password: password,
	}
}

// PlainHTTP reports whether the WebDAV source s is reached over http://, which sends its password unencrypted.
func PlainHTTP(s Source) bool {
	u, err := url.Parse(strings.TrimSpace(s.Config[WebDAVURL]))
	return err == nil && strings.EqualFold(u.Scheme, "http")
}
