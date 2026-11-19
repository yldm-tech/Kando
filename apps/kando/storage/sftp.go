package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ErrUnknownHostKey is the error of connecting to an SFTP server without a host key to check its key against. It comes inside a HostKeyError that holds the key the server offered.
var ErrUnknownHostKey = errors.New("storage: the server's host key is not trusted yet")

// ErrPrivateKey is the error of an SFTP private key that cannot be read: not a PEM key, an encrypted one without its passphrase, or a wrong passphrase. The parser's error comes with it.
var ErrPrivateKey = errors.New("storage: SFTP private key")

// ErrHostKeyMismatch is the error of an SFTP server that offered a key other than the trusted one: the server was reinstalled, or another machine answers in its place. It comes inside a HostKeyError that holds the key the server offered.
var ErrHostKeyMismatch = errors.New("storage: the server's host key is not the trusted one")

// HostKeyError is the error of an SFTP server whose host key Kando does not trust. Key is the key the server offered as a line of authorized_keys, which becomes Remote.HostKey once the person trusts it, and Fingerprint is its SHA-256 fingerprint ("SHA256:..."), the form a server's administrator can compare with ssh-keygen -l. Trusted is the key Remote.HostKey held, "" when it held none. It is ErrUnknownHostKey when Trusted is "", else ErrHostKeyMismatch.
type HostKeyError struct {
	Host        string
	Key         string
	Fingerprint string
	Trusted     string
}

func (e *HostKeyError) Error() string {
	return fmt.Sprintf("%v: %s offered %s", e.Unwrap(), e.Host, e.Fingerprint)
}

func (e *HostKeyError) Unwrap() error {
	if e.Trusted == "" {
		return ErrUnknownHostKey
	}
	return ErrHostKeyMismatch
}

// sftpDialTimeout bounds connecting, the SSH handshake and starting SFTP when ctx sets no deadline of its own.
const sftpDialTimeout = 30 * time.Second

// sftpStore is a Provider over a folder of an SFTP server. It keeps one SSH connection and makes it again when it breaks. Write uploads into a hidden file beside the target and renames it into place with the server's posix-rename, which replaces the target in one step; a server without that extension has the target removed first, so a reader in between finds nothing there.
type sftpStore struct {
	root string
	s    session[*sftpConn]
}

// sftpConn is one SSH connection and the SFTP session on it. dead is set once the connection ends.
type sftpConn struct {
	ssh  *ssh.Client
	sftp *sftp.Client
	dead atomic.Bool
}

func (c *sftpConn) Close() error {
	err := c.sftp.Close()
	if cerr := c.ssh.Close(); err == nil {
		err = cerr
	}
	return err
}

func openSFTP(ctx context.Context, cfg Remote) (*sftpStore, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, errors.New("storage: SFTP needs a host")
	}
	if cfg.User == "" {
		return nil, errors.New("storage: SFTP needs a user")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var auth []ssh.AuthMethod
	if cfg.PrivateKey != "" {
		var signer ssh.Signer
		var err error
		if cfg.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cfg.PrivateKey), []byte(cfg.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrPrivateKey, err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		password := cfg.Password
		// Many NAS systems ask for the password by keyboard-interactive rather than by the password method.
		auth = append(auth, ssh.Password(password), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = password
			}
			return answers, nil
		}))
	}
	if len(auth) == 0 {
		return nil, errors.New("storage: SFTP needs a password or a private key")
	}
	root, err := remoteRoot(cfg.Root)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.Root), "/") {
		root = "/" + root
	} else if root == "" {
		root = "."
	}
	trusted := strings.TrimSpace(cfg.HostKey)
	var want ssh.PublicKey
	if trusted != "" {
		want, _, _, _, err = ssh.ParseAuthorizedKey([]byte(trusted))
		if err != nil {
			return nil, fmt.Errorf("storage: SFTP host key: %w", err)
		}
	}
	st := &sftpStore{root: root}
	st.s.broken = sftpBroken
	st.s.dial = func(ctx context.Context) (*sftpConn, io.Closer, error) {
		c, err := dialSFTP(ctx, addr, cfg.User, auth, want)
		if err != nil {
			var hk *HostKeyError
			if errors.As(err, &hk) {
				hk.Host, hk.Trusted = host, trusted
			}
			return nil, nil, err
		}
		return c, c, nil
	}
	// Connect now, so a wrong host key, password or root is found when the provider is opened, then check the root is a folder.
	if _, err := st.Stat(ctx, ""); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

// dialSFTP connects to addr and starts SFTP, accepting only the host key want; with want nil, or a key other than want, it fails with a HostKeyError holding the key the server offered.
func dialSFTP(ctx context.Context, addr, user string, auth []ssh.AuthMethod, want ssh.PublicKey) (*sftpConn, error) {
	// refused is kept here rather than found in the handshake's error, which need not wrap it.
	var refused *HostKeyError
	config := &ssh.ClientConfig{
		User: user,
		Auth: auth,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if want == nil || !bytes.Equal(key.Marshal(), want.Marshal()) {
				refused = &HostKeyError{Key: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), Fingerprint: ssh.FingerprintSHA256(key)}
				return refused
			}
			return nil
		},
	}
	if want != nil {
		// Ask for the kind of key that is trusted, so a server with keys of several kinds does not offer another one.
		config.HostKeyAlgorithms = hostKeyAlgorithms(want.Type())
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sftpDialTimeout)
		defer cancel()
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	// The handshake does not take a context; closing the connection when ctx ends stops it.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		stop()
		conn.Close()
		if refused != nil {
			return nil, refused
		}
		return nil, err
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	sc, err := sftp.NewClient(client)
	if !stop() {
		err = errors.Join(err, ctx.Err())
	}
	if err != nil {
		if sc != nil {
			sc.Close()
		}
		client.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	c := &sftpConn{ssh: client, sftp: sc}
	go func() {
		_ = client.Wait()
		c.dead.Store(true)
	}()
	return c, nil
}

