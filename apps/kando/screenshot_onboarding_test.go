package main

import (
	"testing"

	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsOnboarding renders the first-run guide's three steps over the prototype's library (screenshot_helpers_test.go), as the prototype's onboarding-1, -2 and -3 show it, to PNG files in the directory named by KANDO_SCREENSHOTS. It is skipped when the variable is unset.
func TestScreenshotsOnboarding(t *testing.T) {
	f := newScreenshotFixture(t)
	for step, name := range []string{"onboarding-1", "onboarding-2", "onboarding-3"} {
		f.shoot(t, name, func(a *app, tt *ui.Tester, _ chan func()) {
			a.openOnboarding()
			a.onb.step = step + 1
		})
	}
	// The second step on a first launch that finds none of the programs: Kando downloads all three.
	f.shoot(t, "onboarding-2-downloading", func(a *app, tt *ui.Tester, _ chan func()) {
		shotKitsDownloading(t, f, a)
		tools := a.pipeline.(media.Tools)
		tools.YTDLP = ""
		a.pipeline = tools
		a.kits.running = map[string]*kitDownload{
			"yt-dlp":      {written: 29 << 20, total: 36 << 20, cancel: func() {}},
			"ffmpeg":      {written: 9 << 20, total: 31 << 20, cancel: func() {}},
			"whisper-cli": {cancel: func() {}},
		}
		a.openOnboarding()
		a.onb.step = 2
	})
	// The second step after the recommended model's download stalled: its row says so in red, and 重试 picks it up where it stopped.
	f.shoot(t, "onboarding-2-model-stalled", func(a *app, tt *ui.Tester, _ chan func()) {
		a.openOnboarding()
		a.onb.step = 2
		a.onb.lastDownload = "large-v3-turbo-q5"
		a.prefs.downloadErr = media.ErrStalled
	})
}
