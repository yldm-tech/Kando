package main

import (
	"testing"

	"kando/apps/kando/feedback"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsFeedback renders the 反馈问题 dialog over the library, as 帮助 › 反馈问题… opens it, and over the open film with its subtitle line ticked, as the player's ⋯ › 反馈这部影片的字幕… opens it (screenshot_helpers_test.go).
func TestScreenshotsFeedback(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "feedback", func(a *app, tt *nsgotest.App, updates chan func()) {
		a.feedback.system = feedback.SystemName("darwin", "27.2", "arm64")
		a.loadToolVersions()
		screenshotWait(t, tt, a, updates, "tool versions", func() bool { return len(a.prefs.toolVersions) == 3 })
		a.openFeedback(feedback.KindBug)
		refresh(tt, a)
	})
	f.shoot(t, "feedback-film", func(a *app, tt *nsgotest.App, updates chan func()) {
		a.feedback.system = feedback.SystemName("darwin", "27.2", "arm64")
		a.loadToolVersions()
		screenshotWait(t, tt, a, updates, "tool versions", func() bool { return len(a.prefs.toolVersions) == 3 })
		screenshotOpen(t, a, tt, updates, "j1", protoCues[3].Start)
		a.FeedbackSubtitles()
		refresh(tt, a)
		tt.Press("id:feedback-subs")
		if !a.feedback.form.Subs {
			t.Fatal("the subtitle line is not ticked")
		}
	})
}
