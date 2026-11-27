package relayhost

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"kando/apps/kando/cloud"
	psync "kando/packages/sync"
)

// fakeS3 is the hosted bucket: path style, one bucket, and a session key that reaches only the account's folder, as EveryAPI's scoped keys do. A request without the current session token, or for a key outside the folder, is refused.
type fakeS3 struct {
	srv    *httptest.Server
	bucket string
	prefix string

	mu      gosync.Mutex
	tokens  map[string]bool
	objects map[string][]byte
	seen    []string
}

func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{bucket: "kando-hosted", prefix: "acct-42/", tokens: map[string]bool{}, objects: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>refused</Message></Error>`, code)
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	token := r.Header.Get("X-Amz-Security-Token")
	if q.Get("X-Amz-Signature") != "" {
		token = q.Get("X-Amz-Security-Token")
	} else if !strings.Contains(r.Header.Get("Authorization"), "x-amz-security-token") {
		f.fail(w, http.StatusForbidden, "AccessDenied")
		return
	}
	f.seen = append(f.seen, token)
	if !f.tokens[token] {
		f.fail(w, http.StatusForbidden, "InvalidToken")
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != f.bucket {
		f.fail(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	scope := key
	if r.Method == http.MethodGet && key == "" {
		scope = q.Get("prefix")
	}
	if !strings.HasPrefix(scope, f.prefix) {
		f.fail(w, http.StatusForbidden, "AccessDenied")
		return
	}
	switch {
	case r.Method == http.MethodGet && key == "":
		type content struct {
			Key          string
			Size         int
			LastModified string
		}
		var out struct {
			XMLName  xml.Name  `xml:"ListBucketResult"`
			Contents []content `xml:"Contents"`
		}
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, q.Get("prefix")) {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		for _, k := range keys {
			out.Contents = append(out.Contents, content{Key: k, Size: len(f.objects[k]), LastModified: "2026-11-27T09:00:00Z"})
		}
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		body, ok := f.objects[key]
		if !ok {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			f.fail(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	case r.Method == http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// fakeEveryAPI answers the hosted relay's endpoints for the key "sk-good": a new session key to fakeS3 at each request for one, good for life, with the account's quota and use, or the error set in refuse.
type fakeEveryAPI struct {
	srv  *httptest.Server
	s3   *fakeS3
	now  func() time.Time
	life time.Duration

	mu         gosync.Mutex
	fetches    int
	quota      int64
	used       int64
	writes     bool
	refuse     int
	refuseCode string
	plus       Plus
	auths      []string
	// block, when set, holds a request for a key until it is closed or the request ends.
	block chan struct{}
}

const goodKey = "sk-good-0123456789abcdef"

func newFakeEveryAPI(t *testing.T, s3 *fakeS3, now func() time.Time) *fakeEveryAPI {
	f := &fakeEveryAPI{s3: s3, now: now, life: 12 * time.Hour, quota: 50 << 30, used: 1 << 30, writes: true}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeEveryAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	if block := f.block; block != nil && r.URL.Path == "/api/kando/relay/credentials" {
		f.mu.Unlock()
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
		f.mu.Lock()
	}
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+goodKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_key","message":"bad key ` + r.Header.Get("Authorization") + `"}}`))
		return
	}
	if f.refuse != 0 {
		w.WriteHeader(f.refuse)
		fmt.Fprintf(w, `{"error":{"code":%q,"message":"Subscribe at https://everyapi.ai/pricing for $4.99"}}`, f.refuseCode)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/kando/relay/credentials":
		f.fetches++
		token := fmt.Sprintf("session-%d", f.fetches)
		f.s3.mu.Lock()
		f.s3.tokens[token] = true
		f.s3.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"endpoint": f.s3.srv.URL, "region": "auto", "bucket": f.s3.bucket, "prefix": f.s3.prefix,
			"access_key_id": fmt.Sprintf("AK%d", f.fetches), "secret_access_key": "hosted-secret-value", "session_token": token,
			"expires_at": f.now().Add(f.life).Unix(), "quota_bytes": f.quota, "used_bytes": f.used, "write_allowed": f.writes,
		})
	case r.Method == http.MethodGet && r.URL.Path == "/api/kando/plus":
		_ = json.NewEncoder(w).Encode(map[string]any{"active": f.plus.Active, "plan_name": f.plus.PlanName, "renews_at": f.plus.RenewsAt.Unix(),
			"expires_at": 0, "ai_allowance": map[string]any{"usd": 2}, "relay": map[string]any{"quota_bytes": f.plus.Relay.QuotaBytes, "used_bytes": f.plus.Relay.UsedBytes}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeEveryAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fetches
}

func (f *fakeS3) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

func (f *fakeEveryAPI) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// keyOf is a Key that answers key, and goodKey once asked to renew, counting the renewals.
func keyOf(key string, renewals *int) func(context.Context, bool) (string, error) {
	return func(_ context.Context, renew bool) (string, error) {
		if renew {
			*renewals++
			return goodKey, nil
		}
		return key, nil
	}
}

type rig struct {
	clock  time.Time
	s3     *fakeS3
	api    *fakeEveryAPI
	client *Client
	bucket *Bucket
	states []State
}

func newRig(t *testing.T) *rig {
	g := &rig{clock: time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)}
	now := func() time.Time { return g.clock }
	g.s3 = newFakeS3(t)
	g.api = newFakeEveryAPI(t, g.s3, now)
	var renewals int
	g.client = &Client{Base: g.api.srv.URL, Key: keyOf(goodKey, &renewals)}
	g.bucket = NewBucket(g.client, now, func(s State) { g.states = append(g.states, s) })
	return g
}

