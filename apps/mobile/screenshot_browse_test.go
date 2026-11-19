package main

import (
	"testing"

	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
)

// Browse's start, with a video looked at before, and the bar over a page of one video that sends it to the computer.
func TestScreenshotsBrowser(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	st := store.Open(dir)
	prefs, err := st.Prefs()
	if err != nil {
		t.Fatal(err)
	}
	prefs.Visited = []store.Visit{{URL: "https://www.ted.com/talks/ken_robinson_do_schools_kill_creativity", Title: "Do schools kill creativity?"}}
	if err := st.SavePrefs(prefs); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(browseTab, msgs.BrowseTitle())
	p.app.WaitFor("id:browse-recent", wait)
	shot(p, "browse-start")
	p.app.Press("id:browse-site-youtube")
	p.app.Navigate(browseWeb, core.WebNavigation{URL: "https://m.youtube.com/watch?v=jNQXAC9IVRw", Title: "Me at the zoo - YouTube"})
	p.app.WaitFor("id:browse-video", wait)
	shot(p, "browse-video")
}
