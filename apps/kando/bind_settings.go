package main

import (
	"kando/apps/kando/clitool"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo"
)

// The settings window's reads and writes (settings.App). The 账号与上传 category's are in bind_account.go; the subtitle settings the player shares (Untranslated, YieldSubs and their setters) in bind_film.go.

func (a *app) Tools() (media.Tools, bool) {
	tools, ok := a.pipeline.(media.Tools)
	return tools, ok
}

func (a *app) Language() *string { return &a.prefs.language }

func (a *app) InterfaceLanguage() string { return a.prefs.interfaceLanguage }

func (a *app) SetInterfaceLanguage(tag string) {
	if !locales.Supported(tag) {
		tag = locales.Default
	}
	a.prefs.interfaceLanguage = tag
	mygo.App.SetLocale(tag)
	if w := a.prefs.win; w != nil && !w.IsDestroyed() {
		w.SetTitle(locales.SettingsWindowTitle())
	}
}

func (a *app) TranslateContext() *bool { return &a.prefs.translateContext }

func (a *app) TranslatePrompt() *string { return &a.prefs.translatePrompt }

func (a *app) Save() error { return a.saveSettings() }

// Redetect looks for the programs Kando runs, and for the kando command, again.
func (a *app) Redetect() {
	a.redetectTools()
	a.cliToolCheck()
}

func (a *app) ToolDownload(program string) settings.ToolFetch {
	progress, running := a.kitsProgress(program)
	return settings.ToolFetch{Available: a.kitsCanInstall(program), Running: running, Progress: progress, Err: kitsError(a.kits.errs[program])}
}

func (a *app) DownloadTool(program string) { a.kitsInstall(program) }

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

func (a *app) DownloadErr() string { return speechDownloadError(a.prefs.downloadErr) }

func (a *app) CancelDownload() { a.cancelDownload() }

func (a *app) DownloadModel(id string) { a.downloadSpeechModel(id) }

func (a *app) OwnsSpeechModel(id string) bool { return a.speechModelOwned(id) }

func (a *app) SpeechModelInUse(path string) bool { return a.speechModelInUse(path) }

func (a *app) AskTrashSpeechModel(id string) { a.askTrashSpeechModel(id) }

func (a *app) TrashingSpeechModel() (*bool, string) { return &a.prefs.trashOpen, a.prefs.trashModel }

func (a *app) TrashSpeechModel(id string) { a.trashSpeechModel(id) }

func (a *app) SpeechLanguage() string {
	tools, _ := a.pipeline.(media.Tools)
	return tools.SpeechLanguage
}

func (a *app) SetSpeechLanguage(code string) { a.useSpeechLanguage(code) }

// CurrentPreset and Presets read the presets' names and blurbs in the interface's language each time the page draws them.
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

// LLMResult is the last connection test's outcome as the page shows it: "" before any, the error when it failed, or locales.SettingsAIConnected when it passed.
func (a *app) LLMResult() string {
	if a.prefs.llmTestOK {
		return locales.SettingsAIConnected()
	}
	return a.prefs.llmTestErr
}

func (a *app) SelectLLM(id string) { a.selectLLM(id) }

func (a *app) LLMModels() []string { return a.prefs.llmModels }

func (a *app) LoadingLLMModels() bool { return a.prefs.llmModelsLoading }

func (a *app) LLMModelsErr() string {
	if a.prefs.llmModelsErr == nil {
		return ""
	}
	return llmModelsError(a.prefs.llmModelsErr)
}

func (a *app) RetryLLMModels() { a.fetchLLMModels(true) }

func (a *app) EveryAPI() settings.EveryAPI { return a.everyAPIStatus() }

func (a *app) GetEveryAPI() { a.getEveryAPI() }

// OpenSettings shows the settings window at section, one of the settings package's Section values; "" or a section it does not know opens 通用. The window opens beside the main one, which keeps its page and its open film. It records the request in the settings' own state (prefs.open, prefs.section), which is what other areas and their tests read.
func (a *app) OpenSettings(section string) {
	if !settings.Known(section) {
		section = settings.SectionGeneral
	}
	a.prefs.open = true
	a.prefs.section = section
	a.findSpeechModels()
	if section == settings.SectionGeneral {
		a.cliToolCheck()
	}
	a.showSettingsWindow()
}

func (a *app) SettingsSection() string {
	if a.prefs.section == "" {
		return settings.SectionGeneral
	}
	return a.prefs.section
}

// SetSettingsSection shows section; 通用 looks for the kando command again, as a terminal may have installed or removed it since.
func (a *app) SetSettingsSection(section string) {
	if section == settings.SectionGeneral && a.SettingsSection() != section {
		a.cliToolCheck()
	}
	a.prefs.section = section
}

func (a *app) SettingsQuery() *string { return &a.prefs.query }

func (a *app) ShortcutQuery() *string { return &a.prefs.shortcutQuery }

// Shortcuts is the app's shortcut table as the 快捷键 page lists it: the rows with a key.
func (a *app) Shortcuts() []settings.Shortcut {
	var out []settings.Shortcut
	for _, s := range a.shortcuts() {
		if s.Key != 0 {
			out = append(out, settings.Shortcut{Group: groupTitle(s.Group), Label: s.Label, Mods: s.Mods, Key: s.Key})
		}
	}
	return out
}

// CLI is where the kando command stands, from the last look (state_cli.go), with what an MCP client runs.
func (a *app) CLI() settings.CLI {
	st := a.cliTool.status
	command := st.MCPCommand()
	return settings.CLI{
		Known:         a.cliTool.known,
		Installed:     st.Ours,
		Command:       st.Command,
		Link:          st.Link,
		Exe:           st.Exe,
		Installable:   st.Installable,
		Installing:    a.cliTool.installing,
		Err:           a.cliTool.err,
		Temporary:     st.Temporary,
		MCPConfig:     clitool.MCPConfig(command),
		ClaudeCommand: clitool.ClaudeCommand(command),
		Copied:        a.cliTool.copied,
	}
}

func (a *app) InstallCLI() { a.cliToolStartInstall() }

func (a *app) CLICopied(text string) { a.cliTool.copied = text }
