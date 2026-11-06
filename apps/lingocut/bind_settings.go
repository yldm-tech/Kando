package main

import (
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

func (a *app) Language() *string { return &a.language }

func (a *app) TranslateContext() *bool { return &a.translateContext }

func (a *app) TranslatePrompt() *string { return &a.translatePrompt }

func (a *app) Save() error { return a.saveSettings() }

func (a *app) LibraryMenu(m *ui.Menu) { sidebar.Menu(m, a) }

func (a *app) Redetect() { a.redetectTools() }

func (a *app) ToolVersion(path string) string {
	if a.toolVersions == nil {
		return ""
	}
	return a.toolVersions[path]
}

func (a *app) SpeechDirs() []string { return a.speechSearchDirs() }

func (a *app) UseSpeechModel(path string) { a.useSpeechModel(path) }

func (a *app) Downloading() string { return a.downloading }

func (a *app) DownloadWritten() int64 { return a.downloadWritten }

func (a *app) DownloadTotal() int64 { return a.downloadTotal }

func (a *app) DownloadErr() string { return a.downloadErr }

func (a *app) CancelDownload() { a.cancelDownload() }

func (a *app) DownloadModel(id string) { a.downloadSpeechModel(id) }

func (a *app) CurrentPreset() (settings.Preset, bool) {
	preset, ok := llmPresetByID(a.llmProvider)
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

func (a *app) LLMProvider() string { return a.llmProvider }

func (a *app) SetLLMProvider(id string) { a.llmProvider = id }

func (a *app) ProviderID(base string) string { return llmProviderID(base) }

func (a *app) LLMBase() *string { return &a.llmBase }

func (a *app) LLMKey() *string { return &a.llmKey }

func (a *app) LLMModel() *string { return &a.llmModel }

func (a *app) ShowLLMKey() bool { return a.showLLMKey }

func (a *app) ToggleLLMKey() { a.showLLMKey = !a.showLLMKey }

func (a *app) ApplyLLM() { a.applyLLM() }

func (a *app) TestingLLM() bool { return a.testingLLM }

func (a *app) TestLLM() { a.testLLM() }

func (a *app) LLMResult() string { return a.llmTestResult }

func (a *app) SelectLLM(id string) { a.selectLLM(id) }
