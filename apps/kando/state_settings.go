package main

import (
	"context"
	"strings"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/chrome"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/theme"
	kcore "kando/packages/core"
	"kando/packages/media"

	"kando/apps/kando/live"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// settingsUI is the state of the settings window, a window of its own that ⌘,, the sidebar's gear and 看懂 › 设置… open (over the main window where the platform opens none): the translation's language and prompt, the speech models and their download, the AI service and the tools' versions, and the window itself. The model and the AI service in use are the pipeline's (media.Tools); these fields are what the window edits and shows.
type settingsUI struct {
	interfaceLanguage string
	language          string
	translatePrompt   string
	translateContext  bool
	// translateContextLines is how many earlier lines go with a batch while translateContext is on, 0 for media.DefaultContextLines.
	translateContextLines int
	// review is whether a job proofreads the translation as a whole once it is made (the final proofread, media.WithReview), and reviewAuto which of its changes apply without the user.
	review     bool
	reviewAuto kcore.ProofAuto
	// suggestTerms is whether a job asks the AI service for the terms of the film it translated, for its glossary (media.WithTermSuggestions).
	suggestTerms bool
	// translateSource is the translation source chosen in Settings › 翻译, a script plugin's translator, zero for the AI service (state_plugins_translate.go).
	translateSource savedTranslateSource
	// shortcutPreset is the preset of Settings › 快捷键 the player's keys follow, one of hotkey.Presets.
	shortcutPreset string
	// appearance is the one the windows show: settings.AppearanceLight or AppearanceDark, or "" to follow the system's.
	appearance     string
	modelsDir      string
	extraModelDirs []string
	// speechModels maps a whisper catalog model's ID to its file on disk, for the models findSpeechModels found.
	speechModels    map[string]string
	downloading     string
	downloadWritten int64
	downloadTotal   int64
	downloadErr     error
	downloadCancel  context.CancelFunc
	speechSource    string // where the whisper models download from, one of media.SpeechSources: "" for Hugging Face
	llmProvider     string
	llmBase         string
	llmModel        string
	llmKey          string
	// llmKeyOwner is the endpoint the key belongs to, so it is not sent elsewhere when the Base URL moves to another host; llmServices are the services used before, by preset id, which choosing one again takes back (selectLLM).
	llmKeyOwner llmKeyOwner
	llmServices map[string]savedLLMService
	// everyAPIOpenErr says why 获取 EveryAPI could not open EveryAPI's website, "" when it did, was not pressed, or the tools were detected again since. everyAPIOpenGen counts the presses, so only the last one's answer is kept.
	everyAPIOpenErr string
	everyAPIOpenGen int
	// trashModel is the whisper catalog model whose move to the Trash waits to be confirmed while trashOpen. trashRelease gives back the modal flag (holdModal) the confirmation holds, nil while it is closed; speechModals gives it back once the confirmation has closed.
	trashModel   string
	trashOpen    bool
	trashRelease func()
	// testingLLM is set while the connection test runs; llmTestOK is set when the last one passed and llmTestErr is why it failed, which the page words in the language it is shown in (LLMResult). llmTestGen counts the tests and the changes of the service, key or model that forget them (llmForgetTest), so only the answer of a test about the settings in use is kept.
	testingLLM bool
	llmTestOK  bool
	llmTestErr error
	llmTestGen int
	// llmModels are the model ids the AI service offers, fetched for the service, Base URL and key in llmModelsFor; llmModelsLoading is set while a fetch runs and llmModelsErr is why the last one failed, which LLMModelsErr words in the language it is shown in. llmEdited is when the Base URL or the key was last typed, so a fetch waits for typing to pause, and llmWaiting is set while a timer waits for that pause. listModels asks a service for its models, and pingLLM whether it translates with the key and model given, as 测试连接 does; tests replace them.
	llmModels        []string
	llmModelsFor     string
	llmModelsLoading bool
	llmModelsErr     error
	llmEdited        time.Time
	llmWaiting       bool
	listModels       func(context.Context, media.ChatTranslator) ([]string, error)
	pingLLM          func(context.Context, media.ChatTranslator) error
	// open and section are the settings the user asked for: OpenSettings sets open and the settings.Section value it was given, the window's sidebar changes section, and closing the window clears open. Code and tests outside the settings area read these, never app.page, which the settings window leaves alone (D2). section "" is 通用.
	open    bool
	section string
	// query is the text of the window's search field, shortcutQuery that of the 快捷键 page.
	query, shortcutQuery string
	// win is the open settings window, nil while it is closed; opening is set while its opening waits for the post that opens it (showSettingsWindow). inWindow is set while the settings show over the main window, where the platform opened no window for them (settingsOverlay).
	win      *core.Window
	opening  bool
	inWindow bool
	// overMain shows the settings over the main window even where the platform opens windows. The tests set it: nsgotest's host opens the app's windows on a desktop, but its steps act on the main window alone.
	// TODO(nsgo): let nsgotest's steps (Press, Type, Find, Key, Screenshot) act on a window the app opened other than the main one, then test the settings in their own window and drop overMain; nsgotest has Opened and SetWindowEnvironment for the other windows but no handle to act in them, and nsgo#437 was closed until that API is split out
	overMain bool
	// frame is where the settings window was last, as desktop.Frame reports it, which settings.json keeps so the window opens there again (savedInterface).
	frame core.WindowFrame
}

func newSettingsUI() settingsUI {
	return settingsUI{interfaceLanguage: locales.Default, language: "简体中文", translatePrompt: media.DefaultSubtitlePrompt, translateContext: true, review: true, reviewAuto: kcore.ProofAutoConsistency, suggestTerms: true, listModels: func(ctx context.Context, t media.ChatTranslator) ([]string, error) { return t.Models(ctx) }, pingLLM: func(ctx context.Context, t media.ChatTranslator) error { return t.Ping(ctx) }}
}

// savedInterface is the settings window's own part of settings.json: the interface language, kept separate from savedTranslate, whose language is the target language of generated subtitles, and where the window was, which MyGo's StateKey used to keep in a file of its own.
type savedInterface struct {
	InterfaceLanguage string `json:"interfaceLanguage,omitempty"`
	// Appearance is the one the windows show, light or dark, "" to follow the system's (Settings › 通用 › 外观).
	Appearance     string      `json:"appearance,omitempty"`
	SettingsWindow *savedFrame `json:"settingsWindow,omitempty"`
}

func (s *savedInterface) clean() {
	if !locales.Supported(s.InterfaceLanguage) {
		s.InterfaceLanguage = locales.Default
	}
	s.SettingsWindow = s.SettingsWindow.cleaned()
	if s.Appearance != settings.AppearanceLight && s.Appearance != settings.AppearanceDark {
		s.Appearance = ""
	}
}

func (a *app) saveInterface(saved *savedSettings) {
	saved.savedInterface = savedInterface{InterfaceLanguage: a.prefs.interfaceLanguage, Appearance: a.prefs.appearance, SettingsWindow: savedFrameOf(a.prefs.frame)}
}

func (a *app) restoreInterface(saved savedSettings) {
	if saved.InterfaceLanguage != "" {
		a.prefs.interfaceLanguage = saved.InterfaceLanguage
	}
	a.prefs.frame = saved.SettingsWindow.windowFrame()
	a.prefs.appearance = saved.Appearance
}

// settingsAppearance is the appearance the app shows, as nsgo.Appearance and core.Scope.SetAppearance take it: the one chosen in Settings › 通用, or the system's (theme.Appearance).
func (a *app) settingsAppearance() core.Appearance {
	switch a.prefs.appearance {
	case settings.AppearanceLight:
		return core.AppearanceLight
	case settings.AppearanceDark:
		return core.AppearanceDark
	}
	return theme.Appearance
}

// useAppearance shows the windows in appearance, settings.AppearanceLight or AppearanceDark, or as the system does for any other, at once, and saves it.
func (a *app) useAppearance(appearance string) {
	if appearance != settings.AppearanceLight && appearance != settings.AppearanceDark {
		appearance = ""
	}
	if a.prefs.appearance == appearance {
		return
	}
	a.prefs.appearance = appearance
	if a.cx != nil {
		a.cx.SetAppearance(a.settingsAppearance())
	}
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSaveFailed(err.Error())
	}
}

