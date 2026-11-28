package main

import (
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Settings shows the computer that processes the links with the AI service it translates with, by its name and model, and turns its context switch off there; out of reach, the computer keeps it as it was and Settings says it could not change it. Its page says it answers, and unpairing it there leaves Settings offering to pair one.
func TestSettingsShowsTheComputersServiceAndSetsItsContext(t *testing.T) {
	d := newDesktop(t)
	d.SetService("OpenAI", "gpt-4.1-mini")
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.PendingAlerts())
	id := store.DesktopID(d.Fingerprint)
	p.waitLearned(id)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitUntil(wait, func() bool {
		return p.texts("id:settings-service") == msgs.ComputerService()+"\nOpenAI · gpt-4.1-mini"
	})
	if got, want := p.texts("id:settings-profile-detail"), msgs.SettingsProfileDetail("Studio", "简体中文"); got != want {
		t.Errorf("the app's card says %q, want %q", got, want)
	}
	if !p.app.Find("id:computer-context-switch").Props.Checked {
		t.Fatal("the context switch is off while the computer's is on")
	}

	p.app.Press("id:computer-context-switch")
	p.app.WaitUntil(wait, func() bool { return !d.Context() })
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:computer-context-switch").Props.Checked })

	d.SetOff(true)
	// The computer takes the change before its reply reaches the phone, which keeps the switch disabled until then.
	p.app.WaitUntil(wait, func() bool { return p.app.Enabled("id:computer-context-switch") })
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
	p.app.Press("id:settings-computer")
	p.app.WaitFor("id:computer-page", wait)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:computer-name") == "Studio" })
	p.app.Press("id:computer-unpair")
	p.app.Answer(1)
	p.app.WaitForGone("id:computer-page", wait)
	p.app.WaitFor("id:settings-pair", wait)
	if p.app.Exists("id:settings-computer") {
		t.Fatal("Settings shows a processing computer with none paired")
	}
}

// Before a sync reached the computer its page says its settings show once it does, and the switch cannot be turned; with no service set up there, the page says so.
func TestAComputersPageBeforeItIsReached(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:computer-context-switch", wait)
	if !p.app.Find("id:computer-context-switch").Props.Disabled {
		t.Fatal("the context switch can be turned before the computer answered")
	}
	p.app.Press("id:settings-computer")
	p.app.WaitFor("id:computer-page", wait)
	p.app.WaitFor("text:"+msgs.ComputerUnknown(), wait)

	d.SetOff(false)
	p.app.Back()
	p.app.Press("id:settings-computer")
	p.app.WaitUntil(wait, func() bool {
		return p.texts("id:computer-service") == msgs.ComputerService()+"\n"+msgs.ComputerServiceNone()
	})
	p.app.Back()
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:computer-context-switch").Props.Disabled })
}

// With two computers paired, the computer page lists both, the first paired checked; the one picked there is kept as where new links go.
func TestTheComputerPagePicksWhereNewLinksGo(t *testing.T) {
	d, e := newDesktop(t), newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWithBoth(t, d, e, dir))
	first := store.DesktopID(d.Fingerprint)
	p.openComputers()
	p.app.WaitFor("id:settings-computer-laptop", wait)
	// With nothing picked, new links go to the first computer that answers (session.SendTo), and the two answer in either order: the laptop may be checked until the first one answers too, after which the first paired is.
	p.app.WaitUntil(wait, func() bool {
		return p.app.Find("id:settings-computer-"+first).Props.Checked && !p.app.Find("id:settings-computer-laptop").Props.Checked
	})
	p.app.Press("id:settings-computer-laptop")
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-computer-laptop").Props.Checked })
	if p.app.Find("id:settings-computer-" + first).Props.Checked {
		t.Fatal("two computers are checked")
	}
	p.app.WaitUntil(wait, func() bool { prefs, _ := store.Open(dir).Prefs(); return prefs.SendTo == "laptop" })
}

