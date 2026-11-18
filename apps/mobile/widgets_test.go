package main

import (
	"testing"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/store"

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
	f.PositionMS, f.PositionAt = 3500, now().Add(-time.Hour)
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
	if got := p.app.Find("id:settings-widget-streak-days").Text; got != "3" {
		t.Fatalf("the streak widget shows %q days, want 3", got)
	}
	if got := p.app.Find("id:settings-widget-streak-due").Text; got != msgs.WidgetDue(0) {
		t.Fatalf("the streak widget says %q due, want nothing", got)
	}
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-widget-film-text").Text == "你好吗？" })
	if got, want := p.app.Find("id:settings-widget-wide-foot").Text, msgs.WidgetFoot(0, 3); got != want {
		t.Fatalf("the medium widget's foot reads %q, want %q", got, want)
	}
	if got := p.app.Find("id:settings-widget-lock-due").Text; got != "0" {
		t.Fatalf("the Lock Screen's ring shows %q due", got)
	}
}
