package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/study"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// field is a record's field of value v, as the desktop changed it at now().
func field(v any) sync.Field { return sync.Field{Value: sync.Value(v)} }

// studyRecords are a word looked up in the talk and a line saved from it, both due at now(), and a word not due for a week, as the desktop's library holds them.
func studyRecords() []sync.Record {
	due := sync.Review{Due: now().Add(-time.Hour)}
	return []sync.Record{
		{Kind: sync.KindWord, ID: "w-morning", Film: "f1", Fields: map[string]sync.Field{
			sync.FieldWord: field("おはよう"), sync.FieldReading: field("ohayō"), sync.FieldMeaning: field("早上好"), sync.FieldSpoken: field("ja"),
			sync.FieldContext: field("おはようございます。"), sync.FieldContextTranslation: field("早上好。"), sync.FieldTitle: field("Morning Talk"),
			sync.FieldStartMS: field(1000), sync.FieldSavedAt: field(now().Add(-48 * time.Hour)), sync.FieldReview: field(due),
		}},
		{Kind: sync.KindWord, ID: "w-later", Film: "f1", Fields: map[string]sync.Field{
			sync.FieldWord: field("また"), sync.FieldMeaning: field("再"), sync.FieldTitle: field("Morning Talk"),
			sync.FieldSavedAt: field(now().Add(-72 * time.Hour)), sync.FieldReview: field(sync.Review{Box: 2, Due: now().Add(7 * 24 * time.Hour)}),
		}},
		{Kind: sync.KindLine, ID: "l-fine", Film: "f1", Fields: map[string]sync.Field{
			sync.FieldOriginal: field("Fine, thanks."), sync.FieldTranslation: field("很好，谢谢。"), sync.FieldNote: field("a polite answer"), sync.FieldTitle: field("Morning Talk"),
			sync.FieldStartMS: field(5000), sync.FieldEndMS: field(6500), sync.FieldSavedAt: field(now().Add(-time.Hour)), sync.FieldReview: field(due),
		}},
	}
}

// studyDesktop is newDesktop with the study records in its library.
func studyDesktop(t *testing.T) *synctest.Desktop {
	t.Helper()
	d := newDesktop(t)
	d.SetState("lib", now().Add(-time.Hour), studyRecords()...)
	return d
}

// waitLearned waits until the phone has kept the learning state of a library of computer id, as it does once it read it.
func (p *phone) waitLearned(id string) {
	p.t.Helper()
	p.app.WaitUntil(wait, func() bool {
		all, _ := store.Open(p.dir).Learnings()
		for k := range all {
			if k.Desktop == id {
				return true
			}
		}
		return false
	})
}

// reviewOf is the schedule of record kind id of the desktop's library.
func reviewOf(t *testing.T, d *synctest.Desktop, kind, id string) sync.Review {
	t.Helper()
	r, ok := d.State("lib", kind, id)
	if !ok {
		t.Fatalf("the desktop has no %s %s", kind, id)
	}
	var rv sync.Review
	if err := json.Unmarshal(r.Fields[sync.FieldReview].Value, &rv); err != nil {
		t.Fatal(err)
	}
	return rv
}

// The Learn tab reads the computer's words and lines, reviews those due a card at a time, the one not known coming back at the end, and the computer takes the new schedules.
func TestLearnReviewsTheComputersWordsAndLines(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	// The Learn tab counts what is due before it is opened.
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:bar-badge-tab-learn") && p.texts("id:bar-badge-tab-learn") == "2" })
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(2) })
	if got, want := p.texts("id:learn-vocabulary-section"), msgs.LearnWordsDue(2, 1); !strings.Contains(got, want) {
		t.Errorf("the word list's heading says %q, want %q", got, want)
	}

	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-page", wait)
	// The word was due longest, so it comes first: its answer shows once the card is turned over, and the grades wait for it.
	if got := p.texts("id:review-text"); got != "おはよう" {
		t.Fatalf("the first card shows %q", got)
	}
	if p.app.Exists("id:review-meaning") {
		t.Fatal("the meaning shows before it was asked for")
	}
	if !p.app.Find("id:review-known").Props.Disabled {
		t.Fatal("the grades can be given before the answer shows")
	}
	if got, want := p.texts("id:review-progress"), msgs.ReviewProgress(1, 2); got != want {
		t.Errorf("the review's counter says %q, want %q", got, want)
	}
	// The word was in box 0, so known it comes back tomorrow.
	if got, want := p.texts("id:review-known"), msgs.ReviewKnown(1); got != want {
		t.Errorf("the known grade says %q, want %q", got, want)
	}
	p.app.Press("id:review-show")
	if got := p.texts("id:review-meaning"); got != "早上好" {
		t.Fatalf("the answer shows %q", got)
	}
	p.app.Press("id:review-again")
	if got := p.texts("id:review-text"); got != "Fine, thanks." {
		t.Fatalf("the second card shows %q", got)
	}
	p.app.Press("id:review-show")
	if got := p.texts("id:review-note"); got != "a polite answer" {
		t.Errorf("the line's note shows %q", got)
	}
	p.app.Press("id:review-known")
	// The word not known comes back.
	if got := p.texts("id:review-text"); got != "おはよう" {
		t.Fatalf("the word not known did not come back: %q", got)
	}
	// The counter counts the items, as the summary does: the word come back keeps its number.
	if got, want := p.texts("id:review-progress"), msgs.ReviewProgress(2, 2); got != want {
		t.Errorf("the review's counter says %q on the word come back, want %q", got, want)
	}
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.WaitFor("id:review-done", wait)
	if got, want := p.texts("id:review-done-summary"), msgs.ReviewDoneSummary(2, 1); got != want {
		t.Errorf("the review's end says %q, want %q", got, want)
	}
	p.app.Press("id:review-finish")
	p.app.WaitFor("id:learn-page", wait)

	p.app.WaitUntil(wait, func() bool {
		return reviewOf(t, d, sync.KindWord, "w-morning").Box == 1 && reviewOf(t, d, sync.KindLine, "l-fine").Box == 1
	})
	if got := reviewOf(t, d, sync.KindLine, "l-fine").Due; !got.Equal(now().Add(study.Intervals[0])) {
		t.Errorf("the line is next due %v, want a day after the review", got)
	}
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDone() })
}