func (g *rig) last(t *testing.T) State {
	t.Helper()
	if len(g.states) == 0 {
		t.Fatal("nothing told")
	}
	return g.states[len(g.states)-1]
}

func TestPlusReadsEveryAPI(t *testing.T) {
	g := newRig(t)
	renews := time.Date(2026, 12, 27, 0, 0, 0, 0, time.UTC)
	g.api.set(func() {
		g.api.plus = Plus{Active: true, PlanName: "Kando Plus", RenewsAt: unixTime{renews}, Relay: Usage{QuotaBytes: 50 << 30, UsedBytes: 3 << 30}}
	})
	p, err := g.client.Plus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !p.Active || p.PlanName != "Kando Plus" || !p.RenewsAt.Equal(renews) || !p.ExpiresAt.IsZero() || p.Relay.QuotaBytes != 50<<30 || p.Relay.UsedBytes != 3<<30 {
		t.Fatalf("plus %+v", p)
	}
}

// A key EveryAPI refuses is asked for anew once, then the request goes through with the new one; a key refused again is ErrUnauthorized. Neither the key nor the answer's words are in the error.
func TestClientRenewsARefusedKeyOnce(t *testing.T) {
	g := newRig(t)
	var renewals int
	g.client.Key = keyOf("sk-stale-0123456789abcdef", &renewals)
	if _, err := g.client.Credentials(context.Background()); err != nil || renewals != 1 {
		t.Fatalf("credentials %v after %d renewals", err, renewals)
	}
	g.client.Key = func(context.Context, bool) (string, error) { return "sk-wrong-0123456789abcdef", nil }
	_, err := g.client.Credentials(context.Background())
	if !errors.Is(err, ErrUnauthorized) || strings.Contains(err.Error(), "sk-") {
		t.Fatalf("a wrong key: %v", err)
	}
	g.client.Key = func(context.Context, bool) (string, error) { return "", nil }
	if _, err := g.client.Credentials(context.Background()); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key: %v", err)
	}
	g.client.Key = nil
	if _, err := g.client.Plus(context.Background()); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key func: %v", err)
	}
}

// EveryAPI's error codes read as their errors; an answer it gives with a price and a link in it shows neither.
func TestClientReadsEveryAPIsErrors(t *testing.T) {
	g := newRig(t)
	for code, want := range map[string]error{"plus_required": ErrPlusRequired, "relay_disabled": ErrRelayDisabled, "quota_exceeded": ErrQuotaExceeded} {
		g.api.set(func() { g.api.refuse, g.api.refuseCode = http.StatusForbidden, code })
		if _, err := g.client.Credentials(context.Background()); !errors.Is(err, want) {
			t.Fatalf("%s: %v", code, err)
		}
	}
	g.api.set(func() { g.api.refuse, g.api.refuseCode = http.StatusBadGateway, "upstream" })
	_, err := g.client.Credentials(context.Background())
	if err == nil || strings.Contains(err.Error(), "everyapi.ai/pricing") || strings.Contains(err.Error(), "$") {
		t.Fatalf("a 502: %v", err)
	}
}

