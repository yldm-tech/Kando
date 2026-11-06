package main

import (
	"context"
	"strings"

	"lingocut/packages/media"
)

// settingsUI is the state of the settings pages: the translation's language and prompt, the speech models and their download, the AI service and the tools' versions. The model and the AI service in use are the pipeline's (media.Tools); these fields are what the pages edit and show.
type settingsUI struct {
	language         string
	translatePrompt  string
	translateContext bool
	modelsDir        string
	extraModelDirs   []string
	downloading      string
	downloadWritten  int64
	downloadTotal    int64
	downloadErr      string
	downloadCancel   context.CancelFunc
	llmProvider      string
	llmBase          string
	llmModel         string
	llmKey           string
	showLLMKey       bool
	llmCredential    func(context.Context, bool) (media.Endpoint, error)
	testingLLM       bool
	llmTestResult    string
	toolVersions     map[string]string
	versionGen       int
	// open and section are the settings the user asked for: OpenSettings sets open and the settings.Section value it was given, and they stay until the settings close. Code and tests outside the settings area read these, never app.page, because the settings become a window of their own (D2). Until then the settings are sidebar pages, so OpenSettings also selects the page and the settings close when the window shows another page (settingsHooks.afterFrame).
	open    bool
	section string
}

func newSettingsUI() settingsUI {
	return settingsUI{language: "简体中文", translatePrompt: media.DefaultSubtitlePrompt, translateContext: true}
}

// savedTranslate is the translation's part of settings.json.
type savedTranslate struct {
	Language         string `json:"language,omitempty"`
	TranslatePrompt  string `json:"translatePrompt,omitempty"`
	TranslateContext *bool  `json:"translateContext,omitempty"`
}

func (s *savedTranslate) clean() {
	s.Language = strings.TrimSpace(s.Language)
	s.TranslatePrompt = strings.TrimSpace(s.TranslatePrompt)
}

func (a *app) saveTranslate(saved *savedSettings) {
	contextOn := a.prefs.translateContext
	saved.savedTranslate = savedTranslate{
		Language:         a.prefs.language,
		TranslatePrompt:  a.prefs.translatePrompt,
		TranslateContext: &contextOn,
	}
}

func (a *app) restoreTranslate(saved savedSettings) {
	if saved.Language != "" {
		a.prefs.language = saved.Language
	}
	if saved.TranslatePrompt != "" {
		a.prefs.translatePrompt = saved.TranslatePrompt
	}
	if saved.TranslateContext != nil {
		a.prefs.translateContext = *saved.TranslateContext
	}
}

// settingsHooks: the settings pages save the speech model, the AI service and the translation, and ask the tools for their versions once the window exists.
func (a *app) settingsHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			a.saveSpeech(saved)
			a.saveLLM(saved)
			a.saveTranslate(saved)
		},
		restore: func(saved savedSettings) {
			a.restoreSpeech(saved)
			a.restoreLLM(saved)
			a.restoreTranslate(saved)
		},
		ready:      a.loadToolVersions,
		afterFrame: a.followSettingsPage,
	}
}

// followSettingsPage keeps open and section in step with the sidebar while the settings are its pages: a settings page chosen in the sidebar opens them there, and another page closes them. It goes once the settings have a window of their own.
func (a *app) followSettingsPage() {
	page, _ := settingsSectionPage(a.prefs.section)
	switch {
	case !isSettingsPage(a.page):
		a.prefs.open = false
	case !a.prefs.open || page != a.page:
		a.prefs.open = true
		a.prefs.section = a.page
	}
}

// settingsKeys are the settings' shortcuts.
func (a *app) settingsKeys() []shortcut { return nil }
