package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/relayhost"
	"kando/packages/media"
	psync "kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// hostedKey is the EveryAPI key the fake EveryAPI takes.
const hostedKey = "sk-hosted-0123456789abcdef"

// hostedFake is EveryAPI's side of 看懂托管 in the test: its endpoints, for hostedKey, and the hosted bucket, path style, which takes only the session tokens EveryAPI gave and only keys under the account's folder "acct/". plus is the account's Kando Plus, full its storage full, and refuse the error code EveryAPI answers a request for a key with, "" for none. deletes counts what was deleted from the bucket.
type hostedFake struct {
	api, s3 *httptest.Server

	mu      gosync.Mutex
	plus    bool
	full    bool
	refuse  string
	fetches int
	tokens  map[string]bool
	objects map[string][]byte
	deletes int
}

func newHostedFake(t *testing.T) *hostedFake {
	t.Helper()
	f := &hostedFake{tokens: map[string]bool{}, objects: map[string][]byte{}}
	f.s3 = httptest.NewServer(http.HandlerFunc(f.serveS3))
	t.Cleanup(f.s3.Close)
	f.api = httptest.NewServer(http.HandlerFunc(f.serveAPI))
	t.Cleanup(f.api.Close)
	api := phoneHostAPI
	phoneHostAPI = f.api.URL
	t.Cleanup(func() { phoneHostAPI = api })
	return f
}

func (f *hostedFake) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *hostedFake) serveAPI(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+hostedKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	quota, used := int64(50<<30), int64(1200<<20)
	if f.full {
		used = quota
	}
	switch r.URL.Path {
	case "/api/kando/plus":
		_ = json.NewEncoder(w).Encode(map[string]any{"active": f.plus, "plan_name": "Kando Plus", "relay": map[string]any{"quota_bytes": quota, "used_bytes": used}})
	case "/api/kando/relay/credentials":
		if f.refuse != "" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":{"code":%q,"message":"Renew at https://everyapi.ai/pricing for $4.99"}}`, f.refuse)
			return
		}
		f.fetches++
		token := fmt.Sprintf("tok-%d", f.fetches)
		f.tokens[token] = true
		_ = json.NewEncoder(w).Encode(map[string]any{"endpoint": f.s3.URL, "region": "auto", "bucket": "kando-hosted", "prefix": "acct/",
			"access_key_id": "AKHOSTED", "secret_access_key": "hosted-secret", "session_token": token, "expires_at": time.Now().Add(24 * time.Hour).Unix(),
			"quota_bytes": quota, "used_bytes": used, "write_allowed": true})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *hostedFake) serveS3(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	token := r.Header.Get("X-Amz-Security-Token")
	if r.URL.Query().Get("X-Amz-Signature") != "" {
		token = r.URL.Query().Get("X-Amz-Security-Token")
	}
	key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/kando-hosted"), "/")
	scope := key
	if key == "" {
		scope = r.URL.Query().Get("prefix")
	}
	if !f.tokens[token] || !strings.HasPrefix(scope, "acct/") {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>denied</Message></Error>`))
		return
	}
	switch {
	case r.Method == http.MethodGet && key == "":
		w.Header().Set("Content-Type", "application/xml")
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult>`)
		for k, body := range f.objects {
			if strings.HasPrefix(k, scope) {
				fmt.Fprintf(&b, `<Contents><Key>%s</Key><Size>%d</Size><LastModified>2026-11-27T09:00:00Z</LastModified></Contents>`, k, len(body))
			}
		}
		b.WriteString(`</ListBucketResult>`)
		_, _ = w.Write([]byte(b.String()))
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
	case r.Method == http.MethodDelete:
		f.deletes++
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		body, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}
}

