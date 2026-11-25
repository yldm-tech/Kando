package remotefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"sync/atomic"
)

// Kind is the protocol a Remote is reached by.
type Kind string

const (
	// KindSMB is a share of a NAS or a Windows computer, over SMB 2 or 3.
	KindSMB Kind = "smb"
	// KindWebDAV is a folder of a WebDAV server, over HTTPS or HTTP.
	KindWebDAV Kind = "webdav"
	// KindSFTP is a folder of a server reached over SSH.
	KindSFTP Kind = "sftp"
)

// Remote is where a provider on another machine keeps its files and how to reach it. Which fields count depends on Kind:
//
//   - KindSMB: Host, Port (445 when 0), Share, Root (a folder in the share, "" for the share's top), User, Password and Domain (often "" or "WORKGROUP"), and RequireSigning, which refuses a server that does not sign every message (SMB message signing, whose key comes from the password, so a machine in the middle that does not know it cannot change what is read); a guest session cannot sign, so it needs RequireSigning off.
//   - KindWebDAV: URL (the server's https:// or http:// address, with any path the server puts its files under), Root (a folder below URL), User and Password, and CertFingerprint, the fingerprint (CertFingerprint) of the certificate the person trusted for an https:// server whose certificate the system does not trust, as a NAS's own self-signed one: with it set, that certificate alone is taken, in place of the system's verification; see CertError.
//   - KindSFTP: Host, Port (22 when 0), Root (an absolute folder, or one relative to the user's home folder, "" for the home folder itself), User, Password or PrivateKey (PEM, with Passphrase when it is encrypted) or both, and HostKey, the server's public key as a line of authorized_keys ("ssh-ed25519 AAAA..."), without which no connection is made: see HostKeyError.
//
// Password, PrivateKey and Passphrase are secrets.
type Remote struct {
	Kind       Kind
	Host       string
	Port       int
	Share      string
	URL        string
	Root       string
	User       string
	Password   string
	Domain     string
	PrivateKey string
	Passphrase string
	HostKey    string
	// RequireSigning is SMB's: see KindSMB.
	RequireSigning bool
	// CertFingerprint is WebDAV's: see KindWebDAV.
	CertFingerprint string
}

// OpenRemote connects to the provider cfg describes and checks that its root can be read: an SMB share is mounted and an SFTP session started with the server's key checked against cfg.HostKey, a WebDAV folder is asked for its properties. The provider and the Closer are the same value; closing it ends the connection, and the provider cannot be used after. A provider that keeps a connection makes it again once when an operation finds it broken.
func OpenRemote(ctx context.Context, cfg Remote) (Provider, io.Closer, error) {
	var (
		p interface {
			Provider
			io.Closer
		}
		err error
	)
	switch cfg.Kind {
	case KindSMB:
		p, err = openSMB(ctx, cfg)
	case KindWebDAV:
		p, err = openWebDAV(ctx, cfg)
	case KindSFTP:
		p, err = openSFTP(ctx, cfg)
	default:
		return nil, nil, fmt.Errorf("storage: unknown kind of remote %q", cfg.Kind)
	}
	if err != nil {
		return nil, nil, err
	}
	return p, p, nil
}

// remoteRoot is a root folder as a person types it, "/films/" or "films" alike, made plain and relative, or ErrInvalidPath when it climbs above where it starts.
func remoteRoot(root string) (string, error) {
	return CleanPath(strings.Trim(strings.TrimSpace(root), "/"))
}

// within is the clean path p below the clean root, "" for both being the top.
func within(root, p string) string {
	switch {
	case root == "":
		return p
	case p == "":
		return root
	}
	return root + "/" + p
}

// parent is the folder the clean path p lies in, "" for the root.
func parent(p string) string {
	if dir := path.Dir(p); dir != "." {
		return dir
	}
	return ""
}

// errClosed is the error of a provider used after Close.
var errClosed = errors.New("storage: provider is closed")

// session keeps one connection of type C, made by dial when first needed, and makes it again when an operation finds it broken. gen counts the connections made, so a failure seen on a connection that was already replaced does not drop the new one.
type session[C any] struct {
	dial   func(ctx context.Context) (C, io.Closer, error)
	broken func(err error) bool

	mu     sync.Mutex
	conn   C
	closer io.Closer
	gen    int
	closed bool
}

// get is the live connection and its generation, dialing one when there is none.
func (s *session[C]) get(ctx context.Context) (C, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var zero C
	if s.closed {
		return zero, 0, errClosed
	}
	if s.closer == nil {
		conn, closer, err := s.dial(ctx)
		if err != nil {
			return zero, 0, err
		}
		s.conn, s.closer = conn, closer
		s.gen++
	}
	return s.conn, s.gen, nil
}

// drop closes the connection of generation gen when err says it is broken and it is still the live one, so the next get dials again. It is whether err said so.
func (s *session[C]) drop(gen int, err error) bool {
	if err == nil || !s.broken(err) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen == gen && s.closer != nil {
		_ = s.closer.Close()
		var zero C
		s.conn, s.closer = zero, nil
	}
	return true
}

// do runs fn on the live connection and, when it fails because the connection broke, once more on a new one. fn must be safe to run twice.
func (s *session[C]) do(ctx context.Context, fn func(C) error) error {
	conn, gen, err := s.get(ctx)
	if err != nil {
		return err
	}
	err = fn(conn)
	if !s.drop(gen, err) || ctx.Err() != nil {
		return err
	}
	conn, _, err = s.get(ctx)
	if err != nil {
		return err
	}
	return fn(conn)
}

// Close closes the connection; get fails after it.
func (s *session[C]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.closer == nil {
		return nil
	}
	err := s.closer.Close()
	var zero C
	s.conn, s.closer = zero, nil
	return err
}

// checkRange is Open's check of offset and length.
func checkRange(p string, offset, length int64) error {
	if offset < 0 || length < -1 {
		return fmt.Errorf("%s: bad range %d+%d", p, offset, length)
	}
	return nil
}

// countingReader counts the bytes read through it. The count is atomic: net/http may still read a request's body on its own goroutine, to discard what is left of it, after the response came back to the caller that then reads the count.
type countingReader struct {
	r io.Reader
	n atomic.Int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n.Add(int64(n))
	return n, err
}

// shortBody is Write's error for a reader that gave n of size bytes.
func shortBody(p string, n, size int64) error {
	return fmt.Errorf("%s: %d of %d bytes: %w", p, n, size, io.ErrUnexpectedEOF)
}

// fileInfo is what a server's fs.FileInfo tells of the file or folder name, as an Info.
func fileInfo(name string, fi fs.FileInfo) Info {
	info := Info{Name: name, Modified: fi.ModTime(), Dir: fi.IsDir()}
	if !info.Dir {
		info.Size = fi.Size()
	}
	return info
}

// limitedFile is a file read up to a length, closed as the file is.
type limitedFile struct {
	io.Reader
	io.Closer
}