// A review made away from the computer is kept on the phone, and the computer takes it once it can be reached again.
func TestAReviewAwayFromTheComputerIsSentLater(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	p := startPhone(t, dir, secrets)
	p.waitLearned(store.DesktopID(d.Fingerprint))

	release := d.Hold()
	p = startPhone(t, dir, secrets)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(2) })
	p.app.Press("id:learn-review-start")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-finish")
	if got := reviewOf(t, d, sync.KindWord, "w-morning").Box; got != 0 {
		t.Fatalf("the desktop took the review while it was out of reach: box %d", got)
	}
	release()
	p.app.PullToRefresh("id:learn-scroll")
	p.app.WaitUntil(wait, func() bool { return reviewOf(t, d, sync.KindWord, "w-morning").Box == 1 })
}

// The vocabulary lists the words, the latest saved first, with their schedule; one removed, once confirmed, goes on the computer too.
func TestTheVocabularyRemovesAWordOnTheComputerToo(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.PendingAlerts())
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-word-w-morning", wait)
	if got := p.texts("id:learn-word-w-later"); !strings.Contains(got, msgs.LearnDueIn(7)) || !strings.Contains(got, "再") {
		t.Errorf("the word not due says %q", got)
	}
	if got := p.texts("id:learn-word-w-morning"); !strings.Contains(got, msgs.LearnDueNow()) {
		t.Errorf("the word due says %q", got)
	}
	// A word opens its sheet, with the line it was looked up in, which removes it.
	p.app.Press("id:learn-word-w-later")
	p.app.WaitFor("id:word-sheet", wait)
	if got := p.texts("id:word-text"); got != "また" {
		t.Errorf("the word's sheet shows %q", got)
	}
	p.app.Press("id:word-remove")
	if len(p.app.Alerts()) != 1 {
		t.Fatalf("removing a word asked %d questions, want one", len(p.app.Alerts()))
	}
	p.app.Answer(0)
	if !p.app.Exists("id:learn-word-w-later") {
		t.Fatal("a removal cancelled removed the word")
	}
	p.app.Press("id:word-remove")
	p.app.Answer(1)
	p.app.WaitForGone("id:learn-word-w-later", wait)
	p.app.WaitForGone("id:word-sheet", wait)
	p.app.WaitUntil(wait, func() bool { r, ok := d.State("lib", sync.KindWord, "w-later"); return ok && r.Removed != nil })
}

