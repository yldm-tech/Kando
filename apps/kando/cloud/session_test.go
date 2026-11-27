package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// A bucket opened with a session key signs every request with its session token, in the header the signature covers, and every presigned link with it in the query, so a store that checks the token takes both.
func TestSessionBucketSignsWithTheSessionToken(t *testing.T) {
	store := &memS3{bucket: "hosted", pageSize: 1000}
	var mu sync.Mutex
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Amz-Security-Token")
		signed := r.Header.Get("Authorization")
		if q := r.URL.Query(); q.Get("X-Amz-Signature") != "" {
			token, signed = q.Get("X-Amz-Security-Token"), q.Get("X-Amz-SignedHeaders")
		} else if !strings.Contains(signed, "x-amz-security-token") {
			// The token must be among the signed headers, or a store would take it from anyone.
			http.Error(w, "the token is not signed", http.StatusForbidden)
			return
		}
		mu.Lock()
		tokens = append(tokens, token)
		mu.Unlock()
		if token != "session-token" {
			http.Error(w, "no session token", http.StatusForbidden)
			return
		}
		store.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b, err := OpenSessionBucket(srv.URL, "", "hosted", Session{AccessKey: "AK", Secret: "SK", Token: "session-token", Expires: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Put(ctx, "u1/a", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if data, err := b.Get(ctx, "u1/a", 100); err != nil || string(data) != "hello" {
		t.Fatalf("get %q, %v", data, err)
	}
	if objects, err := b.List(ctx, "u1/"); err != nil || len(objects) != 1 {
		t.Fatalf("list %v, %v", objects, err)
	}
	link, err := b.PresignGet(ctx, "u1/a", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if u.Query().Get("X-Amz-Security-Token") != "session-token" || strings.Contains(link, "SK") {
		t.Fatalf("link %s", link)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned GET: %d", resp.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(tokens) < 4 {
		t.Fatalf("tokens %q", tokens)
	}
}

// A session key without its token, secret or endpoint opens no bucket.
func TestSessionBucketNeedsTheWholeKey(t *testing.T) {
	for _, s := range []Session{{AccessKey: "AK", Secret: "SK"}, {AccessKey: "AK", Token: "t"}, {Secret: "SK", Token: "t"}} {
		if _, err := OpenSessionBucket("https://r2.example", "auto", "b", s); err == nil {
			t.Fatalf("opened with %+v", s)
		}
	}
	if _, err := OpenSessionBucket("", "auto", "b", Session{AccessKey: "AK", Secret: "SK", Token: "t"}); err == nil {
		t.Fatal("opened without an endpoint")
	}
}
