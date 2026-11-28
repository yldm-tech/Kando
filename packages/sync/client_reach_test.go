package sync

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	gosync "sync"
	"sync/atomic"
	"testing"
	"time"
)

// silentListener stands for a desktop's network as the phone sees it once the desktop is gone without hanging up: while silent, every connection it has accepted, and every one it accepts, stays open but carries nothing either way, as one left half-open by a desktop quit and started again while the phone slept. A connection silenced stays so once the desktop answers again; only those accepted after it carry what they are sent.
type silentListener struct {
	net.Listener
	silent atomic.Bool
	mu     gosync.Mutex
	conns  []*silentConn
}

type silentConn struct {
	net.Conn
	dead atomic.Bool
}

func (l *silentListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	sc := &silentConn{Conn: c}
	sc.dead.Store(l.silent.Load())
	l.mu.Lock()
	l.conns = append(l.conns, sc)
	l.mu.Unlock()
	return sc, nil
}

// silence stops or resumes the desktop's answers.
func (l *silentListener) silence(on bool) {
	l.silent.Store(on)
	if !on {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.conns {
		c.dead.Store(true)
	}
}

// hangUp closes every connection, silenced or not, as the test ends.
func (l *silentListener) hangUp() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.conns {
		c.Conn.Close()
	}
}

func (c *silentConn) Read(p []byte) (int, error) {
	for {
		n, err := c.Conn.Read(p)
		if err != nil || !c.dead.Load() {
			return n, err
		}
	}
}

func (c *silentConn) Write(p []byte) (int, error) {
	if c.dead.Load() {
		return len(p), nil
	}
	return c.Conn.Write(p)
}

// fastBounds makes the client's connection bounds short for the test's sake.
func fastBounds(t *testing.T) {
	t.Helper()
	was := [...]time.Duration{dialFor, handshakeFor, pingAfter, pingFor}
	dialFor, handshakeFor, pingAfter, pingFor = time.Second, 300*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { dialFor, handshakeFor, pingAfter, pingFor = was[0], was[1], was[2], was[3] })
}

// http2Desktop serves handler over HTTP/2, as Kando's server does, behind a listener the test can silence.
func http2Desktop(t *testing.T, handler http.Handler) (*silentListener, *Client) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.EnableHTTP2 = true
	l := &silentListener{Listener: srv.Listener}
	srv.Listener = l
	srv.StartTLS()
	t.Cleanup(srv.Close)
	t.Cleanup(l.hangUp)
	c := NewClient(netip.MustParseAddrPort(strings.TrimPrefix(srv.URL, "https://")), Fingerprint(srv.Certificate().Raw), "tok")
	t.Cleanup(c.Close)
	return l, c
}

// A request on a connection the desktop stopped answering on fails once the desktop answers no ping, rather than waiting as long as its context lets it, and the next goes over a new connection to the desktop answering again.
func TestClientGivesUpOnAConnectionTheDesktopNoLongerAnswers(t *testing.T) {
	fastBounds(t)
	var protos atomic.Int32
	l, c := http2Desktop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protos.Store(int32(r.ProtoMajor))
		_ = json.NewEncoder(w).Encode(Libraries{Libraries: []Library{{ID: "lib"}}})
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := c.Libraries(ctx); err != nil || protos.Load() != 2 {
		t.Fatalf("libraries: %v over HTTP/%d", err, protos.Load())
	}

	l.silence(true)
	began := time.Now()
	if _, err := c.Libraries(ctx); err == nil {
		t.Fatal("a desktop that does not answer answered")
	}
	if took := time.Since(began); took > 5*time.Second {
		t.Fatalf("the request waited %v for a desktop that does not answer", took)
	}

	l.silence(false)
	if libs, err := c.Libraries(ctx); err != nil || len(libs) != 1 {
		t.Fatalf("libraries once the desktop answers again: %+v, %v", libs, err)
	}
}

// A stream the desktop sends slowly, as a long video or an event wait, is not cut while the desktop answers the pings, however long it takes.
func TestClientKeepsASlowStreamTheDesktopAnswers(t *testing.T) {
	fastBounds(t)
	const chunks = 12
	_, c := http2Desktop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		for range chunks {
			_, _ = io.WriteString(w, "x")
			w.(http.Flusher).Flush()
			time.Sleep(150 * time.Millisecond)
		}
	}))
	data, _, err := c.Poster(context.Background(), "f1")
	if err != nil || len(data) != chunks {
		t.Fatalf("a slow poster came as %d bytes, %v", len(data), err)
	}
}

// Reconnect sends what follows over a new connection, leaving the request under way on the one before.
func TestClientReconnectLeavesARequestUnderWay(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	_, c := http2Desktop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PosterPath("slow") {
			w.Header().Set("Content-Type", "image/jpeg")
			close(started)
			<-release
			_, _ = io.WriteString(w, "jpeg")
			return
		}
		_ = json.NewEncoder(w).Encode(Libraries{})
	}))
	ctx := context.Background()
	got := make(chan error, 1)
	go func() {
		data, _, err := c.Poster(ctx, "slow")
		if err == nil && string(data) != "jpeg" {
			err = io.ErrUnexpectedEOF
		}
		got <- err
	}()
	<-started
	c.Reconnect()
	if _, err := c.Libraries(ctx); err != nil {
		t.Fatalf("libraries after Reconnect: %v", err)
	}
	close(release)
	if err := <-got; err != nil {
		t.Fatalf("the request under way as the client reconnected: %v", err)
	}
}
