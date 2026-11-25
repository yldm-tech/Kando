package nas

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	gosync "sync"

	"kando/packages/remotefs"
	"kando/packages/sync"
)

// Reader reads a source's folders and files, by paths relative to its root as remotefs's are, and has nothing that writes: the phone only reads a NAS (docs/media-sources.md 12.2). Close ends its connection.
type Reader interface {
	remotefs.Reader
	List(ctx context.Context, dir string) ([]remotefs.Info, error)
	Close() error
}

// Dialer connects to a server as remotefs does (Connect, Shares); a test gives its own.
type Dialer struct {
	Connect func(ctx context.Context, cfg remotefs.Remote) (remotefs.Provider, io.Closer, error)
	Shares  func(ctx context.Context, cfg remotefs.Remote) ([]string, error)
}

// Remotefs is the Dialer of the real servers.
var Remotefs = Dialer{Connect: remotefs.OpenRemote, Shares: remotefs.ListSMBShares}

// Open connects to r with d and checks it can be read: one folder, or, for SMB without a share, the whole server, whose root lists its shares (remotefs.ListSMBShares) and whose path's first name is the share it is in, mounted the first time a path reaches into it, as the computer's media sources do.
func (d Dialer) Open(ctx context.Context, r remotefs.Remote) (Reader, error) {
	if r.Kind != remotefs.KindSMB || r.Share != "" {
		p, c, err := d.Connect(ctx, r)
		if err != nil {
			return nil, err
		}
		return provider{p, c}, nil
	}
	if strings.Trim(r.Root, "/") != "" {
		return nil, fmt.Errorf("nas: a folder %q needs a share: %w", r.Root, remotefs.ErrInvalidPath)
	}
	if _, err := d.Shares(ctx, r); err != nil {
		return nil, err
	}
	return &server{cfg: r, d: d, mounted: map[string]provider{}}, nil
}

// provider is a remotefs.Provider read alone: the Reader keeps it from writing.
type provider struct {
	p remotefs.Provider
	c io.Closer
}

func (p provider) Stat(ctx context.Context, path string) (remotefs.Info, error) {
	return p.p.Stat(ctx, path)
}

func (p provider) Open(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	return p.p.Open(ctx, path, offset, length)
}

func (p provider) List(ctx context.Context, dir string) ([]remotefs.Info, error) {
	return p.p.List(ctx, dir)
}

func (p provider) Close() error { return p.c.Close() }

// server is every share of an SMB server.
type server struct {
	cfg remotefs.Remote
	d   Dialer

	mu      gosync.Mutex
	mounted map[string]provider
	closed  bool
}

// share is the share name, mounted the first time.
func (s *server) share(ctx context.Context, name string) (provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return provider{}, errors.New("nas: the source is closed")
	}
	if m, ok := s.mounted[name]; ok {
		return m, nil
	}
	cfg := s.cfg
	cfg.Share = name
	p, c, err := s.d.Connect(ctx, cfg)
	if err != nil {
		return provider{}, err
	}
	m := provider{p, c}
	s.mounted[name] = m
	return m, nil
}

// split is the share p is in and the path in it.
func split(p string) (share, rest string, err error) {
	clean, err := remotefs.CleanPath(p)
	if err != nil {
		return "", "", err
	}
	share, rest, _ = strings.Cut(clean, "/")
	return share, rest, nil
}

func (s *server) List(ctx context.Context, dir string) ([]remotefs.Info, error) {
	share, rest, err := split(dir)
	if err != nil {
		return nil, err
	}
	if share == "" {
		names, err := s.d.Shares(ctx, s.cfg)
		if err != nil {
			return nil, err
		}
		out := make([]remotefs.Info, len(names))
		for i, name := range names {
			out[i] = remotefs.Info{Name: name, Dir: true}
		}
		return out, nil
	}
	m, err := s.share(ctx, share)
	if err != nil {
		return nil, err
	}
	return m.List(ctx, rest)
}

func (s *server) Stat(ctx context.Context, p string) (remotefs.Info, error) {
	share, rest, err := split(p)
	if err != nil {
		return remotefs.Info{}, err
	}
	if share == "" {
		return remotefs.Info{Dir: true}, nil
	}
	m, err := s.share(ctx, share)
	if err != nil {
		return remotefs.Info{}, err
	}
	info, err := m.Stat(ctx, rest)
	if err == nil && rest == "" {
		info.Name = share
	}
	return info, err
}

func (s *server) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	share, rest, err := split(p)
	if err != nil {
		return nil, err
	}
	if share == "" || rest == "" {
		return nil, fmt.Errorf("%q: %w", p, remotefs.ErrIsDir)
	}
	m, err := s.share(ctx, share)
	if err != nil {
		return nil, err
	}
	return m.Open(ctx, rest, offset, length)
}

func (s *server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var errs []error
	for _, m := range s.mounted {
		errs = append(errs, m.Close())
	}
	s.mounted = nil
	return errors.Join(errs...)
}

// IsVideo reports whether the file name is a video the phone downloads and plays, by its extension: one of the types the computer serves the phone its videos as (sync.MediaType).
func IsVideo(name string) bool {
	return strings.HasPrefix(sync.MediaType(name), "video/")
}

// Listing is what a folder of a source shows of infos: its folders and videos, folders first, each in the order of their names, without hidden ones (a name that starts with a dot) nor a name that is not valid UTF-8, whose path would not come back the same.
func Listing(infos []remotefs.Info) []remotefs.Info {
	var out []remotefs.Info
	for _, info := range infos {
		if strings.HasPrefix(info.Name, ".") || info.Name == "" || !validName(info.Name) || !info.Dir && !IsVideo(info.Name) {
			continue
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b remotefs.Info) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// validName reports whether name is valid UTF-8 without a replacement character, which a name in an old NAS's own encoding is read with.
func validName(name string) bool {
	return strings.ToValidUTF8(name, "�") == name && !strings.ContainsRune(name, '�')
}

// Version names the content of a file as info tells it, so a download that goes on from its part does so only if the file did not change: the server's ETag, else its size and time.
func Version(info remotefs.Info) string {
	if info.ETag != "" {
		return info.ETag
	}
	return strconv.FormatInt(info.Size, 10) + "-" + strconv.FormatInt(info.Modified.UnixNano(), 10)
}

// Join is the path of name in the folder dir, both relative to a source's root.
func Join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
