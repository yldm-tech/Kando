package remotefs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	gosync "sync"
	"sync/atomic"
	"testing"
	"time"
)

// wedgingConn is a connection to a store that, as an SMB connection can, is left unusable by a request cut off by its context: every request after it waits until its own context ends. answer blocks the requests until it is closed, so a test can cut one off while it waits.
type wedgingConn struct {
	answer <-chan struct{}
	mu     gosync.Mutex
	wedged bool
	closed bool
}

// ask is a request on the connection.
func (c *wedgingConn) ask(ctx context.Context) error {
	c.mu.Lock()
	wedged, closed := c.wedged, c.closed
	c.mu.Unlock()
	if closed {
		return errors.New("use of a closed connection")
	}
	if wedged {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case <-c.answer:
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		c.wedged = true
		c.mu.Unlock()
		return ctx.Err()
	}
}

func (c *wedgingConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

// wedgingStore is a Reader of one file, "film.mp4", over a session of wedgingConns, as smbStore is over SMB connections: a Stat asks the connection, and so does each Read of an opened file.
type wedgingStore struct {
	s      session[*wedgingConn]
	answer chan struct{}
	dials  atomic.Int32
	body   string
}

func newWedgingStore(cancelBreaks bool) *wedgingStore {
	w := &wedgingStore{answer: make(chan struct{}), body: "0123456789"}
	w.s.broken = func(error) bool { return false }
	w.s.cancelBreaks = cancelBreaks
	w.s.dial = func(context.Context) (*wedgingConn, io.Closer, error) {
		w.dials.Add(1)
		c := &wedgingConn{answer: w.answer}
		return c, c, nil
	}
	return w
}

func (w *wedgingStore) Stat(ctx context.Context, p string) (Info, error) {
	if p != "film.mp4" {
		return Info{}, ErrNotExist
	}
	err := w.s.do(ctx, func(c *wedgingConn) error { return c.ask(ctx) })
	return Info{Name: p, Size: int64(len(w.body)), Modified: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}, err
}

func (w *wedgingStore) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	var conn *wedgingConn
	gen, err := w.s.doOn(ctx, func(c *wedgingConn) error { conn = c; return nil })
	if err != nil {
		return nil, err
	}
	return w.s.guard(ctx, gen, &wedgingFile{conn: conn, ctx: ctx, r: strings.NewReader(w.body[offset:])}), nil
}

// wedgingFile reads its bytes, each read a request on its connection.
type wedgingFile struct {
	conn *wedgingConn
	ctx  context.Context
	r    io.Reader
}

func (f *wedgingFile) Read(b []byte) (int, error) {
	if err := f.conn.ask(f.ctx); err != nil {
		return 0, err
	}
	return f.r.Read(b)
}

func (f *wedgingFile) Close() error { return nil }

// A request cut off by its context while the store has not answered, as a phone pausing a download before its first byte, leaves the connection to be made anew: the next request is answered on a new connection rather than waiting on the wedged one.
func TestARequestCutOffDoesNotWedgeTheNextOne(t *testing.T) {
	w := newWedgingStore(true)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := w.Stat(ctx, "film.mp4"); !errors.Is(err, context.Canceled) {
		t.Fatalf("the request cut off: %v", err)
	}
	close(w.answer)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := w.Stat(ctx, "film.mp4"); err != nil {
		t.Fatalf("the next request: %v", err)
	}
	if n := w.dials.Load(); n != 2 {
		t.Fatalf("%d connections made, want 2", n)
	}
}

// A file whose read is cut off by its context, as a download paused partway, or that is closed after its context ended, leaves its connection to be made anew too.
func TestAReadCutOffDoesNotWedgeTheNextOne(t *testing.T) {
	w := newWedgingStore(true)
	ctx, cancel := context.WithCancel(context.Background())
	f, err := w.Open(ctx, "film.mp4", 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := f.Read(make([]byte, 4)); !errors.Is(err, context.Canceled) {
		t.Fatalf("the read cut off: %v", err)
	}
	f.Close()
	close(w.answer)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := w.Stat(ctx, "film.mp4"); err != nil || w.dials.Load() != 2 {
		t.Fatalf("the next request: %v, on connection %d", err, w.dials.Load())
	}
}

// Handler gives a store firstByteTimeout to find the file and give its first bytes: one that does not answer fails with 504 then, rather than holding the request until the store gives up, and the request after it, once the store answers, is served on a new connection.
func TestHandlerDoesNotWaitOnAStoreThatDoesNotAnswer(t *testing.T) {
	was := firstByteTimeout
	firstByteTimeout = 100 * time.Millisecond
	t.Cleanup(func() { firstByteTimeout = was })
	w := newWedgingStore(true)
	srv := httptest.NewServer(Handler(w, "film.mp4"))
	t.Cleanup(srv.Close)

	start := time.Now()
	resp, _ := get(t, srv.URL, http.Header{"Range": {"bytes=0-"}})
	if resp.StatusCode != http.StatusGatewayTimeout || time.Since(start) > 5*time.Second {
		t.Fatalf("a store that does not answer: %d after %v", resp.StatusCode, time.Since(start))
	}
	close(w.answer)
	resp, body := get(t, srv.URL, http.Header{"Range": {"bytes=2-"}})
	if resp.StatusCode != http.StatusPartialContent || body != "23456789" {
		t.Fatalf("once the store answers: %d %q", resp.StatusCode, body)
	}
}

// A store that finds the file but gives none of its bytes in time has its answer cut off then; one that gives its first bytes in time goes on past firstByteTimeout.
func TestHandlerCutsOffAFileWhoseFirstBytesDoNotCome(t *testing.T) {
	was := firstByteTimeout
	firstByteTimeout = 200 * time.Millisecond
	t.Cleanup(func() { firstByteTimeout = was })
	slow := &slowReader{first: time.Hour}
	srv := httptest.NewServer(Handler(slow, "film.mp4"))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("a body whose first bytes do not come: %v after %v", err, time.Since(start))
	}

	slow.first, slow.rest = 0, 100*time.Millisecond
	resp, body := get(t, srv.URL, nil)
	if resp.StatusCode != http.StatusOK || body != "0123456789" {
		t.Fatalf("a slow body begun in time: %d %q", resp.StatusCode, body)
	}
}

// slowReader is a Reader of one file whose first byte takes first and each byte after it rest.
type slowReader struct{ first, rest time.Duration }

func (s *slowReader) Stat(context.Context, string) (Info, error) {
	return Info{Name: "film.mp4", Size: 10, Modified: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}, nil
}

func (s *slowReader) Open(ctx context.Context, _ string, offset, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(&slowBody{ctx: ctx, s: *s, left: "0123456789"[offset:]}), nil
}

type slowBody struct {
	ctx   context.Context
	s     slowReader
	left  string
	begun bool
}

func (b *slowBody) Read(p []byte) (int, error) {
	if b.left == "" {
		return 0, io.EOF
	}
	wait := b.s.rest
	if !b.begun {
		wait = b.s.first
	}
	select {
	case <-time.After(wait):
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
	b.begun = true
	p[0] = b.left[0]
	b.left = b.left[1:]
	return 1, nil
}
