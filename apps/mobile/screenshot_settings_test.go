package main

import (
	"testing"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsWidgets is Settings › Widgets on an iPhone, the talk left partway and three days watched in a row.
func TestScreenshotsWidgets(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	f := keepTalk(t, dir)
	f.PositionMS, f.PositionAt = 3500, now().Add(-time.Hour)
	if err := store.Open(dir).SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	watched := learning.Watched{}
	for back := range 3 {
		watched.Add(now().AddDate(0, 0, -back), 20*time.Minute)
	}
	if err := learning.Open(dir).SaveWatched(watched); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS}))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-goal", wait)
	// A target reports its own platform: an Android phone has no widgets.
	if !p.app.Exists("id:settings-widgets") {
		t.Skip("the target has no widgets")
	}
	p.app.Press("id:settings-widgets")
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:settings-widget-film-text") && p.app.Find("id:settings-widget-film-text").Text == "你好吗？"
	})
	shot(p, "settings-widgets")
}

// TestScreenshotsTabletShortcuts is Settings › Keyboard Shortcuts on a tablet on its side.
func TestScreenshotsTabletShortcuts(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	if !p.app.Exists("id:sidebar") {
		t.Skip("the target is not a tablet")
	}
	p.app.Press("id:sidebar-tab-settings")
	p.app.WaitFor("id:settings-shortcuts", wait)
	p.app.Press("id:settings-shortcuts")
	p.app.WaitFor("id:settings-shortcuts-page", wait)
	shot(p, "tablet-shortcuts")
}
