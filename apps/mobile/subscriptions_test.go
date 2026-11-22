package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/study"
	"kando/packages/sync"
)

// earthScience is a channel the desktop follows for the library of the talk, with two new videos waiting for the user.
func earthScience() sync.Subscription {
	return sync.Subscription{ID: "s1", Library: "lib", Name: "Earth Science", Source: "https://www.youtube.com/@earthscience", Kind: sync.SubscriptionChannel, Platform: sync.PlatformYouTube, AddPerCheck: 3,
		Waiting: []sync.SubscriptionVideo{{URL: "https://www.youtube.com/watch?v=lllllllllll", Title: "How Rivers Shape Landscapes", DurationMS: 720_000}, {URL: "https://www.youtube.com/watch?v=sssssssssss", Title: "Tides in 40 Seconds", DurationMS: 40_000}}}
}

// The Library starts with the day's study: the minutes played on the phone today, the days in a row and the words of the computers' vocabularies.
func TestTheLibraryShowsTheDaysStudy(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "learning"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "learning", "watched.json"), []byte(`{"`+study.Day(now())+`": 750, "`+study.Day(now().AddDate(0, 0, -1))+`": 600}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d := studyDesktop(t)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-stats", wait)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:films-stat-words-value") == "2\n"+msgs.FilmsStatUnitWords(2) })
	if got, want := p.texts("id:films-stat-today-value"), "12\n"+msgs.FilmsStatUnitMinutes(12); got != want {
		t.Errorf("today's minutes read %q, want %q", got, want)
	}
	if got, want := p.texts("id:films-stat-streak-value"), "2\n"+msgs.FilmsStatUnitDays(2); got != want {
		t.Errorf("the streak reads %q, want %q", got, want)
	}
	if got, want := p.app.Find("id:films-stat-today").Props.Label, msgs.FilmsStatToday()+", "+msgs.LearnMinutes(12); got != want {
		t.Errorf("today's tile is read as %q, want %q", got, want)
	}
}

// The Library lists the computer's subscriptions with the count of their new videos; a subscription's sheet sends them all to the computer's queue, turns its auto-download on, and unsubscribes after asking, which the computer does too.
func TestTheLibraryListsTheComputersSubscriptions(t *testing.T) {
	d := newDesktop(t)
	d.AddSubscription(earthScience())
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	mark := "id:films-subscription-" + id + "-s1"
	p.app.WaitFor(mark, wait)
	if got := p.texts("id:films-subscription-new-" + id + "-s1"); got != msgs.FilmsSubscriptionNew(2) {
		t.Fatalf("the badge reads %q", got)
	}
	if got, want := p.app.Find(mark).Props.Label, msgs.FilmsSubscriptionLabel("Earth Science", 2); got != want {
		t.Fatalf("the subscription is read as %q, want %q", got, want)
	}
	p.app.Press(mark)
	p.app.WaitFor("id:films-subscription-sheet", wait)
	if got := p.texts("id:films-subscription-waiting"); got != msgs.FilmsSubscriptionWaiting(2) {
		t.Fatalf("the sheet heads its videos %q", got)
	}
	p.app.Find("id:films-subscription-sheet >> text:How Rivers Shape Landscapes")
	p.app.Find("id:films-subscription-sheet >> text:" + msgs.FilmsSubscriptionMeta("YouTube", msgs.FilmsSubscriptionChannel(), "Studio"))

	// The switch waits for the computer's queue to be followed, as it needs to know the computer lets the phone change it.
	p.app.WaitFor("id:films-subscription-auto-switch", wait)
	p.app.Press("id:films-subscription-auto-switch")
	p.app.WaitUntil(wait, func() bool { subs := d.Subscriptions(); return len(subs) == 1 && subs[0].Auto })
	// The computer makes the change before the phone reads its answer, and the subscription takes no other action until then.
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:films-subscription-take").Props.Disabled })

	p.app.Press("id:films-subscription-take")
	p.app.WaitUntil(wait, func() bool { return len(d.Jobs()) == 2 })
	if jobs := d.Jobs(); jobs[0].Link != earthScience().Waiting[0].URL || jobs[1].Link != earthScience().Waiting[1].URL {
		t.Fatalf("the computer's queue is %+v", jobs)
	}
	p.app.WaitFor("id:films-subscription-none", wait)
	if got := p.texts("id:films-subscription-none"); got != msgs.FilmsSubscriptionNoneAuto() {
		t.Fatalf("with nothing waiting the sheet says %q", got)
	}
	p.app.WaitForGone("id:films-subscription-new-"+id+"-s1", wait)

	p.app.Press("id:films-subscription-unsubscribe")
	if got := p.app.Alerts(); len(got) != 1 || got[0].Title != msgs.FilmsUnsubscribeTitle("Earth Science") {
		t.Fatalf("unsubscribing asks %+v", got)
	}
	p.app.Answer(1)
	p.app.WaitUntil(wait, func() bool { return len(d.Subscriptions()) == 0 })
	// The row leaves as the phone reads the computer's answer, which also starts the sheet's motion out.
	p.app.WaitForGone("id:films-subscriptions", wait)
	p.app.WaitForGone("id:films-subscription-sheet", wait)
}

// A phone the computer made read-only sees the subscriptions without the actions that change them.
func TestAReadOnlyPhoneOnlyLooksAtTheSubscriptions(t *testing.T) {
	d := newDesktop(t)
	d.AddSubscription(earthScience())
	d.SetReadOnly(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor("id:films-subscription-"+id+"-s1", wait)
	p.app.Press("id:films-subscription-" + id + "-s1")
	p.app.WaitFor("id:films-subscription-read-only", wait)
	if got := p.texts("id:films-subscription-read-only"); got != msgs.FilmsSubscriptionReadOnly("Studio") {
		t.Fatalf("the sheet says %q", got)
	}
	for _, action := range []string{"id:films-subscription-take", "id:films-subscription-auto-switch", "id:films-subscription-unsubscribe"} {
		if p.app.Exists(action) {
			t.Errorf("a read-only phone is offered %s", action)
		}
	}
}

// A channel's link in the Add sheet, marked as one in its field, subscribes the computer to it rather than queueing it, and the toast says so; a computer that does not share its subscriptions takes it as a link.
func TestTheAddSheetSubscribesToAChannel(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-add", wait)
	p.app.Press("id:films-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://www.youtube.com/@earthscience")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:add-note") == msgs.AddSubscribeNote("Studio") })
	if got := p.texts("id:add-site"); got != msgs.AddChannel() {
		t.Fatalf("the link's chip reads %q", got)
	}
	p.app.Press("id:add-send")
	p.app.WaitUntil(wait, func() bool { return len(d.Subscriptions()) == 1 })
	if subs := d.Subscriptions(); subs[0].Source != "https://www.youtube.com/@earthscience" || len(d.Jobs()) != 0 {
		t.Fatalf("the computer's subscriptions are %+v, its jobs %+v", subs, d.Jobs())
	}
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.AddSubscribed("https://www.youtube.com/@earthscience", "Studio"))
	})
	p.app.WaitFor("id:films-subscription-"+store.DesktopID(d.Fingerprint)+"-sub1", wait)
}

// An older computer that does not share its subscriptions lists none, and a channel's link goes to it as a link.
func TestAComputerWithoutSubscriptionsTakesAChannelAsALink(t *testing.T) {
	d := newDesktop(t)
	d.AddSubscription(earthScience())
	d.SetNoSubscriptions(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.sendLink("https://www.youtube.com/@earthscience")
	// The Jobs page shows as the link is sent, before the computer has taken it.
	p.app.WaitUntil(wait, func() bool { return len(d.Jobs()) > 0 })
	if jobs := d.Jobs(); len(jobs) != 1 || jobs[0].Link != "https://www.youtube.com/@earthscience" {
		t.Fatalf("the computer's queue is %+v", jobs)
	}
	if p.app.Exists("id:films-subscriptions") || slices.ContainsFunc(d.Subscriptions(), func(s sync.Subscription) bool { return s.ID != "s1" }) {
		t.Fatal("a computer without subscriptions shows them or was subscribed")
	}
}