// savedTranslate is the translation's part of settings.json.
type savedTranslate struct {
	Language         string `json:"language,omitempty"`
	TranslatePrompt  string `json:"translatePrompt,omitempty"`
	TranslateContext *bool  `json:"translateContext,omitempty"`
	// TranslateContextLines is how many earlier lines go with a batch, 0 for media.DefaultContextLines.
	TranslateContextLines int `json:"translateContextLines,omitempty"`
	// Review is whether the final proofread runs, nil for on; ReviewAuto which of its changes apply without the user, "" for the default (core.ProofAutoConsistency).
	Review     *bool  `json:"review,omitempty"`
	ReviewAuto string `json:"reviewAuto,omitempty"`
	// SuggestTerms is whether a job asks for the terms of the film it translated, nil for on.
	SuggestTerms *bool `json:"suggestTerms,omitempty"`
	// TranslateSource is the script plugin's translator that translates the films in place of the AI service, nil for the AI service (state_plugins_translate.go).
	TranslateSource *savedTranslateSource `json:"translateSource,omitempty"`
}

func (s *savedTranslate) clean() {
	s.Language = strings.TrimSpace(s.Language)
	s.TranslatePrompt = strings.TrimSpace(s.TranslatePrompt)
	if s.ReviewAuto != "" {
		s.ReviewAuto = string(kcore.CleanProofAuto(kcore.ProofAuto(s.ReviewAuto)))
	}
	if s.TranslateContextLines != 0 {
		s.TranslateContextLines = min(max(s.TranslateContextLines, settings.MinContextLines), media.MaxContextLines)
	}
	if s.TranslateSource != nil && !s.TranslateSource.valid() {
		s.TranslateSource = nil
	}
}

