package main

import (
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The player's keys step between the lines, replay and loop the line, hold the film and switch the subtitle mode, as the buttons do.
func TestThePlayerTakesItsKeys(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.Key("right")
	if s := p.seeks(); s[len(s)-1] != 5*sec {
		t.Fatalf("→ went to %v, want the next line at 5s", s)
	}
	p.at(5200 * time.Millisecond)
	p.app.Key("left")
	if s := p.seeks(); s[len(s)-1] != 3*sec {
		t.Fatalf("← went to %v, want the line before at 3s", s)
	}
	p.at(4 * sec)
	p.app.Key("r")
	if s := p.seeks(); s[len(s)-1] != 3*sec {
		t.Fatalf("R went to %v, want the line's start at 3s", s)
	}
	p.app.Key("l")
	if !p.app.Find("id:player-card-loop").Props.Selected {
		t.Fatal("L did not loop the line")
	}
	p.app.Key("l")
	if p.app.Find("id:player-card-loop").Props.Selected {
		t.Fatal("L again did not stop the loop")
	}
	p.app.Key("space")
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("Space did not hold the film")
	}
	p.app.Key("space")
	if p.app.Find("id:player-video").Video.Paused {
		t.Fatal("Space again did not play the film")
	}
	p.at(3200 * time.Millisecond)
	p.app.Key("m")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerModeOriginal(), wait)
	if p.app.Find("id:player-card-original").Props.Label != "How are you?" || p.app.Find("id:player-card-translation").Props.Label != msgs.PlayerHiddenLine() {
		t.Fatal("M did not switch to the original alone")
	}
	for _, want := range []string{msgs.PlayerModeTranslation(), msgs.PlayerModeHidden(), msgs.PlayerModeBilingual()} {
		p.app.Key("m")
		p.app.WaitFor("id:player-toasts >> text:"+want, wait)
	}
	if p.app.Find("id:player-card-translation").Props.Label != "你好吗？" {
		t.Fatal("M four times did not come back to both lines")
	}
}

// Minimized, the player leaves the keys alone: the film plays on in the mini player, and Space and the arrows do nothing to it.
func TestTheMiniPlayerTakesNoKeys(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.Back()
	p.app.WaitFor("id:player-mini", wait)
	seeks := len(p.seeks())
	p.app.Key("space")
	p.app.Key("right")
	if p.app.Find("id:player-video").Video.Paused {
		t.Fatal("Space held the film in the mini player")
	}
	if s := p.seeks(); len(s) != seeks {
		t.Fatalf("→ moved the film in the mini player: %v", s[seeks:])
	}
}

// Under the player over the app, the shell's keys rest: ⌘N opens no Add sheet behind it.
func TestThePlayerCoversTheShellsKeys(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Key("mod+n")
	if p.app.Exists("id:add-link") {
		t.Fatal("⌘N opened the Add sheet under the player")
	}
	p.app.Back()
	p.app.WaitFor("id:player-mini", wait)
	p.app.Key("mod+n")
	p.app.WaitFor("id:add-link", wait)
}

// S saves the line the card shows, and E has the computer explain it.
func TestThePlayerSavesAndExplainsFromTheKeyboard(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanation("A greeting for the morning.")
	p, _ := studyPlayer(t, d)
	p.app.Key("s")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerSavedToast(), wait)
	p.app.Key("e")
	p.app.WaitFor("id:player-explain-text", wait)
	if x := d.Explains(); len(x) != 1 || x[0].Lines[x[0].Index].Original != "Good morning." {
		t.Fatalf("the computer was asked %+v, want the line the card shows", x)
	}
}

// On a tablet ⌘1 and on show the sections, ⌘N the Add sheet and ⌘K the Library's search from any section; Settings lists them all with their keys.
func TestATabletTakesTheShellsKeysAndListsThem(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the idiom is the platform's on a target")
	}
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet, OS: core.OSIOS}))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.Key("mod+2")
	p.app.WaitFor("id:jobs-page", wait)
	p.app.Key("mod+3")
	p.app.WaitFor("id:learn-page", wait)
	p.app.Key("mod+k")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:films-search") && p.app.Find("id:films-search").Focused })
	p.app.Key("mod+n")
	p.app.WaitFor("id:add-link", wait)
	p.app.Key("escape")
	p.app.WaitForGone("id:add-link", wait)

	p.app.Key("mod+4")
	p.app.WaitFor("id:settings-shortcuts", wait)
	if got := p.texts("id:settings-shortcuts"); !strings.Contains(got, msgs.SettingsShortcutsValue()) {
		t.Fatalf("the shortcuts' row reads %q", got)
	}
	p.app.Press("id:settings-shortcuts")
	p.app.WaitFor("id:settings-shortcuts-page", wait)
	for id, want := range map[string]string{
		"play":     msgs.ShortcutsSpace(),
		"lines":    "← / →",
		"explain":  "E",
		"search":   "⌘K",
		"sections": "⌘1 – ⌘4",
	} {
		if got := p.app.Find("id:settings-shortcut-keys-" + id).Text; got != want {
			t.Errorf("the keys of %s read %q, want %q", id, got, want)
		}
	}
	names := strings.Join([]string{msgs.FilmsTitle(), msgs.JobsTitle(), msgs.LearnTitle(), msgs.SettingsTitle()}, msgs.ShortcutsJoin())
	if got := p.texts("id:settings-shortcut-sections"); !strings.HasPrefix(got, names) {
		t.Errorf("the row of ⌘1 – ⌘4 reads %q, want the sections %q", got, names)
	}
}

// A phone lists no keyboard shortcuts in Settings, and an Android one no widgets, which iOS alone has.
func TestAPhoneListsNoShortcutsAndAndroidNoWidgets(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSAndroid}))
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-goal", wait)
	if p.app.Exists("id:settings-shortcuts") || p.app.Exists("id:settings-widgets") {
		t.Fatal("an Android phone lists keyboard shortcuts or widgets")
	}
}
