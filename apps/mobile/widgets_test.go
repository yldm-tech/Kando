package main

import (
	"testing"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/store"
	"kando/apps/mobile/widgets"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// On an iPhone Settings › Widgets says how to add them over what each shows now: the streak of the days films played, the film left partway with the line it was left on, the two together and what is due on the Lock Screen.
func TestSettingsShowsTheWidgetsAsTheyAre(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	dir := t.TempDir()
	f := keepTalk(t, dir)
	f.PositionMS, f.PositionAt = 7500, now().Add(-time.Hour)
	if err := store.Open(dir).SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	watched := learning.Watched{}
	watched.Add(now(), 20*time.Minute)
	watched.Add(now().AddDate(0, 0, -1), 20*time.Minute)
	watched.Add(now().AddDate(0, 0, -2), 20*time.Minute)
	if err := learning.Open(dir).SaveWatched(watched); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS}))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-widgets", wait)
	p.app.Press("id:settings-widgets")
	p.app.WaitFor("id:settings-widgets-page", wait)
	if got, want := p.app.Find("id:settings-widgets-how").Text, msgs.SettingsWidgetsHow(msgs.AppName()); got != want {
		t.Fatalf("the page says %q, want %q", got, want)
	}
	// The time watched is read off disk as the phone starts, and the widgets count the streak once it is.
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-widget-streak-days").Text == "3" })
	if got := p.app.Find("id:settings-widget-streak-due").Text; got != msgs.WidgetDue(0) {
		t.Fatalf("the streak widget says %q due, want nothing", got)
	}
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-widget-film-text").Text == "再见。" })
	if got, want := p.app.Find("id:settings-widget-wide-foot").Text, msgs.WidgetFoot(0, 3); got != want {
		t.Fatalf("the medium widget's foot reads %q, want %q", got, want)
	}
	if got := p.app.Find("id:settings-widget-lock-due").Text; got != "0" {
		t.Fatalf("the Lock Screen's ring shows %q due", got)
	}
}

// A tap on the Continue widget opens its film: one kept on the phone plays, also when the tap launched the app, before the store was read; one not on the phone opens its page, to download it. A link of a film the phone does not list, or of another scheme, opens nothing.
func TestTheContinueWidgetOpensItsFilm(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	f.PositionMS, f.PositionAt = 7500, now().Add(-time.Hour)
	if err := store.Open(dir).SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir)
	p.app.OpenLinks(widgets.Link{Film: f.Key()}.URL())
	p.app.WaitFor("id:player-video", wait)

	d := newDesktop(t)
	dir = t.TempDir()
	p = startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(talkRow(id), wait)
	p.app.OpenLinks(widgets.Link{Film: store.Key(id, "nofilm")}.URL(), "kando://open?url=x")
	p.app.OpenLinks(widgets.Link{Film: store.Key(id, "f1")}.URL())
	p.app.WaitFor("id:film-download", wait)
	if p.app.Exists("id:player-video") {
		t.Fatal("a film not on the phone plays")
	}
}

// A tap on the study widget opens the review over the tabs while anything is due, and Learn, which says nothing is, otherwise.
func TestTheStudyWidgetOpensTheReview(t *testing.T) {
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.app.WaitUntil(wait, func() bool { return p.texts("id:bar-badge-tab-learn") == "2" })
	p.app.OpenLinks(widgets.Link{Review: true}.URL())
	p.app.WaitFor("id:review-page", wait)

	dir = t.TempDir()
	keepTalk(t, dir)
	p = startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.OpenLinks(widgets.Link{Review: true}.URL())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewNone() })
	if p.app.Exists("id:review-page") {
		t.Fatal("the review opens with nothing due")
	}
}
