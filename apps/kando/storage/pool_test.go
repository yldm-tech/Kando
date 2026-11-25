package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kando/packages/remotefs"
	"kando/packages/remotefs/providertest"
)

// closeCounter counts the times it was closed.
type closeCounter struct{ n *int }

func (c closeCounter) Close() error {
	*c.n++
	return nil
}

// TestPoolOpensATargetOnceAndAgainWhenItChanges checks that a Pool opens a target the first time it is asked for it and keeps it, opens it again once its settings changed, closing the old one, tries again after a failure, serves a file through its proxy by ranges until its URL is revoked, and through Handler, answers 502 for a target it cannot open, and opens nothing once closed.
func TestPoolOpensATargetOnceAndAgainWhenItChanges(t *testing.T) {
	ctx := context.Background()
	local := NewLocal(t.TempDir())
	providertest.Write(t, local, "Kando/video.mp4", "picture")
	opened, closed := 0, 0
	fail := false
	p := NewPool(func(context.Context, Target) (remotefs.Provider, io.Closer, error) {
		if fail {
			return nil, nil, errors.New("connection refused")
		}
		opened++
		return local, closeCounter{&closed}, nil
	})
	t.Cleanup(p.Close)
	nas := Target{ID: "nas", Remote: &remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}}
	for range 2 {
		if _, err := p.Provider(ctx, nas); err != nil {
			t.Fatal(err)
		}
	}
	if opened != 1 {
		t.Fatalf("opened %d times", opened)
	}
	edited := Target{ID: "nas", Remote: &remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/other"}}
	if _, err := p.Provider(ctx, edited); err != nil || opened != 2 || closed != 1 {
		t.Fatalf("after an edit: opened %d, closed %d, %v", opened, closed, err)
	}

	// Before the proxy starts there is nothing to revoke.
	p.Revoke("http://127.0.0.1:1/token/video.mp4")
	url, err := p.URL(ctx, edited, "Kando/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Range", "bytes=0-2")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "pic" {
		t.Fatalf("the proxy serves %d %q", resp.StatusCode, body)
	}
	p.Revoke(url)
	if resp, err := http.Get(url); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a revoked URL answers %v, %v", resp, err)
	} else {
		resp.Body.Close()
	}
	if again, err := p.URL(ctx, edited, "Kando/video.mp4"); err != nil || again == url {
		t.Fatalf("the URL after a revoke is %q, was %q, %v", again, url, err)
	}
	if _, err := p.URL(ctx, edited, "Kando/missing.mp4"); !errors.Is(err, remotefs.ErrNotExist) {
		t.Fatalf("URL of a missing file = %v", err)
	}

	rec := httptest.NewRecorder()
	p.Handler(edited, "Kando/video.mp4").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "picture" {
		t.Fatalf("remotefs.Handler serves %d %q", rec.Code, rec.Body.String())
	}
	fail = true
	other := Target{ID: "other", Remote: &remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://off.local/dav"}}
	rec = httptest.NewRecorder()
	p.Handler(other, "video.mp4").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("remotefs.Handler of a target that cannot open answers %d", rec.Code)
	}
	fail = false
	if _, err := p.Provider(ctx, other); err != nil {
		t.Fatalf("a target that failed is not tried again: %v", err)
	}

	p.Close()
	if closed != 3 {
		t.Fatalf("Close closed %d connections in all, want 3", closed)
	}
	if _, err := p.Provider(ctx, nas); !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("remotefs.Provider after Close = %v", err)
	}
}