// hostKeyAlgorithms are the algorithms a host key of type kind is offered under.
func hostKeyAlgorithms(kind string) []string {
	if kind == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{kind}
}

// sftpBroken is whether err says the connection is gone. A request sent as the connection closes fails with io.EOF, which no request of a live session ends with: reading a file to its end does not go through the session.
func sftpBroken(err error) bool {
	return errors.Is(err, sftp.ErrSSHFxConnectionLost) || errors.Is(err, sftp.ErrSSHFxNoConnection) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
}

// do runs fn with the SFTP session, dialing again first when the connection is known to have ended, and once more on a new connection when fn finds it broken.
func (s *sftpStore) do(ctx context.Context, fn func(c *sftp.Client) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c, gen, err := s.s.get(ctx); err == nil && c.dead.Load() {
		s.s.drop(gen, sftp.ErrSSHFxConnectionLost)
	}
	return s.s.do(ctx, func(c *sftpConn) error { return fn(c.sftp) })
}

// path is the server's path of the clean path p.
func (s *sftpStore) path(p string) string {
	if p == "" {
		return s.root
	}
	return path.Join(s.root, p)
}

// sftpErr is err with a missing path read as ErrNotExist.
func sftpErr(err error) error {
	if err != nil && errors.Is(err, fs.ErrNotExist) && !errors.Is(err, ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrNotExist, err)
	}
	return err
}

func (s *sftpStore) Stat(ctx context.Context, p string) (Info, error) {
	p, err := cleanPath(p)
	if err != nil {
		return Info{}, err
	}
	var info Info
	err = s.do(ctx, func(c *sftp.Client) error {
		fi, err := c.Stat(s.path(p))
		if err != nil {
			return err
		}
		info = localInfo(path.Base(p), fi)
		if p == "" {
			info.Name = ""
		}
		return nil
	})
	return info, sftpErr(err)
}

// List is what lies in the folder dir, leaving out the files a Write is still writing.
func (s *sftpStore) List(ctx context.Context, dir string) ([]Info, error) {
	dir, err := cleanPath(dir)
	if err != nil {
		return nil, err
	}
	var out []Info
	err = s.do(ctx, func(c *sftp.Client) error {
		entries, err := c.ReadDirContext(ctx, s.path(dir))
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
	return out, sftpErr(err)
}

func (s *sftpStore) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if err := checkRange(p, offset, length); err != nil {
		return nil, err
	}
	p, err := cleanFile(p)
	if err != nil {
		return nil, err
	}
	var f *sftp.File
	err = s.do(ctx, func(c *sftp.Client) error {
		var err error
		f, err = c.Open(s.path(p))
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
		return nil, sftpErr(err)
	}
	if length < 0 {
		return f, nil
	}
	return limitedFile{Reader: io.LimitReader(f, length), Closer: f}, nil
}

// Write uploads r into a hidden file beside p and renames it over p.
func (s *sftpStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
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
	c := conn.sftp
	temp := path.Join(path.Dir(s.path(p)), tempPrefix+rand.Text())
	err = func() error {
		f, err := c.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return err
		}
		stop := context.AfterFunc(ctx, func() { f.Close() })
		defer stop()
		n, err := f.ReadFrom(io.LimitReader(r, size))
		if err == nil && n < size {
			err = shortBody(p, n, size)
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = ctx.Err()
		}
		return err
	}()
	if err == nil {
		err = s.replace(c, temp, s.path(p))
	}
	if err != nil {
		if !s.s.drop(gen, err) {
			_ = c.Remove(temp)
		}
		return sftpErr(err)
	}
	return nil
}

// replace renames from over to, in one step with posix-rename, else by removing a file at to first.
func (s *sftpStore) replace(c *sftp.Client, from, to string) error {
	if _, ok := c.HasExtension("posix-rename@openssh.com"); ok {
		return c.PosixRename(from, to)
	}
	if fi, err := c.Stat(to); err == nil && !fi.IsDir() {
		if err := c.Remove(to); err != nil {
			return err
		}
	}
	return c.Rename(from, to)
}

// Remove removes the file or the empty folder at p.
func (s *sftpStore) Remove(ctx context.Context, p string) error {
	p, err := cleanFile(p)
	if err != nil {
		return err
	}
	err = s.do(ctx, func(c *sftp.Client) error {
		name := s.path(p)
		fi, err := c.Lstat(name)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return c.Remove(name)
		}
		entries, err := c.ReadDirContext(ctx, name)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s: %w", p, ErrNotEmpty)
		}
		return c.RemoveDirectory(name)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Move renames from to to, making the folders above to; a file at to is replaced as Write replaces it.
func (s *sftpStore) Move(ctx context.Context, from, to string) error {
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
	err = s.do(ctx, func(c *sftp.Client) error {
		if _, err := c.Lstat(s.path(from)); err != nil {
			return err
		}
		if err := c.MkdirAll(s.path(parent(to))); err != nil {
			return err
		}
		return s.replace(c, s.path(from), s.path(to))
	})
	return sftpErr(err)
}

func (s *sftpStore) MkdirAll(ctx context.Context, dir string) error {
	dir, err := cleanPath(dir)
	if err != nil {
		return err
	}
	return sftpErr(s.do(ctx, func(c *sftp.Client) error { return c.MkdirAll(s.path(dir)) }))
}

// Close ends the SSH connection.
func (s *sftpStore) Close() error { return s.s.Close() }
