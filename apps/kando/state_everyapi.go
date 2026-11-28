package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/everyapi"
	"kando/apps/kando/films"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Settings › AI 服务 › EveryAPI 账号: Kando's own sign-in to EveryAPI, the AI service run by YLDM, which makes Kando too (package everyapi). Signed in, EveryAPI without an API Key translates with the account's key (media.Tools.UseEveryAPILogin), before the everyapi command-line program's sign-in and after any key; the page shows who is signed in and what the account has, with 充值 and 退出.
//
// The key and the refresh token are kept in the system's secure store, in the partition everyapiPartition, never in settings.json, which says only that Kando is signed in (savedEveryAPI). Where the system has no secure store, as a Linux session without a Secret Service, they are kept in settings.json instead, and the log says so. They are read once the window is ready (everyapiLoad), off the engine's goroutine, as the system may ask the user first: restore runs before the app's scope exists, when the store cannot be reached. Until they are read, the sign-in makes every request wait for them (everyapiCred.wait), and the sidebar says it is checking it rather than that it is missing.

// everyapiPartition is the partition of the system's secure store the sign-in is kept in (ns.Scope.ProvideSecrets).
const everyapiPartition = "everyapi"

// The accounts of the secure store the sign-in is kept under.
const (
	everyapiAccountKey     = "key"
	everyapiAccountRefresh = "refresh"
	everyapiAccountExpires = "expires"
)

// everyapiSignInWait is how long a sign-in waits for the user in the browser, or for the code to be entered.
const everyapiSignInWait = 10 * time.Minute

// everyapiLoadWait bounds how long a request waits for the sign-in to be read from the secure store at startup, as the system may be asking the user to allow it.
const everyapiLoadWait = 2 * time.Minute

// everyapiVault is where the sign-in is kept: the secure store's partition (ns.Secrets), or the one a test sets (everyapiUI.vault).
type everyapiVault interface {
	Get(account string) (string, bool, error)
	Set(account, value string) error
	Delete(account string) error
}

// everyapiUI is the state of the EveryAPI sign-in.
type everyapiUI struct {
	// cfg is where the sign-in goes: EveryAPI's own servers, unless settings.json names another API (savedEveryAPI.EveryAPIBaseURL), or a test does.
	cfg everyapi.Config
	// signedIn is what settings.json says: Kando signed in to EveryAPI, and name the account's name, as the page last showed it. plain is set where the tokens are kept in settings.json, as fallback, for want of a secure store.
	signedIn bool
	name     string
	plain    bool
	fallback everyapi.Tokens
	// cred is the sign-in the pipeline uses (UseEveryAPILogin), nil when Kando is not signed in. loaded is set once its tokens were read (everyapiLoad); missing when they were not found where settings.json says they are.
	cred    *everyapiCred
	loaded  bool
	missing bool
	// signingIn is a sign-in under way, which cancelSignIn stops; device is its code, once it signs in with one. signInGen numbers the sign-ins, so the answer of one that was stopped is dropped. problem is why the last one failed, or why the sign-in ended, nil when nothing went wrong.
	signingIn    bool
	cancelSignIn context.CancelFunc
	device       *everyapi.Device
	signInGen    int
	// attempts says which sign-in may keep its tokens, from the sign-in's own goroutine (everyapiKeepAttempt).
	attempts *everyapiAttempts
	problem  error
	// notSaved is set while the last renewal of the key could not be kept (everyapi.SessionHooks.PersistFailed).
	notSaved bool
	// summary is what the account has, as EveryAPI last said; summaryLoading while it is asked, summaryErr why it could not be. summaryGen numbers the asks. pageOpen is whether Settings › AI 服务 showed at the last change, so opening it asks once (everyapiFollowPage). quotaSeen are the jobs whose out-of-credit failure asked already (everyapiAfterQuota).
	summary        everyapi.Summary
	haveSummary    bool
	summaryLoading bool
	summaryErr     error
	summaryGen     int
	pageOpen       bool
	quotaSeen      map[string]bool
	// topUpFailed is the top-up page's address when the browser did not open it.
	topUpFailed string
	// vault, when set, is the store a test keeps the sign-in in.
	vault func() everyapiVault
}

func newEveryapiUI() everyapiUI {
	return everyapiUI{loaded: true, quotaSeen: map[string]bool{}, attempts: &everyapiAttempts{}}
}

// everyapiAttempts is the sign-in attempt that is current (everyapiUI.signInGen), as the attempts' goroutines read it, and the lock their writes to where the sign-in is kept take, so an attempt that is no longer current, as one stopped while EveryAPI was already giving it tokens, never writes its tokens over those of the attempt after it, nor deletes them.
type everyapiAttempts struct {
	mu      sync.Mutex
	current atomic.Int64
}

