package script

import (
	"errors"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
)

// panelFixture is a plugin with three panels: deck, which shows the parameters its view was given, broken, which throws as it renders, and spin, which never returns.
var panelFixture = fixture{id: "acme.panel", extra: `"contributes": {"panels": [{"id": "deck", "export": "Deck", "title": {"en": "Deck"}}, {"id": "broken", "export": "Broken", "title": {"en": "Broken"}}, {"id": "spin", "export": "Spin", "title": {"en": "Spin"}}]}`,
	code: `import { host, mount, Text } from "nsgo"
export function Deck() { return <Text testID="deck">{"deck " + host.params.panel + " " + host.params.export}</Text> }
export function Broken() { throw new Error("broken panel") }
export function Spin() { for (;;) {} }
if (host.params.panel === "deck") mount(Deck)
if (host.params.panel === "broken") mount(Broken)
if (host.params.panel === "spin") mount(Spin)
`}

// TestScriptPanel checks a panel's view (Runner.Panel): it shows the export its parameters name, apart from the plugin loaded for its functions, which mounts nothing; one that throws shows the fallback in its place, with what it threw, and each failure, a retry's too, counts for the plugin's breaker, which turns it off at the third.
func TestScriptPanel(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	p := s.pack(t, panelFixture)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan string, 1)
	var failed []error
	start := func(id string) *nsgotest.App {
		return nsgotest.Start(t, func(cx *core.Scope) core.Node {
			r := &Runner{Post: cx.Post, Root: t.TempDir(), Watch: DefaultWatch, Stopped: func(_, _, reason string) { stopped <- reason }}
			pl, err := Load(cx, v, plugins.Grant{}, core.NewHost())
			if err != nil {
				t.Fatal(err)
			}
			i := 0
			for i < len(p.Contributes.Panels) && p.Contributes.Panels[i].ID != id {
				i++
			}
			return core.View{Children: []core.Node{
				core.Text{Content: "host", TestID: "host"},
				r.Panel(pl, v, plugins.Grant{}, core.NewHost(), p.Contributes.Panels[i], func(cx *core.Scope, err error, retry func()) core.Node {
					failed = append(failed, err)
					return base.Button{TestID: "retry", Label: "retry", OnPress: func(core.PressEvent) { retry() }, Children: []core.Node{core.Text{Content: "retry"}}}
				}),
			}}
		})
	}
	app := start("deck")
	if got := app.Find("id:deck").Text; got != "deck deck Deck" {
		t.Fatalf("the panel shows %q\n%s", got, app.Dump())
	}
	if len(failed) > 0 || app.Find("id:host").Text != "host" {
		t.Fatalf("failed %v", failed)
	}
	app = start("broken")
	if !app.Exists("id:retry") || len(failed) != 1 || !strings.Contains(failed[0].Error(), "broken panel") {
		t.Fatalf("a panel that throws: %v\n%s", failed, app.Dump())
	}
	if app.Find("id:host").Text != "host" {
		t.Fatal("the host's own view is gone")
	}
	for range 2 {
		app.Press("id:retry")
		app.Settle()
	}
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonFailed || len(failed) != 3 {
			t.Errorf("stopped for %q after %d failures", reason, len(failed))
		}
	default:
		t.Fatalf("not stopped after %d failures", len(failed))
	}
}

// TestScriptPanelPastItsBudget checks that a panel whose render runs past Limits' budget is cut there and shows the fallback, the window's goroutine held no longer than that.
func TestScriptPanelPastItsBudget(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	p := s.pack(t, panelFixture)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	start := time.Now()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		r := &Runner{Post: cx.Post, Watch: DefaultWatch}
		pl, err := Load(cx, v, plugins.Grant{}, core.NewHost())
		if err != nil {
			t.Fatal(err)
		}
		return r.Panel(pl, v, plugins.Grant{}, core.NewHost(), p.Contributes.Panels[2], func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{TestID: "failed", Content: "failed"}
		})
	})
	if !app.Exists("id:failed") || !errors.Is(failed, tsx.ErrBudget) && !contains(failed, "budget") {
		t.Fatalf("a panel past its budget: %v", failed)
	}
	if took := time.Since(start); took > slow(3*time.Second) {
		t.Errorf("it held the window %v", took)
	}
}
