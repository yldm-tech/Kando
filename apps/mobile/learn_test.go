package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(2) })
	if got, want := p.texts("id:learn-vocabulary"), msgs.LearnWordCount(2); !strings.Contains(got, want) {
		t.Errorf("the vocabulary's row says %q, want %q", got, want)
	}

	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-page", wait)
	// The word was due longest, so it comes first: its answer shows once asked for.
	if got := p.texts("id:review-text"); got != "おはよう" {
		t.Fatalf("the first card shows %q", got)
	}
	if p.app.Exists("id:review-meaning") {
		t.Fatal("the meaning shows before it was asked for")
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
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.WaitFor("id:review-done", wait)
	if got, want := p.texts("id:review-done-again"), msgs.ReviewDoneAgain(1); got != want {
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
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewNone() })
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
	p.app.WaitFor("id:learn-vocabulary", wait)
	p.app.Press("id:learn-vocabulary")
	p.app.WaitFor("id:vocabulary-item-w-morning", wait)
	if got := p.texts("id:vocabulary-item-w-later"); !strings.Contains(got, msgs.LearnDueIn(7)) || !strings.Contains(got, "再") {
		t.Errorf("the word not due says %q", got)
	}
	p.app.Press("id:vocabulary-remove-w-later")
	if len(p.app.Alerts()) != 1 {
		t.Fatalf("removing a word asked %d questions, want one", len(p.app.Alerts()))
	}
	p.app.Answer(0)
	if !p.app.Exists("id:vocabulary-item-w-later") {
		t.Fatal("a removal cancelled removed the word")
	}
	p.app.Press("id:vocabulary-remove-w-later")
	p.app.Answer(1)
	p.app.WaitForGone("id:vocabulary-item-w-later", wait)
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
	prefs, err := store.Open(dir).Prefs()
	if err != nil || prefs.Export.Format != study.VocabCSV || !prefs.Export.NoLines {
		t.Fatalf("kept %+v, %v; want the CSV without the lines", prefs.Export, err)
	}
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
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-goal-minutes") == msgs.LearnMinutes(12) })
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
	p.app.Press("id:onboarding-target-ko")
	p.app.Press("id:onboarding-continue")
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
	if !p.app.Exists("id:learn-vocabulary") {
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