// hostedApp is Phone Sync's test app with EveryAPI as the AI service, its key entered, and the storage relay's switch on.
func hostedApp(t *testing.T) (*app, *nsgotest.App, chan func(), *hostedFake) {
	t.Helper()
	f := newHostedFake(t)
	a, tt, updates, _, _ := phoneSyncApp(t)
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, APIKey: hostedKey, Model: media.EveryAPIModel}}
	a.phone.relay = true
	return a, tt, updates, f
}

// Without Kando Plus, 看懂托管 cannot be turned on and 订阅… opens EveryAPI's page; once the account has Plus, the switch has the relay go through the hosted storage, whose use the row says, and a phone that joins gets links to the account's folder signed with the session token.
func TestPhoneRelayHostedNeedsPlus(t *testing.T) {
	a, tt, updates, f := hostedApp(t)
	phoneSyncOn(t, a, tt, updates)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.host.plusKnown && !a.phone.host.checking })
	if !tt.Exists("id:settings-phone-relay-hosted-subscribe") || tt.Enabled("id:settings-phone-relay-hosted") {
		t.Fatalf("without Plus: texts %q", tt.Texts())
	}
	a.SetPhoneRelayHosted(true)
	if a.phone.host.on {
		t.Fatal("看懂托管 turned on without Plus")
	}
	f.set(func() { f.plus = true })
	tt.Press("settings-phone-relay-hosted-subscribe")
	if !slices.Contains(tt.OpenedURLs, relayhost.PlansURL) {
		t.Fatalf("opened %q", tt.OpenedURLs)
	}
	// Back from subscribing, the page checks again.
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.host.plus.Active })
	if tt.Exists("id:settings-phone-relay-hosted-subscribe") || !tt.Enabled("id:settings-phone-relay-hosted") {
		t.Fatalf("with Plus: texts %q", tt.Texts())
	}
	if r := a.PhoneSync().Relay; !r.Plus || r.HostedQuota != 50<<30 || r.HostedUsed != 1200<<20 {
		t.Fatalf("relay %+v", r)
	}
	running := a.phone.server
	tt.Press("settings-phone-relay-hosted")
	waitUntil(t, tt, a, updates, func() bool {
		return a.phone.server != nil && a.phone.server != running && a.phone.relayOK
	})
	if !strings.HasPrefix(a.phone.relaySig, "hosted:") || a.PhoneSync().Relay.Store != testlocale.Messages().SettingsPhoneRelayHosted() || tt.Exists("id:settings-phone-relay-store") {
		t.Fatalf("sig %q, relay %+v", a.phone.relaySig, a.PhoneSync().Relay)
	}
	data, err := os.ReadFile(a.settingsPath)
	if err != nil || !strings.Contains(string(data), `"phoneRelayHosted": true`) {
		t.Fatalf("settings.json: %s, %v", data, err)
	}
	c := phoneClient(t, a, "")
	paired, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.RelayJoin, error) {
		var joined psync.RelayJoin
		return joined, c.Call(ctx, psync.OpRelayJoin, nil, &joined)
	})
	if err != nil {
		t.Fatal(err)
	}
	folder := f.s3.URL + "/kando-hosted/acct/" + psync.RelayPrefix(a.phone.server.Fingerprint())
	if !strings.HasPrefix(joined.Grant.State, folder+psync.RelayStateName(paired.DeviceID)+"?") || !strings.Contains(joined.Grant.Slots[0].Put, "X-Amz-Security-Token=tok-") {
		t.Fatalf("grant %+v", joined.Grant)
	}
	if strings.Contains(joined.Grant.State, "hosted-secret") || !joined.Grant.Expires.Before(time.Now().Add(25*time.Hour)) {
		t.Fatalf("grant state %s, expires %v", joined.Grant.State, joined.Grant.Expires)
	}
	// Off again, the relay goes back to the storage of 账号与上传, of which none is set up.
	running = a.phone.server
	tt.Press("settings-phone-relay-hosted")
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != running })
	if a.phone.relaySig != "" || a.PhoneSync().Relay.Hosted {
		t.Fatalf("sig %q", a.phone.relaySig)
	}
}

