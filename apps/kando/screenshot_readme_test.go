package main

import (
	"testing"

	"kando/apps/kando/settings"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsReadme renders the four pictures the READMEs show, in the language of KANDO_SCREENSHOTS_LANG with the fixture's content for its reader (useProtoContent): the library with the status line ready, the player on the first line of How Rivers Shape Landscapes with the side panel's list of lines, and Settings › 通用 and › AI 服务. docs/assets/<lang>/ keeps them as library.png, player.png, settings-general.png and settings-ai.png.
func TestScreenshotsReadme(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "readme-library", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
		tt.Frame()
	})
	f.shoot(t, "readme-player", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
	})
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionGeneral, "readme-settings-general", shown, models, turbo)
	shootSettings(t, f, settings.SectionAI, "readme-settings-ai", shown, models, turbo)
}
