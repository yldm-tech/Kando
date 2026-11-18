package main

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	"kando/packages/core"
	psync "kando/packages/sync"
)

// A phone pauses a job where it is: the running job stops and waits, held, with the phone's name on its card and 在这台电脑上继续, while the jobs behind it go on; it goes on once let go, from the card or from the phone.
func TestPhoneSyncPausesAJobUntilItIsContinued(t *testing.T) {
	a, tt, updates, _, c := phonePaired(t)
	m := testlocale.Messages()
	created, err := phoneSend(t, a, tt, updates, c, "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	id := created.Job.ID
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() == id })
	act := func(op string) (psync.Job, error) {
		return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, op, id) })
	}
	paused, err := act(psync.OpJobPause)
	if err != nil || paused.State != psync.JobPaused {
		t.Fatalf("paused %+v, %v", paused, err)
	}
	// The job behind it, the library's waiting clip, starts in its place.
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() == "clip" })
	job, _ := a.library.Find(id)
	if job.Stage != core.StageQueued || job.PausedBy != "Evan's iPhone" || job.Err != nil {
		t.Fatalf("the paused job %+v", job)
	}
	if again, err := act(psync.OpJobPause); err != nil || again.State != psync.JobPaused {
		t.Fatalf("paused again %+v, %v", again, err)
	}
	a.Select(pageFilms)
	a.prefs.open = false
	refresh(tt, a)
	if !hasText(tt, m.FilmsQueuePausedOn("Evan's iPhone")) || !tt.Exists("id:queue-continue-here") {
		t.Fatalf("the card does not say the phone paused it: %q", tt.Texts())
	}
	// Nothing starts it while it is held: the clip ends, and it still waits.
	a.Cancel("clip")
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() == "" })
	if job, _ := a.library.Find(id); job.Stage != core.StageQueued || job.PausedBy == "" {
		t.Fatalf("the held job after the clip ended %+v", job)
	}
	// It is not work left to do: the Dock badge and the status icon do not count it.
	if n, w := a.queueOutstanding(), a.trayStatus().Waiting; n != 0 || w != 0 {
		t.Fatalf("a held job counted: %d outstanding, %d waiting", n, w)
	}
	// 在这台电脑上继续 lets it go on.
	tt.Press("queue-continue-here")
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() == id })
	if job, _ := a.library.Find(id); job.PausedBy != "" {
		t.Fatalf("the job let go on still says it is paused: %+v", job)
	}
	// Paused once more, the phone lets it go on itself.
	if _, err := act(psync.OpJobPause); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() == "" })
	resumed, err := act(psync.OpJobResume)
	if err != nil || resumed.State == psync.JobPaused {
		t.Fatalf("resumed %+v, %v", resumed, err)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() == id })
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobPause, "nope") })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
}

// The read-only switch on Settings › 手机同步 makes a phone read-only from its next call: it reads the jobs and the learning state, its link and its put are refused and change nothing, and it may change things again once the switch is off. Each change of the switch is in the page's list of what happened.
func TestPhoneSyncReadOnlySwitchRefusesThePhonesWrites(t *testing.T) {
	a, p, dir, talk, c, clock := phoneStateLibrary(t)
	m := testlocale.Messages()
	lib := phoneLibraryID(dir)
	id := a.phone.devices[0].ID
	settingsOpenAt(a, p.tt, settings.SectionPhone)
	p.tt.Press("settings-phone-read-only-" + id)
	if !a.phone.devices[0].ReadOnly {
		t.Fatal("the switch did not make the phone read-only")
	}
	if !hasText(p.tt, m.SettingsPhoneDidReadOnly("Evan's iPhone")) {
		t.Fatalf("the page does not say the phone was made read-only: %q", p.tt.Texts())
	}
	h, err := phoneAsk(t, p, c.Hello)
	if err != nil || !h.ReadOnly {
		t.Fatalf("hello %+v, %v", h, err)
	}
	jobs := len(a.library.Jobs)
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.JobCreated, error) {
		return c.CreateJob(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw", psync.JobOptions{})
	})
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	clock.Add(time.Minute)
	put := []psync.Record{{Kind: psync.KindPosition, ID: "yt", Film: "yt", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Value(120000), Stamp: psync.Stamp{At: clock.Now()}}}}}
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, put) })
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	if len(a.library.Jobs) != jobs || a.player.positions[talk] != 90*time.Second {
		t.Fatalf("a read-only phone changed the desktop: %d jobs added, position %v", len(a.library.Jobs)-jobs, a.player.positions[talk])
	}
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) }); err != nil {
		t.Fatalf("a read-only phone cannot read: %v", err)
	}
	p.tt.Press("settings-phone-read-only-" + id)
	if a.phone.devices[0].ReadOnly {
		t.Fatal("the switch did not let the phone change things again")
	}
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, put) }); err != nil {
		t.Fatal(err)
	}
	if a.player.positions[talk] != 120*time.Second {
		t.Fatalf("position %v once the phone may write", a.player.positions[talk])
	}
}

