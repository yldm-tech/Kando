package main

import (
	"context"
	"encoding/json"
	"strings"

	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsUI is the state of the settings window, a child window of the main one that ⌘,, the sidebar's gear and LingoCut › 设置… open: the translation's language and prompt, the speech models and their download, the AI service and the tools' versions, and the window itself. The model and the AI service in use are the pipeline's (media.Tools); these fields are what the window edits and shows.
type settingsUI struct {
	language         string
	translatePrompt  string
	translateContext bool
	modelsDir        string
	extraModelDirs   []string
	// speechModels maps a whisper catalog model's ID to its file on disk, for the models findSpeechModels found.
	speechModels    map[string]string
	downloading     string
	downloadWritten int64
	downloadTotal   int64
	downloadErr     string
	downloadCancel  context.CancelFunc
	llmProvider     string
	llmBase         string
	llmModel        string
	llmKey          string
	llmCredential   func(context.Context, bool) (media.Endpoint, error)
	testingLLM      bool
	llmTestResult   string
	toolVersions    map[string]string
	versionGen      int
	// open and section are the settings the user asked for: OpenSettings sets open and the settings.Section value it was given, the window's sidebar changes section, and closing the window clears open. Code and tests outside the settings area read these, never app.page, which the settings window leaves alone (D2). section "" is 通用.
	open    bool
	section string
	// query is the text of the window's search field, shortcutQuery that of the 快捷键 page.
	query, shortcutQuery string
	// win is the open window, nil while it is closed; opening is set while its creation waits for the main thread.
	win     *mygo.Window
	opening bool
	// mainDrawn and windowDrawn are the settings, as saveSettings writes them, that the main window and the settings window last drew. A window that draws settings the other has not drawn yet asks the other for a frame.
	mainDrawn, windowDrawn string
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

// settingsHooks: the settings window saves the speech model, the AI service and the translation, asks the tools for their versions once the main window exists, looks for the whisper models on disk again when the tools change, redraws after each background report (downloads, tool versions, the connection test), opens for a settings page chosen in the main sidebar, and keeps the two windows drawing the same settings.
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
		ready:        a.loadToolVersions,
		toolsChanged: func(bool) { a.findSpeechModels() },
		afterUpdate:  a.redrawSettings,
		afterFrame: func() {
			a.followSettingsPage()
			a.shareSettings(false)
		},
	}
}

// followSettingsPage opens the settings window at the settings page chosen in the main window's sidebar, whose 设置 rows stay until the sidebar drops them (P1-1), and puts the main window back on 全部影片, which no longer draws the settings.
func (a *app) followSettingsPage() {
	switch a.page {
	case pageGeneral, pageSpeech, pageAI, pageTranslate, pageAccount:
	default:
		return
	}
	section := a.page
	a.Select(pageFilms)
	a.OpenSettings(section)
	if a.win != nil {
		a.win.Invalidate()
	}
}

// settingsKeys are the settings' shortcuts.
func (a *app) settingsKeys() []shortcut { return nil }

// settingsView is the settings window's content.
func (a *app) settingsView(c *ui.Context) {
	t, dark := palette(c)
	settings.Window(c, t, dark, appVersion, a)
	a.shareSettings(true)
}

// showSettingsWindow brings the settings window to the front, creating it the first time. The window is made on the main thread after the frame that asked for it, not while a view is being built. Without a main window, as in tests, there is nothing to open.
func (a *app) showSettingsWindow() {
	if a.win == nil || a.prefs.opening {
		return
	}
	a.prefs.opening = true
	a.win.Update(func() {
		a.prefs.opening = false
		a.openSettingsWindow()
	})
}

func (a *app) openSettingsWindow() {
	if w := a.prefs.win; w != nil && !w.IsDestroyed() {
		w.Show()
		w.Invalidate()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             "设置",
		Width:             880,
		Height:            640,
		MinWidth:          720,
		MinHeight:         480,
		StateKey:          "settings",
		Parent:            a.win,
		DisableMaximize:   true,
		DisableFullScreen: true,
		TitleBarStyle:     mygo.TitleBarHidden,
		// The traffic lights sit in a band 50 high, as the prototype's settings rail has them.
		// No material: the prototype's settings draw their rail and pages in plain colors, without a backdrop blur.
		TrafficLightPosition: &mygo.Point{X: 18, Y: 18},
		Content:              ui.View(a.settingsView),
	})
	a.prefs.win = w
	w.OnClosed(func() {
		if a.prefs.win == w {
			a.prefs.win = nil
			a.prefs.open = false
			a.prefs.mainDrawn, a.prefs.windowDrawn = "", ""
		}
	})
}

// redrawSettings asks the open settings window for a frame.
func (a *app) redrawSettings() {
	if a.prefs.win != nil {
		a.prefs.win.Invalidate()
	}
}

// shareSettings keeps the two windows showing the same settings. Each calls it at the end of its frame, inSettings telling which one drew; when this frame drew settings the other window has not, such as a library chosen in the main window's sidebar or a subtitle mode chosen in the settings window, the other one gets a frame. Changes from background jobs reach both through the app's update.
func (a *app) shareSettings(inSettings bool) {
	if a.prefs.win == nil || a.win == nil {
		return
	}
	data, _ := json.Marshal(a.settingsFile())
	if !a.prefs.drew(inSettings, string(data)) {
		return
	}
	if inSettings {
		a.win.Invalidate()
	} else {
		a.prefs.win.Invalidate()
	}
}

// drew records settings, as one window drew them, and reports whether the other window has not drawn them yet.
func (s *settingsUI) drew(inSettings bool, settings string) bool {
	if inSettings {
		s.windowDrawn = settings
		return settings != s.mainDrawn
	}
	s.mainDrawn = settings
	return settings != s.windowDrawn
}