// errEveryapiStale is the tokens of a sign-in attempt that is no longer the current one, which are not kept.
var errEveryapiStale = errors.New("everyapi: a later sign-in took the place of this one")

// everyapiKeepAttempt keeps the tokens of attempt gen (everyapiKeep) only while it is the current one, under the attempts' lock; otherwise it keeps nothing and says errEveryapiStale.
func everyapiKeepAttempt(attempts *everyapiAttempts, gen int, vault everyapiVault, t everyapi.Tokens) (plain bool, err error) {
	attempts.mu.Lock()
	defer attempts.mu.Unlock()
	if attempts.current.Load() != int64(gen) {
		return false, errEveryapiStale
	}
	return everyapiKeep(vault, t)
}

// everyapiForgetAttempt deletes the tokens of attempt gen from vault, which it kept while it was the current one, unless what vault keeps now is no longer them, as a later attempt wrote its own.
func everyapiForgetAttempt(attempts *everyapiAttempts, gen int, vault everyapiVault, t everyapi.Tokens) {
	attempts.mu.Lock()
	defer attempts.mu.Unlock()
	if attempts.current.Load() == int64(gen) {
		return
	}
	if kept, _, err := vault.Get(everyapiAccountRefresh); err == nil && kept == t.Refresh {
		_ = everyapiForgetStored(vault)
	}
}

// everyapiLogin is the browser sign-in (everyapi.Login). Tests replace it.
var everyapiLogin = everyapi.Login

// everyapiDeviceName is how EveryAPI's list of keys names this computer: the name the user gave it (desktop.Name), as "Ada's MacBook Pro", else its host name. A sign-in reads it as it starts, so it names the computer as it is called then. UI thread only.
func (a *app) everyapiDeviceName() string { return desktop.Name(a.cx) }

// savedEveryAPI is the sign-in's part of settings.json: whether Kando is signed in, the account's name as the page shows it, and the EveryAPI API it signed in at when it is not EveryAPI's own. The key and the refresh token are only here where the system has no secure store (EveryAPIFallbackKey, EveryAPIFallbackRefresh).
type savedEveryAPI struct {
	EveryAPISignedIn        bool   `json:"everyAPISignedIn,omitempty"`
	EveryAPIName            string `json:"everyAPIName,omitempty"`
	EveryAPIBaseURL         string `json:"everyAPIBaseURL,omitempty"`
	EveryAPIFallbackKey     string `json:"everyAPIFallbackKey,omitempty"`
	EveryAPIFallbackRefresh string `json:"everyAPIFallbackRefresh,omitempty"`
}

// clean keeps a base URL only when it is https, or http on this computer for a server under test, as the refresh token goes there; a name and tokens only while signed in; and the tokens only as a pair.
func (s *savedEveryAPI) clean() {
	s.EveryAPIName = strings.TrimSpace(s.EveryAPIName)
	s.EveryAPIBaseURL = strings.TrimRight(strings.TrimSpace(s.EveryAPIBaseURL), "/")
	if u, err := url.Parse(s.EveryAPIBaseURL); s.EveryAPIBaseURL != "" && (err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && media.LocalEndpoint(s.EveryAPIBaseURL)))) {
		s.EveryAPIBaseURL = ""
	}
	s.EveryAPIFallbackKey = strings.TrimSpace(s.EveryAPIFallbackKey)
	s.EveryAPIFallbackRefresh = strings.TrimSpace(s.EveryAPIFallbackRefresh)
	if !s.EveryAPISignedIn {
		s.EveryAPIName = ""
	}
	if !s.EveryAPISignedIn || s.EveryAPIFallbackKey == "" || s.EveryAPIFallbackRefresh == "" {
		s.EveryAPIFallbackKey, s.EveryAPIFallbackRefresh = "", ""
	}
}

// everyapiHooks: the sign-in is taken back at startup from settings.json alone, which makes requests wait for it, and read from the secure store once the window is ready; the area runs before the queue's, whose ready starts the jobs left waiting, which then wait for it rather than fail. Opening Settings › AI 服务 asks EveryAPI what the account has, and so does a job that failed because the account is out of credit; nothing else does.
func (a *app) everyapiHooks() areaHooks {
	return areaHooks{
		save:         a.saveEveryAPI,
		restore:      a.restoreEveryAPI,
		ready:        a.everyapiLoad,
		shelfChanged: a.everyapiAfterQuota,
		afterChange:  a.everyapiFollowPage,
	}
}

