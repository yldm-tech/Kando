package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// awayFrom turns d off on its network and waits, on the Jobs page, until the phone follows it through its storage, then leaves the page for the Library.
func (p *phone) awayFrom(d *synctest.Desktop) {
	p.t.Helper()
	id := store.DesktopID(d.Fingerprint)
	d.SetOff(true)
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.openJobs()
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsViaStorage() })
	p.app.HeaderBack("id:jobs-page")
	p.app.WaitForGone("id:jobs-page", wait)
}

// relayTurns lets a few turns of the phone's follower and the computer's relay go by, for a test that checks that something did not happen.
func relayTurns() { time.Sleep(10 * relayEvery) }

// Away from the computer's network, the learning state goes through its storage both ways: a word the computer saves shows in Learn, a word removed on the phone goes on the computer, and the language picked on the phone is the computer's too. A phone the computer made read-only puts nothing there and still takes the computer's changes; let write again, its change goes.
func TestAwayFromTheComputerTheLearningStateGoesThroughItsStorage(t *testing.T) {
	d := studyDesktop(t)
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.PendingAlerts())
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.awayFrom(d)

	d.SetState("lib", now(), sync.Record{Kind: sync.KindWord, ID: "w-away", Film: "f1", Fields: map[string]sync.Field{
		sync.FieldWord: field("さようなら"), sync.FieldMeaning: field("再见"), sync.FieldTitle: field("Morning Talk"),
		sync.FieldSavedAt: field(now()), sync.FieldReview: field(sync.Review{Due: now().Add(24 * time.Hour)}),
	}})
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:learn-vocabulary"), msgs.LearnWordCount(3)) })

	p.app.Press("id:learn-vocabulary")
	p.app.WaitFor("id:vocabulary-item-w-later", wait)
	p.app.Press("id:vocabulary-remove-w-later")
	p.app.Answer(1)
	p.app.WaitForGone("id:vocabulary-item-w-later", wait)
	p.app.WaitUntil(wait, func() bool { r, ok := d.State("lib", sync.KindWord, "w-later"); return ok && r.Removed != nil })

	// Read-only, the phone's removal waits on it, while the computer's changes still come.
	d.SetReadOnly(true)
	relayTurns()
	p.app.Press("id:vocabulary-remove-w-away")
	p.app.Answer(1)
	p.app.WaitForGone("id:vocabulary-item-w-away", wait)
	d.SetState("lib", now(), sync.Record{Kind: sync.KindWord, ID: "w-morning", Fields: map[string]sync.Field{sync.FieldMeaning: field("早安")}})
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:vocabulary-item-w-morning"), "早安") })
	relayTurns()
	if r, _ := d.State("lib", sync.KindWord, "w-away"); r.Removed != nil {
		t.Fatal("the computer took a change of a read-only phone through its storage")
	}
	d.SetReadOnly(false)
	p.app.WaitUntil(wait, func() bool { r, ok := d.State("lib", sync.KindWord, "w-away"); return ok && r.Removed != nil })

	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-target")
	p.app.WaitFor("id:settings-target-page", wait)
	p.app.Press("id:settings-target-ja")
	p.app.WaitUntil(wait, func() bool { return d.Target() == "ja" })
}