// The bucket asks for no key until it is used; then every key goes under the account's folder, which listing leaves out, and every request carries the session token.
func TestBucketWorksInTheAccountsFolder(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	if g.api.count() != 0 {
		t.Fatal("a key was asked for before the bucket was used")
	}
	if err := g.bucket.Put(ctx, "kando-relay/ab/outbox/D1/state.json", []byte("state")); err != nil {
		t.Fatal(err)
	}
	if !g.s3.has("acct-42/kando-relay/ab/outbox/D1/state.json") {
		t.Fatal("the object is not in the account's folder")
	}
	data, err := g.bucket.Get(ctx, "kando-relay/ab/outbox/D1/state.json")
	if err != nil || string(data) != "state" {
		t.Fatalf("get %q, %v", data, err)
	}
	if _, err := g.bucket.Get(ctx, "kando-relay/ab/none"); !errors.Is(err, psync.ErrNoObject) {
		t.Fatalf("a missing object: %v", err)
	}
	objects, err := g.bucket.List(ctx, "kando-relay/ab/")
	if err != nil || len(objects) != 1 || objects[0].Key != "kando-relay/ab/outbox/D1/state.json" {
		t.Fatalf("list %+v, %v", objects, err)
	}
	sizes, err := g.bucket.Sizes(ctx, "kando-relay/")
	if err != nil || len(sizes) != 1 || sizes[0].Key != "kando-relay/ab/outbox/D1/state.json" || sizes[0].Size != 5 {
		t.Fatalf("sizes %+v, %v", sizes, err)
	}
	if ok, err := g.bucket.Exists(ctx, "kando-relay/ab/outbox/D1/state.json"); !ok || err != nil {
		t.Fatalf("exists %v, %v", ok, err)
	}
	link, err := g.bucket.PresignGet(ctx, "kando-relay/ab/outbox/D1/state.json", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if !strings.HasPrefix(u.Path, "/kando-hosted/acct-42/") || u.Query().Get("X-Amz-Security-Token") != "session-1" || strings.Contains(link, "hosted-secret-value") {
		t.Fatalf("link %s", link)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the link: %d", resp.StatusCode)
	}
	path := filepath.Join(t.TempDir(), "film.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.bucket.PutFile(ctx, "kando-relay/ab/films/x/video.mp4", path, "video/mp4", cloud.Resume{}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := g.bucket.Delete(ctx, "kando-relay/ab/outbox/D1/state.json"); err != nil {
		t.Fatal(err)
	}
	if g.api.count() != 1 {
		t.Fatalf("%d keys asked for", g.api.count())
	}
	g.s3.mu.Lock()
	defer g.s3.mu.Unlock()
	for _, token := range g.s3.seen {
		if token != "session-1" {
			t.Fatalf("a request with the token %q", token)
		}
	}
	if s := g.last(t); s.Quota != 50<<30 || s.Used != 1<<30 || !s.Writes || s.Full || s.Err != nil {
		t.Fatalf("told %+v", s)
	}
}

// A key is renewed once half its life is spent, and the links signed after carry the new one; LinksUntil is when the key in use lapses.
func TestBucketRenewsItsKeyBeforeItLapses(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	until, err := g.bucket.LinksUntil(ctx)
	if err != nil || !until.Equal(g.clock.Add(12*time.Hour)) {
		t.Fatalf("links until %v, %v", until, err)
	}
	g.clock = g.clock.Add(5 * time.Hour)
	if _, err := g.bucket.LinksUntil(ctx); err != nil || g.api.count() != 1 {
		t.Fatalf("renewed before half its life: %d, %v", g.api.count(), err)
	}
	g.clock = g.clock.Add(time.Hour)
	until, err = g.bucket.LinksUntil(ctx)
	if err != nil || g.api.count() != 2 || !until.Equal(g.clock.Add(12*time.Hour)) {
		t.Fatalf("after half its life: %d keys, until %v, %v", g.api.count(), until, err)
	}
	link, _ := g.bucket.PresignGet(ctx, "x", time.Hour)
	if !strings.Contains(link, "X-Amz-Security-Token=session-2") {
		t.Fatalf("link %s", link)
	}
	// EveryAPI does not answer: the key held signs on while it is good.
	g.clock = g.clock.Add(7 * time.Hour)
	g.api.set(func() { g.api.refuse, g.api.refuseCode = http.StatusServiceUnavailable, "" })
	if _, err := g.bucket.Get(ctx, "x"); !errors.Is(err, psync.ErrNoObject) {
		t.Fatalf("the key held: %v", err)
	}
	if s := g.last(t); s.Err == nil {
		t.Fatal("the failed renewal was not told")
	}
	g.clock = g.clock.Add(5 * time.Hour)
	if _, err := g.bucket.Get(ctx, "x"); err == nil || errors.Is(err, psync.ErrNoObject) {
		t.Fatalf("a lapsed key used: %v", err)
	}
}

// Once Kando Plus ended and EveryAPI's grace after it, EveryAPI refuses the key: the bucket stops at once, even with a key it holds still good.
func TestBucketStopsWhenPlusEnds(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	if err := g.bucket.Put(ctx, "a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	g.clock = g.clock.Add(7 * time.Hour)
	g.api.set(func() { g.api.refuse, g.api.refuseCode = http.StatusForbidden, "plus_required" })
	if _, err := g.bucket.Get(ctx, "a"); !errors.Is(err, ErrPlusRequired) {
		t.Fatalf("after Plus: %v", err)
	}
	if s := g.last(t); !errors.Is(s.Err, ErrPlusRequired) {
		t.Fatalf("told %+v", s)
	}
	if !g.s3.has("acct-42/a") {
		t.Fatal("what was there went")
	}
}

// A key that may not write reads on but writes nothing: no object, no link to put one, no film. One whose storage is full writes the relay's small objects but no film, and says it is full.
func TestBucketWritesWhatEveryAPILets(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "film.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.api.set(func() { g.api.used = g.api.quota })
	if err := g.bucket.Put(ctx, "a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := g.bucket.PutFile(ctx, "films/v.mp4", path, "video/mp4", cloud.Resume{}, nil, nil); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("a film into a full storage: %v", err)
	}
	if s := g.last(t); !s.Full || !s.Writes {
		t.Fatalf("told %+v", s)
	}
	g.bucket = NewBucket(g.client, func() time.Time { return g.clock }, func(s State) { g.states = append(g.states, s) })
	g.api.set(func() { g.api.writes = false })
	if _, err := g.bucket.Get(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := g.bucket.Put(ctx, "b", []byte("1")); !errors.Is(err, ErrWritesStopped) {
		t.Fatalf("put: %v", err)
	}
	if _, err := g.bucket.PresignPut(ctx, "b", time.Hour); !errors.Is(err, ErrWritesStopped) {
		t.Fatalf("presign put: %v", err)
	}
	if err := g.bucket.PutFile(ctx, "films/v.mp4", path, "video/mp4", cloud.Resume{}, nil, nil); !errors.Is(err, ErrWritesStopped) {
		t.Fatalf("put file: %v", err)
	}
	// EveryAPI saying the storage is full at the request for a key is told as full too.
	g.api.set(func() { g.api.refuse, g.api.refuseCode = http.StatusForbidden, "quota_exceeded" })
	g.clock = g.clock.Add(13 * time.Hour)
	if _, err := g.bucket.Get(ctx, "a"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("get: %v", err)
	}
	if s := g.last(t); !s.Full || !errors.Is(s.Err, ErrQuotaExceeded) {
		t.Fatalf("told %+v", s)
	}
}

// The storage relay runs in the hosted bucket as in the user's own: a phone's grant points into the account's folder, carries the session token, and lapses with the key.
func TestRelayRunsInTheHostedBucket(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	relay := &psync.Relay{Bucket: g.bucket, Prefix: psync.RelayPrefix([]byte{1, 2, 3, 4, 5, 6, 7, 8}), Desk: &psync.Desk{Journal: psync.NewJournal(), Desktop: "Studio"},
		Devices: func() []psync.RelayDevice { return nil }, Now: func() time.Time { return g.clock }, LinksUntil: g.bucket.LinksUntil}
	grant, err := relay.NewGrant(ctx, "D1")
	if err != nil {
		t.Fatal(err)
	}
	if !grant.Expires.Equal(g.clock.Add(12*time.Hour)) || !strings.Contains(grant.State, "/kando-hosted/acct-42/kando-relay/") || !strings.Contains(grant.Slots[0].Put, "X-Amz-Security-Token=session-1") {
		t.Fatalf("grant expires %v, state %s", grant.Expires, grant.State)
	}
	req, _ := http.NewRequest(http.MethodPut, grant.Slots[0].Put, strings.NewReader("sealed"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the phone's PUT: %d", resp.StatusCode)
	}
}

// Nothing the bucket's errors say, which the log and the page show, holds the key's secret, its session token or the EveryAPI key.
func TestNoSecretsInErrorsOrLogs(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	if _, err := g.bucket.PresignGet(ctx, "x", time.Hour); err != nil {
		t.Fatal(err)
	}
	g.s3.mu.Lock()
	g.s3.tokens = map[string]bool{}
	g.s3.mu.Unlock()
	_, err := g.bucket.Get(ctx, "x")
	if err == nil {
		t.Fatal("a refused token read")
	}
	for _, secret := range []string{"hosted-secret-value", "session-1", goodKey} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("the error %q holds %q", err, secret)
		}
	}

}

// requests is how many requests EveryAPI was sent.
func (f *fakeEveryAPI) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.auths)
}

