package everyapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer is EveryAPI's OAuth server as far as Kando uses it: /authorize, which approves at once and sends the browser back with a code, the token endpoint's three grants with the refresh token rotated on every use and the whole sign-in revoked when a used one comes back (reuse detection), the device grant, revocation, /api/usage/account and /api/status.
type fakeServer struct {
	*httptest.Server
	t *testing.T

	mu        sync.Mutex
	next      int
	codes     map[string]string // code -> PKCE challenge
	refreshes map[string]bool   // refresh token -> still usable
	keys      map[string]bool   // key -> still usable
	revoked   bool
	refreshed atomic.Int32
	requests  atomic.Int32
	// tokenError, when set, is the token endpoint's answer to every grant.
	tokenError string
	devicePoll int
	// deviceName is what the last grant named this computer.
	deviceName string
	quota      int64
	status     int
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{t: t, codes: map[string]string{}, refreshes: map[string]bool{}, keys: map[string]bool{}, quota: 6_200_000}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", f.authorize)
	mux.HandleFunc("/api/oauth2/token", f.token)
	mux.HandleFunc("/api/oauth2/device", f.device)
	mux.HandleFunc("/api/oauth2/revoke", f.revoke)
	mux.HandleFunc("/api/usage/account", f.account)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
	})
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) config() Config {
	return Config{APIBase: f.URL, WebBase: f.URL, DeviceName: "Test Mac", Client: f.Client()}
}

// mint gives a new key and refresh token.
func (f *fakeServer) mint() (string, string) {
	f.next++
	key, refresh := fmt.Sprintf("sk-everyapi-key%02d-0123456789abcdef", f.next), fmt.Sprintf("refresh%02d", f.next)
	f.keys[key], f.refreshes[refresh] = true, true
	return key, refresh
}

func (f *fakeServer) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != ClientID || q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" || q.Has("scope") || q.Has("client_secret") {
		http.Error(w, "bad authorize request "+r.URL.RawQuery, http.StatusBadRequest)
		return
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || redirect.Hostname() != "127.0.0.1" || redirect.Path != "/callback" {
		http.Error(w, "redirect not allowed", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.next++
	code := fmt.Sprintf("code%d", f.next)
	f.codes[code] = q.Get("code_challenge")
	f.mu.Unlock()
	back := redirect.Query()
	back.Set("code", code)
	back.Set("state", q.Get("state"))
	redirect.RawQuery = back.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (f *fakeServer) reply(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeServer) grant(w http.ResponseWriter) {
	key, refresh := f.mint()
	f.reply(w, http.StatusOK, map[string]any{"access_token": key, "token_type": "bearer", "expires_in": 90 * 24 * 3600, "refresh_token": refresh, "scope": "api"})
}

func (f *fakeServer) token(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.PostFormValue("client_id") != ClientID || r.PostFormValue("client_secret") != "" || r.PostFormValue("scope") != "" {
		f.reply(w, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
		return
	}
	if f.tokenError != "" {
		f.reply(w, http.StatusBadRequest, map[string]any{"error": "access_denied", "error_description": f.tokenError})
		return
	}
	f.deviceName = r.PostFormValue("device_name")
	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		challenge, ok := f.codes[r.PostFormValue("code")]
		sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			f.reply(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "PKCE verification failed"})
			return
		}
		delete(f.codes, r.PostFormValue("code"))
		f.grant(w)
	case "refresh_token":
		f.refreshed.Add(1)
		refresh := r.PostFormValue("refresh_token")
		usable, known := f.refreshes[refresh]
		if !known || f.revoked {
			f.reply(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "invalid or expired refresh token"})
			return
		}
		if !usable {
			// Reuse detection: a refresh token used twice ends the whole sign-in.
			f.revoked = true
			f.reply(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "invalid or expired refresh token"})
			return
		}
		f.refreshes[refresh] = false
		for key := range f.keys {
			f.keys[key] = false
		}
		f.grant(w)
	case "urn:ietf:params:oauth:grant-type:device_code":
		if r.PostFormValue("device_code") != "device-1" {
			f.reply(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
			return
		}
		if f.devicePoll++; f.devicePoll < 2 {
			f.reply(w, http.StatusBadRequest, map[string]any{"error": "authorization_pending"})
			return
		}
		f.grant(w)
	default:
		f.reply(w, http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
	}
}

