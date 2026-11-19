package main

import (
	"testing"

	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A computer's page shows the AI service it translates with, by its name and model, and turns its context switch off there; out of reach, the computer keeps it as it was and the page says it could not change it.
func TestAComputersPageShowsItsServiceAndSetsItsContext(t *testing.T) {
	d := newDesktop(t)
	d.SetService("OpenAI", "gpt-4.1-mini")
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.PendingAlerts())
	id := store.DesktopID(d.Fingerprint)
	p.waitLearned(id)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer-" + id)
	p.app.WaitFor("id:computer-page", wait)
	p.app.WaitUntil(wait, func() bool {
		return p.texts("id:computer-service") == msgs.ComputerService()+"\nOpenAI · gpt-4.1-mini"
	})
	if !p.app.Find("id:computer-context-switch").Props.Checked {
		t.Fatal("the context switch is off while the computer's is on")
	}

	p.app.Press("id:computer-context-switch")
	p.app.WaitUntil(wait, func() bool { return !d.Context() })
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:computer-context-switch").Props.Checked })

	d.SetOff(true)
	p.app.Press("id:computer-context-switch")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) == 1 })
	if got, want := p.app.Alerts()[0].Title, msgs.ComputerContextFailed("Studio"); got != want {
		t.Errorf("the alert says %q, want %q", got, want)
	}
	p.app.Answer(0)
	if p.app.Find("id:computer-context-switch").Props.Checked || d.Context() {
		t.Fatal("a change the computer did not take turned the switch on")
	}

	// Unpaired from its page, the computer and its page go.
	d.SetOff(false)
	p.app.Press("id:computer-unpair")
	p.app.Answer(1)
	p.app.WaitForGone("id:computer-page", wait)
	p.app.WaitForGone("id:settings-computer-"+id, wait)
}

// Before a sync reached the computer its page says its settings show once it does, and the switch cannot be turned; with no service set up there, the page says so.
func TestAComputersPageBeforeItIsReached(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	id := store.DesktopID(d.Fingerprint)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer-" + id)
	p.app.WaitFor("id:computer-page", wait)
	p.app.WaitFor("text:"+msgs.ComputerUnknown(), wait)
	if !p.app.Find("id:computer-context-switch").Props.Disabled {
		t.Fatal("the context switch can be turned before the computer answered")
	}

	d.SetOff(false)
	p.app.Back()
	p.app.Press("id:settings-computer-" + id)
	p.app.WaitUntil(wait, func() bool {
		return p.texts("id:computer-service") == msgs.ComputerService()+"\n"+msgs.ComputerServiceNone()
	})
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:computer-context-switch").Props.Disabled })
}
