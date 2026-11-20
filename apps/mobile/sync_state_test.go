package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// otherDevice pairs another device with d, as a tablet of the same learner, and returns its client.
func otherDevice(t *testing.T, d *synctest.Desktop) *sync.Client {
	t.Helper()
	d.Offer("654321")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	resp, err := c.Pair(context.Background(), sync.PairRequest{Code: "654321", Device: "Tablet", Platform: sync.PlatformAndroid})
	c.Close()
	if err != nil {
		t.Fatal(err)
	}
	c = sync.NewClient(d.Addr, d.Fingerprint, resp.Token)
	t.Cleanup(c.Close)
	return c
}

// conflictDesktop is studyDesktop with the note of its saved line "l-fine" changed on another device from an older note than the desktop's, which the desktop keeps as the later but records as a conflict with its own. It returns the desktop and the key the phone lists the conflict under (session.SyncConflict.Key).
func conflictDesktop(t *testing.T) (*synctest.Desktop, string) {
	t.Helper()
	d := studyDesktop(t)
	tablet := otherDevice(t, d)
	res, err := tablet.PutState(context.Background(), "lib", []sync.Record{{Kind: sync.KindLine, ID: "l-fine", Film: "f1", Fields: map[string]sync.Field{
		sync.FieldNote: {Value: sync.Value("said to a friend"), Stamp: sync.Stamp{At: now()}, Base: &sync.Stamp{At: now().Add(-2 * time.Hour), Device: "desktop"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("the put recorded %+v", res.Conflicts)
	}
	d.Offer("123456")
	return d, store.DesktopID(d.Fingerprint) + "/lib/" + res.Conflicts[0].ID
}

// openConflicts starts the phone paired with d and opens Settings › Sync Conflicts once the phone has read the computer's learning state.
func openConflicts(t *testing.T, d *synctest.Desktop) *phone {
	t.Helper()
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.openTab(settingsTab, msgs.SettingsTitle())
	// What Settings shows, not the sync's files, which a target keeps apart from the test's.
	p.app.WaitFor("text:"+msgs.SettingsConflictsDetail(), wait)
	p.app.Press("id:settings-conflicts")
	p.app.WaitFor("id:conflicts-page", wait)
	return p
}

// Settings lists a note two devices changed apart with both versions, where each was made, and keeps both once asked: the computer takes the two notes together and the conflict is gone everywhere. A computer out of reach leaves it open and says so.
func TestASyncConflictIsSettledOnThePhone(t *testing.T) {
	d, key := conflictDesktop(t)
	p := openConflicts(t, d)
	p.app.WaitFor("id:conflict-"+key, wait)
	for sel, want := range map[string]string{
		"id:conflict-kind-" + key: msgs.SettingsConflictNote(), "id:conflict-film-" + key: "Morning Talk", "id:conflict-where-" + key: "0:05",
		"id:conflict-line-" + key: "Fine, thanks.", "id:conflict-kept-" + key + "-text": "said to a friend", "id:conflict-lost-" + key + "-text": "a polite answer",
	} {
		if got := p.texts(sel); got != want {
			t.Errorf("%s says %q, want %q", sel, got, want)
		}
	}
	if got := p.texts("id:conflict-lost-" + key + "-meta"); !strings.HasPrefix(got, "Studio · ") {
		t.Errorf("the version set aside says %q, want the computer's name", got)
	}
	if got := p.texts("id:conflict-kept-" + key + "-meta"); !strings.HasPrefix(got, msgs.SettingsConflictOtherDevice()+" · ") {
		t.Errorf("the version kept says %q, want a device the computer does not name", got)
	}

	d.SetOff(true)
	p.app.Press("id:conflict-both-" + key)
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:conflicts-failure") && p.texts("id:conflicts-failure") == msgs.SettingsConflictUnreachable("Studio")
	})
	if !p.app.Exists("id:conflict-" + key) {
		t.Fatal("the conflict left the list though the computer could not be reached")
	}

	d.SetOff(false)
	p.app.Press("id:conflict-both-" + key)
	p.app.WaitFor("id:conflicts-done", wait)
	if p.app.Exists("id:conflicts-failure") {
		t.Error("the failure still shows")
	}
	r, _ := d.State("lib", sync.KindLine, "l-fine")
	if got := fieldText(r, sync.FieldNote); got != "said to a friend\na polite answer" {
		t.Fatalf("the computer's note is %q", got)
	}
	p.app.HeaderBack("id:conflicts-page")
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:settings-conflicts"), msgs.SettingsConflictsNone()) })
}

// Settings says how many conflicts are open, and Use This keeps the version set aside on the computer.
func TestUseThisKeepsTheVersionPicked(t *testing.T) {
	d, key := conflictDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:settings-conflicts"), msgs.SettingsConflictsCount(1)) })
	p.app.Press("id:settings-conflicts")
	p.app.WaitFor("id:conflict-lost-"+key, wait)
	p.app.Press("id:conflict-lost-" + key)
	p.app.WaitFor("id:conflicts-done", wait)
	r, _ := d.State("lib", sync.KindLine, "l-fine")
	if got := fieldText(r, sync.FieldNote); got != "a polite answer" {
		t.Fatalf("the computer's note is %q", got)
	}
}

// The player sends where the film was left to its computer as it is held, minimized or left, not every few seconds while it plays on, so the computer and the other devices go on from there.
func TestWhereAFilmWasLeftGoesToTheComputer(t *testing.T) {
	d := newDesktop(t)
	p, _ := studyPlayer(t, d)
	puts := d.Puts()
	p.at(6 * sec)
	time.Sleep(300 * time.Millisecond)
	if d.Puts() != puts {
		t.Fatal("the position went to the computer while the film played on")
	}
	p.app.Press("id:player-minimize")
	p.app.WaitUntil(wait, func() bool {
		r, ok := d.State("lib", sync.KindPosition, "f1")
		return ok && string(r.Fields[sync.FieldPositionMS].Value) == "6000"
	})
}