// The export writes the words, and the saved lines, as Anki's notes until another format is picked, and shares the file; what it picks it keeps for the next export.
func TestTheExportGoesThroughTheShareSheet(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-export", wait)
	p.app.Press("id:learn-export")
	p.app.WaitFor("id:export-page", wait)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:export-count") == msgs.ExportCount(2, 1) })
	if !p.app.Find("id:export-format-anki").Props.Checked {
		t.Fatal("the export does not start with Anki's notes")
	}
	p.app.Press("id:export-share")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Shares) == 1 })
	files := p.app.Shares[0].Files
	if len(files) != 1 || filepath.Base(files[0].Path) != study.VocabFile(study.VocabAnki) {
		t.Fatalf("shared %+v, want Anki's notes", files)
	}
	data, err := os.ReadFile(files[0].Path)
	if err != nil || !strings.HasPrefix(string(data), "#separator:tab") || !strings.Contains(string(data), "おはよう") || !strings.Contains(string(data), "Fine, thanks.") {
		t.Fatalf("the notes read %q, %v", data, err)
	}

	p.app.Press("id:export-format-csv")
	p.app.Press("id:export-lines-switch")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:export-count") == msgs.ExportCount(2, 0) })
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:export-share").Props.Disabled })
	p.app.Press("id:export-share")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Shares) == 2 })
	data, err = os.ReadFile(p.app.Shares[1].Files[0].Path)
	if err != nil || !strings.HasPrefix(string(data), "\ufeffWord,Reading,Meaning,Sentence,Translation,Film") || strings.Contains(string(data), "Fine, thanks.") {
		t.Fatalf("the table reads %q, %v", data, err)
	}
	// The session saves its prefs off the engine's goroutine (Session.SetPrefs), so wait for the disk to catch up.
	p.app.WaitUntil(wait, func() bool {
		prefs, err := store.Open(dir).Prefs()
		return err == nil && prefs.Export.Format == study.VocabCSV && prefs.Export.NoLines
	})
}

// Learn shows how long films played on the phone today against the daily goal, which Settings changes.
func TestTheDailyGoal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "learning"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "learning", "watched.json"), []byte(`{"`+study.Day(now())+`": 750}`), 0o600); err != nil {
		t.Fatal(err)
	}
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-goal-minutes") == "12" })
	if got, want := p.texts("id:learn-goal-text"), msgs.LearnGoalLeft(8); got != want {
		t.Errorf("the goal says %q, want %q", got, want)
	}
	if got, want := p.app.Find("id:learn-goal-bar").Props.Label, msgs.LearnGoalOf(20); got != want {
		t.Errorf("the goal's bar is named %q, want %q", got, want)
	}
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-goal")
	p.app.Press("id:settings-goal-10")
	p.openTab(learnTab, msgs.LearnTitle())
	if got, want := p.texts("id:learn-goal-text"), msgs.LearnGoalMet(); got != want {
		t.Errorf("with a goal of 10 minutes the goal says %q, want %q", got, want)
	}
	// The goal met says it is today's once, with no heading over it that says so again.
	if got := strings.Split(p.texts("id:learn-goal"), "\n"); len(got) != 4 || got[0] != "12" || got[2] != msgs.LearnGoalMet() {
		t.Errorf("the goal's card reads %q, want the minutes, the goal, the goal met and the streak", got)
	}
	if got, want := p.app.Find("id:learn-goal-bar").Props.Label, msgs.LearnGoalOf(10); got != want {
		t.Errorf("with a goal of 10 minutes the goal's bar is named %q, want %q", got, want)
	}
}

// The language picked on the phone is the one the computer translates into, and the computer's own choice comes to the phone.
func TestTheTargetLanguageIsTheComputers(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-target")
	p.app.Press("id:settings-target-ja")
	p.app.WaitUntil(wait, func() bool { return d.Target() == "ja" })

	d.SetTarget("ko")
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.PullToRefresh("id:learn-scroll")
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-target-ko").Props.Checked })
}

// A computer that no longer takes the phone shows the page that says so, with what the phone keeps; Not Now leaves it.
func TestARevokedComputerShowsItsPage(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.waitLearned(id)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	d.RevokeAll()
	p.app.PullToRefresh("id:learn-scroll")
	p.app.WaitFor("id:revoked-page", wait)
	if got, want := p.texts("id:revoked-title"), msgs.RevokedTitle("Studio"); got != want {
		t.Errorf("the page says %q, want %q", got, want)
	}
	p.app.Press("id:revoked-later")
	p.app.WaitForGone("id:revoked-page", wait)
	// What the phone kept stays for review away from the computer.
	p.app.WaitFor("id:learn-review-start", wait)
}

// The subtitle style's choices show in its preview and stay with the phone.
func TestTheSubtitleStyle(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-style")
	p.app.WaitFor("id:style-preview", wait)
	before := p.app.Find("id:style-preview-original").TextStyle.Font.Size
	p.app.Press("id:style-size-huge")
	p.app.Press("id:style-color-cyan")
	p.app.Press("id:style-position-top")
	if after := p.app.Find("id:style-preview-original").TextStyle.Font.Size; after <= before {
		t.Errorf("the preview's original is %v after Extra Large, %v before", after, before)
	}
	if !p.app.Find("id:style-color-cyan").Props.Checked || !p.app.Find("id:style-position-top").Props.Checked {
		t.Error("the choices are not checked")
	}
	p.app.WaitUntil(wait, func() bool {
		prefs, err := store.Open(dir).Prefs()
		return err == nil && prefs.Subtitles == store.SubStyle{Size: store.SubHuge, Color: store.SubCyan, Top: true}
	})
}

