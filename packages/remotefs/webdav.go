package remotefs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// webdavCleanupTimeout bounds removing the hidden file of a Write that failed, which runs even when the Write's context has ended.
const webdavCleanupTimeout = 30 * time.Second

// webdavStore is a Provider over a folder of a WebDAV server. Every request carries the user and password, by basic or digest authentication as the server asks; over plain HTTP a basic password can be read on the network. Write uploads into a hidden file beside the target, then moves it over the target with MOVE and Overwrite: T, so the target holds the old file or all of the new one; whether a reader can catch the moment between is up to the server (golang.org/x/net/webdav deletes the target before renaming). A Write cut off with the connection may leave its hidden file behind, which List does not show.
type webdavStore struct {
	base      string
	auth      gowebdav.Authorizer
	transport *http.Transport
	// host is the server's, which a CertError names.
	host string
}

func openWebDAV(ctx context.Context, cfg Remote) (*webdavStore, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil {
		return nil, fmt.Errorf("storage: WebDAV address: %w", err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("storage: WebDAV address %q is not an https:// or http:// address", cfg.URL)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("storage: WebDAV address %q holds more than a server and a path", cfg.URL)
	}
	top, err := remoteRoot(u.Path)
	if err != nil {
		return nil, err
	}
	root, err := remoteRoot(cfg.Root)
	if err != nil {
		return nil, err
	}
	// The client escapes each part of the path it is given, so the base is unescaped, and the server's path joined with the root.
	base := u.Scheme + "://" + u.Host + "/" + within(top, root)
	w := &webdavStore{
		base:      base,
		auth:      gowebdav.NewAutoAuth(cfg.User, cfg.Password),
		transport: http.DefaultTransport.(*http.Transport).Clone(),
		host:      u.Hostname(),
	}
	if pin := strings.TrimSpace(cfg.CertFingerprint); pin != "" && u.Scheme == "https" {
		w.transport.TLSClientConfig = pinnedTLS(u.Hostname(), pin)
	}
	if _, err := w.Stat(ctx, ""); err != nil {
		w.Close()
		return nil, err
	}
	return w, nil
}

// client is a client whose requests run under ctx: the library takes no context, so its transport puts ctx on each request.
func (w *webdavStore) client(ctx context.Context) *gowebdav.Client {
	c := gowebdav.NewAuthClient(w.base, w.auth)
	c.SetTransport(ctxTransport{ctx: ctx, next: w.transport, host: w.host})
	return c
}

// ctxTransport puts ctx on each request, and tells a certificate the system refused as a CertError, with its fingerprint for the person to trust.
type ctxTransport struct {
	ctx  context.Context
	next http.RoundTripper
	host string
}

func (t ctxTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(r.WithContext(t.ctx))
	var refused *tls.CertificateVerificationError
	if err != nil && errors.As(err, &refused) && len(refused.UnverifiedCertificates) > 0 {
		return nil, &CertError{Host: t.host, Fingerprint: CertFingerprint(refused.UnverifiedCertificates[0].Raw), Err: refused.Err}
	}
	var cert *CertError
	if errors.As(err, &cert) && cert.Host == "" {
		cert.Host = t.host
	}
	return resp, err
}

// CertError is the error of a WebDAV server over HTTPS whose certificate the system does not trust, as a NAS's own self-signed one (Err says why, Pinned is ""), or, once the person trusted one (Remote.CertFingerprint, Pinned here), whose certificate is not that one: it was renewed, or another machine answers in its place. Fingerprint is the certificate the server offered (CertFingerprint), which becomes Remote.CertFingerprint once the person compared it with the server's and trusted it, as an SFTP server's key does (HostKeyError); Host is the server's.
type CertError struct {
	Host        string
	Fingerprint string
	Pinned      string
	Err         error
}

func (e *CertError) Error() string {
	if e.Pinned != "" {
		return "storage: the certificate of " + e.Host + ", " + e.Fingerprint + ", is not the one trusted, " + e.Pinned
	}
	return fmt.Sprintf("storage: the certificate of %s, %s, is not trusted: %v", e.Host, e.Fingerprint, e.Err)
}

func (e *CertError) Unwrap() error { return e.Err }

// CertFingerprint is a certificate's SHA-256 fingerprint as browsers show it, its DER bytes hashed: the hexadecimal pairs in capitals joined by colons, after "SHA256 ".
func CertFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	pairs := make([]string, len(sum))
	for i, b := range sum {
		pairs[i] = fmt.Sprintf("%02X", b)
	}
	return "SHA256 " + strings.Join(pairs, ":")
}

// pinnedTLS is how a server whose certificate the person trusted (pin, a CertFingerprint) is reached: that certificate alone, by its fingerprint, in place of the system's verification, as a NAS's own certificate rarely names the address it is reached by.
func pinnedTLS(host, pin string) *tls.Config {
	return &tls.Config{
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // The certificate is checked against the fingerprint trusted, in VerifyConnection.
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("storage: the server sent no certificate")
			}
			if got := CertFingerprint(cs.PeerCertificates[0].Raw); got != pin {
				return &CertError{Host: host, Fingerprint: got, Pinned: pin}
			}
			return nil
		},
	}
}

// webdavErr is err with a 404 read as ErrNotExist.
func webdavErr(err error) error {
	if err != nil && gowebdav.IsErrNotFound(err) {
		return fmt.Errorf("%w: %v", ErrNotExist, err)
	}
	return err
}

