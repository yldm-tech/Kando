package main

import (
	"testing"

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
}
