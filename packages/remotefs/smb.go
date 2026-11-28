package remotefs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// smbDialTimeout bounds connecting, signing in and mounting the share when ctx sets no deadline of its own.
const smbDialTimeout = 30 * time.Second

// smbStore is a Provider over a folder of an SMB share, spoken with SMB 2 or 3 and signed in with NTLM. It keeps one connection and makes it again when it breaks. Write uploads into a hidden file beside the target and renames it over the target in one step (the rename asks the server to replace what is there), so a reader finds the old file or all of the new one.
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
	// A connection closed because a request on it was cut off may never answer the tree disconnect and the logoff, so they get a moment before the TCP connection goes anyway.
	ctx, cancel := context.WithTimeout(context.Background(), smbCloseTimeout)
	defer cancel()
	_ = c.share.WithContext(ctx).Umount()
	_ = c.session.WithContext(ctx).Logoff()
	return c.tcp.Close()
}

// smbCloseTimeout bounds the goodbyes of a connection closing (smbConn.Close).
const smbCloseTimeout = 3 * time.Second

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
	st.s.cancelBreaks = true
	st.s.dial = func(ctx context.Context) (*smb2.Share, io.Closer, error) {
		c, err := dialSMB(ctx, host, addr, share, cfg)
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

// dialSMB connects to host at addr, signed in as cfg says (dialSMBSession); the share is named on the server as \\host\share.
func dialSMB(ctx context.Context, host, addr, share string, cfg Remote) (*smbConn, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, smbDialTimeout)
		defer cancel()
	}
	tcp, session, err := dialSMBSession(ctx, host, addr, cfg)
	if err != nil {
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

// dialSMBSession connects to host at addr and signs in with cfg's user, password and domain over NTLM, asking the server to sign every message when cfg.RequireSigning is set.
func dialSMBSession(ctx context.Context, host, addr string, cfg Remote) (net.Conn, *smb2.Session, error) {
	tcp, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	d := &smb2.Dialer{
		Negotiator: smb2.Negotiator{RequireMessageSigning: cfg.RequireSigning},
		Initiator:  &smb2.NTLMInitiator{User: cfg.User, Password: cfg.Password, Domain: cfg.Domain},
	}
	session, err := d.DialConn(ctx, tcp, host)
	if err != nil {
		tcp.Close()
		return nil, nil, err
	}
	return tcp, session, nil
}

// ListSMBShares are the names of the shares of the SMB server cfg reaches, signed in as cfg says, in the order of their names, leaving out the hidden ones (a name that ends in $, as IPC$ and C$), which hold no films. cfg's Share and Root are not used.
func ListSMBShares(ctx context.Context, cfg Remote) ([]string, error) {
	host := strings.TrimSpace(cfg.Host)
	switch {
	case host == "":
		return nil, errors.New("storage: SMB needs a host")
	case cfg.User == "":
		return nil, errors.New("storage: SMB needs a user (guest for a share open to anyone)")
	}
	port := cfg.Port
	if port == 0 {
		port = 445
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, smbDialTimeout)
		defer cancel()
	}
	tcp, session, err := dialSMBSession(ctx, host, net.JoinHostPort(host, strconv.Itoa(port)), cfg)
	if err != nil {
		return nil, err
	}
	defer tcp.Close()
	defer func() { _ = session.Logoff() }()
	names, err := session.WithContext(ctx).ListSharenames()
	if err != nil {
		return nil, err
	}
	out := names[:0]
	for _, name := range names {
		if name != "" && !strings.HasSuffix(name, "$") {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// smbBroken is whether err says the connection is gone.
func smbBroken(err error) bool {
	var te *smb2.TransportError
	return errors.As(err, &te) || errors.Is(err, net.ErrClosed)
}

// do runs fn with the share bound to ctx, once more on a new connection when it finds the connection broken.
func (s *smbStore) do(ctx context.Context, fn func(share *smb2.Share) error) error {
	_, err := s.doOn(ctx, fn)
	return err
}

// doOn is do, with the generation of the connection fn last ran on (session.doOn).
func (s *smbStore) doOn(ctx context.Context, fn func(share *smb2.Share) error) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.s.doOn(ctx, func(share *smb2.Share) error { return fn(share.WithContext(ctx)) })
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
	p, err := CleanPath(p)
	if err != nil {
		return Info{}, err
	}
	var info Info
	err = s.do(ctx, func(share *smb2.Share) error {
		fi, err := share.Stat(s.path(p))
		if err != nil {
			return err
		}
		info = fileInfo(baseName(p), fi)
		return nil
	})
	return info, smbErr(err)
}

// baseName is the last name of the clean path p, "" for the root.
func baseName(p string) string {
	return p[strings.LastIndexByte(p, '/')+1:]
}

// List is what lies in the folder dir, leaving out the files a Write is still writing.
func (s *smbStore) List(ctx context.Context, dir string) ([]Info, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	var out []Info
	err = s.do(ctx, func(share *smb2.Share) error {
		entries, err := share.ReadDir(s.path(dir))
		if err != nil {
			return err
		}
		out = out[:0]
		for _, fi := range entries {
			if !strings.HasPrefix(fi.Name(), TempPrefix) {
				out = append(out, fileInfo(fi.Name(), fi))
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
	p, err := CleanFile(p)
	if err != nil {
		return nil, err
	}
	var f *smb2.File
	gen, err := s.doOn(ctx, func(share *smb2.Share) error {
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
	// Its reads are bound to ctx: one cut off by it, or the file closed after it, leaves the connection to be made anew (session.cancelBreaks).
	rc := s.s.guard(ctx, gen, f)
	if length < 0 {
		return rc, nil
	}
	return limitedFile{Reader: io.LimitReader(rc, length), Closer: rc}, nil
}

// Write uploads r into a hidden file beside p and renames it over p.
func (s *smbStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
	p, err := CleanFile(p)
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
	temp := within(parent(s.path(p)), TempPrefix+rand.Text())
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
		err = share.Rename(temp, s.path(p))
	}
	if err != nil {
		// An upload cut off by ctx leaves its connection to be made anew, and its hidden file behind, as a crash does, which List does not show.
		if !s.s.cut(ctx, gen, err) && !s.s.drop(gen, err) {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), smbDialTimeout)
			defer cancel()
			_ = conn.WithContext(cleanup).Remove(temp)
		}
		return smbErr(err)
	}
	return nil
}

// Remove removes the file or the empty folder at p.
func (s *smbStore) Remove(ctx context.Context, p string) error {
	p, err := CleanFile(p)
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
			entries, err := share.ReadDir(name)
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

// Move renames from to to, making the folders above to; a file at to is replaced in the same rename.
func (s *smbStore) Move(ctx context.Context, from, to string) error {
	from, err := CleanFile(from)
	if err != nil {
		return err
	}
	to, err = CleanFile(to)
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
		return share.Rename(s.path(from), s.path(to))
	})
	return smbErr(err)
}

func (s *smbStore) MkdirAll(ctx context.Context, dir string) error {
	dir, err := CleanPath(dir)
	if err != nil {
		return err
	}
	return smbErr(s.do(ctx, func(share *smb2.Share) error { return share.MkdirAll(s.path(dir), 0o755) }))
}

// Close unmounts the share and ends the connection.
func (s *smbStore) Close() error { return s.s.Close() }
