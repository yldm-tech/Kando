package script

import (
	"errors"
	"testing"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
)

// TestScriptProbeContributions checks that Probe holds a plugin to export every function its contributions besides commands call, and compares its credentials as it does its capabilities.
func TestScriptProbeContributions(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	probe := func(f fixture) error {
		t.Helper()
		p := s.pack(t, f)
		v, err := Verify(trust, roots, p)
		if err != nil {
			t.Fatal(err)
		}
		e.do(func() { err = Probe(e.root, v) })
		return err
	}
	caps := `{"host": ["kando.net.credentialFetch"]}`
	extra := `"contributes": {"lookups": [{"id": "w", "export": "lookUp", "label": {"en": "W"}}], "annotators": [{"id": "a", "export": "annotate", "label": {"en": "A"}}]},
  "credentials": [{"id": "deepl", "title": {"en": "DeepL"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate"]}]`
	if err := probe(fixture{id: "acme.wiki", caps: caps, extra: extra, code: "export function lookUp() { return null }\nexport function annotate() { return [] }"}); err != nil {
		t.Fatalf("a plugin that says what it is: %v", err)
	}
	wantErr(t, "no annotate", probe(fixture{id: "acme.wiki", caps: caps, extra: extra, code: "export function lookUp() { return null }"}), plugins.ErrTampered)
	wantErr(t, "another host for the key", probe(fixture{id: "acme.wiki", caps: caps, extra: extra, code: "export function lookUp() { return null }\nexport function annotate() { return [] }",
		said: `{"capabilities": ` + caps + `, "contributes": {"lookups": [{"id": "w", "export": "lookUp", "label": {"en": "W"}}], "annotators": [{"id": "a", "export": "annotate", "label": {"en": "A"}}]}, "credentials": [{"id": "deepl", "title": {"en": "DeepL"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["collector.example.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate"]}]}`}), plugins.ErrTampered)
}

// TestScriptHostTimeoutIsNotAFailure checks that a call failing because a host call it made timed out (core.ErrHostTimeout, ETIMEDOUT in JS), as kando.llm.chat does on a slow AI service, is not counted by the breaker, however often (docs/plugin-system.md 6.6).
func TestScriptHostTimeoutIsNotAFailure(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	p := s.pack(t, fixture{id: "acme.chat", caps: `{"host": ["kando.llm.chat"]}`, code: `import { host } from "nsgo"
export async function ask() { return await host.call("kando.llm.chat", { user: "hi" }, { timeout: 20 }) }`})
	v, err := Verify(trust, roots, p)
	if err != nil {
		t.Fatal(err)
	}
	host := core.NewHost()
	// The AI service never answers.
	host.Handle(string(plugins.PermLLMChat), func(*core.HostCall) {})
	var pl *Plugin
	e.do(func() { pl, err = Load(e.root, v, plugins.Grant{Perms: []plugins.Perm{plugins.PermLLMChat}}, host) })
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: DefaultWatch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	for i := range 5 {
		got := wait(t, e.call(r, pl, "ask"), slow(5*time.Second))
		if got.err == nil || core.ErrorCode(got.err) != core.CodeTimedOut {
			t.Fatalf("call %d: %v", i, got.err)
		}
	}
	select {
	case reason := <-stopped:
		t.Fatalf("stopped for %s", reason)
	default:
	}
	if pl.Closed() {
		t.Fatal("closed")
	}
}

// TestScriptPauseForTheCallsPlugin checks that a wait on behalf of a plugin pauses the call running only when it is that plugin's: a host call another plugin made from a timer, after its own call ended, pauses nothing.
func TestScriptPauseForTheCallsPlugin(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	code := `export function wait() { return new Promise(() => {}) }`
	a := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.a", code: code}), plugins.Grant{})
	b := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.b", code: code}), plugins.Grant{})
	r := &Runner{Post: e.post, Watch: DefaultWatch}
	e.call(r, a, "wait")
	paused := func() int {
		var n int
		e.do(func() {
			c := r.running
			c.mu.Lock()
			n = c.paused
			c.mu.Unlock()
		})
		return n
	}
	var resume func()
	e.do(func() { resume = r.PauseFor(b) })
	if paused() != 0 {
		t.Fatal("another plugin's wait paused the call")
	}
	resume()
	e.do(func() { resume = r.PauseFor(a) })
	if paused() != 1 {
		t.Fatal("the plugin's own wait did not pause its call")
	}
	resume()
	e.do(func() { r.Close(a) })
}

// TestScriptPausedTimeIsCapped checks that a call the app waited on in all past Watch.Paused is stopped as slow, however its waits came.
func TestScriptPausedTimeIsCapped(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.loop", code: `export function wait() { return new Promise(() => {}) }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	w := DefaultWatch
	w.Paused = 300 * time.Millisecond
	r := &Runner{Post: e.post, Watch: w, Stopped: func(id, hash, reason string) { stopped <- reason }}
	done := e.call(r, pl, "wait")
	// Waits of 100 ms one after the other, as a loop of host calls makes them.
	for range 5 {
		var resume func()
		e.do(func() { resume = r.PauseFor(pl) })
		time.Sleep(100 * time.Millisecond)
		e.do(resume)
	}
	if got := wait(t, done, slow(5*time.Second)); !errors.Is(got.err, ErrStopped) {
		t.Fatalf("the call: %v", got.err)
	}
	if reason := <-stopped; reason != plugins.ReasonSlow {
		t.Errorf("stopped for %s", reason)
	}
}

// TestScriptProbeTranslators checks that Probe holds a plugin with translators to export translate, and compares their languages, "any" or {from, to}, as kandoManifest writes them.
func TestScriptProbeTranslators(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	probe := func(f fixture) error {
		t.Helper()
		p := s.pack(t, f)
		v, err := Verify(trust, roots, p)
		if err != nil {
			t.Fatal(err)
		}
		e.do(func() { err = Probe(e.root, v) })
		return err
	}
	extra := `"contributes": {"translators": [{"id": "any", "title": {"en": "Any"}, "languages": "any"}, {"id": "zh", "title": {"en": "Into Chinese"}, "languages": {"from": ["en"], "to": ["zh-Hans"]}}]}`
	if err := probe(fixture{id: "acme.mt", extra: extra, code: "export async function translate(lines) { return lines }"}); err != nil {
		t.Fatalf("a translator that says what it is: %v", err)
	}
	wantErr(t, "no translate", probe(fixture{id: "acme.mt", extra: extra, code: "export async function candidates() { return [] }"}), plugins.ErrTampered)
	wantErr(t, "another language", probe(fixture{id: "acme.mt", extra: extra, code: "export async function translate(lines) { return lines }",
		said: `{"contributes": {"translators": [{"id": "any", "title": {"en": "Any"}, "languages": "any"}, {"id": "zh", "title": {"en": "Into Chinese"}, "languages": {"from": ["en"], "to": ["zh-Hant"]}}]}}`}), plugins.ErrTampered)
}