// Where a phone leaves a film, the library offers to continue it here, the hero names the phone, and the page lists it; 从 m:ss 接着看 opens the film there, and × closes the offer until a phone leaves a film again.
func TestPhoneSyncOffersToContinueWhereAPhoneLeftAFilm(t *testing.T) {
	a, p, dir, talk, c, clock := phoneStateLibrary(t)
	m := testlocale.Messages()
	lib := phoneLibraryID(dir)
	a.Select(pageFilms)
	a.prefs.open = false
	refresh(p.tt, a)
	if p.tt.Exists("id:films-handoff") {
		t.Fatal("the library offers to continue a film no phone played")
	}
	leave := func(ms int64) {
		t.Helper()
		clock.Add(time.Minute)
		put := []psync.Record{{Kind: psync.KindPosition, ID: "yt", Film: "yt", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Value(ms), Stamp: psync.Stamp{At: clock.Now()}}}}}
		if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, put) }); err != nil {
			t.Fatal(err)
		}
		refresh(p.tt, a)
	}
	leave(125000)
	if !p.tt.Exists("id:films-handoff") || !hasText(p.tt, m.FilmsHandoff("Evan's iPhone", "Rust Talk", "2:05")) || !hasText(p.tt, m.FilmsHandoffResume("2:05")) {
		t.Fatalf("no offer to continue: %q", p.tt.Texts())
	}
	if !hasText(p.tt, m.FilmsHeroKickerContinueOn("Evan's iPhone")) {
		t.Fatalf("the hero does not name the phone: %q", p.tt.Texts())
	}
	if a.phone.activity[0].Kind != settings.ActivityPosition || a.phone.activity[0].Position != 125*time.Second || a.phone.activity[0].Device != "Evan's iPhone" {
		t.Fatalf("activity %+v", a.phone.activity)
	}
	p.tt.Press("handoff-close")
	refresh(p.tt, a)
	if p.tt.Exists("id:films-handoff") {
		t.Fatal("× did not close the offer")
	}
	leave(130000)
	if !p.tt.Exists("id:films-handoff") {
		t.Fatal("a film left again is not offered")
	}
	p.tt.Press("handoff-resume")
	refresh(p.tt, a)
	if a.openKey == "" || a.player.positions[talk] != 130*time.Second {
		t.Fatalf("接着看 opened %q at %v", a.openKey, a.player.positions[talk])
	}
	if p.tt.Exists("id:films-handoff") {
		t.Fatal("the offer stays once taken")
	}
}

