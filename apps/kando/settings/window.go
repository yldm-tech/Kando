package settings

import (
	"strings"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// EveryAPI is the state of the everyapi command-line program when the AI service is EveryAPI without an API Key, the zero value otherwise or when nothing is wrong with it. Missing is set when the program is not on this computer, Checking while Kando asks it whether someone is signed in, SignedOut when nobody is. Failed says what went wrong when asking it failed for another reason, as a timeout or a crash. OpenErr says why its website could not be opened.
type EveryAPI struct {
	Missing, Checking, SignedOut bool
	Failed, OpenErr              string
}

// Shortcut is one row of the app's shortcut table, as the 快捷键 page lists it: its group's title, what it does and its key in NSGo's form ("mod+,"), which the page writes as the platform does (core.Scope.KeyLabel).
type Shortcut struct {
	Group, Label, Key string
}

// App is the settings the window reads and the edits it reports. The 账号与上传 page is the account package's, which the window shows as Window.Account. The window reads the getters in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of the actions needs to call Changed itself.
type App interface {
	live.State

	// SettingsSection is the Section value the window shows.
	SettingsSection() string
	SetSettingsSection(section string)
	// SettingsQuery is the text of the window's search field.
	SettingsQuery() *string
	SaveErr() string
	// Save writes the settings to settings.json; when it cannot, SaveErr says so.
	Save() error

	LibraryDir() string
	KnownLibraries() []string
	// LibraryLabels names each of dirs as the library drop-down lists them, and DisplayPath writes a path as the page shows it, ~ for the home folder.
	LibraryLabels(dirs []string) []string
	DisplayPath(path string) string
	Choose(dir string)
	BeginNew()
	PickLibrary()
	Reveal(path string)
	Forget(dir string)
	Tools() (media.Tools, bool)
	Redetect()
	// ToolHealth is the copy of program the pipeline runs and whether it works, which its row of 组件 shows; UseComputerTool runs the computer's copy of it rather than Kando's own, or Kando's own again, and UseOwnTool runs Kando's own, downloading it when Kando has none or its copy does not run.
	ToolHealth(program string) ToolHealth
	UseComputerTool(program string, computer bool)
	UseOwnTool(program string)
	// ToolDownload is Kando's own download of program (media.Kit), which a row of 组件 offers while the computer has none; DownloadTool starts it.
	ToolDownload(program string) ToolFetch
	DownloadTool(program string)
	// CLI is where the kando command stands, which the 命令行与 MCP card shows; InstallCLI links it to this app off the UI thread, and CLICopied is told the text the card put on the clipboard.
	CLI() CLI
	InstallCLI()
	CLICopied(text string)
	// BrowserExtension is where the browser extension stands, which the 浏览器扩展 card shows; ConnectBrowsers registers Kando as its host with every browser found, DisconnectBrowsers unregisters it, and ShowExtensionFolder shows the extension's folder in the file manager, each off the UI thread.
	BrowserExtension() BrowserExtension
	ConnectBrowsers()
	DisconnectBrowsers()
	ShowExtensionFolder()
	// LinkStartAtOnce is the card's 链接到达后立即开始: a link the browser extension sends is added at once rather than on the Add sheet. SetLinkStartAtOnce turns it on or off; the page saves after it.
	LinkStartAtOnce() bool
	SetLinkStartAtOnce(on bool)
	// SignIn is the browser whose sign-in every download borrows and what the last check of it found, which the 浏览器登录 card shows. SetSignInBrowser chooses one, "" for none, saves it and checks its sign-in at once, off the UI thread, which on macOS asks the user to allow its key in the Keychain the first time; CheckSignIn checks it again. OpenFullDiskAccess opens the pane of System Settings that lets Kando read Safari's sign-in.
	SignIn() SignIn
	SetSignInBrowser(id string)
	CheckSignIn()
	OpenFullDiskAccess()
	// PlatformSubtitles is whether a link's film takes the subtitles its uploader put on the site as its original lines where they match the speech (core.SubtitlesPlatform), and DownloadQuality the picture its download asks for; the setters change them, and the page saves them.
	PlatformSubtitles() bool
	SetPlatformSubtitles(on bool)
	DownloadQuality() kcore.Quality
	SetDownloadQuality(q kcore.Quality)
	// ImportChapters is 导入平台章节, whether films whose site marks chapters are cut at them unless the user chose otherwise for one; SetImportChapters changes it, and the page saves it.
	ImportChapters() bool
	SetImportChapters(on bool)
	// PhoneSync is where Phone Sync stands, which the 手机同步 page shows. SetPhoneSync turns it on, which starts the server off the UI thread, or off, which stops it, and saves that; NewPhoneCode offers a new pairing code in place of the one shown; RevokePhone unpairs a phone, whose token is refused from then on. SetPhoneReceive turns 接收手机发来的链接 on or off, and SetPhoneAskFirst sets 收到后 to 先问我 or 直接加入队列; both are saved.
	PhoneSync() PhoneSync
	SetPhoneSync(on bool)
	NewPhoneCode()
	RevokePhone(id string)
	SetPhoneReceive(on bool)
	SetPhoneAskFirst(ask bool)
	// SetPhoneReadOnly makes a paired phone read-only (on), so it changes nothing on this computer from its next call on, or lets it change things again; it is kept with the device. ResolvePhoneConflict settles a sync conflict of the open library with the version keep names, as a change made on this computer now, which travels to the phones.
	SetPhoneReadOnly(id string, on bool)
	// SetPhoneRelay turns the storage relay on or off, saves that, and has a running Phone Sync take it up at once.
	SetPhoneRelay(on bool)
	ResolvePhoneConflict(id string, keep PhoneKeep)
	// PhoneNow is the time the page's relative times (刚刚, 5 分钟前) are told against. The app runs the bindings again as they come to say something else, so a binding that reads it follows time.
	PhoneNow() time.Time
	// CheckForUpdates checks for a newer version of Kando at once and shows what it finds.
	CheckForUpdates()
	// JobNotifications is whether Kando posts a notification when a video's job is done or failed while it is in the background; SetJobNotifications changes it.
	JobNotifications() bool
	SetJobNotifications(on bool)
	// Background is how Kando runs with its window closed, which the 后台运行 card shows. SetLaunchAtLogin has the system open Kando at login or no more, where it can; SetKeepRunning and SetTrayIcon change whether closing the window keeps Kando running and whether its status icon shows, which shows or goes at once.
	Background() Background
	SetLaunchAtLogin(on bool)
	SetKeepRunning(on bool)
	SetTrayIcon(on bool)

	// InstalledSpeechModel is the file on disk of the whisper catalog model id, "" when it is not downloaded. The app looks the models up when the window opens, when the model in use changes and on 重新检测, never while the page is drawn.
	InstalledSpeechModel(id string) string
	UseSpeechModel(path string)
	// PickSpeechModel asks for a whisper.cpp model file already on the computer with the system's file panel and uses the one picked, or says under the models why it cannot.
	PickSpeechModel()
	Downloading() string
	DownloadWritten() int64
	DownloadTotal() int64
	DownloadErr() string
	// DownloadMirror reports whether the download DownloadErr is about failed where hf-mirror.com may get the model, as when Hugging Face could not be reached or refused it; DownloadFromMirror makes hf-mirror.com the download source and downloads that model again from there.
	DownloadMirror() bool
	DownloadFromMirror()
	CancelDownload()
	DownloadModel(id string)
	// OwnsSpeechModel reports whether the file of the whisper catalog model id is one Kando downloaded into its own models folder, which it may move to the Trash; a model another app keeps is that app's. AskTrashSpeechModel opens the confirmation of moving it to the Trash, TrashingSpeechModel reports whether that confirmation is open and the model it asks about, CloseTrashSpeechModel closes it, giving back what it held, and TrashSpeechModel moves the file, refusing the model in use, another app's and any while a download runs.
	OwnsSpeechModel(id string) bool
	// SpeechModelInUse reports whether the model file path is one transcriptions use: the one chosen, or the one a running job started with. It cannot be moved to the Trash.
	SpeechModelInUse(path string) bool
	AskTrashSpeechModel(id string)
	TrashingSpeechModel() (open bool, id string)
	CloseTrashSpeechModel()
	TrashSpeechModel(id string)
	// SpeechLanguage is the language spoken in the videos as whisper's code, one of media.SpeechLanguages, or "" for whisper to detect it; SetSpeechLanguage changes and saves it.
	SpeechLanguage() string
	SetSpeechLanguage(code string)
	// SpeechSource is where the whisper models download from, one of media.SpeechSources: "" for Hugging Face; SetSpeechSource changes and saves it.
	SpeechSource() string
	SetSpeechSource(source string)
	// SpeechThreads is how many threads whisper recognizes speech with, 0 for as many as suit the computer, one of media.WhisperThreadChoices otherwise; SpeechHints the words it should spell as written, as typed; each setter changes and saves it. SpeechCommand is the whisper-cli command a job runs with them, the program and its arguments.
	SpeechThreads() int
	SetSpeechThreads(n int)
	SpeechHints() string
	SetSpeechHints(hints string)
	SpeechCommand() []string

	CurrentPreset() (Preset, bool)
	Presets() []Preset
	LLMProvider() string
	SetLLMProvider(id string)
	ProviderID(base string) string
	LLMBase() *string
	LLMKey() *string
	LLMModel() *string
	ApplyLLM()
	TestingLLM() bool
	TestLLM()
	// LLMConnected reports whether the last connection test passed; LLMResult is "" before any test, 连接成功 after one that passed, and otherwise why it failed, in plain words. The page words a pass itself, in the language it is shown in, so it reads LLMResult only when LLMConnected is false. LLMResultDetail is what the service answered when the test failed, "" otherwise.
	LLMConnected() bool
	LLMResult() string
	LLMResultDetail() string
	SelectLLM(id string)
	// OpenKeyPage opens the page of the service in use where an API Key is made (Preset.KeyURL) in the browser.
	OpenKeyPage()
	// LLMModels are the models the AI service offers, fetched while its page shows; LoadingLLMModels reports a fetch in flight, LLMModelsErr why the last one failed, and RetryLLMModels fetches again.
	LLMModels() []string
	LoadingLLMModels() bool
	LLMModelsErr() string
	RetryLLMModels()
	// EveryAPI is what the AI page says about the everyapi command-line program, whose sign-in EveryAPI uses without an API Key; GetEveryAPI opens EveryAPI's website, where the program is downloaded, and Redetect looks for it again.
	EveryAPI() EveryAPI
	GetEveryAPI()
	// LLMBatchSize is how many lines a translation request carries, LLMRetries how many times a failed one is sent again; SetLLMRequests changes both, kept to media's ranges, and saves them.
	LLMBatchSize() int
	LLMRetries() int
	SetLLMRequests(size, retries int)

	Language() *string
	TranslateContext() *bool
	TranslatePrompt() *string
	// TranslateContextLines is how many earlier lines go with a batch while TranslateContext is on; SetTranslateContextLines changes it, kept between MinContextLines and media.MaxContextLines, and Save keeps it.
	TranslateContextLines() int
	SetTranslateContextLines(n int)
	// Review is whether a job proofreads the translation as a whole once it is made (the final proofread); ReviewAuto is which of its changes apply without the user, one of core.ProofAutos, which SetReviewAuto changes. Save keeps them.
	Review() *bool
	ReviewAuto() string
	SetReviewAuto(auto string)

	// Appearance is the one the windows show, AppearanceLight or AppearanceDark, or "" to follow the system's; SetAppearance shows and saves it.
	Appearance() string
	SetAppearance(appearance string)

	// InterfaceLanguage is the app UI language tag, one of locales.Interface.
	// It is separate from Language, which is the language subtitles are translated into.
	InterfaceLanguage() string
	SetInterfaceLanguage(tag string)

	// Cue is the subtitle look the player draws with, which the 字幕外观 page previews and changes; Save keeps it.
	Cue() *subtitle.Look
	SubMode() string
	SetSubMode(mode string)
	SubOrder() string
	SetSubOrder(order string)
	Untranslated() string
	SetUntranslated(mode string)
	YieldSubs() bool
	SetYieldSubs(on bool)

	// Offline is whether offline mode is on, which the 隐私 page's switch turns; SetOffline turns it on or off and saves that. AILocal reports whether the AI service in use runs on this computer (media.ChatTranslator.Local), which offline mode lets through. ClearCache removes the rebuildable caches off the UI thread, and CacheClear is where that stands.
	Offline() bool
	SetOffline(on bool)
	AILocal() bool
	ClearCache()
	CacheClear() CacheClear

	// Shortcuts is the app's shortcut table, its words in the language the app shows under cx.
	Shortcuts(cx *core.Scope) []Shortcut
	// ShortcutQuery is the text of the 快捷键 page's search field.
	ShortcutQuery() *string
	// ShortcutPreset is the preset the player's keys follow, one of hotkey.Presets; SetShortcutPreset moves them at once and saves it.
	ShortcutPreset() string
	SetShortcutPreset(preset string)
}

// screen is what every page reads: the app, and the version 通用 shows.
type screen struct {
	a       App
	version string
}

// The rail's look, as the prototype's settings sidebar: 220 points wide with its line along the end, a translucent gray over the window's, and the search field's gray, from the theme (theme.Of).
const railWidth core.Points = 220

// Window is the settings window's content: the categories with a search field on the left, the chosen one's page on the right under its title. Account is the 账号与上传 page, the account package's. It fills the window it is in, the settings window of its own or the dialog over the main window where the platform opens none.
type Window struct {
	App     App
	Version string
	Account core.Node
	// Close is the window's own close button at the end of the page's title, for a settings window the system draws no controls in (Linux's); nil elsewhere and over the main window.
	Close core.Node
}

// Render implements core.Node.
func (w Window) Render(cx *core.Scope) core.Node {
	s := screen{a: w.App, version: w.Version}
	// The words the window's pieces take as plain strings, the accessibility labels and the buttons' among them, are read as it is built, so it is built again when the interface's language changes, which happens from the language drop-down, whose focus goes then anyway.
	return core.Dynamic(func(cx *core.Scope) core.Node {
		_ = i18n.Language(cx)
		return core.Func(func(cx *core.Scope) core.Node { return s.layout(cx, w.Account, w.Close) })
	})
}

// layout is the window in the language the app shows now.
func (s screen) layout(cx *core.Scope, account, closeButton core.Node) core.Node {
	a := s.a
	section := core.NewMemo(cx, live.Func(a, a.SettingsSection))
	// The page is built again when the category changes; within it, what changes with the app's state follows it in bindings, so a field being typed in keeps the focus.
	page := core.Dynamic(func(cx *core.Scope) core.Node {
		id := section.Get()
		return core.Func(func(cx *core.Scope) core.Node { return s.page(cx, id, account) })
	})
	saveErr := core.Show{When: live.Func(a, func() bool { return a.SaveErr() != "" }), Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		return core.Text{ContentFunc: live.Func(a, a.SaveErr), Style: core.Style{FontSize: 11, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	content := core.View{Style: core.Style{Width: core.Pct(100), Padding: core.Edges{Top: 14, Right: 28, Bottom: 28, Left: 28}, Gap: 6, AlignItems: core.AlignStretch},
		Children: []core.Node{page, saveErr}}
	pane := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), AlignItems: core.AlignStretch}, Children: []core.Node{
		header{a: a, section: section.Get, close: closeButton},
		core.Scroll{TestID: "settings-page", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{content}},
	}}
	// Its text is in the platform's font of the design (theme.FontOf), as the main window's is.
	k := theme.Of(cx)
	return core.View{TestID: "settings-window", Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, AlignItems: core.AlignStretch},
		StyleFunc: func() core.Style { return core.Style{Background: k.Window(), FontFamily: theme.FontOf(cx)} },
		Children:  []core.Node{rail{a: a}, pane}}
}