// saveTranslate keeps the translation prompt only when it is the user's own: the default is saved as nothing, which restoreTranslate reads as the default of the version running, so a better default reaches everyone who did not change it.
func (a *app) saveTranslate(saved *savedSettings) {
	contextOn := a.prefs.translateContext
	prompt := a.prefs.translatePrompt
	if strings.TrimSpace(prompt) == strings.TrimSpace(media.DefaultSubtitlePrompt) {
		prompt = ""
	}
	saved.savedTranslate = savedTranslate{
		Language:         a.prefs.language,
		TranslatePrompt:  prompt,
		TranslateContext: &contextOn,
	}
	review := a.prefs.review
	saved.Review = &review
	suggest := a.prefs.suggestTerms
	saved.SuggestTerms = &suggest
	if a.prefs.reviewAuto != kcore.ProofAutoConsistency {
		saved.ReviewAuto = string(a.prefs.reviewAuto)
	}
	if lines := a.prefs.translateContextLines; lines != media.DefaultContextLines {
		saved.TranslateContextLines = lines
	}
	if source := a.prefs.translateSource; source.Plugin != "" {
		saved.TranslateSource = &source
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
	a.prefs.translateContextLines = saved.TranslateContextLines
	if saved.Review != nil {
		a.prefs.review = *saved.Review
	}
	if saved.SuggestTerms != nil {
		a.prefs.suggestTerms = *saved.SuggestTerms
	}
	a.prefs.reviewAuto = kcore.CleanProofAuto(kcore.ProofAuto(saved.ReviewAuto))
	if saved.TranslateSource != nil {
		a.prefs.translateSource = *saved.TranslateSource
	}
}

// settingsHooks: the settings window saves the speech model, the AI service and the translation, shows the interface in its saved language and appearance once the main window exists, looks for the whisper models on disk again when the tools change, and after every change opens for a settings page chosen in the main sidebar, gives back the trash confirmation's modal flag once it closed and fetches the AI service's models when its settings changed.
func (a *app) settingsHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			a.saveInterface(saved)
			a.saveSpeech(saved)
			a.saveLLM(saved)
			a.saveTranslate(saved)
			saved.ShortcutPreset = a.prefs.shortcutPreset
		},
		restore: func(saved savedSettings) {
			a.restoreInterface(saved)
			a.restoreSpeech(saved)
			a.restoreLLM(saved)
			a.restoreTranslate(saved)
			a.prefs.shortcutPreset = saved.ShortcutPreset
		},
		ready: func() {
			a.SetInterfaceLanguage(a.prefs.interfaceLanguage)
			a.cx.SetAppearance(a.settingsAppearance())
		},
		toolsChanged: func(redetected bool) {
			a.findSpeechModels()
			if redetected {
				a.everyAPIForgetOpen()
			}
		},
		afterChange: func() bool {
			page := a.followSettingsPage()
			modals := a.speechModals()
			models := a.loadLLMModels()
			return page || modals || models
		},
	}
}

// followSettingsPage opens the settings window at the settings page chosen in the main window's sidebar, whose 设置 rows stay until the sidebar drops them (P1-1), and puts the main window back on 全部影片, which no longer draws the settings. It reports whether it did.
func (a *app) followSettingsPage() bool {
	switch a.page {
	case pageGeneral, pageSpeech, pageAI, pageTranslate, pageAccount:
	default:
		return false
	}
	section := a.page
	a.Select(pageFilms)
	a.OpenSettings(section)
	return true
}

// settingsKeys are the settings' shortcuts.
func (a *app) settingsKeys(*core.Scope) []shortcut { return nil }

