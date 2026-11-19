package main

import (
	"testing"
	"time"

	"kando/apps/kando/addsheet"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsAddSheet renders the Add sheet in the design's states:
//   - addsheet-browser: a YouTube video the browser extension sent from a page that was playing it at 1:24, with its title, the site's subtitles, the picture, the sign-in and the chapters
//   - addsheet-playlist: a video of a YouTube playlist pasted in the link field, the whole playlist chosen
//   - addsheet-drm: a Netflix link, which the sheet says cannot be downloaded
func TestScreenshotsAddSheet(t *testing.T) {
	f := newScreenshotFixture(t)
	open := func(t *testing.T, a *app, tt *nsgotest.App, link string, from addsheet.From, title string, at time.Duration) {
		t.Helper()
		screenshotReady(t, f, a)
		apply(tt, a, func() {
			if !a.addSheetAsk(link, from, title, at) {
				t.Fatalf("the sheet did not open for %s", link)
			}
		})
		tt.HoverOut()
	}
	f.shoot(t, "addsheet-browser", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		open(t, a, tt, "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts", addsheet.FromBrowser, "Why Glaciers Move Faster in Summer", 84*time.Second)
	})
	f.shoot(t, "addsheet-playlist", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		open(t, a, tt, "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8", addsheet.FromField, "", 0)
		tt.Press("id:addsheet-whole-list")
	})
	f.shoot(t, "addsheet-drm", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		open(t, a, tt, "https://www.netflix.com/watch/80100172", addsheet.FromField, "", 0)
	})
}