// Away from the computer's network, a sync conflict is settled through its storage, and the computer's own setting changes there too.
func TestAwayFromTheComputerConflictsAndItsSettingsGoThroughItsStorage(t *testing.T) {
	d, key := conflictDesktop(t)
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.waitLearned(id)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.awayFrom(d)

	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-conflicts")
	p.app.WaitFor("id:conflict-both-"+key, wait)
	p.app.Press("id:conflict-both-" + key)
	p.app.WaitFor("id:conflicts-done", wait)
	r, _ := d.State("lib", sync.KindLine, "l-fine")
	if got := fieldText(r, sync.FieldNote); got != "said to a friend\na polite answer" {
		t.Fatalf("the computer's note is %q", got)
	}
	if p.app.Exists("id:conflicts-failure") {
		t.Fatalf("the resolution failed: %s", p.texts("id:conflicts-failure"))
	}
	p.app.HeaderBack("id:conflicts-page")

	p.app.Press("id:settings-computer-" + id)
	p.app.WaitFor("id:computer-page", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:computer-context-switch").Props.Checked })
	p.app.Press("id:computer-context-switch")
	p.app.WaitUntil(wait, func() bool { return !d.Context() })
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:computer-context-switch").Props.Checked })
}

// Away from the computer's network, the subscriptions are read and changed through its storage: the auto-download is turned on there, and a channel's link subscribes the computer to it.
func TestAwayFromTheComputerTheSubscriptionsGoThroughItsStorage(t *testing.T) {
	d := newDesktop(t)
	d.AddSubscription(earthScience())
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	mark := "id:films-subscription-" + id + "-s1"
	p.app.WaitFor(mark, wait)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.awayFrom(d)

	p.app.Press(mark)
	p.app.WaitFor("id:films-subscription-auto-switch", wait)
	p.app.Press("id:films-subscription-auto-switch")
	p.app.WaitUntil(wait, func() bool { subs := d.Subscriptions(); return len(subs) == 1 && subs[0].Auto })
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:films-subscription-auto-switch").Props.Checked })
	p.app.Press("id:films-subscription-take")
	p.app.WaitUntil(wait, func() bool { return len(d.Jobs()) == 2 })
	p.app.WaitFor("id:films-subscription-none", wait)
	p.app.Back()
	p.app.WaitForGone("id:films-subscription-sheet", wait)

	p.app.Press("id:films-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://www.youtube.com/@oceanscience")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:add-note") == msgs.AddSubscribeNote("Studio") })
	p.app.Press("id:add-send")
	p.app.WaitUntil(wait, func() bool { return len(d.Subscriptions()) == 2 })
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.AddSubscribed("https://www.youtube.com/@oceanscience", "Studio"))
	})
}

// When the storage refuses the phone away from the computer's network, the computer's card says why, in the words the computer uses for the same failure, rather than only that it is offline: a key the storage no longer takes has the phone join again on the computer's network, a bucket that is gone is tried again until it is back.
func TestTheJobsPageSaysWhyTheStorageFailed(t *testing.T) {
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.awayFrom(d)
	p.openJobs()
	summary := func() string { return p.texts("id:jobs-summary-" + id) }

	bucket.Refuse(http.StatusForbidden, "SignatureDoesNotMatch")
	p.app.WaitUntil(wait, func() bool { return summary() == msgs.JobsStorageKeys("Studio") })
	if got := p.reach(id); got != msgs.JobsOffline() {
		t.Errorf("the card says the computer is %q", got)
	}
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	if got := p.texts("id:add-note"); got != msgs.AddNoteOffline("Studio") {
		t.Errorf("the Add sheet says %q while the storage refuses the phone", got)
	}
	p.app.Press("id:add-cancel")
	// The links stay refused once the storage takes the key again: they were signed with the old one, and the phone joins again on the computer's network.
	bucket.Refuse(0, "")
	relayTurns()
	if got := summary(); got != msgs.JobsStorageKeys("Studio") {
		t.Fatalf("with its links refused, the card says %q", got)
	}
	d.SetOff(false)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsOnline() })
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 2 })

	d.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsViaStorage() })
	bucket.Refuse(http.StatusNotFound, "NoSuchBucket")
	p.app.WaitUntil(wait, func() bool { return summary() == msgs.JobsStorageNoBucket("Studio") })
	bucket.Refuse(0, "")
	p.app.WaitUntil(wait, func() bool {
		return p.reach(id) == msgs.JobsViaStorage() && summary() == msgs.JobsRelayed(msgs.JobsIdle())
	})
}