func (f *fakeServer) device(w http.ResponseWriter, r *http.Request) {
	if r.PostFormValue("client_id") != ClientID {
		f.reply(w, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
		return
	}
	f.reply(w, http.StatusOK, map[string]any{"device_code": "device-1", "user_code": "ABCD-EFGH", "verification_uri": f.URL + "/device", "verification_uri_complete": f.URL + "/device?user_code=ABCD-EFGH", "expires_in": 600, "interval": 1})
}

func (f *fakeServer) revoke(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.refreshes[r.PostFormValue("token")]; ok {
		f.revoked = true
	}
	f.reply(w, http.StatusOK, map[string]any{})
}

func (f *fakeServer) account(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !f.keys[key] || f.revoked {
		f.reply(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "invalid token"})
		return
	}
	if f.status != 0 {
		f.reply(w, f.status, map[string]any{"success": false, "message": "insufficient balance"})
		return
	}
	f.reply(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"username": "mika", "display_name": "Mika", "wallet": map[string]any{"quota": f.quota, "currency": "USD"}}})
}

// browse is a browser that follows the sign-in page's redirect back to Kando's loopback address.
func browse(t *testing.T) func(string) error {
	return func(link string) error {
		go func() {
			res, err := http.Get(link)
			if err != nil {
				t.Errorf("the browser could not open %s: %v", link, err)
				return
			}
			res.Body.Close()
		}()
		return nil
	}
}

// TestLoginWithPKCE checks the browser sign-in: the page gets a PKCE challenge and a loopback redirect, the code comes back to Kando's listener and is traded, with the verifier and this computer's name, for a key and a refresh token.
func TestLoginWithPKCE(t *testing.T) {
	f := newFakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tokens, err := Login(ctx, f.config(), Browser{Open: browse(t), Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if !tokens.Valid() || !strings.HasPrefix(tokens.Key, "sk-") || tokens.Expires.IsZero() {
		t.Fatalf("tokens = %+v", tokens)
	}
	if f.deviceName != "Test Mac" {
		t.Fatalf("device name %q", f.deviceName)
	}
}

// TestLoginSaysWhyEveryAPIRefused checks that the account EveryAPI will not give a key to is told apart: a paid account needed, no room for another key.
func TestLoginSaysWhyEveryAPIRefused(t *testing.T) {
	for desc, want := range map[string]error{
		"a paid account is required to create or refresh API keys": ErrPaidAccount,
		"token limit reached for this account":                     ErrKeySlotsFull,
	} {
		f := newFakeServer(t)
		f.tokenError = desc
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := Login(ctx, f.config(), Browser{Open: browse(t)})
		cancel()
		if !errors.Is(err, want) || errors.Is(err, ErrDenied) {
			t.Errorf("%s: err = %v, want %v", desc, err, want)
		}
	}
}

// TestLoginFailsWhenTheBrowserDoesNotOpen checks that a browser that cannot open ends the sign-in at once, so the app can offer the code instead.
func TestLoginFailsWhenTheBrowserDoesNotOpen(t *testing.T) {
	f := newFakeServer(t)
	refused := errors.New("no browser")
	_, err := Login(context.Background(), f.config(), Browser{Open: func(string) error { return refused }})
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v", err)
	}
}

// TestDeviceSignIn checks the code sign-in: the code and the page to enter it on, then the key once the user approved, polling while EveryAPI says to wait.
func TestDeviceSignIn(t *testing.T) {
	f := newFakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d, err := StartDevice(ctx, f.config())
	if err != nil {
		t.Fatal(err)
	}
	if d.Code != "ABCD-EFGH" || !strings.HasPrefix(d.URL, f.URL+"/device") {
		t.Fatalf("device = %+v", d)
	}
	tokens, err := d.Wait(ctx)
	if err != nil || !tokens.Valid() {
		t.Fatalf("Wait = %+v, %v", tokens, err)
	}
}

// TestDeviceCodeExpires checks that a code that runs out before the user approved it says so (ErrExpired), as the code's own lifetime ends the polling, rather than as a context that timed out.
func TestDeviceCodeExpires(t *testing.T) {
	f := newFakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d, err := StartDevice(ctx, f.config())
	if err != nil {
		t.Fatal(err)
	}
	d.auth.Expiry = time.Now().Add(200 * time.Millisecond)
	if _, err := d.Wait(ctx); !errors.Is(err, ErrExpired) {
		t.Fatalf("Wait after the code expired = %v", err)
	}
	stopped, stop := context.WithCancel(context.Background())
	stop()
	if _, err := d.Wait(stopped); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait stopped by the caller = %v", err)
	}
}