func (a *app) saveEveryAPI(saved *savedSettings) {
	e := &a.everyapi
	s := savedEveryAPI{EveryAPISignedIn: e.signedIn, EveryAPIName: e.name}
	if e.cfg.APIBase != "" && e.cfg.APIBase != everyapi.DefaultAPIBase {
		s.EveryAPIBaseURL = e.cfg.APIBase
	}
	if e.signedIn && e.plain {
		s.EveryAPIFallbackKey, s.EveryAPIFallbackRefresh = e.fallback.Key, e.fallback.Refresh
	}
	saved.savedEveryAPI = s
}

// restoreEveryAPI takes back from settings.json whether Kando is signed in, without reading the secure store, which only exists once the app runs: signed in, the pipeline gets a sign-in whose requests wait until everyapiLoad read it.
func (a *app) restoreEveryAPI(saved savedSettings) {
	e := &a.everyapi
	if saved.EveryAPIBaseURL != "" {
		e.cfg.APIBase = saved.EveryAPIBaseURL
	}
	e.signedIn, e.name = saved.EveryAPISignedIn, saved.EveryAPIName
	e.plain = saved.EveryAPIFallbackKey != ""
	e.fallback = everyapi.Tokens{Key: saved.EveryAPIFallbackKey, Refresh: saved.EveryAPIFallbackRefresh}
	if !e.signedIn {
		return
	}
	e.cred, e.loaded = newEveryapiCred(), false
	a.everyapiInstall()
}

// everyapiCred is one sign-in as the pipeline uses it, from any goroutine: loaded closes once session is set, or missing says why there is none.
type everyapiCred struct {
	loaded  chan struct{}
	session *everyapi.Session
	missing error
	// plain is set where the tokens are kept in settings.json, for want of a secure store.
	plain atomic.Bool
}

func newEveryapiCred() *everyapiCred { return &everyapiCred{loaded: make(chan struct{})} }

// ready makes the sign-in usable with session, or without one, missing saying why. Call it once.
func (c *everyapiCred) ready(session *everyapi.Session, missing error) {
	c.session, c.missing = session, missing
	close(c.loaded)
}

// wait waits for the sign-in to be read. A deadline the caller set, as the 15 seconds a job's check of the AI service has (media.Tools.CheckTranslate), does not end the wait: the system may be asking the user to allow reading it, and a job that started meanwhile should go on once it is read. Only a cancel does, or everyapiLoadWait.
func (c *everyapiCred) wait(ctx context.Context) error {
	limit := time.NewTimer(everyapiLoadWait)
	defer limit.Stop()
	done := ctx.Done()
	for {
		select {
		case <-c.loaded:
			return nil
		case <-limit.C:
			return media.ErrEveryAPILoginMissing
		case <-done:
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			done = nil
		}
	}
}

// login is the sign-in as media.EveryAPILogin: the session's key, or a renewed one in place of rejected, with the errors media words without naming the command-line program.
func (c *everyapiCred) login(ctx context.Context, rejected *media.Endpoint) (media.Endpoint, error) {
	if err := c.wait(ctx); err != nil {
		return media.Endpoint{}, err
	}
	if c.session == nil {
		return media.Endpoint{}, c.missing
	}
	old := ""
	if rejected != nil {
		old = rejected.APIKey
	}
	tokens, _, err := c.session.Key(ctx, old)
	switch {
	case errors.Is(err, everyapi.ErrSignedOut):
		return media.Endpoint{}, fmt.Errorf("%w (%w)", media.ErrEveryAPILoginSignedOut, err)
	case err != nil:
		return media.Endpoint{}, fmt.Errorf("EveryAPI sign-in: %w", err)
	}
	return media.Endpoint{BaseURL: media.EveryAPIBaseURL, APIKey: tokens.Key}, nil
}

// everyapiInstall gives the pipeline the sign-in Kando holds, or takes it away when it holds none, so EveryAPI without a key falls back to the everyapi program's sign-in (media.Tools.UseEveryAPICLI).
func (a *app) everyapiInstall() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if c := a.everyapi.cred; c != nil {
		a.pipeline = tools.UseEveryAPILogin(c.login)
	} else {
		a.pipeline = tools.UseEveryAPILogin(nil)
	}
}

