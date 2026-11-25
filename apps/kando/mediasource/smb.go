package mediasource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"kando/apps/kando/locales"
	"kando/packages/remotefs"
)

// The keys of an SMB source's fields (SMB): the server, its port, the share and a folder in it, who signs in, the password, which is secret, and NoSigning, the switch that lets Kando read a NAS that does not sign its messages, or a share open to guests, which cannot.
const (
	SMBHost      = "host"
	SMBPort      = "port"
	SMBShare     = "share"
	SMBRoot      = "root"
	SMBUser      = "user"
	SMBDomain    = "domain"
	SMBPassword  = "password"
	SMBNoSigning = "noSigning"
)

// smbDefaultPort is the port SMB is reached at when the source names none.
const smbDefaultPort = 445

// SMB is the built-in kind of a share of a NAS or a Windows computer, read over SMB 2 or 3 and signed in with NTLM (remotefs's SMB). Every message is signed unless the source's NoSigning field is on. A source without a share is the whole server: its root lists the server's shares (remotefs.ListSMBShares), and a path's first name is the share it is in.
func SMB() Kind {
	return Kind{
		ID:    KindSMB,
		Title: func(locale string) string { return locales.In(locale).SettingsSourcesKindSMB() },
		Icon:  "server",
		Phone: true,
		Fields: []Field{
			{Key: SMBHost, Type: FieldText, Title: msg(locales.Messages.AccountLocationHost)},
			{Key: SMBShare, Type: FieldText, Title: msg(locales.Messages.AccountLocationShare), Hint: msg(locales.Messages.SettingsSourcesShareHint)},
			{Key: SMBRoot, Type: FieldText, Title: msg(locales.Messages.AccountLocationRoot), Hint: msg(locales.Messages.AccountLocationRootHintSMB)},
			{Key: SMBUser, Type: FieldText, Title: msg(locales.Messages.AccountLocationUser), Hint: msg(locales.Messages.AccountLocationUserHintSMB)},
			{Key: SMBPassword, Type: FieldSecret, Title: msg(locales.Messages.AccountLocationPassword)},
			{Key: SMBNoSigning, Type: FieldBool, Title: msg(locales.Messages.SettingsSourcesNoSigning), Hint: msg(locales.Messages.SettingsSourcesNoSigningHint)},
			{Key: SMBDomain, Type: FieldText, Title: msg(locales.Messages.AccountLocationDomain), Hint: msg(locales.Messages.AccountLocationDomainHint)},
			{Key: SMBPort, Type: FieldNumber, Title: msg(locales.Messages.AccountLocationPort), Default: strconv.Itoa(smbDefaultPort),
				Hint: func(locale string) string {
					return locales.In(locale).AccountLocationPortHint(strconv.Itoa(smbDefaultPort))
				}},
		},
		Open: func(ctx context.Context, s Source, secrets map[string]string) (Driver, error) {
			return openSMB(ctx, SMBRemote(s, secrets[SMBPassword]), remotefs.OpenRemote, remotefs.ListSMBShares)
		},
	}
}

// msg is the text of a message of the catalog in a language, as a Kind's titles and hints are.
func msg(m func(locales.Messages) string) func(locale string) string {
	return func(locale string) string { return m(locales.In(locale)) }
}

