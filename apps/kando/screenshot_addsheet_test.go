package main

import (
	"testing"
	"time"

	"kando/apps/kando/addsheet"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsAddSheet renders the Add sheet in the design's states, once the fake yt-dlp read the video (Earth Lab's, 12:34 long, with the site's automatic captions and none of the uploader's):
//   - addsheet-browser: a YouTube video the browser extension sent from a page that was playing it at 1:24, with its title, the site's subtitles, the picture, the sign-in and the chapters
//   - addsheet-playlist: a video of a YouTube playlist pasted in the link field, the whole playlist of three chosen, treated as a series
//   - addsheet-auto: a pasted YouTube video, named by its page's title, the site's automatic captions chosen as a reference layer
//   - addsheet-bilibili: a bilibili video without a sign-in, whose best picture is 480p, and its AI subtitles
//   - addsheet-drm: a Netflix link, which the sheet says cannot be downloaded
//   - addsheet-channel: the playlist dialog for a YouTube channel, which asks how many of its latest videos to add now
func TestScreenshotsAddSheet(t *testing.T) {
	f := newScreenshotFixture(t)
	open := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), link string, from addsheet.From, title string, at time.Duration) {
		t.Helper()
		screenshotReady(t, f, a)
		apply(tt, a, func() {
			if !a.addSheetAsk(link, from, title, at) {
				t.Fatalf("the sheet did not open for %s", link)
			}
		})
		if !a.addsheet.look.DRM {
			waitUntil(t, tt, a, updates, func() bool { return !a.addsheet.meta.Reading })
		}
		tt.HoverOut()
	}
	f.shoot(t, "addsheet-browser", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts", addsheet.FromBrowser, "Why Glaciers Move Faster in Summer", 84*time.Second)
	})
	f.shoot(t, "addsheet-playlist", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8", addsheet.FromField, "", 0)
		tt.Press("id:addsheet-whole-list")
		if !tt.Exists("id:addsheet-series") || a.addsheet.meta.Episodes != 3 {
			t.Fatalf("episodes %d, texts %q", a.addsheet.meta.Episodes, tt.Texts())
		}
	})
	f.shoot(t, "addsheet-auto", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts", addsheet.FromField, "", 0)
		tt.Press("id:addsheet-source-auto")
	})
	f.shoot(t, "addsheet-bilibili", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		apply(tt, a, func() { a.signin.browser = "" })
		open(t, a, tt, updates, "https://www.bilibili.com/video/BV1GJ411x7h7", addsheet.FromField, "", 0)
	})
	f.shoot(t, "addsheet-drm", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "https://www.netflix.com/watch/80100172", addsheet.FromField, "", 0)
	})
	f.shoot(t, "addsheet-channel", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotReady(t, f, a)
		apply(tt, a, func() { a.playlistAsk([]string{"https://www.youtube.com/@earthlab"}, playlistFromLibrary) })
		waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
		if !tt.Exists("id:playlist-recent") {
			t.Fatalf("no add-now row: %q", tt.Texts())
		}
		tt.HoverOut()
	})
}