// Turned on, the reminder asks to show notifications and schedules one for the evening something is due.
func TestTheReviewReminder(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Environment(core.Environment{Locale: lang, ScheduledNotifications: true}))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-remind-switch")
	p.app.WaitUntil(wait, func() bool { return len(p.app.PendingNotifications) == 1 })
	n := p.app.PendingNotifications[0]
	if n.Title != msgs.LearnReminderTitle() || n.At.Hour() != 21 {
		t.Errorf("the reminder %+v", n)
	}
	p.app.Press("id:settings-remind-switch")
	p.app.WaitUntil(wait, func() bool { return len(p.app.PendingNotifications) == 0 })
}

// The language picked in the introduction is the one the computer paired there translates into.
func TestTheIntroductionsLanguageGoesToTheComputer(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	p.pickTarget("ko")
	p.app.Press("id:onboarding-continue")
	p.chooseWay("computer")
	p.app.WaitFor("id:onboarding-pairing", wait)
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-fingerprint", wait)
	p.app.Press("id:pair-confirm")
	p.finishPairing(d)
	p.app.WaitUntil(wait, func() bool { return d.Target() == "ko" })
}

// pairedWithBoth keeps d and e in dir as paired, pairing over the protocol, e as "laptop": the test desktops share a certificate, so the ID from it would name both. It returns the option that starts the phone with both tokens in its secure store.
func pairedWithBoth(t *testing.T, d, e *synctest.Desktop, dir string) nsgotest.Option {
	t.Helper()
	var list []store.Desktop
	secrets := map[string]string{}
	for i, x := range []*synctest.Desktop{d, e} {
		c := sync.NewClient(x.Addr, x.Fingerprint, "")
		resp, err := c.Pair(context.Background(), sync.PairRequest{Code: "123456", Device: "Test Phone", Platform: sync.PlatformIOS})
		c.Close()
		if err != nil {
			t.Fatal(err)
		}
		id := store.DesktopID(x.Fingerprint)
		if i == 1 {
			id = "laptop"
		}
		list = append(list, store.Desktop{ID: id, Name: resp.Desktop, Addr: x.Addr, Fingerprint: x.Fingerprint, DeviceID: resp.DeviceID, Paired: now()})
		secrets[store.TokenAccount(id)] = resp.Token
	}
	if err := store.Open(dir).SaveDesktops(list); err != nil {
		t.Fatal(err)
	}
	return nsgotest.Secrets(secrets)
}

// removeWord removes the word of record id from Learn's word list through its sheet, confirming it, and waits until it is gone.
func (p *phone) removeWord(id string) {
	p.t.Helper()
	p.app.Press("id:learn-word-" + id)
	p.app.WaitFor("id:word-remove", wait)
	p.app.Press("id:word-remove")
	p.app.Answer(1)
	p.app.WaitForGone("id:learn-word-"+id, wait)
}

// learnRefresh pulls the Learn tab to sync and waits until the sync is done.
func (p *phone) learnRefresh() {
	p.t.Helper()
	p.app.PullToRefresh("id:learn-scroll")
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:learn-scroll").Scroll.Refreshing })
}