// SMBRemote is how storage reaches the SMB source s with password: its fields as a remotefs.Remote, the port 445 when it names none or one that is not a number, and signing required unless its NoSigning field is "true".
func SMBRemote(s Source, password string) remotefs.Remote {
	c := s.Config
	port, err := strconv.Atoi(strings.TrimSpace(c[SMBPort]))
	if err != nil || port < 1 || port > 65535 {
		port = 0
	}
	return remotefs.Remote{
		Kind:           remotefs.KindSMB,
		Host:           strings.TrimSpace(c[SMBHost]),
		Port:           port,
		Share:          strings.Trim(strings.TrimSpace(c[SMBShare]), `/\`),
		Root:           strings.TrimSpace(c[SMBRoot]),
		User:           strings.TrimSpace(c[SMBUser]),
		Domain:         strings.TrimSpace(c[SMBDomain]),
		Password:       password,
		RequireSigning: c[SMBNoSigning] != "true",
	}
}

// The ways openSMB reaches a server, remotefs's unless a test gives its own.
type (
	openRemote func(ctx context.Context, cfg remotefs.Remote) (remotefs.Provider, io.Closer, error)
	listShares func(ctx context.Context, cfg remotefs.Remote) ([]string, error)
)

// openSMB connects to cfg: one share with open when it names one, else the whole server, whose shares shares lists, each mounted with open as a path first reaches into it. Either is checked first, its root read or its shares listed.
func openSMB(ctx context.Context, cfg remotefs.Remote, open openRemote, shares listShares) (Driver, error) {
	if cfg.Share != "" {
		p, c, err := open(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return &providerDriver{p: p, c: c}, nil
	}
	if strings.Trim(cfg.Root, "/") != "" {
		return nil, fmt.Errorf("mediasource: a folder %q needs a share: %w", cfg.Root, ErrInvalidPath)
	}
	if _, err := shares(ctx, cfg); err != nil {
		return nil, err
	}
	return &serverDriver{cfg: cfg, open: open, shares: shares, mounted: map[string]*providerDriver{}}, nil
}

// providerDriver reads a remotefs.Provider: its List, Stat and Open, with nothing that writes. c closes it.
type providerDriver struct {
	p remotefs.Provider
	c io.Closer
}

func (d *providerDriver) List(ctx context.Context, dir string) ([]Entry, error) {
	infos, err := d.p.List(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(infos))
	for i, info := range infos {
		out[i] = entryOf(info)
	}
	return out, nil
}

func (d *providerDriver) Stat(ctx context.Context, p string) (Entry, error) {
	info, err := d.p.Stat(ctx, p)
	if err != nil {
		return Entry{}, err
	}
	return entryOf(info), nil
}

func (d *providerDriver) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	return d.p.Open(ctx, p, offset, length)
}

func (d *providerDriver) Close() error { return d.c.Close() }

// entryOf is info as an Entry: its Version the ETag when the server gives one, else its size and time, which change when the file does.
func entryOf(info remotefs.Info) Entry {
	e := Entry{Name: info.Name, Size: info.Size, Modified: info.Modified, Dir: info.Dir, Version: info.ETag}
	if e.Version == "" && !e.Dir {
		e.Version = strconv.FormatInt(info.Size, 10) + "-" + strconv.FormatInt(info.Modified.UnixNano(), 10)
	}
	return e
}

// serverDriver reads every share of an SMB server: its root lists the shares, and a path's first name is the share it is in, which is mounted the first time a path reaches into it and kept until Close.
type serverDriver struct {
	cfg    remotefs.Remote
	open   openRemote
	shares listShares

	mu      sync.Mutex
	mounted map[string]*providerDriver
	closed  bool
}

// share is the mounted share name, mounting it the first time.
func (d *serverDriver) share(ctx context.Context, name string) (*providerDriver, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("mediasource: the source is closed")
	}
	if m := d.mounted[name]; m != nil {
		return m, nil
	}
	cfg := d.cfg
	cfg.Share = name
	p, c, err := d.open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	m := &providerDriver{p: p, c: c}
	d.mounted[name] = m
	return m, nil
}

// split is the share p is in and the path in it, or ErrInvalidPath for one that leaves the root.
func split(p string) (share, rest string, err error) {
	clean, err := CleanPath(p)
	if err != nil {
		return "", "", err
	}
	share, rest, _ = strings.Cut(clean, "/")
	return share, rest, nil
}

func (d *serverDriver) List(ctx context.Context, dir string) ([]Entry, error) {
	share, rest, err := split(dir)
	if err != nil {
		return nil, err
	}
	if share == "" {
		names, err := d.shares(ctx, d.cfg)
		if err != nil {
			return nil, err
		}
		out := make([]Entry, len(names))
		for i, name := range names {
			out[i] = Entry{Name: name, Dir: true}
		}
		return out, nil
	}
	m, err := d.share(ctx, share)
	if err != nil {
		return nil, err
	}
	return m.List(ctx, rest)
}

func (d *serverDriver) Stat(ctx context.Context, p string) (Entry, error) {
	share, rest, err := split(p)
	if err != nil {
		return Entry{}, err
	}
	if share == "" {
		return Entry{Dir: true}, nil
	}
	m, err := d.share(ctx, share)
	if err != nil {
		return Entry{}, err
	}
	e, err := m.Stat(ctx, rest)
	if err == nil && rest == "" {
		e.Name = share
	}
	return e, err
}

func (d *serverDriver) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	share, rest, err := split(p)
	if err != nil {
		return nil, err
	}
	if share == "" || rest == "" {
		return nil, fmt.Errorf("%q: %w", p, remotefs.ErrIsDir)
	}
	m, err := d.share(ctx, share)
	if err != nil {
		return nil, err
	}
	return m.Open(ctx, rest, offset, length)
}

// Close unmounts every share mounted.
func (d *serverDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	var errs []error
	for _, m := range d.mounted {
		errs = append(errs, m.Close())
	}
	d.mounted = nil
	return errors.Join(errs...)
}
