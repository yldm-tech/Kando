package main

import (
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// The settings window's reads and writes (settings.App). The 账号与上传 category's are in bind_account.go; the subtitle settings the player shares (Untranslated, YieldSubs and their setters) in bind_film.go.

func (a *app) Tools() (media.Tools, bool) {
	tools, ok := a.pipeline.(media.Tools)
	return tools, ok
}

func (a *app) Language() *string { return &a.prefs.language }

func (a *app) TranslateContext() *bool { return &a.prefs.translateContext }

func (a *app) TranslatePrompt() *string { return &a.prefs.translatePrompt }

func (a *app) Save() error { return a.saveSettings() }

func (a *app) Redetect() { a.redetectTools() }

func (a *app) ToolVersion(path string) string {
	if a.prefs.toolVersions == nil {
		return ""
	}
	return a.prefs.toolVersions[path]
}

func (a *app) InstalledSpeechModel(id string) string { return a.prefs.speechModels[id] }

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

func (a *app) ApplyLLM() { a.applyLLM() }

func (a *app) TestingLLM() bool { return a.prefs.testingLLM }

func (a *app) TestLLM() { a.testLLM() }

func (a *app) LLMResult() string { return a.prefs.llmTestResult }

func (a *app) SelectLLM(id string) { a.selectLLM(id) }

func (a *app) LLMModels() []string { return a.prefs.llmModels }

func (a *app) LoadingLLMModels() bool { return a.prefs.llmModelsLoading }

func (a *app) LLMModelsErr() string { return a.prefs.llmModelsErr }

func (a *app) RetryLLMModels() { a.fetchLLMModels(true) }

// OpenSettings shows the settings window at section, one of the settings package's Section values; "" or a section it does not know opens 通用. The window opens beside the main one, which keeps its page and its open film. It records the request in the settings' own state (prefs.open, prefs.section), which is what other areas and their tests read.
func (a *app) OpenSettings(section string) {
	if !settings.Known(section) {
		section = settings.SectionGeneral
	}
	a.prefs.open = true
	a.prefs.section = section
	a.findSpeechModels()
	a.showSettingsWindow()
}

func (a *app) SettingsSection() string {
	if a.prefs.section == "" {
		return settings.SectionGeneral
	}
	return a.prefs.section
}

func (a *app) SetSettingsSection(section string) { a.prefs.section = section }

func (a *app) SettingsQuery() *string { return &a.prefs.query }

func (a *app) ShortcutQuery() *string { return &a.prefs.shortcutQuery }

// Shortcuts is the app's shortcut table as the 快捷键 page lists it: the rows with a key.
func (a *app) Shortcuts() []settings.Shortcut {
	var out []settings.Shortcut
	for _, s := range a.shortcuts() {
		if s.Key != 0 {
			out = append(out, settings.Shortcut{Group: s.Group, Label: s.Label, Mods: s.Mods, Key: s.Key})
		}
	}
	return out
}