// The language picked on the phone goes to each computer, one asleep as it is picked once it wakes, rather than the phone taking that computer's; one a computer changes then goes to the phone and to the others.
func TestTheTargetLanguageGoesToEveryComputer(t *testing.T) {
	d, e := newDesktop(t), newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWithBoth(t, d, e, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.waitLearned("laptop")
	e.SetOff(true)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-target")
	p.app.Press("id:settings-target-ja")
	p.app.WaitUntil(wait, func() bool { return d.Target() == "ja" })
	p.openTab(learnTab, msgs.LearnTitle())
	p.learnRefresh()

	e.SetOff(false)
	p.learnRefresh()
	p.app.WaitUntil(wait, func() bool { return e.Target() == "ja" })
	p.learnRefresh()
	if d.Target() != "ja" || e.Target() != "ja" {
		t.Fatalf("the computers translate into %q and %q, want ja", d.Target(), e.Target())
	}

	e.SetTarget("ko")
	p.learnRefresh()
	p.app.WaitUntil(wait, func() bool { return d.Target() == "ko" })
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-target-ko").Props.Checked })
}

// Paired again from the page that says a computer no longer takes the phone, the computer shows the page again when it removes the phone again.
func TestAComputerPairedAgainShowsItsPageWhenItRevokesAgain(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	d.RevokeAll()
	p.app.PullToRefresh("id:learn-scroll")
	p.app.WaitFor("id:revoked-page", wait)

	d.Offer("123456")
	p.app.Press("id:revoked-pair")
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-confirm", wait)
	p.app.Press("id:pair-confirm")
	p.app.WaitForGone("id:pair-confirm", wait)
	p.app.WaitForGone("id:revoked-page", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.learnRefresh()

	d.RevokeAll()
	p.app.PullToRefresh("id:learn-scroll")
	p.app.WaitFor("id:revoked-page", wait)
}

// A review made while its library is locked on the computer, which then does not list it, stays on the phone, and the computer takes it once the library is unlocked.
func TestAReviewOfALockedLibraryIsSentOnceItIsUnlocked(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	d.RemoveLibrary("lib")
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-start")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-finish")
	p.app.WaitFor("id:learn-page", wait)
	p.learnRefresh()
	if !p.app.Exists("id:learn-words") {
		t.Fatal("the library's words went from the phone, with the review the computer did not take")
	}

	d.AddLibrary("lib", "Japanese Class")
	p.learnRefresh()
	p.app.WaitUntil(wait, func() bool {
		return reviewOf(t, d, sync.KindWord, "w-morning").Box == 1 && reviewOf(t, d, sync.KindLine, "l-fine").Box == 1
	})
}

// A change the computer refuses goes, and the other changes put with it are kept.
func TestARefusedChangeLeavesTheOthers(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	d.RefuseState(func(r sync.Record) bool { return r.Kind == sync.KindLine && r.ID == "l-fine" })
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-start")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-finish")
	p.app.WaitUntil(wait, func() bool { return reviewOf(t, d, sync.KindWord, "w-morning").Box == 1 })
	if got := reviewOf(t, d, sync.KindLine, "l-fine").Box; got != 0 {
		t.Fatalf("the desktop took the line it refuses: box %d", got)
	}
	// The refused line goes from what the phone has to send: read afresh, it shows due again, as the computer has it.
	p.learnRefresh()
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(1) })
}

// Dictation plays each line due from the film kept on the phone, from just before it starts to just after it ends, and marks what the learner typed once they check it; the grade they give then goes to the computer, as a card's does.
func TestTheReviewDictatesTheLinesOfAFilmOnThePhone(t *testing.T) {
	d := studyDesktop(t)
	dir, f := keptFrom(t, d)
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Environment(core.Environment{Locale: lang, Audio: true}))
	video := filepath.Join(store.Open(dir).FilmDir(f.Desktop, f.Film.ID), f.Video)
	p.app.AudioDurations = map[string]time.Duration{video: 10 * time.Second}
	p.waitLearned(f.Desktop)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(2) })
	p.app.Press("id:learn-review-dictation")
	p.app.Press("id:learn-review-start")

	// The word comes first: its line is the one it was looked up in, which the film's subtitles end at two seconds.
	p.app.WaitFor("id:dictation-card", wait)
	p.app.Press("id:dictation-play")
	if len(p.app.Players) != 1 || !p.app.Players[0].Playing || p.app.Players[0].Position != 750*time.Millisecond || p.app.Players[0].Source.URI != video {
		t.Fatalf("the players %+v; want the film's from 0.75s", p.app.Players)
	}
	p.app.Advance(2 * time.Second)
	if pl := p.app.Players[0]; pl.Playing || pl.Position != 750*time.Millisecond {
		t.Fatalf("the line's clip played on to %v (playing %v); want it stopped after 2.25s and back at its start", pl.Position, pl.Playing)
	}
	p.app.Type("id:dictation-input", "おはよう")
	p.app.Press("id:dictation-check")
	if got, want := p.texts("id:dictation-score"), msgs.DictationScore(4, 9); got != want {
		t.Errorf("the check says %q, want %q", got, want)
	}
	if got := p.texts("id:dictation-translation"); got != "早上好。" {
		t.Errorf("the line's translation shows %q", got)
	}
	p.app.Press("id:review-known")

	p.app.WaitUntil(wait, func() bool { return p.texts("id:dictation-input") == "" })
	p.app.Type("id:dictation-input", "fine THANKS")
	p.app.Press("id:dictation-check")
	if got, want := p.texts("id:dictation-score"), msgs.DictationScore(2, 2); got != want {
		t.Errorf("the check says %q, want %q", got, want)
	}
	p.app.Press("id:review-known")
	p.app.WaitFor("id:review-done", wait)
	p.app.Press("id:review-finish")
	p.app.WaitUntil(wait, func() bool {
		return reviewOf(t, d, sync.KindWord, "w-morning").Box == 1 && reviewOf(t, d, sync.KindLine, "l-fine").Box == 1
	})
	if prefs, err := store.Open(dir).Prefs(); err != nil || prefs.Review != "dictation" {
		t.Errorf("the review mode kept is %q, %v", prefs.Review, err)
	}
}

