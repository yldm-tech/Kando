package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// onIPhone is an iPhone, whose pages NSGo puts under the system's navigation bar.
var onIPhone = nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS})

// containerRoles are the roles iOS gives a view as a container of the elements inside it rather than as an element itself (NSGo's internal/platform/ios/access.go, updateAccessibility).
var containerRoles = map[core.Role]bool{
	core.RoleRadioGroup: true, core.RoleTabList: true, core.RoleMenu: true, core.RoleDialog: true, core.RoleAlertDialog: true, core.RoleAlert: true,
	core.RoleStatus: true, core.RoleListbox: true, core.RoleGroup: true, core.RoleRegion: true, core.RoleNavigation: true, core.RoleTree: true, core.RoleToolbar: true,
}

// screenReaderMiss is the view that keeps v from a screen reader on iOS, nil when VoiceOver and UI automation reach it: a view over it that hides its subtree, or one iOS makes a single element, as it does every labelled view, whose subviews it does not read.
func screenReaderMiss(v *nsgotest.View) *nsgotest.View {
	for p := v.Parent; p != nil; p = p.Parent {
		if p.Props.ElementsHidden {
			return p
		}
		if containerRoles[p.Props.Role] {
			continue
		}
		if p.Props.Label != "" || p.Props.Accessible == core.AccessibleYes || p.Props.Role != core.RoleNone {
			return p
		}
	}
	return nil
}

// wantReached fails t unless the page page shows its title in the header and a screen reader reaches each of controls on it.
func wantReached(t *testing.T, p *phone, page, title string, controls ...string) {
	t.Helper()
	pages := p.app.QueryMounted("id:" + page)
	if len(pages) != 1 {
		t.Fatalf("%d views are %s, want 1", len(pages), page)
	}
	if l := pages[0].Props.Label; l != "" {
		t.Errorf("%s is labelled %q, which makes it one element on iOS, hiding what is on it", page, l)
	}
	shown := false
	for _, v := range p.app.QueryMounted("id:nav-title") {
		shown = shown || v.Text == title && v.Props.Role == core.RoleHeader
	}
	if !shown {
		t.Errorf("no header heads %s %q", page, title)
	}
	for _, c := range controls {
		got := p.app.QueryMounted("id:" + page + " >> " + c)
		if len(got) == 0 {
			t.Errorf("%s has no %s", page, c)
			continue
		}
		if miss := screenReaderMiss(got[0]); miss != nil {
			t.Errorf("a screen reader does not reach %s on %s: %s (id %q, label %q, role %v) hides it", c, page, miss.Kind, miss.Props.TestID, miss.Props.Label, miss.Props.Role)
		}
	}
}

// On an iPhone the computer's page and the pairing page it opens are titled in the system's bar and VoiceOver and UI automation reach every control on them, the page not being one element named by its title.
func TestAnIPhonesScreenReaderReachesTheComputerAndPairingPages(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), onIPhone)
	p.openComputers()
	p.app.WaitUntil(wait, func() bool { return p.texts("id:computer-name") == "Studio" })
	desktop := p.app.QueryMounted("id:computer-page >> id:computer-card")
	if len(desktop) == 0 {
		t.Fatal("the computer's page shows no card")
	}
	wantReached(t, p, "computer-page", msgs.SettingsComputer(),
		"id:computer-name", "id:computer-status", "id:computer-note", "id:computer-service", "id:computer-pair", "id:computer-unpair")

	p.app.Press("id:computer-pair")
	p.app.WaitUntil(wait, func() bool { return len(p.app.QueryMounted("id:pair-page")) == 1 })
	wantReached(t, p, "pair-page", msgs.PairTitle(), "id:pair-steps", "id:pair-address", "id:pair-code", "id:pair-continue")
}
