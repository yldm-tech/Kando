package main

import (
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsOnboarding renders the first-run guide's three steps over the prototype's library (screenshot_helpers_test.go), as the prototype's onboarding-1, -2 and -3 show it, to PNG files in the directory named by KANDO_SCREENSHOTS, and checks that each scene shows the step it is of. It is skipped when the variable is unset.
func TestScreenshotsOnboarding(t *testing.T) {
	f := newScreenshotFixture(t)
	msgs := testlocale.Messages()
	for step, name := range []string{"onboarding-1", "onboarding-2", "onboarding-3"} {
		f.shoot(t, name, func(a *app, tt *nsgotest.App, _ chan func()) {
			a.openOnboarding()
			a.onb.step = step + 1
			refresh(tt, a)
			if counter := msgs.OnboardingStepCounter(step+1, 3); !hasText(tt, counter) {
				t.Fatalf("no %q in %q", counter, tt.Texts())
			}
		})
	}
	// The second step on a first launch that finds none of the programs: Kando downloads all three.
	f.shoot(t, "onboarding-2-downloading", func(a *app, tt *nsgotest.App, _ chan func()) {
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
		refresh(tt, a)
		if !hasText(tt, msgs.OnboardingToolsDownloading()) {
			t.Fatalf("no %q in %q", msgs.OnboardingToolsDownloading(), tt.Texts())
		}
	})
	// The second step after the recommended model's download from Hugging Face stalled: its row says so in red, 重试 picks it up where it stopped, and 从 hf-mirror.com 下载 gets it from the mirror.
	f.shoot(t, "onboarding-2-model-stalled", func(a *app, tt *nsgotest.App, _ chan func()) {
		a.openOnboarding()
		a.onb.step = 2
		a.onb.lastDownload = "large-v3-turbo-q5"
		a.prefs.downloadErr = settings.DownloadError{ID: "large-v3-turbo-q5", Err: media.ErrStalled}
		refresh(tt, a)
		if !tt.Exists("id:onboarding-download-large-v3-turbo-q5") || tt.Find("id:onboarding-download-large-v3-turbo-q5").Props.Label != msgs.OnboardingRetry() || !tt.Exists("id:onboarding-mirror-large-v3-turbo-q5") {
			t.Fatalf("the stalled model offers no %q or mirror: %q", msgs.OnboardingRetry(), tt.Texts())
		}
	})
}
