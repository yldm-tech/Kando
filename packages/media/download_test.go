package media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// shortStall makes downloads give up after d without bytes, for the length of the test.
func shortStall(t *testing.T, d time.Duration) {
	t.Helper()
	saved := stallTimeout
	stallTimeout = d
	t.Cleanup(func() { stallTimeout = saved })
}

// stallingServer answers with a Content-Length of size, sends the first sent bytes of body and then nothing until the client goes away.
func stallingServer(t *testing.T, body []byte, sent int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body[:sent])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadGetStallsMidBody(t *testing.T) {
	shortStall(t, 150*time.Millisecond)
	body := make([]byte, 64<<10)
	srv := stallingServer(t, body, 1000)
	begun := time.Now()
	resp, err := downloadGet(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, err = copyProgress(io.Discard, resp.Body, 0, resp.ContentLength, nil)
	if !errors.Is(err, ErrStalled) || errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	if took := time.Since(begun); took > 5*time.Second {
		t.Fatalf("the stall took %v to notice", took)
	}
}

func TestDownloadGetStallsBeforeTheAnswer(t *testing.T) {
	shortStall(t, 150*time.Millisecond)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(srv.Close)
	if _, err := downloadGet(context.Background(), srv.URL, nil); !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
}

// A slow download that keeps sending is not a stall, however long it takes in all.
func TestDownloadGetKeepsASlowDownload(t *testing.T) {
	shortStall(t, 300*time.Millisecond)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "8")
		for range 8 {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	resp, err := downloadGet(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if n, err := copyProgress(io.Discard, resp.Body, 0, resp.ContentLength, nil); err != nil || n != 8 {
		t.Fatalf("copied %d, err %v", n, err)
	}
}

func TestDownloadGetCancelIsNotAStall(t *testing.T) {
	shortStall(t, time.Minute)
	srv := stallingServer(t, make([]byte, 1000), 10)
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := downloadGet(ctx, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err = copyProgress(io.Discard, resp.Body, 0, resp.ContentLength, nil)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrStalled) || errors.Is(err, ErrDisconnected) {
		t.Fatalf("err = %v, want context.Canceled alone", err)
	}
}

func TestDownloadGetDroppedConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write(make([]byte, 10))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)
	resp, err := downloadGet(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, err = copyProgress(io.Discard, resp.Body, 0, resp.ContentLength, nil)
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("err = %v, want ErrDisconnected", err)
	}
	// Nothing listens once the server is gone.
	srv.Close()
	if _, err := downloadGet(context.Background(), srv.URL, nil); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("err = %v, want ErrDisconnected", err)
	}
}

func TestParseContentRange(t *testing.T) {
	for _, tc := range []struct {
		value       string
		start, size int64
		ok          bool
	}{
		{"bytes 100-199/1000", 100, 1000, true},
		{"bytes 0-0/77691713", 0, 77691713, true},
		{"bytes 100-199/*", 100, -1, true},
		{"bytes */1000", -1, 1000, true},
		{"bytes=100-199/1000", 0, 0, false},
		{"bytes 100-199", 0, 0, false},
		{"bytes x-199/1000", 0, 0, false},
		{"", 0, 0, false},
	} {
		start, size, ok := parseContentRange(tc.value)
		if ok != tc.ok || (ok && (start != tc.start || size != tc.size)) {
			t.Errorf("parseContentRange(%q) = %d, %d, %v", tc.value, start, size, ok)
		}
	}
}