// A line whose film is not on the phone cannot be heard: dictation shows it as a card, and says why.
func TestDictationShowsACardForAFilmNotOnThePhone(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-dictation")
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-no-film", wait)
	if p.app.Exists("id:dictation-card") || p.texts("id:review-text") != "おはよう" || p.app.Find("id:review-show").Props.Label != msgs.ReviewShow() {
		t.Fatalf("a line not on the phone shows %q", p.texts("id:review-page"))
	}
}

// Learn's minutes and streak are the learner's on every device: the computer's time watched of its libraries adds to the phone's, and the phone's own time in a film goes to the film's library on the computer at the next sync.
func TestTheTimeWatchedIsEveryDevices(t *testing.T) {
	d := studyDesktop(t)
	day := study.Day(now())
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindWatched, ID: sync.WatchedID("studio-device", day), Fields: map[string]sync.Field{
		sync.FieldWatchedMS: {Value: sync.Value(10 * 60_000), Stamp: sync.Stamp{At: now().Add(-time.Hour), Device: "studio-device"}}}})
	dir, f := keptFrom(t, d)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(f.Desktop)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-goal-minutes") == "10" })
	if got, want := p.texts("id:learn-week-total"), "10\n"+msgs.LearnMinutesUnit()+"\n"+msgs.LearnStudyTime(); got != want {
		t.Errorf("the week's study time says %q, want %q", got, want)
	}
	if got, want := p.texts("id:learn-week-words"), "2\n"+msgs.LearnWordsUnit(2)+"\n"+msgs.LearnNewWords(); got != want {
		t.Errorf("the week's new words say %q, want %q", got, want)
	}

	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * time.Second, Size: core.Size{Width: 1920, Height: 1080}})
	for at := 500 * time.Millisecond; at <= 6*time.Second; at += 500 * time.Millisecond {
		p.at(at)
	}
	// Back minimizes the player, which keeps where the film is and its time watched.
	p.app.Back()
	p.app.WaitFor("id:player-mini", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.PullToRefresh("id:learn-scroll")
	desktops, err := store.Open(dir).Desktops()
	if err != nil || len(desktops) != 1 {
		t.Fatalf("desktops %+v, %v", desktops, err)
	}
	p.app.WaitUntil(wait, func() bool {
		r, ok := d.State("lib", sync.KindWatched, sync.WatchedID(desktops[0].DeviceID, day))
		if !ok {
			return false
		}
		var ms float64
		return json.Unmarshal(r.Fields[sync.FieldWatchedMS].Value, &ms) == nil && ms >= 5000
	})
}

// A film played for the first time before its computer has answered which of its libraries holds it still gives its time to the computer: the phone keeps the time for the library and puts it there once the computer answers.
func TestTheTimeWatchedBeforeTheLibraryIsKnownReachesTheComputer(t *testing.T) {
	d := studyDesktop(t)
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindWatched, ID: sync.WatchedID("studio-device", study.Day(now())), Fields: map[string]sync.Field{
		sync.FieldWatchedMS: {Value: sync.Value(10 * 60_000), Stamp: sync.Stamp{At: now().Add(-time.Hour), Device: "studio-device"}}}})
	dir, f := keptFrom(t, d)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(f.Desktop)
	release := d.Hold()
	t.Cleanup(release)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	// A film on the phone plays from its row.
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * time.Second, Size: core.Size{Width: 1920, Height: 1080}})
	for at := 500 * time.Millisecond; at <= 6*time.Second; at += 500 * time.Millisecond {
		p.at(at)
	}
	p.app.Back()
	p.app.WaitFor("id:player-mini", wait)
	if films, err := store.Open(dir).Films(); err != nil || len(films) != 1 || films[0].LibraryID != "" {
		t.Fatalf("the film's library is known before the computer answered: %+v, %v", films, err)
	}
	release()
	desktops, err := store.Open(dir).Desktops()
	if err != nil || len(desktops) != 1 {
		t.Fatalf("desktops %+v, %v", desktops, err)
	}
	p.app.WaitUntil(wait, func() bool {
		r, ok := d.State("lib", sync.KindWatched, sync.WatchedID(desktops[0].DeviceID, study.Day(now())))
		if !ok {
			return false
		}
		var ms float64
		return json.Unmarshal(r.Fields[sync.FieldWatchedMS].Value, &ms) == nil && ms >= 6000
	})
}

