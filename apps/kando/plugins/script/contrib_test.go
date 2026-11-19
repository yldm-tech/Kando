package script

import (
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