// TestTokenFailureKeepsTheBodyOut checks that a token endpoint answering without an OAuth error, as a proxy's error page, fails with its status alone: what the page said is not in the error, which a failed job keeps and a phone may be told.
func TestTokenFailureKeepsTheBodyOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, "<html>upstream says: top up at https://pay.example.com</html>")
	}))
	defer srv.Close()
	_, err := Refresh(context.Background(), Config{APIBase: srv.URL, Client: srv.Client()}, "rt-1")
	if err == nil || strings.Contains(err.Error(), "pay.example.com") || !strings.Contains(err.Error(), "502") {
		t.Fatalf("Refresh = %v", err)
	}
}

// TestSessionRenewsOnceForConcurrentRejections checks that requests refused with the same key at the same time renew it once: the others get the key that renewal got, and the refresh token, rotated on every use, is never sent twice, which would end the sign-in. The new tokens are kept before any request gets the new key.
func TestSessionRenewsOnceForConcurrentRejections(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	key, refresh := f.mint()
	f.mu.Unlock()
	var kept atomic.Pointer[Tokens]
	s := NewSession(f.config(), Tokens{Key: key, Refresh: refresh}, SessionHooks{Persist: func(t Tokens) error {
		kept.Store(&t)
		return nil
	}})
	const n = 8
	var wg sync.WaitGroup
	got := make([]string, n)
	for i := range n {
		wg.Go(func() {
			tokens, _, err := s.Key(context.Background(), key)
			if err != nil {
				t.Error(err)
				return
			}
			if k := kept.Load(); k == nil || k.Key != tokens.Key {
				t.Error("a key was handed out before its tokens were kept")
			}
			got[i] = tokens.Key
		})
	}
	wg.Wait()
	if n := f.refreshed.Load(); n != 1 {
		t.Fatalf("renewed %d times, want once", n)
	}
	for _, k := range got {
		if k == key || k != got[0] {
			t.Fatalf("keys handed out %v", got)
		}
	}
	if s.Gen() != 1 {
		t.Fatalf("gen %d", s.Gen())
	}
	// A request refused with the key of the generation before gets the current one without renewing again.
	if tokens, gen, err := s.Key(context.Background(), key); err != nil || tokens.Key != got[0] || gen != 1 || f.refreshed.Load() != 1 {
		t.Fatalf("stale rejection: %v %d %v, renewed %d", tokens.Key, gen, err, f.refreshed.Load())
	}
}

// TestSessionKeepsTheKeyWhenPersistingFails checks that a renewal whose tokens could not be kept still hands out the new key, as the old refresh token is spent at EveryAPI either way, and tells the app, which says the sign-in may not survive a restart; a later renewal that is kept tells it that it is fine again.
func TestSessionKeepsTheKeyWhenPersistingFails(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	key, refresh := f.mint()
	f.mu.Unlock()
	failing := errors.New("keychain locked")
	var persistErr error = errors.New("not told")
	fail := true
	s := NewSession(f.config(), Tokens{Key: key, Refresh: refresh}, SessionHooks{
		Persist: func(Tokens) error {
			if fail {
				return failing
			}
			return nil
		},
		PersistFailed: func(err error) { persistErr = err },
	})
	tokens, _, err := s.Key(context.Background(), key)
	if err != nil || tokens.Key == key {
		t.Fatalf("Key = %v, %v", tokens.Key, err)
	}
	if !errors.Is(persistErr, failing) {
		t.Fatalf("told %v", persistErr)
	}
	fail = false
	if _, _, err := s.Key(context.Background(), tokens.Key); err != nil || persistErr != nil {
		t.Fatalf("second renewal: %v, told %v", err, persistErr)
	}
}