// With nothing picked in Settings and the computer paired first out of reach, new links go to the one that answers: Settings checks it and the Add sheet starts on it.
func TestSettingsAndTheAddSheetSendToTheSameComputer(t *testing.T) {
	d, e := newDesktop(t), newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWithBoth(t, d, e, dir)
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	first := store.DesktopID(d.Fingerprint)
	p.openComputers()
	p.app.WaitFor("id:settings-computer-laptop", wait)
	p.app.WaitUntil(reconnect, func() bool { return p.app.Find("id:settings-computer-laptop").Props.Checked })
	if p.app.Find("id:settings-computer-" + first).Props.Checked {
		t.Fatal("two computers are checked")
	}
	p.app.Back()
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.Press("id:films-add")
	p.app.WaitFor("id:add-computer-laptop", wait)
	if !p.app.Find("id:add-computer-laptop").Props.Checked || p.app.Find("id:add-computer-"+first).Props.Checked {
		t.Fatal("the Add sheet starts on another computer than the one Settings shows")
	}
}

// The computer page shows the computer new links go to; unpairing it there shows the next one, and the page goes with the last.
func TestUnpairingTheComputerShownShowsTheNext(t *testing.T) {
	d, e := newDesktop(t), newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWithBoth(t, d, e, dir))
	first := store.DesktopID(d.Fingerprint)
	p.openComputers()
	p.app.WaitFor("id:settings-computer-laptop", wait)
	p.app.Press("id:settings-computer-" + first)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-computer-" + first).Props.Checked })
	p.app.Press("id:computer-unpair")
	p.app.Answer(1)
	p.app.WaitForGone("id:settings-computer-"+first, wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-computer-laptop").Props.Checked })
	if len(p.app.QueryMounted("id:computer-page")) != 1 {
		t.Fatal("the computer page went with a computer left to show")
	}
	p.app.Press("id:computer-unpair")
	p.app.Answer(1)
	p.app.WaitUntil(wait, func() bool { return len(p.app.QueryMounted("id:computer-page")) == 0 })
	p.app.WaitFor("id:settings-pair", wait)
}

// A computer with the user's storage set up says on its page that the storage serves the phone, while the phone talks to it directly too.
func TestTheComputerPageShowsItsStorageWhileReachedDirectly(t *testing.T) {
	d := newDesktop(t)
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.openComputers()
	p.app.WaitUntil(wait, func() bool { return p.texts("id:computer-storage-value") == msgs.ComputerStorageOK() })
}

// A computer named otherwise since the phone paired with it, as one that named itself by its host name then and by the name its user gave it now, is named as Hello names it once it answers: the phone keeps the new name and shows it, without pairing again.
func TestAPairedComputerTakesTheNameItsHelloGives(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	st := store.Open(dir)
	desks, err := st.Desktops()
	if err != nil || len(desks) != 1 || desks[0].Name != "Studio" {
		t.Fatalf("paired %+v, %v", desks, err)
	}
	desks[0].Name = "842f574b78da"
	if err := st.SaveDesktops(desks); err != nil {
		t.Fatal(err)
	}
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	id := store.DesktopID(d.Fingerprint)
	p.openTab(jobsTab, msgs.JobsTitle())
	card := "id:jobs-computer-" + id
	p.app.WaitFor(card, wait)
	if got := p.texts(card); !strings.Contains(got, "842f574b78da") {
		t.Fatalf("the computer's card says %q before it answers, want the name kept at pairing", got)
	}

	d.SetOff(false)
	p.app.WaitUntil(reconnect, func() bool { got := p.texts(card); return strings.Contains(got, "Studio") && !strings.Contains(got, "842f574b78da") })
	p.app.WaitUntil(wait, func() bool { desks, err := store.Open(dir).Desktops(); return err == nil && len(desks) == 1 && desks[0].Name == "Studio" })
}