// A film watched while the computer is out of reach, before any sync reached it this run, still gives its time to the computer: the library's state, which holds the computer's time, says the computer takes it, and the next sync puts it.
func TestTheTimeWatchedOfflineReachesTheComputer(t *testing.T) {
	d := studyDesktop(t)
	day := study.Day(now())
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindWatched, ID: sync.WatchedID("studio-device", day), Fields: map[string]sync.Field{
		sync.FieldWatchedMS: {Value: sync.Value(10 * 60_000), Stamp: sync.Stamp{At: now().Add(-time.Hour), Device: "studio-device"}}}})
	dir, f := keptFrom(t, d)
	secrets := pairedWith(t, d, dir)
	t.Run("learn", func(t *testing.T) {
		p := startPhone(t, dir, secrets)
		p.waitLearned(f.Desktop)
		p.app.WaitFor(talkRow(f.Desktop), wait)
		p.app.Press(talkRow(f.Desktop))
		p.app.WaitFor("id:player-video", wait)
		p.app.WaitUntil(wait, func() bool {
			films, _ := store.Open(dir).Films()
			return len(films) == 1 && films[0].LibraryID == "lib"
		})
	})

	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * time.Second, Size: core.Size{Width: 1920, Height: 1080}})
	for at := 500 * time.Millisecond; at <= 6*time.Second; at += 500 * time.Millisecond {
		p.at(at)
	}
	// Back minimizes the player, which keeps where the film is and its time watched.
	p.app.Back()
	p.app.WaitFor("id:player-mini", wait)
	desktops, err := store.Open(dir).Desktops()
	if err != nil || len(desktops) != 1 {
		t.Fatalf("desktops %+v, %v", desktops, err)
	}
	mine := sync.WatchedID(desktops[0].DeviceID, day)
	p.app.WaitUntil(wait, func() bool {
		all, _ := store.Open(dir).Learnings()
		for _, l := range all {
			if slices.ContainsFunc(l.Pending, func(r sync.Record) bool { return r.Kind == sync.KindWatched && r.ID == mine }) {
				return true
			}
		}
		return false
	})

	d.SetOff(false)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.PullToRefresh("id:learn-scroll")
	p.app.WaitUntil(reconnect, func() bool {
		r, ok := d.State("lib", sync.KindWatched, mine)
		if !ok {
			return false
		}
		var ms float64
		return json.Unmarshal(r.Fields[sync.FieldWatchedMS].Value, &ms) == nil && ms >= 5000
	})
}

// Learn lists the words and the saved lines itself: the words by how well they are known under the week, a line with its note, which plays the film from the line when the film is on the phone, and a long press removes a line once confirmed.
func TestLearnListsTheWordsAndLinesInline(t *testing.T) {
	d := studyDesktop(t)
	dir, f := keptFrom(t, d)
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.PendingAlerts())
	p.waitLearned(f.Desktop)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-line-l-fine", wait)
	// また is in box 2, still being learned; おはよう in box 0, new.
	if got := p.texts("id:learn-mastery"); !strings.Contains(got, msgs.LearnMasteredCount(0)) || !strings.Contains(got, msgs.LearnLearningCount(1)) || !strings.Contains(got, msgs.LearnNewCount(1)) {
		t.Errorf("the words by how well they are known say %q", got)
	}
	if got := p.texts("id:learn-lines"); !strings.Contains(got, msgs.LearnLineCount(1)) || !strings.Contains(got, "Fine, thanks.") || !strings.Contains(got, "a polite answer") || !strings.Contains(got, "0:05") {
		t.Errorf("the saved lines say %q", got)
	}

	p.app.LongPress("id:learn-line-l-fine")
	if len(p.app.Alerts()) != 1 {
		t.Fatalf("a long press on a line asked %d questions, want one", len(p.app.Alerts()))
	}
	p.app.Answer(0)
	p.app.Press("id:learn-line-play-l-fine")
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(dir).Films()
		return len(films) == 1 && films[0].PositionMS == 5000
	})
}

// With words saved and none due, the review card says the review is done and when the next one is, with nothing to press; the hint of how words get in is for a word list still empty.
func TestTheReviewCardSaysWhenTheNextReviewIs(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(-time.Hour), studyRecords()[1])
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDone() })
	if got, want := p.texts("id:learn-review-detail"), msgs.LearnReviewNext(7); got != want {
		t.Errorf("the review card says %q, want %q", got, want)
	}
	for _, id := range []string{"id:learn-review-start", "id:learn-review-ok", "id:learn-pair"} {
		if p.app.Exists(id) {
			t.Errorf("the review card with nothing due has %s", id)
		}
	}
	// With no line saved, the saved lines' section says where they come from rather than showing its heading over nothing.
	if got := p.texts("id:learn-lines-empty"); got != msgs.LinesEmpty() {
		t.Errorf("the saved lines' section says %q with no line saved, want %q", got, msgs.LinesEmpty())
	}

	empty := newDesktop(t)
	dir = t.TempDir()
	p = startPhone(t, dir, pairedWith(t, empty, dir))
	p.waitLearned(store.DesktopID(empty.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewNone() })
	if got, want := p.texts("id:learn-review-detail"), msgs.LearnReviewNoneDetail(); got != want {
		t.Errorf("the review card of an empty word list says %q, want %q", got, want)
	}
	if p.app.Exists("id:learn-review-ok") {
		t.Error("the review card of an empty word list has OK")
	}
}