// A conflict the phones made shows on Settings › 手机同步, where the user settles it as a phone would: the version chosen is kept on the desktop and the phones are told.
func TestPhoneSyncSettlesAConflictOnThePage(t *testing.T) {
	a, p, dir, _, c, clock := phoneStateLibrary(t)
	lib := phoneLibraryID(dir)
	got, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	if err != nil {
		t.Fatal(err)
	}
	lineID := a.saved.book.Lines[0].ID
	base := phoneStateFind(t, got.Records, psync.KindLine, lineID).Fields[psync.FieldNote].Stamp
	clock.Add(time.Minute)
	a.saved.book.SetNote(lineID, "from the desk")
	a.savedWrite()
	refresh(p.tt, a)
	clock.Add(time.Minute)
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) {
		return c.PutState(ctx, lib, []psync.Record{{Kind: psync.KindLine, ID: lineID, Fields: map[string]psync.Field{psync.FieldNote: {Value: psync.Value("from the phone"), Stamp: psync.Stamp{At: clock.Now()}, Base: &base}}}})
	}); err != nil {
		t.Fatal(err)
	}
	settingsOpenAt(a, p.tt, settings.SectionPhone)
	st := a.PhoneSync()
	if len(st.Conflicts) != 1 || st.Conflicts[0].Kept.Text != "from the phone" || st.Conflicts[0].Kept.Device != "Evan's iPhone" || st.Conflicts[0].Lost.Device != "" || !st.Conflicts[0].Both || st.Conflicts[0].Film != "Rust Talk" || st.Conflicts[0].At != time.Second {
		t.Fatalf("conflicts %+v", st.Conflicts)
	}
	if !st.Stats.Shown || st.Stats.Lines != 1 || st.Stats.Positions != 1 {
		t.Fatalf("stats %+v", st.Stats)
	}
	id := st.Conflicts[0].ID
	p.tt.Press("settings-phone-conflict-lost-" + id)
	waitUntil(t, p.tt, a, p.updates, func() bool { return len(a.PhoneSync().Conflicts) == 0 })
	if i := a.saved.book.Index(lineID); a.saved.book.Lines[i].Note != "from the desk" {
		t.Fatalf("note %q once settled on the page", a.saved.book.Lines[i].Note)
	}
	open, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	if err != nil || len(open.Conflicts) != 0 {
		t.Fatalf("the phone still has conflicts %+v, %v", open.Conflicts, err)
	}
	if l := phoneStateFind(t, open.Records, psync.KindLine, lineID); phoneStateString(t, l.Fields[psync.FieldNote]) != "from the desk" {
		t.Fatalf("the phone's line %+v", l)
	}
	if a.phone.activity[0].Kind != settings.ActivityResolved || a.phone.activity[0].Device != "" {
		t.Fatalf("activity %+v", a.phone.activity)
	}
}

// The page's list of what the phones did keeps the latest phoneActivityKept, and still shows each new one once it is full: logging a new action makes a new list, so the one the page last drew is left as it was and is seen to differ.
func TestPhoneSyncActivityShowsTheLatestOnceFull(t *testing.T) {
	a, p, _, _, _, _ := phoneStateLibrary(t)
	m := testlocale.Messages()
	settingsOpenAt(a, p.tt, settings.SectionPhone)
	for i := range phoneActivityKept + 5 {
		apply(p.tt, a, func() { a.phoneLog("", settings.ActivityLink, "clip "+strconv.Itoa(i), 0, 0) })
		if want := m.SettingsPhoneDidLink("clip " + strconv.Itoa(i)); !hasText(p.tt, want) {
			t.Fatalf("after %d actions the page does not show %q: %q", i+1, want, p.tt.Texts())
		}
	}
	if len(a.phone.activity) != phoneActivityKept || a.phone.activity[0].Title != "clip "+strconv.Itoa(phoneActivityKept+4) {
		t.Fatalf("activity kept %d, latest %+v", len(a.phone.activity), a.phone.activity[0])
	}
}

// While nothing else changes, Settings › 手机同步 follows time: a phone that stopped calling goes offline once phonesync.OnlineWithin has passed, and the relative times of what the phones did move on, without the test telling the app anything.
func TestPhoneSyncPageFollowsTimeWhileIdle(t *testing.T) {
	a, p, _, _, _, clock := phoneStateLibrary(t)
	m := testlocale.Messages()
	dev := a.phone.devices[0]
	settingsOpenAt(a, p.tt, settings.SectionPhone)
	apply(p.tt, a, func() { a.phoneLog("", settings.ActivityLink, "clip", 0, 0) })
	online := m.SettingsPhoneDeviceOnline(dev.Platform)
	if !hasText(p.tt, online) || !hasText(p.tt, m.PhoneSyncAgoNow()) {
		t.Fatalf("the phone is not online or the action not just now: %q", p.tt.Texts())
	}
	clock.Add(2 * time.Minute)
	p.tt.FastForward(2 * time.Minute)
	if !hasText(p.tt, m.PhoneSyncAgoMinutes(2)) {
		t.Fatalf("the action's time did not move on: %q", p.tt.Texts())
	}
	clock.Add(phonesync.OnlineWithin)
	p.tt.FastForward(phonesync.OnlineWithin)
	if hasText(p.tt, online) || !hasText(p.tt, m.SettingsPhoneDeviceSeen(dev.Platform, dev.LastSeen)) {
		t.Fatalf("a phone gone quiet still shows online: %q", p.tt.Texts())
	}
}