// header is the page's title bar: 50 points high with a line under it, the title 15-point bold where the page's content starts, and at its end the window's close button when it has one of its own (Window.Close), clear of the window controls the system draws there (desktop.TitleBar). It moves the window, as the title bar it stands in for does.
type header struct {
	a       App
	section func() string
	close   core.Node
}

// Render implements core.Node.
func (h header) Render(cx *core.Scope) core.Node {
	title := core.Text{TestID: "settings-title", Role: core.RoleHeader, MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
		ContentFunc: func() string { return Title(cx, h.section()) },
		Style:       core.Style{FontSize: 15, LineHeight: 18, FontWeight: ui.Bold}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Text)}
	title.Style.FlexGrow, title.Style.FlexShrink = 1, 1
	bar := core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		Style: core.Style{Direction: core.Row, Height: core.Pt(50), AlignItems: core.AlignCenter, Gap: 12},
		StyleFunc: func() core.Style {
			end := core.Points(28)
			if h.close != nil {
				end = 13
			}
			return core.Style{Padding: core.Edges{Left: 28, Right: end + nsdesktop.TitleBar(cx).Right}}
		},
		Children: []core.Node{title, h.close}}
	line := core.View{Style: core.Style{Height: core.Pt(1), FlexShrink: 0}, StyleFunc: ui.Filled(form.Line(cx))}
	return core.View{Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch}, Children: []core.Node{bar, line}}
}