// TestSessionEndsWhenEveryAPIRevokedIt checks that a sign-in EveryAPI no longer takes ends the session once, tells the app, and fails every later request without asking EveryAPI again.
func TestSessionEndsWhenEveryAPIRevokedIt(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	key, refresh := f.mint()
	f.revoked = true
	f.mu.Unlock()
	ended := 0
	s := NewSession(f.config(), Tokens{Key: key, Refresh: refresh}, SessionHooks{Ended: func(error) { ended++ }})
	if _, _, err := s.Key(context.Background(), key); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := s.Key(context.Background(), ""); !errors.Is(err, ErrSignedOut) || ended != 1 || f.refreshed.Load() != 1 {
		t.Fatalf("after: %v, ended %d, renewed %d", err, ended, f.refreshed.Load())
	}
}

// TestSessionRenewsAKeyAboutToExpire checks that a key EveryAPI says expires within a day is renewed before it is handed out, and kept when the renewal fails, as it still works until then.
func TestSessionRenewsAKeyAboutToExpire(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	key, refresh := f.mint()
	f.mu.Unlock()
	s := NewSession(f.config(), Tokens{Key: key, Refresh: refresh, Expires: time.Now().Add(time.Hour)}, SessionHooks{})
	tokens, _, err := s.Key(context.Background(), "")
	if err != nil || tokens.Key == key || f.refreshed.Load() != 1 {
		t.Fatalf("Key = %v, %v, renewed %d", tokens.Key, err, f.refreshed.Load())
	}
	down := NewSession(Config{APIBase: "http://127.0.0.1:1", Client: f.Client()}, Tokens{Key: key, Refresh: refresh, Expires: time.Now().Add(time.Hour)}, SessionHooks{})
	if tokens, _, err := down.Key(context.Background(), ""); err != nil || tokens.Key != key {
		t.Fatalf("unreachable server: %v, %v", tokens.Key, err)
	}
}

// TestRevokeAndSummary checks the account summary, its balance in dollars at the server's rate, a key EveryAPI refuses there, an account out of credit, and that revoking the refresh token ends the sign-in.
func TestRevokeAndSummary(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	key, refresh := f.mint()
	f.mu.Unlock()
	cfg := f.config()
	s, err := FetchSummary(context.Background(), cfg, key)
	if err != nil || s.Name != "Mika" || s.Balance != 12.4 || s.Currency != "USD" || s.Low() {
		t.Fatalf("summary = %+v, %v", s, err)
	}
	f.status = http.StatusPaymentRequired
	if _, err := FetchSummary(context.Background(), cfg, key); !errors.Is(err, ErrOutOfCredit) {
		t.Fatalf("402: %v", err)
	}
	f.status = 0
	if err := Revoke(context.Background(), cfg, refresh); err != nil {
		t.Fatal(err)
	}
	if _, err := FetchSummary(context.Background(), cfg, key); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("after revoking: %v", err)
	}
}

// TestWebBaseFor checks that the dashboard is found from the API's address as EveryAPI's command-line program finds it.
func TestWebBaseFor(t *testing.T) {
	for api, web := range map[string]string{
		"https://api.everyapi.ai":    "https://app.everyapi.ai",
		"https://api-cn.everyapi.ai": "https://app.everyapi.ai",
		"https://api.example.com/":   "https://app.example.com",
		"http://127.0.0.1:8080":      "http://127.0.0.1:8080",
	} {
		if got := WebBaseFor(api); got != web {
			t.Errorf("WebBaseFor(%q) = %q, want %q", api, got, web)
		}
	}
	if got := TopUpURL(Config{}); got != "https://app.everyapi.ai/wallet" {
		t.Errorf("TopUpURL = %q", got)
	}
}