// The hosted storage full, the films go as subtitles only, with one status line saying so, until there is room again; 只同步字幕 as the user set it stays.
func TestPhoneRelayHostedFullSyncsSubtitlesOnly(t *testing.T) {
	a, tt, updates, f := hostedApp(t)
	f.set(func() { f.plus, f.full = true, true })
	a.phone.host.on = true
	phoneSyncOn(t, a, tt, updates)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.host.stateKnown && a.phone.relayOK })
	m := testlocale.Messages()
	if a.phoneFilmsVideos() || a.phone.films.subtitlesOnly || !a.PhoneSync().Relay.HostedFull {
		t.Fatalf("videos %v, subtitles only %v, relay %+v", a.phoneFilmsVideos(), a.phone.films.subtitlesOnly, a.PhoneSync().Relay)
	}
	if !tt.Exists("id:settings-phone-relay-status >> text:" + m.SettingsPhoneRelayHostedFull()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	// Room again, as EveryAPI says with the next key: the videos go up again.
	a.phoneHostTold(a.phone.gen)(relayhost.State{Quota: 50 << 30, Used: 1 << 30, Writes: true})
	waitUntil(t, tt, a, updates, func() bool { return !a.phone.host.state.Full })
	if !a.phoneFilmsVideos() || tt.Exists("id:settings-phone-relay-status >> text:"+m.SettingsPhoneRelayHostedFull()) {
		t.Fatalf("videos %v; texts %q", a.phoneFilmsVideos(), tt.Texts())
	}
}

// Once Kando Plus and EveryAPI's grace after it ended, EveryAPI refuses the key: the relay goes back to the user's own storage, deleting nothing in the hosted one, and the row says Plus ended; Plus active again has it go back to 看懂托管.
func TestPhoneRelayHostedFallsBackWhenPlusEnds(t *testing.T) {
	a, tt, updates, f := hostedApp(t)
	phoneRelayStore(t, a)
	f.set(func() { f.plus, f.refuse = false, "plus_required" })
	a.phone.host.on = true
	phoneSyncOn(t, a, tt, updates)
	if !strings.HasPrefix(a.phone.relaySig, "hosted:") {
		t.Fatalf("started in %q", a.phone.relaySig)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.phone.host.lapsed })
	running := a.phone.server
	tt.Advance(phoneRelayRestartAfter)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != running })
	m := testlocale.Messages()
	r := a.PhoneSync().Relay
	if a.phone.relaySig == "" || strings.HasPrefix(a.phone.relaySig, "hosted:") || !r.Hosted || !r.HostedLapsed || r.Store == m.SettingsPhoneRelayHosted() {
		t.Fatalf("sig %q, relay %+v", a.phone.relaySig, r)
	}
	if !tt.Exists("id:settings-phone-relay-hosted-row >> text:"+m.SettingsPhoneRelayHostedLapsed()) || !tt.Exists("id:settings-phone-relay-store") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if !a.phone.host.on {
		t.Fatal("falling back turned 看懂托管 off")
	}
	f.mu.Lock()
	deletes := f.deletes
	f.mu.Unlock()
	if deletes != 0 {
		t.Fatalf("%d objects deleted from the hosted storage", deletes)
	}
	// Plus again: the page's check has the relay go back to 看懂托管.
	f.set(func() { f.plus, f.refuse = true, "" })
	a.phone.host.shown = false
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return !a.phone.host.lapsed })
	running = a.phone.server
	tt.Advance(phoneRelayRestartAfter)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != running })
	if !strings.HasPrefix(a.phone.relaySig, "hosted:") {
		t.Fatalf("sig %q", a.phone.relaySig)
	}
}