// rail is the window's sidebar: room for the traffic lights, the search field, and the categories it finds.
type rail struct{ a App }

// Render implements core.Node.
func (r rail) Render(cx *core.Scope) core.Node {
	a := r.a
	// found is the sections the search finds, as their ids joined, so the list is built again only when that changes, not after every change of the app's state.
	found := core.NewMemo(cx, live.Func(a, func() string {
		var ids []string
		for _, sec := range sections {
			if sec.matches(cx, *a.SettingsQuery()) {
				ids = append(ids, sec.id)
			}
		}
		return strings.Join(ids, ",")
	}))
	list := ui.Sidebar{
		TestID: "settings-sections",
		SectionsFunc: func() []ui.SidebarSection {
			ids := found.Get()
			var items []ui.SidebarItem
			for _, sec := range sections {
				if strings.Contains(","+ids+",", ","+sec.id+",") {
					items = append(items, ui.SidebarItem{ID: sec.id, Text: sec.title(cx), Content: categoryContent(cx, sec), TestID: "settings-section-" + sec.id})
				}
			}
			return []ui.SidebarSection{{Items: items}}
		},
		Selected: live.Func(a, a.SettingsSection),
		OnSelect: live.On(a, func(id string) {
			if id != a.SettingsSection() {
				a.SetSettingsSection(id)
			}
		}),
	}
	none := core.Show{When: func() bool { return found.Get() == "" }, Then: func(cx *core.Scope) core.Node {
		return core.Text{Content: locales.SettingsSearchNone(cx), Style: core.Style{FontSize: 12, Padding: core.Symmetric(10, 8)}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
	}}
	// The traffic lights sit in the band at the top, which moves the window; past them, where the title bar says they end.
	top := core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn}, StyleFunc: func() core.Style {
		bar := nsdesktop.TitleBar(cx)
		return core.Style{Height: core.Pt(max(bar.Top, 50)), FlexShrink: 0}
	}}
	k := theme.Of(cx)
	return core.View{TestID: "settings-rail",
		Style: core.Style{Width: core.Pt(railWidth), FlexShrink: 0, AlignItems: core.AlignStretch, Padding: core.Edges{Right: 10, Bottom: 12, Left: 10}, Gap: 2, Borders: core.Edges{Right: 1}},
		StyleFunc: func() core.Style {
			return core.Style{Background: k.Rail(), BorderColors: core.EdgeColors{Right: k.Shade(0.1)}}
		},
		Children: []core.Node{top, search{a: a}, none, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{list}}}}
}

