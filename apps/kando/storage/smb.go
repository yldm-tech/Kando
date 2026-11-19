package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
)

// smbDialTimeout bounds connecting, signing in and mounting the share when ctx sets no deadline of its own.
const smbDialTimeout = 30 * time.Second

// smbStore is a Provider over a folder of an SMB share, spoken with SMB 2 or 3 and signed in with NTLM. It keeps one connection and makes it again when it breaks. Write uploads into a hidden file beside the target and renames it into place. The library's rename does not replace a file, so a file already at the target is first renamed aside and removed once the new one is in place: a reader in that moment finds nothing there, and a rename that fails puts the old file back.
type smbStore struct {
	root string
	s    session[*smb2.Share]
}

// smbConn is the TCP connection, the signed-in session on it and the mounted share, closed together.
type smbConn struct {
	tcp     net.Conn
	session *smb2.Session
	share   *smb2.Share
}

func (c *smbConn) Close() error {
	_ = c.share.Umount()
	_ = c.session.Logoff()
	return c.tcp.Close()
}

func openSMB(ctx context.Context, cfg Remote) (*smbStore, error) {
	host := strings.TrimSpace(cfg.Host)
	share := strings.Trim(strings.TrimSpace(cfg.Share), `/\`)
	switch {
	case host == "":
		return nil, errors.New("storage: SMB needs a host")
	case share == "" || strings.ContainsAny(share, `/\`):
		return nil, fmt.Errorf("storage: SMB share %q is not the name of one share", cfg.Share)
	case cfg.User == "":
		// The library signs in with a user; a share open to anyone is reached as "guest".
		return nil, errors.New("storage: SMB needs a user (guest for a share open to anyone)")
	}
	port := cfg.Port
	if port == 0 {
		port = 445
	}
	root, err := remoteRoot(cfg.Root)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	st := &smbStore{root: root}
	st.s.broken = smbBroken
	st.s.dial = func(ctx context.Context) (*smb2.Share, io.Closer, error) {
		c, err := dialSMB(ctx, addr, share, cfg.User, cfg.Password, cfg.Domain)
		if err != nil {
			return nil, nil, err
		}
		return c.share, c, nil
	}
	if _, err := st.Stat(ctx, ""); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

func dialSMB(ctx context.Context, addr, share, user, password, domain string) (*smbConn, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, smbDialTimeout)
		defer cancel()
	}
	tcp, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	d := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: password, Domain: domain}}
	session, err := d.DialContext(ctx, tcp)
	if err != nil {
		tcp.Close()
		return nil, err
	}
	mounted, err := session.WithContext(ctx).Mount(share)
	if err != nil {
		_ = session.Logoff()
		tcp.Close()
		return nil, err
	}
	return &smbConn{tcp: tcp, session: session, share: mounted}, nil
}

// smbBroken is whether err says the connection is gone.
func smbBroken(err error) bool {
	var te *smb2.TransportError
	return errors.As(err, &te) || errors.Is(err, net.ErrClosed)
}

// do runs fn with the share bound to ctx, once more on a new connection when it finds the connection broken.
func (s *smbStore) do(ctx context.Context, fn func(share *smb2.Share) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.s.do(ctx, func(share *smb2.Share) error { return fn(share.WithContext(ctx)) })
}

// path is the share's path of the clean path p; the library takes slashes.
func (s *smbStore) path(p string) string { return within(s.root, p) }

// smbErr is err with a missing path read as ErrNotExist.
func smbErr(err error) error {
	if err != nil && errors.Is(err, fs.ErrNotExist) && !errors.Is(err, ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrNotExist, err)
	}
	return err
}

func (s *smbStore) Stat(ctx context.Context, p string) (Info, error) {
	p, err := cleanPath(p)
	if err != nil {
		return Info{}, err
	}
	var info Info
	err = s.do(ctx, func(share *smb2.Share) error {
		fi, err := share.Stat(s.path(p))
		if err != nil {
			return err
		}
		info = localInfo(baseName(p), fi)
		return nil
	})
	return info, smbErr(err)
}

// baseName is the last name of the clean path p, "" for the root.
func baseName(p string) string {
	return p[strings.LastIndexByte(p, '/')+1:]
}

// smbReadDir is share.ReadDir, with a panic of it read as an error. The library indexes a listing by offsets the server sends without checking them against the reply (GO-2026-5051, fixed in no release of the library), so a broken or hostile server can make it panic; that panic is in this goroutine and is recovered here rather than ending Kando.
func smbReadDir(share *smb2.Share, name string) (entries []os.FileInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			entries, err = nil, fmt.Errorf("storage: SMB listing of %q is malformed: %v", name, r)
		}
	}()
	return share.ReadDir(name)
}

// List is what lies in the folder dir, leaving out the files a Write is still writing.
func (s *smbStore) List(ctx context.Context, dir string) ([]Info, error) {
	dir, err := cleanPath(dir)
	if err != nil {
		return nil, err
	}
	var out []Info
	err = s.do(ctx, func(share *smb2.Share) error {
		entries, err := smbReadDir(share, s.path(dir))
		if err != nil {
			return err
		}
		out = out[:0]
		for _, fi := range entries {
			if !strings.HasPrefix(fi.Name(), tempPrefix) {
				out = append(out, localInfo(fi.Name(), fi))
			}
		}
		return nil
	})
	return out, smbErr(err)
}

// Open reads the file at p from offset; the reader is bound to ctx.
func (s *smbStore) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if err := checkRange(p, offset, length); err != nil {
		return nil, err
	}
	p, err := cleanFile(p)
	if err != nil {
		return nil, err
	}
	var f *smb2.File
	err = s.do(ctx, func(share *smb2.Share) error {
		var err error
		f, err = share.Open(s.path(p))
		if err != nil {
			return err
		}
		fi, err := f.Stat()
		if err == nil && fi.IsDir() {
			err = fmt.Errorf("%s: %w", p, ErrIsDir)
		}
		if err == nil && offset > 0 {
			_, err = f.Seek(offset, io.SeekStart)
		}
		if err != nil {
			f.Close()
		}
		return err
	})
	if err != nil {
		return nil, smbErr(err)
	}
	if length < 0 {
		return f, nil
	}
	return limitedFile{Reader: io.LimitReader(f, length), Closer: f}, nil
}

// Write uploads r into a hidden file beside p and renames it over p.
func (s *smbStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
	p, err := cleanFile(p)
	if err != nil {
		return err
	}
	if size < 0 {
		return fmt.Errorf("%s: unknown size", p)
	}
	// MkdirAll also finds a broken connection and makes a new one, which the upload, that cannot read r twice, then uses.
	if err := s.MkdirAll(ctx, parent(p)); err != nil {
		return err
	}
	conn, gen, err := s.s.get(ctx)
	if err != nil {
		return err
	}
	share := conn.WithContext(ctx)
	temp := within(parent(s.path(p)), tempPrefix+rand.Text())
	err = func() error {
		f, err := share.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		n, err := io.Copy(f, io.LimitReader(r, size))
		if err == nil && n < size {
			err = shortBody(p, n, size)
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}()
	if err == nil {
		err = s.replace(share, temp, s.path(p))
	}
	if err != nil {
		if !s.s.drop(gen, err) {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), smbDialTimeout)
			defer cancel()
			_ = conn.WithContext(cleanup).Remove(temp)
		}
		return smbErr(err)
	}
	return nil
}

// replace renames from to to; a file at to is renamed aside first and removed once from is in its place, or put back when the rename fails.
func (s *smbStore) replace(share *smb2.Share, from, to string) error {
	fi, err := share.Stat(to)
	if errors.Is(err, fs.ErrNotExist) {
		return share.Rename(from, to)
	}
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%s: %w", to, fs.ErrExist)
	}
	aside := within(parent(to), tempPrefix+rand.Text())
	if err := share.Rename(to, aside); err != nil {
		return err
	}
	if err := share.Rename(from, to); err != nil {
		if back := share.Rename(aside, to); back != nil {
			return errors.Join(err, back)
		}
		return err
	}
	_ = share.Remove(aside)
	return nil
}

// Remove removes the file or the empty folder at p.
func (s *smbStore) Remove(ctx context.Context, p string) error {
	p, err := cleanFile(p)
	if err != nil {
		return err
	}
	err = s.do(ctx, func(share *smb2.Share) error {
		name := s.path(p)
		fi, err := share.Lstat(name)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			entries, err := smbReadDir(share, name)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return fmt.Errorf("%s: %w", p, ErrNotEmpty)
			}
		}
		return share.Remove(name)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Move renames from to to, making the folders above to; a file at to is replaced as Write replaces it.
func (s *smbStore) Move(ctx context.Context, from, to string) error {
	from, err := cleanFile(from)
	if err != nil {
		return err
	}
	to, err = cleanFile(to)
	if err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if strings.HasPrefix(to+"/", from+"/") {
		return fmt.Errorf("%s into itself: %w", from, ErrInvalidPath)
	}
	err = s.do(ctx, func(share *smb2.Share) error {
		if _, err := share.Lstat(s.path(from)); err != nil {
			return err
		}
		if err := share.MkdirAll(s.path(parent(to)), 0o755); err != nil {
			return err
		}
		return s.replace(share, s.path(from), s.path(to))
	})
	return smbErr(err)
}

func (s *smbStore) MkdirAll(ctx context.Context, dir string) error {
	dir, err := cleanPath(dir)
	if err != nil {
		return err
	}
	return smbErr(s.do(ctx, func(share *smb2.Share) error { return share.MkdirAll(s.path(dir), 0o755) }))
}

// Close unmounts the share and ends the connection.
func (s *smbStore) Close() error { return s.s.Close() }
