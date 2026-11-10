package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ErrStalled is a download's error when its server stopped sending: no byte arrived for stallTimeout, or no answer came at all. It is not context.Canceled, which is the caller's own cancel.
var ErrStalled = errors.New("download stalled")

// ErrDisconnected is wrapped by a download's error when the connection to its server could not be made or broke off before the whole file was in.
var ErrDisconnected = errors.New("download connection lost")

// stallTimeout is how long a download waits for its next bytes, or for its server's answer, before it gives up with ErrStalled. Tests shorten it.
var stallTimeout = 45 * time.Second

// downloadClient sends the downloads' requests. It has no overall timeout, which would end a slow download that is still making progress: a server that has not started to answer 30 seconds after the request was sent fails it instead, and the watchdog of downloadGet watches the body.
var downloadClient = &http.Client{Transport: downloadTransport()}

func downloadTransport() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return transport
}

// downloadStatusError is a download's server answering with something other than the file.
type downloadStatusError struct {
	name string
	code int
}

// StatusCode is the HTTP status the server answered with.
func (e *downloadStatusError) StatusCode() int { return e.code }

func (e *downloadStatusError) Error() string {
	return fmt.Sprintf("download %s: HTTP %d", e.name, e.code)
}

// stallWatch ends a download's request once no byte has arrived for stallTimeout. parent is the caller's context, whose own end is never reported as a stall.
type stallWatch struct {
	parent  context.Context
	cancel  context.CancelFunc
	timer   *time.Timer
	stalled atomic.Bool
}

// downloadGet sends a GET for url with header and returns the server's response, whatever its status. From the moment the request is sent until the body is closed, a watchdog ends the request once nothing has arrived for stallTimeout. Its errors, and those of reading the body, are ctx's own once ctx is done, ErrStalled for a stall, and wrap ErrDisconnected for a connection that could not be made or broke off.
func downloadGet(ctx context.Context, url string, header http.Header) (*http.Response, error) {
	watched, cancel := context.WithCancel(ctx)
	w := &stallWatch{parent: ctx, cancel: cancel}
	w.timer = time.AfterFunc(stallTimeout, func() {
		w.stalled.Store(true)
		cancel()
	})
	req, err := http.NewRequestWithContext(watched, http.MethodGet, url, nil)
	if err != nil {
		w.stop()
		return nil, err
	}
	for key, values := range header {
		req.Header[key] = values
	}
	req.Header.Set("User-Agent", "Kando")
	resp, err := downloadClient.Do(req)
	if err != nil {
		w.stop()
		return nil, w.explain(err)
	}
	resp.Body = &watchedBody{ReadCloser: resp.Body, w: w}
	return resp, nil
}

func (w *stallWatch) stop() {
	w.timer.Stop()
	w.cancel()
}

// explain is the error a download reports for err, an error of sending its request or reading its body: nil and io.EOF as they are, the caller's own cancel or deadline, ErrStalled, or err wrapped in ErrDisconnected.
func (w *stallWatch) explain(err error) error {
	if err == nil || err == io.EOF {
		return err
	}
	if ctxErr := w.parent.Err(); ctxErr != nil {
		return ctxErr
	}
	if w.stalled.Load() {
		return ErrStalled
	}
	// The transport's own wait for the answer (ResponseHeaderTimeout) and a dial that timed out are stalls too.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrStalled
	}
	return fmt.Errorf("%w: %w", ErrDisconnected, err)
}

// watchedBody is a download's body: every read that brings bytes puts the watchdog back to a full stallTimeout, and closing it stops the watchdog.
type watchedBody struct {
	io.ReadCloser
	w *stallWatch
}

func (b *watchedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.w.timer.Reset(stallTimeout)
	}
	return n, b.w.explain(err)
}

func (b *watchedBody) Close() error {
	b.w.stop()
	return b.ReadCloser.Close()
}

// parseContentRange reads the Content-Range of a 206 or 416 answer, "bytes 100-199/1000" or "bytes */1000": where the bytes sent start (-1 for "*") and the size of the whole file (-1 when the server writes "*" for it).
func parseContentRange(value string) (start, size int64, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(value), "bytes ")
	if !found {
		return 0, 0, false
	}
	span, whole, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, false
	}
	size = -1
	if whole != "*" {
		n, err := strconv.ParseInt(whole, 10, 64)
		if err != nil || n < 0 {
			return 0, 0, false
		}
		size = n
	}
	if span == "*" {
		return -1, size, true
	}
	first, _, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	return start, size, true
}
