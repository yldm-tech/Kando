package main

import (
	"testing"
	"time"

	"kando/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// TestScreenshotsPlayer renders the prototype's film, How Rivers Shape Landscapes (j1), from the library every scene starts from (screenshot_helpers_test.go), in the states the prototype's player screenshots show: opened and held at the start with the side panel on 字幕 (01), playing on the first line with the controls showing (02), the 片段 and 信息 tabs (04, 08), the subtitle mode menu (09), the style view (10), playing with the controls faded (11), playing muted, the speed menu, playing at 1.5× and playing with its line looped. It also renders the film on its first line (film), the film opened from the YouTube page, the film without a picture, as without ffmpeg, with the side panel off, and a video no job has processed playing with the subtitles exported beside it.
func TestScreenshotsPlayer(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "film", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
	})
	held := func(steps func(a *app, tt *ui.Tester)) func(a *app, tt *ui.Tester, updates chan func()) {
		return func(a *app, tt *ui.Tester, updates chan func()) {
			screenshotOpen(t, a, tt, updates, "j1", 0)
			if steps != nil {
				steps(a, tt)
			}
		}
	}
	f.shoot(t, "player-opened", held(nil))
	f.shoot(t, "player-clips", held(func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "片段") }))
	f.shoot(t, "player-info", held(func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "信息") }))
	f.shoot(t, "player-menu", held(func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "字幕：双语") }))
	f.shoot(t, "player-style", held(func(_ *app, tt *ui.Tester) {
		screenshotClick(t, tt, "字幕：双语")
		screenshotClick(t, tt, "样式…")
	}))
	// Playing 3 seconds in, on the first line: with the pointer just moved the controls show, and once it has rested for idleAfter they fade, the subtitles moving down into their room.
	playing := func(rest time.Duration) func(a *app, tt *ui.Tester, updates chan func()) {
		return func(a *app, tt *ui.Tester, updates chan func()) {
			screenshotOpen(t, a, tt, updates, "j1", 3*time.Second)
			a.player.play.Toggle()
			tt.Frame()
			a.player.film.moved = tt.Now()
			tt.Advance(rest)
			tt.Frame()
			// Let the fade end.
			for range 10 {
				tt.Advance(50 * time.Millisecond)
				tt.Frame()
			}
		}
	}
	f.shoot(t, "player-playing", playing(0))
	f.shoot(t, "player-idle", playing(3*time.Second))
	// Playing with the sound muted by M: the speaker crossed out and the picture saying so.
	f.shoot(t, "player-muted", func(a *app, tt *ui.Tester, updates chan func()) {
		playing(0)(a, tt, updates)
		tt.Key(0, ui.KeyM)
		tt.Frame()
	})
	f.shoot(t, "player-speed-menu", held(func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "速度：1×") }))
	// Playing at 1.5×, two presses of ⇧. up from 1×: the speed button and the picture say so.
	f.shoot(t, "player-speed", func(a *app, tt *ui.Tester, updates chan func()) {
		playing(0)(a, tt, updates)
		tt.Key(ui.Shift, ui.KeyPeriod)
		tt.Key(ui.Shift, ui.KeyPeriod)
		tt.Frame()
	})
	// A video no job has processed, from a library copied without its job records (sidecarLibrary), plays with the subtitles exported beside it.
	f.shoot(t, "player-sidecar-subtitles", func(a *app, tt *ui.Tester, updates chan func()) {
		dir, video := f.sidecarLibrary(t)
		f.showLibrary(t, a, tt, updates, dir)
		screenshotOpen(t, a, tt, updates, video, protoCues[0].Start)
	})
	// Playing with the line looped by L: the transport's loop toggle lit and the picture saying so.
	f.shoot(t, "player-loop", func(a *app, tt *ui.Tester, updates chan func()) {
		playing(0)(a, tt, updates)
		tt.Key(0, ui.KeyL)
		tt.Frame()
	})
	f.shoot(t, "player-youtube", func(a *app, tt *ui.Tester, updates chan func()) {
		a.page = pageYouTube
		tt.Frame()
		screenshotOpen(t, a, tt, updates, "j1", protoCues[1].Start)
	})
	// Off macOS the player names the system's file manager, here the Linux one, and the style view's font menu offers a sans-serif and a serif font, shown open.
	usePlatform(t, breeze.Linux)
	f.shoot(t, "player-info-linux", held(func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "信息") }))
	f.shoot(t, "player-style-fonts-linux", held(func(_ *app, tt *ui.Tester) {
		screenshotClick(t, tt, "字幕：双语")
		screenshotClick(t, tt, "样式…")
		screenshotClick(t, tt, "默认")
	}))
	breeze.SetPlatform(breeze.Auto)
	// Without ffmpeg the film has no picture and no transport.
	f.shoot(t, "player-no-picture", func(a *app, tt *ui.Tester, updates chan func()) {
		a.pipeline = media.Tools{}
		a.player.panel = false
		screenshotOpen(t, a, tt, updates, "j1", protoCues[1].Start)
	})
}