// The hosted relay's failures read in the page's words, never EveryAPI's, which may hold a price or a link; and a log line quoting a hosted link loses its session token in the log file and the feedback issue.
func TestPhoneRelayHostedKeepsSecretsAndPricesOut(t *testing.T) {
	m := testlocale.Messages()
	for err, want := range map[error]string{
		relayhost.ErrQuotaExceeded: m.SettingsPhoneRelayHostedFull(),
		relayhost.ErrNoKey:         m.SettingsPhoneRelayHostedSignIn(),
		relayhost.ErrUnauthorized:  m.SettingsPhoneRelayHostedSignIn(),
		relayhost.ErrPlusRequired:  m.SettingsPhoneRelayHostedLapsed(),
		relayhost.ErrRelayDisabled: m.SettingsPhoneRelayHostedUnavailable(),
		errors.New("EveryAPI answered HTTP 402: top up at https://everyapi.ai for $5"): m.SettingsPhoneRelayHostedUnavailable(),
	} {
		if got := phoneHostFailure(m, err); got != want {
			t.Errorf("%v: %q, want %q", err, got, want)
		}
	}
	line := "phonesync: relay: state: PUT https://r2.example/kando-hosted/acct/kando-relay/ab/outbox/D1/state.json?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKHOSTED%2F20261127%2Fauto%2Fs3%2Faws4_request&X-Amz-Security-Token=tok-secret-session&X-Amz-Signature=abc: 403"
	for _, cleaned := range []string{feedback.Scrubber{}.Clean(line), feedback.Scrubber{}.NoSecrets(line)} {
		if strings.Contains(cleaned, "tok-secret-session") {
			t.Fatalf("scrubbed %q", cleaned)
		}
	}
}

// The key the hosted relay asks with is the AI service's while it is EveryAPI: the key entered, else the everyapi program's sign-in; none for another service.
func TestPhoneRelayHostedAsksWithTheAIServicesKey(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	ctx := context.Background()
	if _, err := a.phoneHostKey()(ctx, false); !errors.Is(err, relayhost.ErrNoKey) {
		t.Fatalf("no tools: %v", err)
	}
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-deepseek-0123456789"}}
	if _, err := a.phoneHostKey()(ctx, false); !errors.Is(err, relayhost.ErrNoKey) {
		t.Fatalf("another service: %v", err)
	}
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, APIKey: hostedKey}}
	if key, err := a.phoneHostKey()(ctx, false); key != hostedKey || err != nil {
		t.Fatalf("entered key: %q, %v", key, err)
	}
	renewed := false
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Credential: func(_ context.Context, renew bool) (media.Endpoint, error) {
		renewed = renewed || renew
		return media.Endpoint{BaseURL: media.EveryAPIBaseURL, APIKey: "sk-cli-0123456789abcdef"}, nil
	}}}
	if key, err := a.phoneHostKey()(ctx, true); key != "sk-cli-0123456789abcdef" || err != nil || !renewed {
		t.Fatalf("the program's sign-in: %q, %v, renewed %v", key, err, renewed)
	}
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Credential: func(context.Context, bool) (media.Endpoint, error) {
		return media.Endpoint{}, media.ErrEveryAPISignedOut
	}}}
	if _, err := a.phoneHostKey()(ctx, false); !errors.Is(err, relayhost.ErrNoKey) || !errors.Is(err, media.ErrEveryAPISignedOut) {
		t.Fatalf("signed out: %v", err)
	}
}

// settings.json keeps 看懂托管 chosen.
func TestPhoneRelayHostedIsSaved(t *testing.T) {
	var saved savedSettings
	saved.PhoneRelayHosted = true
	data, err := json.Marshal(saved)
	if err != nil || !strings.Contains(string(data), `"phoneRelayHosted":true`) {
		t.Fatalf("%s, %v", data, err)
	}
	var back savedSettings
	if err := json.Unmarshal(data, &back); err != nil || !back.PhoneRelayHosted {
		t.Fatalf("%+v, %v", back.savedPhoneRelayHost, err)
	}
}