// settingsView is the settings window's content, built once under the window's scope cx. Both windows read the same state through the revision (package live), so what one changes shows in the other at once. The window's frame is followed for the next time it opens, and the window is forgotten as its scope goes, however it closes.
func (a *app) settingsView(cx *core.Scope) core.Node {
	w := nsdesktop.CurrentWindow(cx)
	cx.Cleanup(func() {
		a.settingsClosed(w)
		a.cx.Post(a.Changed)
	})
	cx.Effect(func() {
		if f := nsdesktop.Frame(cx); f.Width > 0 && f.Height > 0 {
			f.Animated = false
			a.prefs.frame = f
		}
	})
	theme.Apply(cx)
	// The window's controls sit in a band 50 high, as the prototype's settings rail has them: macOS's traffic lights at its start, the caption buttons NSGo draws at its end on Windows (minimize, and maximize disabled, as the window does not maximize), and on Linux, where GTK draws none in a hidden title bar, a round close button of GNOME's header bars at the end of the page's title (chrome.Close). It is a window of its own, which the system's switcher and taskbar list, so it keeps the window controls of its platform rather than the ✕ the design draws on a dialog over the main window. No material: the prototype's settings draw their rail and pages in plain colors, without a backdrop blur.
	nsdesktop.SetTitleBar(cx, nsdesktop.TitleBarHidden, 50)
	nsdesktop.SetWindowControls(cx, nsdesktop.WindowControls{TrafficLights: core.Point{X: 18, Y: 18}})
	cx.Effect(func() { nsdesktop.SetTitle(cx, locales.SettingsWindowTitle(cx)) })
	return a.settingsWindow(chrome.Close(cx))
}

// settingsWindow is the settings, with the 账号与上传 page of package account, the 插件 page, which reads the plugins' state (bind_plugins.go), 字幕外观's 逐词标注 card, which reads how the lines are marked (state_plugins_marks.go), and 翻译's 风格预设 row, which reads the translate-style packs (state_plugins_styles.go), in the settings window or over the main window. closeButton is the settings window's own close button where the system draws none, nil over the main window, whose dialog closes as dialogs do.
func (a *app) settingsWindow(closeButton core.Node) core.Node {
	return settings.Window{App: a, Version: appVersion, Account: account.View{App: a}, Plugins: settings.Plugins{App: a}, Marks: settings.Marks{App: a}, Styles: settings.Styles{App: a}, Sources: settings.Sources{App: a}, Close: closeButton}
}

// showSettingsWindow brings the settings window to the front, opening it the first time. It opens from a post under the app's scope, which owns it, rather than under the handler that asked, whose part of the window may go first, as the sidebar's gear goes when a film fills the window. Where the platform opens no windows of the app's, the settings show over the main window instead (settingsOverlay). Without the app's scope, as in tests that start no app, there is nothing to open.
func (a *app) showSettingsWindow() {
	if a.cx == nil || a.prefs.opening {
		return
	}
	if w := a.prefs.win; w != nil && !w.Closed() {
		w.Focus()
		return
	}
	a.prefs.opening = true
	a.cx.Post(func() {
		a.prefs.opening = false
		a.openSettingsWindow()
		a.Changed()
	})
}

// openSettingsWindow opens the settings window, a window of the app's own, or the settings over the main window where the platform opens none or the tests ask for that (overMain).
func (a *app) openSettingsWindow() {
	if a.prefs.overMain {
		a.prefs.inWindow = true
		return
	}
	// TODO(nsgo#108): open the settings as a child window of the main one, which moves, minimizes and comes forward with it, as MyGo's WindowOptions.Parent did https://github.com/nsgo-dev/nsgo/issues/108
	w := nsdesktop.OpenWindow(a.cx, nsdesktop.WindowOptions{
		Title:       locales.SettingsWindowTitle(a.cx),
		Width:       880,
		Height:      640,
		Key:         "settings",
		Frame:       a.prefs.frame,
		Constraints: core.WindowConstraints{MinWidth: 720, MinHeight: 480, NoMaximize: true},
	}, a.settingsView)
	if w == nil {
		a.prefs.inWindow = true
		return
	}
	a.prefs.win = w
}

// settingsClosed forgets the settings window w as it closes (settingsView), and the settings the user asked for with it.
func (a *app) settingsClosed(w *core.Window) {
	if a.prefs.win != w {
		return
	}
	a.prefs.win = nil
	a.prefs.open = false
	a.prefs.trashOpen = false
	a.speechModals()
}

// settingsOverlay shows the settings over the main window while they are open where the platform opened no window for them (openSettingsWindow): a dialog as large as the settings window, which Escape and a press beside it close.
func (a *app) settingsOverlay(*core.Scope) core.Node {
	return ui.Dialog{
		Open: live.Func(a, func() bool { return a.prefs.open && a.prefs.inWindow }),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.prefs.open, a.prefs.inWindow, a.prefs.trashOpen = false, false, false
				a.speechModals()
				a.Changed()
			}
		},
		Bare:     true,
		Style:    core.Style{Width: core.Pt(880), Height: core.Pt(640), MaxWidth: core.Pct(100), MaxHeight: core.Pct(100)},
		Children: []core.Node{a.settingsWindow(nil)},
		TestID:   "settings-in-window",
	}
}
