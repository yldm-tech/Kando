package main

import (
	"slices"
	"testing"

	"kando/apps/kando/settings"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsReadme renders the four pictures the READMEs show, in the language of KANDO_SCREENSHOTS_LANG with the fixture's content for its reader (useProtoContent): the library with the status line ready and no failed job, the player on the first line of How Rivers Shape Landscapes with the side panel's list of lines, and Settings › 通用 and › AI 服务. docs/assets/<lang>/ keeps them as library.png, player.png, settings-general.png and settings-ai.png.
func TestScreenshotsReadme(t *testing.T) {
	f := newScreenshotFixture(t)
	// The library is the first picture of the READMEs and the landing page, and the one link previews show, so 处理中 shows the running and queued jobs without the prototype's failed ones, whose red cards would read as a broken app.
	working := *f
	working.jobs = slices.DeleteFunc(slices.Clone(f.jobs), func(job core.Job) bool { return job.Stage == core.StageFailed })
	working.shoot(t, "readme-library", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
		tt.Frame()
	})
	f.shoot(t, "readme-player", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
	})
	shown, models, turbo := settingsShotFolders(t, f)
	// The 命令行与 MCP card at the foot shows the app after it looked for the kando command, as the window shows it a moment after it opens.
	shootSettings(t, f, settings.SectionGeneral, "readme-settings-general", shown, models, turbo, func(a *app) { shotCLITool(f, a, true) })
	shootSettings(t, f, settings.SectionAI, "readme-settings-ai", shown, models, turbo)
}