// A renewal that fails is not asked again by every operation: EveryAPI hears from the bucket once per retryAfter while the key held signs on, and what the bucket tells keeps that key's numbers, so a storage full stays full.
func TestBucketAsksEveryAPIOnceAWhileItFails(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.api.set(func() { g.api.used = g.api.quota })
	if _, err := g.bucket.Get(ctx, "x"); !errors.Is(err, psync.ErrNoObject) {
		t.Fatal(err)
	}
	g.clock = g.clock.Add(7 * time.Hour)
	g.api.set(func() { g.api.refuse, g.api.refuseCode = http.StatusServiceUnavailable, "" })
	for range 10 {
		if _, err := g.bucket.Get(ctx, "x"); !errors.Is(err, psync.ErrNoObject) {
			t.Fatalf("the key held: %v", err)
		}
		if _, err := g.bucket.LinksUntil(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := g.api.requests(); n != 2 {
		t.Fatalf("EveryAPI asked %d times, want 2", n)
	}
	if s := g.last(t); s.Err == nil || !s.Full || !s.Writes || s.Quota != 50<<30 {
		t.Fatalf("told %+v", s)
	}
	g.clock = g.clock.Add(retryAfter)
	if _, err := g.bucket.Get(ctx, "x"); !errors.Is(err, psync.ErrNoObject) {
		t.Fatal(err)
	}
	if n := g.api.requests(); n != 3 {
		t.Fatalf("EveryAPI asked %d times after retryAfter, want 3", n)
	}
}

// A link is signed with the key held while a renewal waits for EveryAPI, as a pairing code signs its mailbox on the UI thread; an operation that needs the new key gives up with its context rather than waiting the request out.
func TestBucketSignsWithTheKeyHeldWhileARenewalWaits(t *testing.T) {
	g := newRig(t)
	var mu gosync.Mutex
	g.bucket = NewBucket(g.client, func() time.Time { return g.clock }, func(State) { mu.Lock(); defer mu.Unlock() })
	ctx := context.Background()
	if _, err := g.bucket.Get(ctx, "x"); !errors.Is(err, psync.ErrNoObject) {
		t.Fatal(err)
	}
	g.clock = g.clock.Add(7 * time.Hour)
	block := make(chan struct{})
	g.api.set(func() { g.api.block = block })
	renewed := make(chan error, 1)
	go func() {
		_, err := g.bucket.LinksUntil(ctx)
		renewed <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for g.api.requests() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the renewal was not asked")
		}
		time.Sleep(time.Millisecond)
	}
	signCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	link, err := g.bucket.PresignGet(signCtx, "pair/box", 5*time.Minute)
	if err != nil || !strings.Contains(link, "X-Amz-Security-Token=session-1") {
		t.Fatalf("link %s, %v", link, err)
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelWait()
	if _, err := g.bucket.LinksUntil(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting for the renewal: %v", err)
	}
	close(block)
	if err := <-renewed; err != nil {
		t.Fatal(err)
	}
}
