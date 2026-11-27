package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/everyapi"
	"kando/apps/kando/feedback"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeEveryAPI is EveryAPI's OAuth server and account summary as the app uses them: /authorize approves at once and sends the browser back with a code, the token endpoint trades it and rotates the refresh token on every use, revoking the whole sign-in when a used one comes back, /revoke ends a sign-in, and /api/usage/account says what the account has. It counts every request, so a test can tell nothing was sent.
type fakeEveryAPI struct {
	*httptest.Server
	mu        sync.Mutex
	next      int
	refreshes map[string]bool
	keys      map[string]bool
	// revoked is the whole sign-in ended, as reuse detection does; /revoke ends only the sign-in of the token it was given, as EveryAPI revokes one refresh family, so a stopped sign-in given back does not end the one after it.
	revoked bool
	// revokedTokens are the refresh tokens /revoke was given; keyOf is the key each refresh token came with.
	revokedTokens map[string]bool
	keyOf         map[string]string
	refreshed     atomic.Int32
	requests      atomic.Int32
	quota         int64
}

func newFakeEveryAPI(t *testing.T) *fakeEveryAPI {
	f := &fakeEveryAPI{refreshes: map[string]bool{}, keys: map[string]bool{}, revokedTokens: map[string]bool{}, keyOf: map[string]string{}, quota: 6_200_000}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		back, _ := url.Parse(q.Get("redirect_uri"))
		values := back.Query()
		values.Set("code", "code-1")
		values.Set("state", q.Get("state"))
		back.RawQuery = values.Encode()
		http.Redirect(w, r, back.String(), http.StatusFound)
	})
	mux.HandleFunc("/api/oauth2/token", f.token)
	mux.HandleFunc("/api/oauth2/revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		if token := r.PostFormValue("token"); f.keyOf[token] != "" {
			f.revokedTokens[token] = true
			f.refreshes[token], f.keys[f.keyOf[token]] = false, false
		}
		f.mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("/api/usage/account", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); !f.keys[key] || f.revoked {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"success":false}`)
			return
		}
		fmt.Fprintf(w, `{"success":true,"data":{"username":"mika","display_name":"Mika","wallet":{"quota":%d,"currency":"USD"},"pricing":{"quota_per_unit":500000}}}`, f.quota)
	})
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// mint gives a new key and refresh token; f.mu is held.
func (f *fakeEveryAPI) mint() everyapi.Tokens {
	f.next++
	t := everyapi.Tokens{Key: fmt.Sprintf("sk-everyapi-%02d0123456789abcdef", f.next), Refresh: fmt.Sprintf("rt%02dsecretrefreshtoken0123456789", f.next)}
	f.keys[t.Key], f.refreshes[t.Refresh], f.keyOf[t.Refresh] = true, true, t.Key
	return t
}

// signIn is a sign-in the server gave, as one kept from an earlier launch.
func (f *fakeEveryAPI) signIn() everyapi.Tokens {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mint()
}

func (f *fakeEveryAPI) token(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(status int, body map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	switch r.PostFormValue("grant_type") {
	case "refresh_token":
		f.refreshed.Add(1)
		usable, known := f.refreshes[r.PostFormValue("refresh_token")]
		if !known || !usable || f.revoked {
			f.revoked = f.revoked || known
			reply(http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
			return
		}
		f.refreshes[r.PostFormValue("refresh_token")] = false
	case "authorization_code":
	default:
		reply(http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
		return
	}
	t := f.mint()
	reply(http.StatusOK, map[string]any{"access_token": t.Key, "token_type": "bearer", "expires_in": 7776000, "refresh_token": t.Refresh})
}

// memoryVault keeps the sign-in in memory as the secure store would; fail, when set, is what writing a refresh token fails with, and unavailable makes every call fail as a system without a secure store does.
type memoryVault struct {
	mu          sync.Mutex
	items       map[string]string
	fail        error
	unavailable bool
}

func (v *memoryVault) Get(account string) (string, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.unavailable {
		return "", false, ns.ErrSecretsUnavailable
	}
	s, ok := v.items[account]
	return s, ok, nil
}

func (v *memoryVault) Set(account, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.unavailable {
		return ns.ErrSecretsUnavailable
	}
	if v.fail != nil && account == everyapiAccountRefresh {
		return v.fail
	}
	v.items[account] = value
	return nil
}

func (v *memoryVault) Delete(account string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.unavailable {
		return ns.ErrSecretsUnavailable
	}
	delete(v.items, account)
	return nil
}

func (v *memoryVault) get(account string) string {
	s, _, _ := v.Get(account)
	return s
}

// everyapiTestApp starts the app with EveryAPI as its AI service, without a key, its sign-in going to f and kept in vault; cli, when set, is the everyapi program the tools found.
func everyapiTestApp(t *testing.T, f *fakeEveryAPI, vault *memoryVault, cli string) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	llmEnv(t, "", "", "")
	tools := media.Tools{EveryAPI: cli, Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}.UseEveryAPICLI()
	a, tt, updates := newTestApp(t, tools)
	a.syncLLM(tools)
	a.everyapi.cfg = everyapi.Config{APIBase: f.URL, WebBase: f.URL, DeviceName: "Test Mac", Client: f.Client()}
	a.everyapi.vault = func() everyapiVault { return vault }
	return a, tt, updates
}

// browseTo stands in for the browser: it opens the page the app asks it to, following the sign-in page back to the app's loopback address.
func browseTo(t *testing.T) {
	t.Helper()
	saved := openExternal
	openExternal = func(_ *ns.Scope, link string, done func(bool)) {
		go func() {
			if res, err := http.Get(link); err == nil {
				res.Body.Close()
			}
		}()
		done(true)
	}
	t.Cleanup(func() { openExternal = saved })
}

func pipelineTranslator(a *app) media.ChatTranslator {
	tools, _ := a.pipeline.(media.Tools)
	return tools.Translator
}

// TestEveryAPISignInAndOut checks the sign-in from Settings › AI 服务: 登录 opens EveryAPI's page in the browser, the sign-in is kept in the secure store, not settings.json, and EveryAPI then translates with the account's key; the page shows the account's name and balance. A key typed comes before the sign-in, and clearing it brings the sign-in back. 退出 signs out at EveryAPI and in the store, and EveryAPI falls back to the everyapi program's sign-in.
func TestEveryAPISignInAndOut(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	browseTo(t)
	a, tt, updates := everyapiTestApp(t, f, vault, "/opt/bin/everyapi")
	msgs := testlocale.Messages()
	page := settingsOpenAt(a, tt, settings.SectionAI)
	if !hasText(page, msgs.EveryAPIUsingCLI()) || page.Exists("id:settings-everyapi-get") {
		t.Fatalf("signed out with the program: %q", page.Texts())
	}
	page.Press("id:settings-everyapi-signin")
	waitUntil(t, tt, a, updates, func() bool { return a.everyapi.signedIn && a.everyapi.haveSummary })
	tr := pipelineTranslator(a)
	if tr.CredentialSource != media.CredentialLogin || tr.APIKey != "" {
		t.Fatalf("translator source %v key %q", tr.CredentialSource, tr.APIKey)
	}
	endpoint, err := tr.Credential(context.Background(), false)
	if err != nil || endpoint.BaseURL != media.EveryAPIBaseURL || endpoint.APIKey != vault.get(everyapiAccountKey) || vault.get(everyapiAccountRefresh) == "" {
		t.Fatalf("credential %+v, %v; vault %v", endpoint, err, vault.items)
	}
	var saved savedSettings
	a.saveEveryAPI(&saved)
	if !saved.EveryAPISignedIn || saved.EveryAPIName != "Mika" || saved.EveryAPIFallbackKey != "" || saved.EveryAPIFallbackRefresh != "" {
		t.Fatalf("settings.json keeps %+v", saved.savedEveryAPI)
	}
	if !hasText(page, msgs.EveryAPIAccountLine("Mika", "$12.40")) || !page.Exists("id:settings-everyapi-topup") {
		t.Fatalf("signed in: %q", page.Texts())
	}

	// A key comes first; without it the sign-in is back.
	*a.LLMKey() = "sk-typed-key"
	a.ApplyLLM()
	if tr := pipelineTranslator(a); tr.Credential != nil || tr.APIKey != "sk-typed-key" {
		t.Fatalf("with a key: source %v key %q", tr.CredentialSource, tr.APIKey)
	}
	*a.LLMKey() = ""
	a.ApplyLLM()
	if tr := pipelineTranslator(a); tr.CredentialSource != media.CredentialLogin {
		t.Fatalf("without the key: source %v", tr.CredentialSource)
	}

	refresh(tt, a)
	page.Press("id:settings-everyapi-signout")
	if tr := pipelineTranslator(a); tr.CredentialSource != media.CredentialCLI || a.everyapi.signedIn {
		t.Fatalf("signed out: source %v, signed in %v", tr.CredentialSource, a.everyapi.signedIn)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !(func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.revokedTokens) > 0 }()) || vault.get(everyapiAccountRefresh) != "" {
		if time.Now().After(deadline) {
			t.Fatalf("signing out did not revoke the sign-in or delete it: vault %v", vault.items)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestEveryAPIStartupWaitsForTheStoredSignIn checks a launch with the sign-in kept from before: restore gives the pipeline the sign-in from settings.json alone, the sidebar says it is checking it rather than that it is missing, and a job's check of the AI service started before the store was read waits for it and goes on with the account's key once it is, as a job left waiting at the last quit resumes.
func TestEveryAPIStartupWaitsForTheStoredSignIn(t *testing.T) {
	f := newFakeEveryAPI(t)
	tokens := f.signIn()
	vault := &memoryVault{items: map[string]string{everyapiAccountKey: tokens.Key, everyapiAccountRefresh: tokens.Refresh}}
	a, _, updates := everyapiTestApp(t, f, vault, "")
	tools := toolsInstalled
	tools.WhisperModel = filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(tools.WhisperModel, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
	a.pipeline = tools.UseEveryAPICLI()
	a.restoreEveryAPI(savedSettings{savedEveryAPI: savedEveryAPI{EveryAPISignedIn: true, EveryAPIName: "Mika"}})
	if tr := pipelineTranslator(a); tr.CredentialSource != media.CredentialLogin {
		t.Fatalf("restored source %v", tr.CredentialSource)
	}
	a.checkReadiness(true)
	if a.side.ready.status.State != sidebar.Checking {
		t.Fatalf("readiness before the store was read: %+v", a.side.ready.status)
	}
	if acc := a.everyapiAccount(); !acc.Loading || acc.SignedIn {
		t.Fatalf("page before the store was read: %+v", acc)
	}
	checked := make(chan error, 1)
	pipeline := a.pipeline.(media.Tools)
	go func() { checked <- pipeline.CheckTranslate() }()
	select {
	case err := <-checked:
		t.Fatalf("the check did not wait for the sign-in: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	a.everyapiLoad()
	applyNext(t, updates)
	if err := <-checked; err != nil {
		t.Fatalf("CheckTranslate = %v", err)
	}
	if endpoint, err := pipeline.Translator.Credential(context.Background(), false); err != nil || endpoint.APIKey != tokens.Key {
		t.Fatalf("credential %+v, %v", endpoint, err)
	}
	if !a.everyapi.loaded || !a.everyapiAccount().SignedIn {
		t.Fatalf("after loading: %+v", a.everyapiAccount())
	}
}

// TestEveryAPIStoredSignInMissing checks that a sign-in settings.json says Kando holds but the store does not have says so, in words that name no command-line program, rather than falling back to another account.
func TestEveryAPIStoredSignInMissing(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	a, _, updates := everyapiTestApp(t, f, vault, "/opt/bin/everyapi")
	a.restoreEveryAPI(savedSettings{savedEveryAPI: savedEveryAPI{EveryAPISignedIn: true}})
	a.everyapiLoad()
	applyNext(t, updates)
	tr := pipelineTranslator(a)
	_, err := tr.Credential(context.Background(), false)
	if !errors.Is(err, media.ErrEveryAPILoginMissing) || strings.Contains(err.Error(), "everyapi auth") || tr.CredentialSource != media.CredentialLogin {
		t.Fatalf("source %v, err %v", tr.CredentialSource, err)
	}
	if acc := a.everyapiAccount(); acc.SignedIn || acc.Problem != settings.EveryAPILoginMissing {
		t.Fatalf("page: %+v", acc)
	}
}

// everyapiSignedInWith signs the app in with tokens, as a sign-in that just finished.
func everyapiSignedInWith(a *app, tokens everyapi.Tokens) {
	a.everyapi.signInGen++
	a.everyapiSignedIn(a.everyapi.signInGen, tokens, false, nil)
}

// TestEveryAPIConcurrentRejectionsRenewOnce checks that requests refused with the same key at the same time renew it once through the pipeline's sign-in, keeping the new refresh token in the store before any of them gets the new key, and that a sign-in EveryAPI ended signs Kando out with the page saying so, falling back to no sign-in.
func TestEveryAPIConcurrentRejectionsRenewOnce(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	a, _, updates := everyapiTestApp(t, f, vault, "")
	tokens := f.signIn()
	everyapiSignedInWith(a, tokens)
	tr := pipelineTranslator(a)
	const n = 6
	var wg sync.WaitGroup
	keys := make([]string, n)
	for i := range n {
		wg.Go(func() {
			endpoint, err := tr.Renew(context.Background(), media.Endpoint{BaseURL: media.EveryAPIBaseURL, APIKey: tokens.Key})
			if err != nil {
				t.Error(err)
				return
			}
			if vault.get(everyapiAccountKey) != endpoint.APIKey {
				t.Error("a renewed key was handed out before it was kept")
			}
			keys[i] = endpoint.APIKey
		})
	}
	wg.Wait()
	if got := f.refreshed.Load(); got != 1 {
		t.Fatalf("renewed %d times, want once", got)
	}
	for _, k := range keys {
		if k == tokens.Key || k != keys[0] {
			t.Fatalf("keys %v", keys)
		}
	}

	// EveryAPI ends the sign-in: the request says to sign in again, and Kando signs out.
	f.mu.Lock()
	f.revoked = true
	f.mu.Unlock()
	_, err := tr.Renew(context.Background(), media.Endpoint{APIKey: keys[0]})
	if !errors.Is(err, media.ErrEveryAPILoginSignedOut) {
		t.Fatalf("err = %v", err)
	}
	for a.everyapi.signedIn {
		applyNext(t, updates)
	}
	if acc := a.everyapiAccount(); acc.Problem != settings.EveryAPIEnded || !pipelineTranslator(a).EveryAPIUnsigned() {
		t.Fatalf("after the sign-in ended: %+v", acc)
	}
}

// TestEveryAPIRenewalNotKept checks that a renewed sign-in the store would not take still translates, and the page says the sign-in may not survive a restart.
func TestEveryAPIRenewalNotKept(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	a, tt, updates := everyapiTestApp(t, f, vault, "")
	tokens := f.signIn()
	everyapiSignedInWith(a, tokens)
	vault.mu.Lock()
	vault.fail = errors.New("the keychain refused")
	vault.mu.Unlock()
	endpoint, err := pipelineTranslator(a).Renew(context.Background(), media.Endpoint{APIKey: tokens.Key})
	if err != nil || endpoint.APIKey == tokens.Key {
		t.Fatalf("renewed %+v, %v", endpoint, err)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.everyapi.notSaved })
	page := settingsOpenAt(a, tt, settings.SectionAI)
	if !hasText(page, testlocale.Messages().EveryAPINotSaved()) {
		t.Fatalf("page: %q", page.Texts())
	}
}

// TestEveryAPIWithoutSecureStore checks that where the system has no secure store the sign-in is kept in settings.json, and read back from it at the next launch.
func TestEveryAPIWithoutSecureStore(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}, unavailable: true}
	browseTo(t)
	a, tt, updates := everyapiTestApp(t, f, vault, "")
	page := settingsOpenAt(a, tt, settings.SectionAI)
	page.Press("id:settings-everyapi-signin")
	waitUntil(t, tt, a, updates, func() bool { return a.everyapi.signedIn })
	var saved savedSettings
	a.saveEveryAPI(&saved)
	saved.clean()
	if !strings.HasPrefix(saved.EveryAPIFallbackKey, "sk-") || saved.EveryAPIFallbackRefresh == "" {
		t.Fatalf("settings.json keeps %+v", saved.savedEveryAPI)
	}
	b, _, later := everyapiTestApp(t, f, vault, "")
	b.restoreEveryAPI(saved)
	b.everyapiLoad()
	applyNext(t, later)
	if endpoint, err := pipelineTranslator(b).Credential(context.Background(), false); err != nil || endpoint.APIKey != saved.EveryAPIFallbackKey {
		t.Fatalf("next launch: %+v, %v", endpoint, err)
	}
}

// TestEveryAPISecretsAreScrubbed checks that a feedback issue carries neither the key nor the refresh token of the sign-in, wherever they appear.
func TestEveryAPISecretsAreScrubbed(t *testing.T) {
	f := newFakeEveryAPI(t)
	a, _, _ := everyapiTestApp(t, f, &memoryVault{items: map[string]string{}}, "")
	tokens := f.signIn()
	everyapiSignedInWith(a, tokens)
	a.everyapi.fallback = everyapi.Tokens{Key: "sk-fallback-0123456789abcdef", Refresh: "fallbackrefreshtoken"}
	text := fmt.Sprintf("renewing with %s failed for %s, then %s and %s", tokens.Refresh, tokens.Key, a.everyapi.fallback.Refresh, a.everyapi.fallback.Key)
	clean := a.feedbackScrubber().Clean(text)
	for _, secret := range []string{tokens.Refresh, tokens.Key, "fallbackrefreshtoken", "sk-fallback"} {
		if strings.Contains(clean, secret) {
			t.Fatalf("%q kept %q", clean, secret)
		}
	}
	if got := (feedback.Scrubber{}).Clean("key sk-everyapi-0123456789abcdefghij"); strings.Contains(got, "sk-everyapi-0123") {
		t.Fatalf("an sk- key without the app's secrets: %q", got)
	}
}

// TestEveryAPIOfflineSendsNothing checks that in offline mode signing in, asking what the account has and signing out send EveryAPI nothing.
func TestEveryAPIOfflineSendsNothing(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	browseTo(t)
	a, tt, _ := everyapiTestApp(t, f, vault, "")
	everyapiSignedInWith(a, f.signIn())
	a.everyapi.summaryLoading = false
	a.privacySetOffline(true)
	sent := f.requests.Load()
	a.everyapiRefresh()
	a.everyapiSignIn()
	if a.everyapi.signingIn || a.everyapi.summaryLoading {
		t.Fatalf("offline: signing in %v, asking %v", a.everyapi.signingIn, a.everyapi.summaryLoading)
	}
	page := settingsOpenAt(a, tt, settings.SectionAI)
	page.Press("id:settings-everyapi-signout")
	time.Sleep(300 * time.Millisecond)
	if got := f.requests.Load(); got != sent {
		t.Fatalf("offline mode sent %d requests", got-sent)
	}
}

// TestEveryAPIOutOfCreditOffersTopUp checks that the card of a job EveryAPI refused because the wallet is empty offers 充值, which opens the account's top-up page, while one whose plan allowance is spent, or another service's, does not.
func TestEveryAPIOutOfCreditOffersTopUp(t *testing.T) {
	f := newFakeEveryAPI(t)
	a, _, _ := everyapiTestApp(t, f, &memoryVault{items: map[string]string{}}, "")
	var opened []string
	saved := openExternal
	openExternal = func(_ *ns.Scope, link string, done func(bool)) {
		opened = append(opened, link)
		done(true)
	}
	t.Cleanup(func() { openExternal = saved })
	refused := func(reason string) core.Job {
		body := `{"error":{"code":"insufficient_user_quota","quota_reason":"` + reason + `"}}`
		return core.Job{ID: "job-" + reason, Source: "https://example.com/v", Stage: core.StageFailed, Err: &media.StageError{Stage: media.StageTranslate, Err: errors.New("translation API returned 403 Forbidden: " + body)}}
	}
	wallet := refused(media.QuotaBalance)
	a.library.Upsert(wallet)
	r, ok := a.FailureRemedy(films.Film{JobID: wallet.ID})
	if !ok || len(r.Actions) == 0 || r.Actions[0].ID != remedyTopUp || r.Actions[0].Label != testlocale.Messages().EveryAPITopUp() {
		t.Fatalf("empty wallet: %+v, %v", r, ok)
	}
	a.RunRemedy(films.Film{JobID: wallet.ID}, remedyTopUp)
	if len(opened) != 1 || opened[0] != f.URL+"/wallet" {
		t.Fatalf("opened %v", opened)
	}
	plan := refused(media.QuotaPlan)
	a.library.Upsert(plan)
	if r, ok := a.FailureRemedy(films.Film{JobID: plan.ID}); ok && len(r.Actions) > 0 && r.Actions[0].ID == remedyTopUp {
		t.Fatalf("spent plan offers a top-up: %+v", r)
	}
	tools := a.pipeline.(media.Tools)
	tools.Translator.BaseURL = "https://api.openai.com/v1"
	a.pipeline = tools
	if r, ok := a.FailureRemedy(films.Film{JobID: wallet.ID}); ok && len(r.Actions) > 0 && r.Actions[0].ID == remedyTopUp {
		t.Fatalf("another service offers EveryAPI's top-up: %+v", r)
	}
}

// TestEveryAPIStaleSignInKeepsNothing checks the sign-in that was stopped while EveryAPI was already giving it tokens: the user cancels it and signs in again, the new sign-in finishes, and only then does the old one get its tokens. The old one keeps nothing where the sign-in is kept and gives its tokens back to EveryAPI, and the new sign-in stays the one the store and the pipeline hold.
func TestEveryAPIStaleSignInKeepsNothing(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	browseTo(t)
	release := make(chan struct{})
	entered := make(chan struct{})
	var calls atomic.Int32
	old := f.signIn()
	saved := everyapiLogin
	everyapiLogin = func(ctx context.Context, cfg everyapi.Config, b everyapi.Browser) (everyapi.Tokens, error) {
		if calls.Add(1) == 1 {
			// EveryAPI had answered already: the stop comes too late for this exchange.
			close(entered)
			<-release
			return old, nil
		}
		return saved(ctx, cfg, b)
	}
	t.Cleanup(func() { everyapiLogin = saved })
	a, tt, updates := everyapiTestApp(t, f, vault, "")
	page := settingsOpenAt(a, tt, settings.SectionAI)
	page.Press("id:settings-everyapi-signin")
	<-entered
	page.Press("id:settings-everyapi-cancel")
	page.Press("id:settings-everyapi-signin")
	waitUntil(t, tt, a, updates, func() bool { return a.everyapi.signedIn })
	current := vault.get(everyapiAccountRefresh)
	if current == "" || current == old.Refresh {
		t.Fatalf("the new sign-in was not kept: %v", vault.items)
	}
	close(release)
	revoked := func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.revokedTokens[old.Refresh] }
	deadline := time.After(10 * time.Second)
	for !revoked() {
		select {
		case fn := <-updates:
			apply(tt, a, fn)
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatal("the stopped sign-in did not give its tokens back")
		}
	}
	if got := vault.get(everyapiAccountRefresh); got != current {
		t.Fatalf("the store holds %q, want the new sign-in's %q", got, current)
	}
	endpoint, err := pipelineTranslator(a).Credential(context.Background(), false)
	if err != nil || endpoint.APIKey != vault.get(everyapiAccountKey) || endpoint.APIKey == old.Key || !a.everyapi.signedIn {
		t.Fatalf("credential %+v, %v", endpoint, err)
	}
}

// TestEveryAPISignInAgainIsCheckedAgain checks that signing in again after the sidebar's check found Kando's own sign-in failing, as one not found in the secure store, drops that answer: the new sign-in is checked, and the sidebar and the palette no longer say the old one is missing.
func TestEveryAPISignInAgainIsCheckedAgain(t *testing.T) {
	f := newFakeEveryAPI(t)
	vault := &memoryVault{items: map[string]string{}}
	a, tt, updates := everyapiTestApp(t, f, vault, "")
	tools := toolsInstalled
	tools.WhisperModel = filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(tools.WhisperModel, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
	a.pipeline = tools.UseEveryAPICLI()
	a.restoreEveryAPI(savedSettings{savedEveryAPI: savedEveryAPI{EveryAPISignedIn: true}})
	a.everyapiLoad()
	waitUntil(t, tt, a, updates, func() bool { return a.everyapi.loaded })
	a.checkReadiness(true)
	waitUntil(t, tt, a, updates, func() bool { return a.side.ready.signInDone })
	if err := a.signInFailed(pipelineTranslator(a)); !errors.Is(err, media.ErrEveryAPILoginMissing) {
		t.Fatalf("the missing sign-in checked as %v", err)
	}

	everyapiSignedInWith(a, f.signIn())
	waitUntil(t, tt, a, updates, func() bool { return a.side.ready.signInDone && !a.everyapi.summaryLoading })
	if err := a.signInFailed(pipelineTranslator(a)); err != nil {
		t.Fatalf("after signing in again the sign-in still checks as %v", err)
	}
	if a.side.ready.status.State != sidebar.Ready {
		t.Fatalf("readiness after signing in again: %+v", a.side.ready.status)
	}
}
