package main

import (
	"lingocut/apps/lingocut/account"
	"lingocut/apps/lingocut/settings"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// The settings pages' reads and writes (settings.App).

func (a *app) Tools() (media.Tools, bool) {
	tools, ok := a.pipeline.(media.Tools)
	return tools, ok
}

func (a *app) Language() *string { return &a.prefs.language }

func (a *app) TranslateContext() *bool { return &a.prefs.translateContext }

func (a *app) TranslatePrompt() *string { return &a.prefs.translatePrompt }

func (a *app) Save() error { return a.saveSettings() }

func (a *app) LibraryMenu(m *ui.Menu) { sidebar.Menu(m, a) }

func (a *app) Redetect() { a.redetectTools() }

func (a *app) ToolVersion(path string) string {
	if a.prefs.toolVersions == nil {
		return ""
	}
	return a.prefs.toolVersions[path]
}

func (a *app) SpeechDirs() []string { return a.speechSearchDirs() }

func (a *app) UseSpeechModel(path string) { a.useSpeechModel(path) }

func (a *app) Downloading() string { return a.prefs.downloading }

func (a *app) DownloadWritten() int64 { return a.prefs.downloadWritten }

func (a *app) DownloadTotal() int64 { return a.prefs.downloadTotal }

func (a *app) DownloadErr() string { return a.prefs.downloadErr }

func (a *app) CancelDownload() { a.cancelDownload() }

func (a *app) DownloadModel(id string) { a.downloadSpeechModel(id) }

func (a *app) CurrentPreset() (settings.Preset, bool) {
	preset, ok := llmPresetByID(a.prefs.llmProvider)
	if !ok {
		return settings.Preset{}, false
	}
	return settings.Preset{ID: preset.ID, Name: preset.Name, Blurb: preset.Blurb, Group: preset.Group}, true
}

func (a *app) Presets() []settings.Preset {
	src := llmPresets()
	out := make([]settings.Preset, len(src))
	for i, preset := range src {
		out[i] = settings.Preset{ID: preset.ID, Name: preset.Name, Blurb: preset.Blurb, Group: preset.Group}
	}
	return out
}

func (a *app) LLMProvider() string { return a.prefs.llmProvider }

func (a *app) SetLLMProvider(id string) { a.prefs.llmProvider = id }

func (a *app) ProviderID(base string) string { return llmProviderID(base) }

func (a *app) LLMBase() *string { return &a.prefs.llmBase }

func (a *app) LLMKey() *string { return &a.prefs.llmKey }

func (a *app) LLMModel() *string { return &a.prefs.llmModel }

func (a *app) ShowLLMKey() bool { return a.prefs.showLLMKey }

func (a *app) ToggleLLMKey() { a.prefs.showLLMKey = !a.prefs.showLLMKey }

func (a *app) ApplyLLM() { a.applyLLM() }

func (a *app) TestingLLM() bool { return a.prefs.testingLLM }

func (a *app) TestLLM() { a.testLLM() }

func (a *app) LLMResult() string { return a.prefs.llmTestResult }

func (a *app) SelectLLM(id string) { a.selectLLM(id) }

// OpenSettings shows the settings at section, one of the settings package's Section values; "" opens 通用. The settings are sidebar pages until they get a window of their own, so this closes the open film; 字幕外观 and 快捷键 have no page yet and open 通用.
func (a *app) OpenSettings(section string) {
	switch section {
	case settings.SectionGeneral, settings.SectionSpeech, settings.SectionAI, settings.SectionTranslate, settings.SectionAccount:
	default:
		section = settings.SectionGeneral
	}
	a.Select(section)
}

// settingsPage draws the settings page a.page names, 账号 among them, and reports whether a.page is one. view shows the library otherwise.
func (a *app) settingsPage(c *ui.Context, t *ui.Theme) bool {
	switch a.page {
	case pageGeneral, pageSpeech, pageAI, pageTranslate:
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(920).Gap(22).Children(func() {
				settings.Show(c, t, appVersion, a)
			})
		})
	case pageAccount:
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(920).Gap(22).Children(func() {
				account.Show(c, t, a)
			})
		})
	default:
		return false
	}
	return true
}