func webdavInfo(name string, fi os.FileInfo) Info {
	info := fileInfo(name, fi)
	if f, ok := fi.(gowebdav.File); ok {
		info.ETag = f.ETag()
	} else if f, ok := fi.(*gowebdav.File); ok {
		info.ETag = f.ETag()
	}
	return info
}

func (w *webdavStore) Stat(ctx context.Context, p string) (Info, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Info{}, err
	}
	fi, err := w.client(ctx).Stat(p)
	if err != nil {
		return Info{}, webdavErr(err)
	}
	if f, ok := fi.(*gowebdav.File); !ok || f == nil {
		// A 207 that held no properties of p.
		return Info{}, fmt.Errorf("%s: %w", p, ErrNotExist)
	}
	info := webdavInfo(path.Base(p), fi)
	if p == "" {
		info.Name = ""
	}
	return info, nil
}

// List is what lies in the folder dir, leaving out the files a Write is still writing.
func (w *webdavStore) List(ctx context.Context, dir string) ([]Info, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	entries, err := w.client(ctx).ReadDir(dir)
	if err != nil {
		return nil, webdavErr(err)
	}
	out := make([]Info, 0, len(entries))
	for _, fi := range entries {
		if !strings.HasPrefix(fi.Name(), TempPrefix) {
			out = append(out, webdavInfo(fi.Name(), fi))
		}
	}
	return out, nil
}

// Open asks for the file's size, then for the bytes wanted with a Range request; a server that ignores the range sends the whole file, of which the bytes before offset are skipped.
func (w *webdavStore) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if err := checkRange(p, offset, length); err != nil {
		return nil, err
	}
	p, err := CleanFile(p)
	if err != nil {
		return nil, err
	}
	// Stat first: a GET of a folder is a listing page on some servers and an error on others, and a range from the end of the file is 416.
	info, err := w.Stat(ctx, p)
	if err != nil {
		return nil, err
	}
	if info.Dir {
		return nil, fmt.Errorf("%s: %w", p, ErrIsDir)
	}
	if length < 0 || offset+length > info.Size {
		length = max(info.Size-offset, 0)
	}
	if length == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}
	body, err := w.client(ctx).ReadStreamRange(p, offset, length)
	if err != nil {
		return nil, webdavErr(err)
	}
	return limitedFile{Reader: io.LimitReader(body, length), Closer: body}, nil
}

// Write uploads r into a hidden file beside p and moves it over p.
func (w *webdavStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
	p, err := CleanFile(p)
	if err != nil {
		return err
	}
	if size < 0 {
		return fmt.Errorf("%s: unknown size", p)
	}
	c := w.client(ctx)
	// Making the folder also settles how the server wants to be authenticated, so the upload goes through on its first request: the library resends a body that was refused, and keeps a copy of all of it to do so unless it can seek back.
	if dir := parent(p); dir != "" {
		if err := c.MkdirAll(dir, 0o755); err != nil {
			return webdavErr(err)
		}
	} else if _, err := c.Stat(""); err != nil {
		return webdavErr(err)
	}
	temp := path.Join(parent(p), TempPrefix+rand.Text())
	body := &onceSeeker{countingReader: countingReader{r: io.LimitReader(r, size)}}
	err = c.WriteStreamWithLength(temp, body, size, 0o644)
	if n := body.n.Load(); err == nil && n < size {
		err = shortBody(p, n, size)
	}
	if err == nil {
		err = c.Rename(temp, p, true)
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), webdavCleanupTimeout)
		defer cancel()
		_ = w.client(cleanup).Remove(temp)
		return webdavErr(err)
	}
	return nil
}

// onceSeeker is a body read once that says it can seek, so the library streams it rather than copying it into memory to resend; seeking back to the start works only while nothing has been read, so a resend after part of the body went fails instead of sending the wrong bytes.
type onceSeeker struct {
	countingReader
}

func (o *onceSeeker) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && (whence == io.SeekStart || whence == io.SeekCurrent) && o.n.Load() == 0 {
		return 0, nil
	}
	return 0, errors.New("storage: the body cannot be read again")
}

// Remove removes the file or the empty folder at p. DELETE removes a folder with all it holds, so a folder is listed first.
func (w *webdavStore) Remove(ctx context.Context, p string) error {
	p, err := CleanFile(p)
	if err != nil {
		return err
	}
	info, err := w.Stat(ctx, p)
	if errors.Is(err, ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Dir {
		entries, err := w.client(ctx).ReadDir(p)
		if err != nil {
			return webdavErr(err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s: %w", p, ErrNotEmpty)
		}
	}
	return webdavErr(w.client(ctx).Remove(p))
}

// Move makes the folders above to, then moves from to to with MOVE and Overwrite: T.
func (w *webdavStore) Move(ctx context.Context, from, to string) error {
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
	if _, err := w.Stat(ctx, from); err != nil {
		return err
	}
	// Servers answer a MOVE into a missing folder with 409 or, as golang.org/x/net/webdav does, 403, so the folder is made first.
	if err := w.MkdirAll(ctx, parent(to)); err != nil {
		return err
	}
	return webdavErr(w.client(ctx).Rename(from, to, true))
}

func (w *webdavStore) MkdirAll(ctx context.Context, dir string) error {
	dir, err := CleanPath(dir)
	if err != nil {
		return err
	}
	if dir == "" {
		_, err := w.Stat(ctx, "")
		return err
	}
	return webdavErr(w.client(ctx).MkdirAll(dir, 0o755))
}

// Close drops the idle connections to the server.
func (w *webdavStore) Close() error {
	w.transport.CloseIdleConnections()
	return nil
}