// everyapiStore is where the sign-in is kept: the secure store's partition everyapiPartition, or a test's. Without the app's scope, one that fails with ns.ErrSecretsUnavailable.
func (a *app) everyapiStore() everyapiVault {
	if a.everyapi.vault != nil {
		return a.everyapi.vault()
	}
	if a.cx == nil {
		return ns.Secrets{}
	}
	inner := a.cx.Child()
	var secrets ns.Secrets
	inner.Run(func() {
		inner.ProvideSecrets(everyapiPartition)
		secrets = inner.Secrets()
	})
	inner.Dispose()
	return secrets
}

// everyapiRead is the sign-in vault keeps; one without a key and a refresh token is not there.
func everyapiRead(vault everyapiVault) (everyapi.Tokens, error) {
	var t everyapi.Tokens
	var expires string
	for account, into := range map[string]*string{everyapiAccountKey: &t.Key, everyapiAccountRefresh: &t.Refresh, everyapiAccountExpires: &expires} {
		v, _, err := vault.Get(account)
		if err != nil {
			return everyapi.Tokens{}, err
		}
		*into = v
	}
	if expires != "" {
		t.Expires, _ = time.Parse(time.RFC3339, expires)
	}
	return t, nil
}

// everyapiWrite keeps t in vault, the refresh token first, as it is the one that must not be lost.
func everyapiWrite(vault everyapiVault, t everyapi.Tokens) error {
	expires := ""
	if !t.Expires.IsZero() {
		expires = t.Expires.UTC().Format(time.RFC3339)
	}
	for _, kv := range [][2]string{{everyapiAccountRefresh, t.Refresh}, {everyapiAccountKey, t.Key}, {everyapiAccountExpires, expires}} {
		var err error
		if kv[1] == "" {
			err = vault.Delete(kv[0])
		} else {
			err = vault.Set(kv[0], kv[1])
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// everyapiForgetStored deletes the sign-in from vault.
func everyapiForgetStored(vault everyapiVault) error {
	for _, account := range []string{everyapiAccountRefresh, everyapiAccountKey, everyapiAccountExpires} {
		if err := vault.Delete(account); err != nil {
			return err
		}
	}
	return nil
}

// everyapiKeep keeps the tokens of a new sign-in before the pipeline uses them: in vault, or, where the system has no secure store, in settings.json (plain), which the log says. It runs off the engine's goroutine.
func everyapiKeep(vault everyapiVault, t everyapi.Tokens) (plain bool, err error) {
	err = everyapiWrite(vault, t)
	if errors.Is(err, ns.ErrSecretsUnavailable) {
		log.Printf("everyapi: no secure store, so the sign-in is kept in settings.json")
		return true, nil
	}
	return false, err
}

// everyapiSessionHooks is what the session of cred tells the app: a renewal is kept where the sign-in is kept, in vault or, without a secure store, in settings.json through the engine's goroutine, before its key is handed out; a sign-in EveryAPI ended signs Kando out and says so; a renewal that could not be kept is said on the page.
func (a *app) everyapiSessionHooks(cred *everyapiCred, vault everyapiVault) everyapi.SessionHooks {
	e, update := &a.everyapi, a.update
	return everyapi.SessionHooks{
		Persist: func(t everyapi.Tokens) error {
			if !cred.plain.Load() {
				err := everyapiWrite(vault, t)
				if !errors.Is(err, ns.ErrSecretsUnavailable) {
					return err
				}
				log.Printf("everyapi: no secure store, so the renewed sign-in is kept in settings.json")
				cred.plain.Store(true)
			}
			saved := make(chan error, 1)
			update(func() {
				if e.cred != cred {
					saved <- nil
					return
				}
				e.plain, e.fallback = true, t
				saved <- a.saveSettings()
			})
			select {
			case err := <-saved:
				return err
			case <-time.After(10 * time.Second):
				return errors.New("settings.json was not saved in time")
			}
		},
		Ended: func(err error) {
			log.Printf("everyapi: EveryAPI no longer takes the sign-in: %v", err)
			update(func() {
				if e.cred == cred {
					a.everyapiSignOutWith(everyapi.ErrSignedOut, false)
				}
			})
		},
		PersistFailed: func(err error) {
			if err != nil {
				log.Printf("everyapi: keeping the renewed sign-in failed: %v", err)
			}
			update(func() {
				if e.cred == cred {
					e.notSaved = err != nil
				}
			})
		},
	}
}

// everyapiLoad reads the sign-in settings.json says Kando holds from the secure store, off the engine's goroutine, as the system may ask the user first, and makes it usable; requests made meanwhile waited for it. Where it is not found, Kando stays signed in with a sign-in that says it is missing (media.ErrEveryAPILoginMissing), so a job says to sign in again rather than fall back to another account behind the user's back. Once read, the areas that follow the AI service check it again.
func (a *app) everyapiLoad() {
	e := &a.everyapi
	cred := e.cred
	if !e.signedIn || cred == nil || e.loaded {
		e.loaded = true
		return
	}
	vault := a.everyapiStore()
	cfg, plain, fallback, update := e.cfg, e.plain, e.fallback, a.update
	hooks := a.everyapiSessionHooks(cred, vault)
	go func() {
		tokens, err := fallback, error(nil)
		if !plain {
			tokens, err = everyapiRead(vault)
		}
		missing := error(nil)
		switch {
		case err != nil:
			log.Printf("everyapi: reading the sign-in from the secure store failed: %v", err)
			missing = media.ErrEveryAPILoginMissing
		case !tokens.Valid():
			log.Printf("everyapi: settings.json says Kando is signed in, but the sign-in is not in the secure store")
			missing = media.ErrEveryAPILoginMissing
		}
		var session *everyapi.Session
		if missing == nil {
			cred.plain.Store(plain)
			session = everyapi.NewSession(cfg, tokens, hooks)
		}
		cred.ready(session, missing)
		update(func() {
			if e.cred != cred {
				return
			}
			e.loaded, e.missing = true, missing != nil
			a.areasLLMChanged()
		})
	}()
}

// everyapiSignIn signs in in the browser, or with a code where the browser does not open; offline mode, which reaches no service, refuses at once.
func (a *app) everyapiSignIn() {
	e := &a.everyapi
	if e.signingIn || a.privacy.offline {
		return
	}
	a.everyapiStartSignIn(false)
}

// everyapiUseCode stops the browser sign-in under way and signs in with a code instead, for a browser whose page cannot come back to Kando.
func (a *app) everyapiUseCode() {
	if a.privacy.offline {
		return
	}
	a.everyapiStartSignIn(true)
}

// everyapiCancelSignIn stops the sign-in under way.
func (a *app) everyapiCancelSignIn() {
	e := &a.everyapi
	e.signInGen++
	e.attempts.current.Store(int64(e.signInGen))
	if e.cancelSignIn != nil {
		e.cancelSignIn()
	}
	e.signingIn, e.cancelSignIn, e.device = false, nil, nil
}

// errEveryapiNotOpened is the browser not opening EveryAPI's sign-in page, which falls back to the code.
var errEveryapiNotOpened = errors.New("no app opened the sign-in page")

// everyapiStartSignIn starts a sign-in, with a code when code is set and otherwise in the browser, falling back to the code when the browser does not open, in place of any under way. The tokens it gets are kept (everyapiKeepAttempt) before the pipeline uses them, and only while it is still the current attempt.
func (a *app) everyapiStartSignIn(code bool) {
	e := &a.everyapi
	if e.cancelSignIn != nil {
		e.cancelSignIn()
	}
	e.signInGen++
	gen := e.signInGen
	e.attempts.current.Store(int64(gen))
	attempts := e.attempts
	ctx, cancel := context.WithTimeout(a.privacyContext(), everyapiSignInWait)
	e.signingIn, e.cancelSignIn, e.device, e.problem = true, cancel, nil, nil
	cfg, update, language, vault := e.cfg, a.update, a.prefs.interfaceLanguage, a.everyapiStore()
	cfg.DeviceName = a.everyapiDeviceName()
	open := a.everyapiOpener(ctx)
	go func() {
		var tokens everyapi.Tokens
		var err error
		if !code {
			tokens, err = everyapiLogin(ctx, cfg, everyapi.Browser{Open: open, Language: language})
			code = errors.Is(err, errEveryapiNotOpened)
		}
		if code {
			var d *everyapi.Device
			if d, err = everyapi.StartDevice(ctx, cfg); err == nil {
				update(func() {
					if gen == e.signInGen {
						e.device = d
					}
				})
				tokens, err = d.Wait(ctx)
			}
		}
		plain := false
		if err == nil {
			plain, err = everyapiKeepAttempt(attempts, gen, vault, tokens)
		}
		update(func() { a.everyapiSignedIn(gen, tokens, plain, err) })
	}()
}

// everyapiOpener opens a page in the browser from the sign-in's goroutine, through the engine's (openExternal), and says whether it opened.
func (a *app) everyapiOpener(ctx context.Context) func(string) error {
	update, cx := a.update, a.cx
	return func(link string) error {
		opened := make(chan bool, 1)
		update(func() {
			if cx == nil {
				opened <- false
				return
			}
			cx.Run(func() { openExternal(cx, link, func(ok bool) { opened <- ok }) })
		})
		select {
		case ok := <-opened:
			if !ok {
				return errEveryapiNotOpened
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// everyapiSignedIn takes the answer of the sign-in gen: signed in, the pipeline uses the account from now on, settings.json says so, the areas that follow the AI service check it, and the page asks what the account has. A sign-in stopped meanwhile that got tokens anyway gives them back to EveryAPI.
func (a *app) everyapiSignedIn(gen int, tokens everyapi.Tokens, plain bool, err error) {
	e := &a.everyapi
	if gen != e.signInGen || errors.Is(err, errEveryapiStale) {
		// An attempt a later one took the place of, or one that was stopped, keeps nothing: tokens it got anyway go back to EveryAPI, and tokens it kept while it was still the current one are deleted unless a later attempt has written its own since.
		if err == nil && !plain {
			vault, attempts := a.everyapiStore(), e.attempts
			go everyapiForgetAttempt(attempts, gen, vault, tokens)
		}
		if err == nil || errors.Is(err, errEveryapiStale) {
			a.everyapiRevokeOnly(tokens)
		}
		return
	}
	e.signingIn, e.cancelSignIn, e.device = false, nil, nil
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("everyapi: signing in failed: %v", err)
			e.problem = err
		}
		return
	}
	if e.cred != nil && e.cred.session != nil {
		// Signed in again over a sign-in still held: the old one is given back.
		old := e.cred.session.End()
		a.everyapiRevokeOnly(old)
	}
	cred := newEveryapiCred()
	cred.plain.Store(plain)
	e.cred, e.signedIn, e.loaded, e.missing = cred, true, true, false
	e.plain, e.fallback = plain, everyapi.Tokens{}
	if plain {
		e.fallback = tokens
	}
	e.problem, e.notSaved, e.name = nil, false, ""
	e.summary, e.haveSummary, e.summaryErr = everyapi.Summary{}, false, nil
	cred.ready(everyapi.NewSession(e.cfg, tokens, a.everyapiSessionHooks(cred, a.everyapiStore())), nil)
	a.everyapiInstall()
	// What the sidebar's check found about Kando's own sign-in, and a check of it still running, is about the one this replaces, as one not found in the secure store: dropped, so the areas below check the new one.
	if r := &a.side.ready; r.signInSource == media.CredentialLogin {
		r.gen++
		r.asking, r.signInDone, r.signInErr = false, false, nil
	}
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsAISaveFailed(err.Error())
	}
	a.areasLLMChanged()
	a.everyapiRefresh()
}

// everyapiSignOut signs Kando out of EveryAPI: the pipeline goes back to the everyapi program's sign-in, or none, at once, and off the engine's goroutine the sign-in is deleted from where it is kept and given back to EveryAPI, which revokes its key, except in offline mode, which sends nothing.
func (a *app) everyapiSignOut() { a.everyapiSignOutWith(nil, true) }

// everyapiSignOutWith signs out, problem being why the page then says, nil when the user signed out; revoke gives the sign-in back to EveryAPI, which a sign-in EveryAPI ended itself needs not.
func (a *app) everyapiSignOutWith(problem error, revoke bool) {
	e := &a.everyapi
	a.everyapiCancelSignIn()
	cred := e.cred
	e.cred, e.signedIn, e.loaded, e.missing, e.name = nil, false, true, false, ""
	e.plain, e.fallback, e.notSaved, e.problem = false, everyapi.Tokens{}, false, problem
	e.summaryGen++
	e.summary, e.haveSummary, e.summaryLoading, e.summaryErr = everyapi.Summary{}, false, false, nil
	a.everyapiInstall()
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsAISaveFailed(err.Error())
	}
	a.areasLLMChanged()
	vault, cfg, offline := a.everyapiStore(), e.cfg, a.privacy.offline
	go func() {
		var tokens everyapi.Tokens
		if cred != nil {
			select {
			case <-cred.loaded:
				if cred.session != nil {
					tokens = cred.session.End()
				}
			case <-time.After(30 * time.Second):
			}
		}
		if err := everyapiForgetStored(vault); err != nil && !errors.Is(err, ns.ErrSecretsUnavailable) {
			log.Printf("everyapi: deleting the sign-in from the secure store failed: %v", err)
		}
		if !revoke || tokens.Refresh == "" {
			return
		}
		if offline {
			log.Printf("everyapi: offline mode is on, so the sign-in was not revoked at EveryAPI")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := everyapi.Revoke(ctx, cfg, tokens.Refresh); err != nil {
			log.Printf("everyapi: revoking the sign-in failed: %v", err)
		}
	}()
}

// everyapiRevokeOnly gives tokens back to EveryAPI off the engine's goroutine; offline mode sends nothing.
func (a *app) everyapiRevokeOnly(tokens everyapi.Tokens) {
	if tokens.Refresh == "" || a.privacy.offline {
		return
	}
	cfg := a.everyapi.cfg
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := everyapi.Revoke(ctx, cfg, tokens.Refresh); err != nil {
			log.Printf("everyapi: revoking a sign-in no longer used failed: %v", err)
		}
	}()
}

// everyapiRefresh asks EveryAPI what the account has, off the engine's goroutine, unless Kando is not signed in, its sign-in is not read yet, offline mode is on, or it is asked already. A key EveryAPI refuses there is renewed once, as a request's is.
func (a *app) everyapiRefresh() {
	e := &a.everyapi
	cred := e.cred
	if cred == nil || !e.loaded || e.missing || e.summaryLoading || a.privacy.offline {
		return
	}
	e.summaryGen++
	gen := e.summaryGen
	e.summaryLoading = true
	cfg, update, net := e.cfg, a.update, a.privacyContext()
	go func() {
		ctx, cancel := context.WithTimeout(net, 20*time.Second)
		defer cancel()
		endpoint, err := cred.login(ctx, nil)
		var s everyapi.Summary
		if err == nil {
			s, err = everyapi.FetchSummary(ctx, cfg, endpoint.APIKey)
			if errors.Is(err, everyapi.ErrSignedOut) {
				if endpoint, err = cred.login(ctx, &endpoint); err == nil {
					s, err = everyapi.FetchSummary(ctx, cfg, endpoint.APIKey)
				}
			}
		}
		update(func() { a.everyapiSummaryLanded(gen, s, err) })
	}()
}

// everyapiSummaryLanded keeps what the ask gen found, and the account's name, which settings.json keeps for the next start.
func (a *app) everyapiSummaryLanded(gen int, s everyapi.Summary, err error) {
	e := &a.everyapi
	if gen != e.summaryGen {
		return
	}
	e.summaryLoading = false
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("everyapi: asking what the account has failed: %v", err)
		}
		e.summaryErr = err
		return
	}
	e.summary, e.haveSummary, e.summaryErr = s, true, nil
	if s.Name != "" && s.Name != e.name {
		e.name = s.Name
		if err := a.saveSettings(); err != nil {
			a.saveErr = a.messages().SettingsAISaveFailed(err.Error())
		}
	}
}

// everyapiFollowPage asks what the account has as Settings › AI 服务 opens, once each time it does; the ask itself runs off the engine's goroutine (everyapiRefresh). It reports whether that changed what the page reads.
func (a *app) everyapiFollowPage() bool {
	e := &a.everyapi
	open := a.prefs.open && a.SettingsSection() == settings.SectionAI
	if open == e.pageOpen {
		return false
	}
	e.pageOpen = open
	if !open || e.summaryLoading {
		return false
	}
	a.everyapiRefresh()
	return e.summaryLoading
}

// everyapiAfterQuota asks again what the account has once a job failed, or a film was left untranslated, because EveryAPI answered that the account is out of credit, once for each job.
func (a *app) everyapiAfterQuota() {
	e := &a.everyapi
	if e.cred == nil {
		return
	}
	asked := false
	for _, job := range a.library.Jobs {
		if e.quotaSeen[job.ID] || !everyapiOutOfCredit(job) {
			continue
		}
		e.quotaSeen[job.ID] = true
		if !asked {
			asked = true
			a.everyapiRefresh()
		}
	}
}

// everyapiOutOfCredit reports whether job failed to translate, or was left untranslated, because the AI service said the account is out of credit.
func everyapiOutOfCredit(job core.Job) bool {
	if failed := jobfailure.Of(job); failed != nil {
		return jobfailure.OutOfCredit(failed)
	}
	return job.TranslationErr != "" && job.TranslationSource == "" && media.ClassifyTranslate(job.TranslationErr) == media.TranslateQuota
}

// everyapiTopUp opens EveryAPI's top-up page in the browser; when no app opened it, the page says where it is.
func (a *app) everyapiTopUp() {
	link := everyapi.TopUpURL(a.everyapi.cfg)
	a.everyapi.topUpFailed = ""
	if a.cx == nil {
		return
	}
	a.cx.Run(func() {
		openExternal(a.cx, link, func(opened bool) {
			if !opened {
				a.everyapi.topUpFailed = link
				a.warn(a.messages().EveryAPITopUpFailed(link), "")
			}
			a.Changed()
		})
	})
}

// everyapiProvider reports whether the AI service in use is EveryAPI, with a key or a sign-in, whose top-up page fixes an account out of credit.
func (a *app) everyapiProvider() bool {
	tools, ok := a.pipeline.(media.Tools)
	return ok && tools.Translator.BaseURL == media.EveryAPIBaseURL
}

// remedyTopUp opens EveryAPI's top-up page, for the card of a job EveryAPI refused as the account is out of credit.
const remedyTopUp = "topup"

// everyapiTopUpRemedy is what the card of job says and offers when it failed because EveryAPI said the account is out of credit: why, with 充值 and 重试. It is the desktop's: the phone reads why the job failed (jobfailure.Hint) and offers no purchase.
func (a *app) everyapiTopUpRemedy(job core.Job) (films.Remedy, bool) {
	failed := jobfailure.Of(job)
	if failed == nil || !jobfailure.OutOfCredit(failed) || !a.everyapiProvider() {
		return films.Remedy{}, false
	}
	m := a.messages()
	return films.Remedy{Headline: m.PrecheckTranslateQuota(), Actions: []films.RemedyAction{
		{ID: remedyTopUp, Label: m.EveryAPITopUp(), Primary: true},
		{ID: remedyRetry, Label: m.QueueRemedyRetry()},
	}}, true
}

// everyapiAccount is what Settings › AI 服务 says of the sign-in (settings.EveryAPIAccount).
func (a *app) everyapiAccount() settings.EveryAPIAccount {
	e := &a.everyapi
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.Translator.BaseURL != media.EveryAPIBaseURL || tools.Translator.APIKey != "" {
		return settings.EveryAPIAccount{}
	}
	acc := settings.EveryAPIAccount{
		Shown:     true,
		Loading:   e.signedIn && !e.loaded,
		SignedIn:  e.signedIn && e.loaded && !e.missing,
		SigningIn: e.signingIn,
		Offline:   a.privacy.offline,
		Name:      e.name,
		NotSaved:  e.notSaved,
		CLI:       tools.EveryAPI != "",
		UsingCLI:  tools.Translator.CredentialSource == media.CredentialCLI && a.signInFailed(tools.Translator) == nil,
	}
	if e.device != nil {
		acc.Code, acc.CodeURL = e.device.Code, e.device.URL
	}
	if e.haveSummary {
		acc.Balance, acc.Low = everyapiMoney(e.summary), e.summary.Low()
	}
	acc.BalanceFailed = !e.summaryLoading && e.summaryErr != nil && !e.haveSummary
	if acc.Name == "" {
		acc.Name = "EveryAPI"
	}
	switch {
	case e.missing:
		acc.Problem = settings.EveryAPILoginMissing
	case e.problem == nil:
	case errors.Is(e.problem, everyapi.ErrPaidAccount):
		acc.Problem = settings.EveryAPINeedsTopUp
	case errors.Is(e.problem, everyapi.ErrKeySlotsFull):
		acc.Problem = settings.EveryAPISlotsFull
	case errors.Is(e.problem, everyapi.ErrDenied):
		acc.Problem = settings.EveryAPIDenied
	case errors.Is(e.problem, everyapi.ErrExpired):
		acc.Problem = settings.EveryAPIExpired
	case errors.Is(e.problem, everyapi.ErrUnavailable):
		acc.Problem = settings.EveryAPIUnavailable
	case errors.Is(e.problem, everyapi.ErrSignedOut):
		acc.Problem = settings.EveryAPIEnded
	default:
		acc.Problem, acc.Detail = settings.EveryAPIFailed, films.Shorten(e.problem.Error(), 120)
	}
	return acc
}

// everyapiMoney writes the balance of s as the page shows it: dollars with a $ and two decimals, another currency after its amount.
func everyapiMoney(s everyapi.Summary) string {
	if s.Currency == "" || s.Currency == "USD" {
		return fmt.Sprintf("$%.2f", s.Balance)
	}
	return fmt.Sprintf("%.2f %s", s.Balance, s.Currency)
}

// everyapiSecrets are the sign-in's tokens the app holds, which a feedback issue must not carry (feedbackSecrets): the session's, and those settings.json keeps without a secure store.
func (e *everyapiUI) everyapiSecrets() []string {
	out := []string{e.fallback.Key, e.fallback.Refresh}
	if c := e.cred; c != nil {
		select {
		case <-c.loaded:
			if c.session != nil {
				t := c.session.Tokens()
				out = append(out, t.Key, t.Refresh)
			}
		default:
		}
	}
	return out
}
