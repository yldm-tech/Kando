package mediasource

import (
	"context"
	"strconv"
	"strings"

	"kando/apps/kando/locales"
	"kando/packages/remotefs"
)

// The keys of an SFTP source's fields (SFTP): the server, its port, the folder, who signs in, the password, the private key and its passphrase, which are secret, and the server's key Kando trusts.
const (
	SFTPHost       = "host"
	SFTPPort       = "port"
	SFTPRoot       = "root"
	SFTPUser       = "user"
	SFTPPassword   = "password"
	SFTPPrivateKey = "privateKey"
	SFTPPassphrase = "passphrase"
	SFTPHostKey    = "hostKey"
)

// sftpDefaultPort is the port SFTP is reached at when the source names none.
const sftpDefaultPort = 22

// SFTP is the built-in kind of a folder of a server reached over SSH (remotefs's SFTP), signed in with a password, a private key or both. Kando connects only to a server whose key it trusts: the first connection asks the person to trust the key the server offers, and a key that changed afterwards is refused until the person replaces it (remotefs.HostKeyError), which the source keeps in its HostKey field.
func SFTP() Kind {
	return Kind{
		ID:    KindSFTP,
		Title: func(locale string) string { return locales.In(locale).SettingsSourcesKindSFTP() },
		Icon:  "server",
		Phone: true,
		Fields: []Field{
			{Key: SFTPHost, Type: FieldText, Title: msg(locales.Messages.AccountLocationHost)},
			{Key: SFTPRoot, Type: FieldText, Title: msg(locales.Messages.AccountLocationRoot), Hint: msg(locales.Messages.AccountLocationRootHintSFTP)},
			{Key: SFTPUser, Type: FieldText, Title: msg(locales.Messages.AccountLocationUser)},
			{Key: SFTPPassword, Type: FieldSecret, Title: msg(locales.Messages.AccountLocationPassword)},
			{Key: SFTPPrivateKey, Type: FieldSecret, Multiline: true, Title: msg(locales.Messages.AccountLocationPrivateKey), Hint: msg(locales.Messages.AccountLocationPrivateKeyHint)},
			{Key: SFTPPassphrase, Type: FieldSecret, Title: msg(locales.Messages.AccountLocationPassphrase)},
			{Key: SFTPPort, Type: FieldNumber, Title: msg(locales.Messages.AccountLocationPort), Default: strconv.Itoa(sftpDefaultPort),
				Hint: func(locale string) string {
					return locales.In(locale).AccountLocationPortHint(strconv.Itoa(sftpDefaultPort))
				}},
			{Key: SFTPHostKey, Type: FieldHostKey, Title: msg(locales.Messages.AccountLocationHostKey), Hint: msg(locales.Messages.AccountLocationHostKeyHint)},
		},
		Open: func(ctx context.Context, s Source, secrets map[string]string) (Driver, error) {
			p, c, err := remotefs.OpenRemote(ctx, SFTPRemote(s, secrets))
			if err != nil {
				return nil, err
			}
			return &providerDriver{p: p, c: c}, nil
		},
	}
}

// SFTPRemote is how storage reaches the SFTP source s with its secrets by key: its fields as a remotefs.Remote, the port 22 when it names none or one that is not a number. The passphrase is kept as typed, as a space may be part of it.
func SFTPRemote(s Source, secrets map[string]string) remotefs.Remote {
	c := s.Config
	port, err := strconv.Atoi(strings.TrimSpace(c[SFTPPort]))
	if err != nil || port < 1 || port > 65535 {
		port = 0
	}
	return remotefs.Remote{
		Kind:       remotefs.KindSFTP,
		Host:       strings.TrimSpace(c[SFTPHost]),
		Port:       port,
		Root:       strings.TrimSpace(c[SFTPRoot]),
		User:       strings.TrimSpace(c[SFTPUser]),
		Password:   secrets[SFTPPassword],
		PrivateKey: strings.TrimSpace(secrets[SFTPPrivateKey]),
		Passphrase: secrets[SFTPPassphrase],
		HostKey:    strings.TrimSpace(c[SFTPHostKey]),
	}
}