// categoryContent is a category of the rail: its colored tile and its title, as the prototype's settings sidebar draws it. ui.Sidebar marks the chosen one.
func categoryContent(cx *core.Scope, sec section) core.Node {
	colors := ui.ColorsOf(cx)
	tile := core.SVGView{SVG: sec.icon, Style: core.Style{Width: core.Pt(22), Height: core.Pt(22), FlexShrink: 0}}
	label := ui.Text(sec.title(cx), 13, ui.Regular, colors.Text)
	label.MaxLines, label.Style.FlexGrow, label.Style.FlexShrink = 1, 1, 1
	return core.Group([]core.Node{tile, label})
}

// search is the rail's search field, which narrows the categories to those whose title or keywords hold what is typed: 28 points high, its corners 8, gray without a border, a magnifying glass before the text and a clear button after it while it holds something. The button or Escape empties it.
type search struct{ a App }

// Render implements core.Node.
func (s search) Render(cx *core.Scope) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	query := live.Func(a, func() string { return *a.SettingsQuery() })
	field := &core.Ref{}
	// Emptying the field, by its clear button or Escape, also takes the keyboard out of it, as MyGo's SearchField did.
	clear := live.Do(a, func() {
		*a.SettingsQuery() = ""
		field.Blur()
	})
	focused := core.NewSignal(cx, false)
	escape := core.Show{When: func() bool { return focused.Get() && query() != "" }, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, clear)
		return nil
	}}
	// TODO(nsgo#144): give core.Input a search role, so assistive technology reads the field as a search field as MyGo's SearchField was https://github.com/nsgo-dev/nsgo/issues/144
	input := base.Input{Ref: field, TestID: "settings-search",
		Label:       locales.SettingsSearchLabel(cx),
		Placeholder: locales.SettingsSearchPlaceholder(cx),
		Value:       query,
		OnChange:    live.On(a, func(text string) { *a.SettingsQuery() = text }),
		OnFocus:     func() { focused.Set(true) },
		OnBlur:      func() { focused.Set(false) },
		Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0), Height: core.Pt(28), CornerRadius: 8, FontSize: 13,
			Padding: core.Edges{Left: 28, Right: 28}},
		StyleFunc: func() core.Style {
			return core.Style{Background: theme.Of(cx).Shade(0.06), Color: colors.Text(), PlaceholderColor: colors.Faint(), CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection}
		},
	}
	clearButton := core.Show{When: func() bool { return query() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(4), Right: core.Pt(4)}, Children: []core.Node{
			ui.IconButton{Icon: lucide.X, LabelFunc: func() string { return locales.SettingsSearchClear(cx) }, Size: 20, OnPress: clear, TestID: "settings-search-clear"},
		}}
	}}
	glass := core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(7), Left: core.Pt(9)},
		Children: []core.Node{ui.Icon(lucide.Search, 14, colors.Muted)}}
	return core.View{Style: core.Style{FlexShrink: 0, Margin: core.Edges{Bottom: 8}}, Children: []core.Node{input, glass, clearButton, escape}}
}

// page builds the category id: the account package's page for 账号与上传, which the window is given, and its own pages for the others.
func (s screen) page(cx *core.Scope, id string, account core.Node) core.Node {
	switch id {
	case SectionSpeech:
		return speech{s}
	case SectionAI:
		return ai{s}
	case SectionTranslate:
		return translate{s}
	case SectionLook:
		return look{s}
	case SectionPrivacy:
		return privacy{s}
	case SectionShortcuts:
		return shortcuts{s}
	case SectionPhone:
		return phone{s}
	case SectionAccount:
		if account != nil {
			return account
		}
		return core.Group(nil)
	default:
		return general{s}
	}
}
