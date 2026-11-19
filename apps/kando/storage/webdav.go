package storage

import (
	"context"
	"crypto/rand"
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
	c.SetTransport(ctxTransport{ctx: ctx, next: w.transport})
	return c
}

type ctxTransport struct {
	ctx  context.Context
	next http.RoundTripper
}

func (t ctxTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.next.RoundTrip(r.WithContext(t.ctx))
}

// webdavErr is err with a 404 read as ErrNotExist.
func webdavErr(err error) error {
	if err != nil && gowebdav.IsErrNotFound(err) {
		return fmt.Errorf("%w: %v", ErrNotExist, err)
	}
	return err
}

func webdavInfo(name string, fi os.FileInfo) Info {
	info := localInfo(name, fi)
	if f, ok := fi.(gowebdav.File); ok {
		info.ETag = f.ETag()
	} else if f, ok := fi.(*gowebdav.File); ok {
		info.ETag = f.ETag()
	}
	return info
}

func (w *webdavStore) Stat(ctx context.Context, p string) (Info, error) {
	p, err := cleanPath(p)
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
	dir, err := cleanPath(dir)
	if err != nil {
		return nil, err
	}
	entries, err := w.client(ctx).ReadDir(dir)
	if err != nil {
		return nil, webdavErr(err)
	}
	out := make([]Info, 0, len(entries))
	for _, fi := range entries {
		if !strings.HasPrefix(fi.Name(), tempPrefix) {
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
	p, err := cleanFile(p)
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
	p, err := cleanFile(p)
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
	temp := path.Join(parent(p), tempPrefix+rand.Text())
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
	p, err := cleanFile(p)
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
	dir, err := cleanPath(dir)
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
