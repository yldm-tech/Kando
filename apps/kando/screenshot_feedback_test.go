package main

import (
	"testing"

	"kando/apps/kando/feedback"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsFeedback renders the 反馈问题 dialog over the library, as 帮助 › 反馈问题… opens it, and over the open film with its subtitle line ticked, as the player's ⋯ › 反馈这部影片的字幕… opens it (screenshot_helpers_test.go).
func TestScreenshotsFeedback(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "feedback", func(a *app, tt *ui.Tester, updates chan func()) {
		a.feedback.system = feedbackSystemName("darwin", "27.2", "arm64")
		a.loadToolVersions()
		screenshotWait(t, tt, updates, "tool versions", func() bool { return len(a.prefs.toolVersions) == 3 })
		a.openFeedback(feedback.KindBug)
		tt.Frame()
	})
	f.shoot(t, "feedback-film", func(a *app, tt *ui.Tester, updates chan func()) {
		a.feedback.system = feedbackSystemName("darwin", "27.2", "arm64")
		a.loadToolVersions()
		screenshotWait(t, tt, updates, "tool versions", func() bool { return len(a.prefs.toolVersions) == 3 })
		screenshotOpen(t, a, tt, updates, "j1", protoCues[3].Start)
		a.FeedbackSubtitles()
		tt.Frame()
		a.feedback.form.Subs = true
		tt.Frame()
	})
}