// While the reminder is on, Learn says when it comes under the review mode, and that leads to Settings.
func TestLearnSaysWhenTheReminderComes(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Environment(core.Environment{Locale: lang, ScheduledNotifications: true}))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review", wait)
	if p.app.Exists("id:learn-reminder") {
		t.Fatal("Learn tells of a reminder that is off")
	}
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-remind-switch")
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-reminder", wait)
	if got, want := p.texts("id:learn-reminder"), msgs.LearnReminderCaption("21:00"); got != want {
		t.Errorf("the reminder's line says %q, want %q", got, want)
	}
	p.app.Press("id:learn-reminder")
	p.app.WaitFor("id:settings-page", wait)
}

// Turned over, a review card's front is hidden from the screen reader, which reads the back alone; before, the back is.
func TestTheReviewCardHidesTheFaceTurnedAway(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-card", wait)
	if p.app.Find("id:review-show").Props.ElementsHidden || !p.app.Find("id:review-back").Props.ElementsHidden {
		t.Fatal("before it turns over, the card does not show its front alone to the screen reader")
	}
	p.app.Press("id:review-show")
	p.app.WaitFor("id:review-answer", wait)
	if !p.app.Find("id:review-show").Props.ElementsHidden || p.app.Find("id:review-back").Props.ElementsHidden {
		t.Fatal("turned over, the card does not show its back alone to the screen reader")
	}
}

// Before a review card turns over, its back, laid over the front and only turned away, lets touches through to the front: on iOS a view takes the touch whether or not it handles it, so a back that took it kept the card from ever turning. Turned over, the front lets them through to the back.
func TestTheReviewCardTakesTouchesOnTheFaceShown(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-card", wait)
	if p.app.Find("id:review-back").Props.PointerEvents != core.PointerEventsNone || p.app.Find("id:review-show").Props.PointerEvents == core.PointerEventsNone {
		t.Fatal("before it turns over, the card does not take touches on its front alone")
	}
	p.app.Press("id:review-show")
	p.app.WaitFor("id:review-answer", wait)
	if p.app.Find("id:review-back").Props.PointerEvents == core.PointerEventsNone || p.app.Find("id:review-show").Props.PointerEvents != core.PointerEventsNone {
		t.Fatal("turned over, the card does not take touches on its back alone")
	}
}

// A saved line plays its film, and a word's sheet leads back to it, only once the film is whole on the phone: one still downloading has no video to play.
func TestLearnPlaysOnlyAWholeFilm(t *testing.T) {
	dir := t.TempDir()
	learnSample(t, dir)
	st := store.Open(dir)
	films, err := st.Films()
	if err != nil || len(films) != 1 {
		t.Fatalf("the sample keeps %d films: %v", len(films), err)
	}
	f := films[0]
	f.Video = ""
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-line-l-see", wait)
	if p.app.Exists("id:learn-line-play-l-see") {
		t.Error("a line plays a film still downloading")
	}
	p.app.Press("id:learn-word-w-station")
	p.app.WaitFor("id:word-sheet", wait)
	if p.app.Exists("id:word-back-to-film") {
		t.Error("a word's sheet leads back to a film still downloading")
	}
}

// Where the platform speaks text, a word's sheet says the word in its film's language.
func TestTheWordSheetSaysTheWord(t *testing.T) {
	p := wordSheetOf(t, true)
	p.app.Press("id:word-say")
	if len(p.app.Spoken) != 1 || p.app.Spoken[0].Text != "駅" || p.app.Spoken[0].Lang != "ja" {
		t.Errorf("the speaker said %+v, want 駅 in ja", p.app.Spoken)
	}
}

// Where the platform speaks no text, a word's sheet has no speaker.
func TestTheWordSheetHasNoSpeakerWithoutSpeech(t *testing.T) {
	p := wordSheetOf(t, false)
	if p.app.Exists("id:word-say") {
		t.Error("the word's sheet offers to say the word where the platform speaks no text")
	}
}

// wordSheetOf opens the sheet of a word of the sample, on a platform that speaks text or not.
func wordSheetOf(t *testing.T, speaks bool) *phone {
	t.Helper()
	dir := t.TempDir()
	learnSample(t, dir)
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, Speak: speaks}))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-word-w-station", wait)
	p.app.Press("id:learn-word-w-station")
	p.app.WaitFor("id:word-sheet", wait)
	return p
}
