package settings

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeApp is the settings the window reads, kept as plain fields as the app keeps them, with the calls the window made.
type fakeApp struct {
	live.Rev
	section, query, saveErr string
	dir                     string
	libraries               []string
	tools                   media.Tools
	known                   bool
	versions                map[string]string
	fetch                   map[string]ToolFetch
	cli                     CLI
	ext                     BrowserExtension
	signIn                  SignIn
	phone                   PhoneSync
	background              Background
	notify                  bool
	heardAlone              bool
	quality                 kcore.Quality
	evenChapters            bool
	models                  map[string]string
	owned                   map[string]bool
	downloading             string
	written, total          int64
	downloadErr             string
	downloadMirror          bool
	trashOpen               bool
	trashID                 string
	spoken                  string
	source                  string
	provider                string
	base, key, model        string
	testing, connected      bool
	result, resultDetail    string
	llmModels               []string
	loadingModels           bool
	modelsErr               string
	everyAPI                EveryAPI
	language                string
	context                 bool
	prompt                  string
	interfaceLang           string
	cue                     subtitle.Look
	subMode, subOrder       string
	untranslated            string
	yield                   bool
	shortcutQuery           string
	offline, aiLocal        bool
	cacheClear              CacheClear
	calls                   []string
}

func newFakeApp() *fakeApp {
	return &fakeApp{
		section:       SectionGeneral,
		dir:           "/films/我的影片.kando",
		libraries:     []string{"/films/我的影片.kando"},
		tools:         media.Tools{FFmpeg: "/opt/bin/ffmpeg", Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL}},
		known:         true,
		versions:      map[string]string{"/opt/bin/ffmpeg": "8.0"},
		fetch:         map[string]ToolFetch{"yt-dlp": {Available: true}},
		models:        map[string]string{"base": "/models/ggml-base.bin"},
		owned:         map[string]bool{"base": true},
		provider:      "everyapi",
		base:          media.EveryAPIBaseURL,
		language:      "简体中文",
		quality:       kcore.QualityBest,
		context:       true,
		prompt:        media.DefaultSubtitlePrompt,
		interfaceLang: locales.Default,
		subMode:       subtitle.Bilingual,
		subOrder:      subtitle.TranslationFirst,
		untranslated:  subtitle.UntranslatedDim,
		yield:         true,
	}
}

func (f *fakeApp) call(s string) { f.calls = append(f.calls, s) }

func (f *fakeApp) SettingsSection() string     { return f.section }
func (f *fakeApp) SetSettingsSection(s string) { f.call("section " + s); f.section = s }
func (f *fakeApp) SettingsQuery() *string      { return &f.query }
func (f *fakeApp) SaveErr() string             { return f.saveErr }
func (f *fakeApp) Save() error                 { f.call("save"); return nil }
func (f *fakeApp) LibraryDir() string          { return f.dir }
func (f *fakeApp) KnownLibraries() []string    { return f.libraries }
func (f *fakeApp) LibraryLabels(dirs []string) []string {
	out := make([]string, len(dirs))
	for i, dir := range dirs {
		out[i] = strings.TrimSuffix(filepath.Base(dir), ".kando")
	}
	return out
}
func (f *fakeApp) DisplayPath(path string) string        { return path }
func (f *fakeApp) Choose(dir string)                     { f.call("choose " + dir); f.dir = dir }
func (f *fakeApp) BeginNew()                             { f.call("new") }
func (f *fakeApp) PickLibrary()                          { f.call("pick") }
func (f *fakeApp) Reveal(path string)                    { f.call("reveal " + path) }
func (f *fakeApp) Forget(dir string)                     { f.call("forget " + dir) }
func (f *fakeApp) Tools() (media.Tools, bool)            { return f.tools, f.known }
func (f *fakeApp) Redetect()                             { f.call("redetect") }
func (f *fakeApp) ToolVersion(path string) string        { return f.versions[path] }
func (f *fakeApp) ToolDownload(program string) ToolFetch { return f.fetch[program] }
func (f *fakeApp) DownloadTool(program string)           { f.call("download " + program) }
func (f *fakeApp) CLI() CLI                              { return f.cli }
func (f *fakeApp) InstallCLI()                           { f.call("install cli") }
func (f *fakeApp) CLICopied(text string)                 { f.cli.Copied = text }
func (f *fakeApp) BrowserExtension() BrowserExtension    { return f.ext }
func (f *fakeApp) ConnectBrowsers()                      { f.call("connect browsers") }
func (f *fakeApp) DisconnectBrowsers()                   { f.call("disconnect browsers") }
func (f *fakeApp) ShowExtensionFolder()                  { f.call("show extension folder") }
func (f *fakeApp) SignIn() SignIn                        { return f.signIn }
func (f *fakeApp) SetSignInBrowser(id string)            { f.call("sign in with " + id); f.signIn.Browser = id }
func (f *fakeApp) CheckSignIn()                          { f.call("check sign-in") }
func (f *fakeApp) OpenFullDiskAccess()                   { f.call("full disk access") }
func (f *fakeApp) CheckForUpdates()                      { f.call("check updates") }
func (f *fakeApp) PhoneSync() PhoneSync                  { return f.phone }
func (f *fakeApp) SetPhoneSync(on bool) {
	f.call("phone sync " + strconv.FormatBool(on))
	f.phone.On = on
}
func (f *fakeApp) NewPhoneCode()         { f.call("new phone code") }
func (f *fakeApp) RevokePhone(id string) { f.call("revoke " + id) }
func (f *fakeApp) SetPhoneReceive(on bool) {
	f.phone.Receive = on
	f.call(fmt.Sprintf("receive %v", on))
}
func (f *fakeApp) SetPhoneAskFirst(ask bool) {
	f.phone.AskFirst = ask
	f.call(fmt.Sprintf("ask first %v", ask))
}
func (f *fakeApp) JobNotifications() bool                { return f.notify }
func (f *fakeApp) SetJobNotifications(on bool)           { f.notify = on }
func (f *fakeApp) Background() Background                { return f.background }
func (f *fakeApp) SetLaunchAtLogin(on bool)              { f.background.LaunchAtLogin = on }
func (f *fakeApp) SetKeepRunning(on bool)                { f.background.KeepRunning = on }
func (f *fakeApp) SetTrayIcon(on bool)                   { f.background.TrayIcon = on }
func (f *fakeApp) PlatformSubtitles() bool               { return !f.heardAlone }
func (f *fakeApp) SetPlatformSubtitles(on bool)          { f.heardAlone = !on }
func (f *fakeApp) DownloadQuality() kcore.Quality        { return f.quality }
func (f *fakeApp) SetDownloadQuality(q kcore.Quality)    { f.quality = q }
func (f *fakeApp) ImportChapters() bool                  { return !f.evenChapters }
func (f *fakeApp) SetImportChapters(on bool)             { f.evenChapters = !on }
func (f *fakeApp) InstalledSpeechModel(id string) string { return f.models[id] }
func (f *fakeApp) UseSpeechModel(path string)            { f.call("use " + path); f.tools.WhisperModel = path }
func (f *fakeApp) PickSpeechModel()                      { f.call("pick model") }
func (f *fakeApp) Downloading() string                   { return f.downloading }
func (f *fakeApp) DownloadWritten() int64                { return f.written }
func (f *fakeApp) DownloadTotal() int64                  { return f.total }
func (f *fakeApp) DownloadErr() string                   { return f.downloadErr }
func (f *fakeApp) DownloadMirror() bool                  { return f.downloadMirror }
func (f *fakeApp) DownloadFromMirror()                   { f.call("download from mirror") }
func (f *fakeApp) CancelDownload()                       { f.call("cancel download") }
func (f *fakeApp) DownloadModel(id string)               { f.call("download model " + id) }
func (f *fakeApp) OwnsSpeechModel(id string) bool        { return f.owned[id] }
func (f *fakeApp) SpeechModelInUse(path string) bool     { return path == f.tools.WhisperModel }
func (f *fakeApp) AskTrashSpeechModel(id string)         { f.trashOpen, f.trashID = true, id }
func (f *fakeApp) TrashingSpeechModel() (bool, string)   { return f.trashOpen, f.trashID }
func (f *fakeApp) CloseTrashSpeechModel()                { f.trashOpen = false }
func (f *fakeApp) TrashSpeechModel(id string)            { f.call("trash " + id) }
func (f *fakeApp) SpeechLanguage() string                { return f.spoken }
func (f *fakeApp) SetSpeechLanguage(code string)         { f.spoken = code }
func (f *fakeApp) SpeechSource() string                  { return f.source }
func (f *fakeApp) SetSpeechSource(source string)         { f.source = source }
func (f *fakeApp) CurrentPreset() (Preset, bool) {
	if preset, ok := PresetByID(testlocale.Messages(), f.provider); ok {
		return preset, true
	}
	return Preset{ID: f.provider, Name: f.provider}, true
}
func (f *fakeApp) Presets() []Preset {
	return []Preset{{ID: "everyapi", Name: "EveryAPI", Group: GroupCloud}, {ID: "ollama", Name: "Ollama", Group: GroupLocal}}
}
func (f *fakeApp) LLMProvider() string             { return f.provider }
func (f *fakeApp) SetLLMProvider(id string)        { f.provider = id }
func (f *fakeApp) ProviderID(string) string        { return "custom" }
func (f *fakeApp) LLMBase() *string                { return &f.base }
func (f *fakeApp) LLMKey() *string                 { return &f.key }
func (f *fakeApp) LLMModel() *string               { return &f.model }
func (f *fakeApp) ApplyLLM()                       { f.call("apply") }
func (f *fakeApp) TestingLLM() bool                { return f.testing }
func (f *fakeApp) TestLLM()                        { f.call("test") }
func (f *fakeApp) LLMConnected() bool              { return f.connected }
func (f *fakeApp) LLMResult() string               { return f.result }
func (f *fakeApp) LLMResultDetail() string         { return f.resultDetail }
func (f *fakeApp) SelectLLM(id string)             { f.call("select " + id); f.provider = id }
func (f *fakeApp) OpenKeyPage()                    { f.call("key page " + f.provider) }
func (f *fakeApp) LLMModels() []string             { return f.llmModels }
func (f *fakeApp) LoadingLLMModels() bool          { return f.loadingModels }
func (f *fakeApp) LLMModelsErr() string            { return f.modelsErr }
func (f *fakeApp) RetryLLMModels()                 { f.call("retry models") }
func (f *fakeApp) EveryAPI() EveryAPI              { return f.everyAPI }
func (f *fakeApp) GetEveryAPI()                    { f.call("get everyapi") }
func (f *fakeApp) Language() *string               { return &f.language }
func (f *fakeApp) TranslateContext() *bool         { return &f.context }
func (f *fakeApp) TranslatePrompt() *string        { return &f.prompt }
func (f *fakeApp) InterfaceLanguage() string       { return f.interfaceLang }
func (f *fakeApp) SetInterfaceLanguage(tag string) { f.call("language " + tag); f.interfaceLang = tag }
func (f *fakeApp) Cue() *subtitle.Look             { return &f.cue }
func (f *fakeApp) SubMode() string                 { return f.subMode }
func (f *fakeApp) SetSubMode(mode string)          { f.subMode = mode }
func (f *fakeApp) SubOrder() string                { return f.subOrder }
func (f *fakeApp) SetSubOrder(order string)        { f.subOrder = order }
func (f *fakeApp) Untranslated() string            { return f.untranslated }
func (f *fakeApp) SetUntranslated(mode string)     { f.untranslated = mode }
func (f *fakeApp) YieldSubs() bool                 { return f.yield }
func (f *fakeApp) SetYieldSubs(on bool)            { f.yield = on }
func (f *fakeApp) ShortcutQuery() *string          { return &f.shortcutQuery }
func (f *fakeApp) Offline() bool                   { return f.offline }
func (f *fakeApp) SetOffline(on bool)              { f.call("offline " + strconv.FormatBool(on)); f.offline = on }
func (f *fakeApp) AILocal() bool                   { return f.aiLocal }
func (f *fakeApp) ClearCache()                     { f.call("clear cache"); f.cacheClear = CacheClear{Busy: true} }
func (f *fakeApp) CacheClear() CacheClear          { return f.cacheClear }
func (f *fakeApp) Shortcuts(cx *core.Scope) []Shortcut {
	return []Shortcut{{Group: "看懂", Label: "设置…", Key: "mod+,"}, {Group: "显示", Label: "全屏", Key: "f"}, {Group: "显示", Label: "返回资料库", Key: "escape"}}
}

// start shows the settings window of a, 880 by 640 as it opens, on a Mac, in testlocale's language. account stands for the 账号与上传 page.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	return startOn(t, a, core.OSMacOS)
}

// startOn is start on the desktop of os, with that system's words for its file manager and keys.
func startOn(t *testing.T, a *fakeApp, os core.OS) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop, AppleKeyboard: os == core.OSMacOS})
	account := core.Text{TestID: "account-page", Content: "account"}
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{Window{App: a, Version: "1.2.3", Account: account}}}
	}, nsgotest.Size(880, 640), nsgotest.Environment(env))
}

// change runs fn on the app's state as a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func whole(s string) string { return `text:"` + s + `"` }

func TestWindowListsTheSectionsAndSwitches(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, title := range []string{msgs.SettingsSectionGeneral(), msgs.SettingsSectionSpeech(), msgs.SettingsSectionAI(), msgs.SettingsSectionTranslate(), msgs.SettingsSectionLook(), msgs.SettingsSectionShortcuts(), msgs.SettingsSectionAccount()} {
		if !app.Exists(whole(title)) {
			t.Fatalf("no %q in %q", title, app.Texts())
		}
	}
	if got := app.Find("id:settings-title").Text; got != msgs.SettingsSectionGeneral() {
		t.Fatalf("title %q", got)
	}
	app.Press("id:settings-section-" + SectionTranslate)
	if a.section != SectionTranslate || !slices.Contains(a.calls, "section "+SectionTranslate) {
		t.Fatalf("section %q, calls %q", a.section, a.calls)
	}
	if got := app.Find("id:settings-title").Text; got != msgs.SettingsSectionTranslate() || !app.Exists(whole(msgs.SettingsTranslateTarget())) {
		t.Fatalf("title %q, texts %q", got, app.Texts())
	}
	app.Press("id:settings-section-" + SectionAccount)
	if !app.Exists("id:account-page") {
		t.Fatal("the account page is not the one the window was given")
	}
}

func TestSearchNarrowsTheSections(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Type("id:settings-search", "bucket")
	if !app.Exists("id:settings-section-"+SectionAccount) || app.Exists("id:settings-section-"+SectionLook) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Type("id:settings-search", "没有这种设置")
	if !app.Exists(whole(msgs.SettingsSearchNone())) || app.Exists("id:settings-section-"+SectionAccount) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Key("escape")
	if a.query != "" || app.Exists(whole(msgs.SettingsSearchNone())) {
		t.Fatalf("Escape left %q", a.query)
	}
	if app.Exists("id:settings-search-clear") {
		t.Fatal("the empty field shows its clear button")
	}
	app.Type("id:settings-search", "bucket")
	app.Press("id:settings-search-clear")
	if a.query != "" || !app.Exists("id:settings-section-"+SectionLook) || app.Exists("id:settings-search-clear") {
		t.Fatalf("the clear button left %q", a.query)
	}
}

// TestFieldsStayWhileTheAppChanges types into Base URL, then changes the app's state the field does not show, as every report does: the field stays the same view, so it keeps the focus and what was typed.
func TestFieldsStayWhileTheAppChanges(t *testing.T) {
	a := newFakeApp()
	a.section = SectionAI
	app := start(t, a)
	app.Focus("id:settings-llm-base")
	app.Type("id:settings-llm-base", "/v2")
	field := app.Find("id:settings-llm-base")
	if a.base != media.EveryAPIBaseURL+"/v2" || !slices.Contains(a.calls, "apply") || !field.Focused {
		t.Fatalf("base %q, calls %q, focused %v", a.base, a.calls, field.Focused)
	}
	change(app, a, func() { a.written = 10 })
	change(app, a, func() {})
	again := app.Find("id:settings-llm-base")
	if again.ID != field.ID || !again.Focused || again.Text != a.base {
		t.Fatalf("the field was built again (%d, then %d) or lost the focus (%v): %q", field.ID, again.ID, again.Focused, again.Text)
	}
}

func TestWindowFollowsTheLanguage(t *testing.T) {
	a := newFakeApp()
	var root *core.Scope
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		a.Attach(cx)
		return Window{App: a, Version: "1.2.3"}
	}, nsgotest.Size(880, 640), nsgotest.Environment(env))
	app.Press("id:settings-interface-language")
	app.Press("id:settings-interface-language-0")
	if a.interfaceLang != "en" || !slices.Contains(a.calls, "save") {
		t.Fatalf("language %q, calls %q", a.interfaceLang, a.calls)
	}
	root.SetLocale("en")
	app.Settle()
	en := locales.In("en")
	for _, text := range []string{en.SettingsSectionGeneral(), en.SettingsGeneralInterfaceLanguage(), en.UpdatesCheck()} {
		if !app.Exists(whole(text)) {
			t.Fatalf("no %q in English: %q", text, app.Texts())
		}
	}
	if got := app.Find("id:settings-search").Input.Placeholder; got != en.SettingsSearchPlaceholder() {
		t.Fatalf("the search field's placeholder is %q", got)
	}
}

// TestPageScrollsWithinTheWindow checks that a page longer than the window scrolls inside it: NSGo's views keep the height of their content unless they may shrink, so a window whose boxes only grew would stretch past its bottom and leave nothing to scroll.
func TestPageScrollsWithinTheWindow(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	page := app.Find("id:settings-page")
	if page.Frame.Size.Height >= 640 || page.Content.Height <= page.Frame.Size.Height {
		t.Fatalf("the page is %v high in a window 640 high, its content %v", page.Frame.Size.Height, page.Content.Height)
	}
	app.Scroll("id:settings-page", 4000)
	if page := app.Find("id:settings-page"); page.Offset.Y <= 0 {
		t.Fatalf("the page did not scroll: offset %v", page.Offset)
	}
}

// TestGeneralPageLinksWhereToGetAProgram checks that a program Kando cannot download, here whisper-cli, links to the page it is got from, which one it downloads, yt-dlp, does not need; and that where Kando downloads none of them, as on Windows, the footnote says how to install them all with Scoop.
func TestGeneralPageLinksWhereToGetAProgram(t *testing.T) {
	app := start(t, newFakeApp())
	msgs := testlocale.Messages()
	if !app.Exists("id:settings-tool-page-whisper-cli") || app.Exists("id:settings-tool-page-yt-dlp") {
		t.Fatalf("download page links: whisper-cli %v, yt-dlp %v", app.Exists("id:settings-tool-page-whisper-cli"), app.Exists("id:settings-tool-page-yt-dlp"))
	}
	if !app.Exists(whole(msgs.SettingsGeneralToolPage())) {
		t.Fatalf("no %q in %q", msgs.SettingsGeneralToolPage(), app.Texts())
	}
	if _, kits := media.KitFor("ffmpeg"); !kits {
		if note := msgs.SettingsGeneralToolsNoteScoop(scoopCommand()); scoopCommand() == "" || !app.Exists(whole(note)) {
			t.Fatalf("no %q in %q", note, app.Texts())
		}
	}
}

func TestGeneralPage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, text := range []string{"我的影片", "/films/我的影片.kando", "1.2.3", "8.0", msgs.SettingsGeneralDownload(), msgs.SettingsGeneralNotInstalled()} {
		if !app.Exists(whole(text)) {
			t.Fatalf("missing %q in %q", text, app.Texts())
		}
	}
	if app.Exists(whole(msgs.SettingsGeneralForget())) {
		t.Fatal("the only library can be taken off the list")
	}
	app.Press("id:settings-check-updates")
	app.Press("id:settings-tool-download-yt-dlp")
	app.Press("id:settings-redetect")
	app.Press("id:settings-library-reveal")
	for _, call := range []string{"check updates", "download yt-dlp", "redetect", "reveal /films/我的影片.kando"} {
		if !slices.Contains(a.calls, call) {
			t.Fatalf("no %q in %q", call, a.calls)
		}
	}
	// A download under way shows its progress; one that failed offers 重试.
	change(app, a, func() { a.fetch["yt-dlp"] = ToolFetch{Available: true, Running: true, Progress: 0.25} })
	if v := app.Find("id:settings-tool-progress-yt-dlp"); v == nil || !app.Exists(whole(msgs.SettingsGeneralToolDetail(msgs.SettingsGeneralUseYTDLP(), msgs.SettingsGeneralDownloading()))) {
		t.Fatalf("no progress: %q", app.Texts())
	}
	change(app, a, func() { a.fetch["yt-dlp"] = ToolFetch{Available: true, Err: "服务器返回 HTTP 503"} })
	if !app.Exists(whole(msgs.SettingsGeneralRetry())) || !app.Exists(whole(msgs.SettingsGeneralToolDetail(msgs.SettingsGeneralUseYTDLP(), msgs.SettingsGeneralDownloadFailed("服务器返回 HTTP 503")))) {
		t.Fatalf("no 重试: %q", app.Texts())
	}
	// A second library can be chosen, and the one open taken off the list.
	change(app, a, func() { a.libraries = append(a.libraries, "/films/课程.kando") })
	app.Press("id:settings-library")
	app.Press(whole("课程"))
	if a.dir != "/films/课程.kando" {
		t.Fatalf("library %q, calls %q", a.dir, a.calls)
	}
	app.Press("id:settings-library-forget")
	if !slices.Contains(a.calls, "forget /films/课程.kando") {
		t.Fatalf("calls %q", a.calls)
	}
	// Without a library, the rows offer to make or open one.
	change(app, a, func() { a.dir = "" })
	app.Press("id:settings-library-new")
	app.Press("id:settings-library-open")
	if !slices.Contains(a.calls, "new") || !slices.Contains(a.calls, "pick") {
		t.Fatalf("calls %q", a.calls)
	}
}

// 在线视频 turns the site's subtitles off and on, chooses the picture downloads ask for, 仅字幕 with a note of what it keeps, and turns the site's chapters off and on, saving each change.
func TestGeneralPageChoosesHowLinksDownload(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, text := range []string{msgs.SettingsGeneralOnline(), msgs.SettingsGeneralPlatformSubtitles(), msgs.SettingsGeneralQualityBest()} {
		if !app.Exists(whole(text)) {
			t.Fatalf("missing %q in %q", text, app.Texts())
		}
	}
	saves := func() int {
		n := 0
		for _, call := range a.calls {
			if call == "save" {
				n++
			}
		}
		return n
	}
	app.Press("id:settings-platform-subtitles")
	if !a.heardAlone || saves() != 1 {
		t.Fatalf("heard alone %v, saves %d", a.heardAlone, saves())
	}
	app.Press("id:settings-platform-subtitles")
	if a.heardAlone {
		t.Fatal("the site's subtitles stayed off")
	}
	app.Press("id:settings-download-quality")
	app.Press("role:option " + msgs.SettingsGeneralQuality720())
	if a.quality != kcore.Quality720 || saves() != 3 {
		t.Fatalf("quality %q, saves %d", a.quality, saves())
	}
	if !app.Exists(whole(msgs.SettingsGeneralChapters())) {
		t.Fatalf("missing %q in %q", msgs.SettingsGeneralChapters(), app.Texts())
	}
	app.Press("id:settings-import-chapters")
	if !a.evenChapters || saves() != 4 {
		t.Fatalf("even chapters %v, saves %d", a.evenChapters, saves())
	}
	app.Press("id:settings-import-chapters")
	if a.evenChapters || saves() != 5 {
		t.Fatalf("chapters stayed off, saves %d", saves())
	}
	// 仅字幕 keeps no video, which the row says under it while it is chosen.
	if app.Exists(whole(msgs.SettingsGeneralQualitySubtitlesHint())) {
		t.Fatal("the note of 仅字幕 shows for 720p")
	}
	app.Press("id:settings-download-quality")
	app.Press("role:option " + msgs.SettingsGeneralQualitySubtitles())
	if a.quality != kcore.QualitySubtitles || saves() != 6 {
		t.Fatalf("quality %q, saves %d", a.quality, saves())
	}
	// The note says how the film is watched without its video: streamed where this computer streams films, else after downloading its video.
	hint, other := msgs.SettingsGeneralQualitySubtitlesHint(), msgs.SettingsGeneralQualitySubtitlesHintNoStream()
	if !media.StreamsHere() {
		hint, other = other, hint
	}
	if !app.Exists(whole(hint)) || app.Exists(whole(other)) {
		t.Fatalf("missing %q in %q", hint, app.Texts())
	}
}

func TestCLICardCopies(t *testing.T) {
	a := newFakeApp()
	a.cli = CLI{Known: true, Installable: true, Exe: "/Applications/Kando.app/Contents/MacOS/Kando", MCPConfig: `{"command": "kando"}`, ClaudeCommand: "claude mcp add kando -- kando mcp"}
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:settings-cli-install")
	if !slices.Contains(a.calls, "install cli") {
		t.Fatalf("calls %q", a.calls)
	}
	app.Press("id:settings-cli-copy-config")
	if app.Clipboard != a.cli.MCPConfig || !app.Exists(whole(msgs.SettingsGeneralCLICopied())) {
		t.Fatalf("clipboard %q, texts %q", app.Clipboard, app.Texts())
	}
	app.Press("id:settings-cli-copy-command")
	if app.Clipboard != a.cli.ClaudeCommand {
		t.Fatalf("clipboard %q", app.Clipboard)
	}
	// While Kando runs from its disk image there is nothing to install or copy.
	change(app, a, func() { a.cli.Temporary, a.cli.MCPConfig, a.cli.ClaudeCommand = true, "", "" })
	if app.Exists("id:settings-cli-install") || app.Enabled("id:settings-cli-copy-config") || !app.Exists(whole(msgs.SettingsGeneralCLIMoveFirst())) {
		t.Fatalf("texts %q", app.Texts())
	}
}

// TestModelMenuNamesTheFileManager checks that ⋯ after a downloaded model offers to show its file in the file manager of the system Kando runs on, as every other such item does, rather than in Finder everywhere.
func TestModelMenuNamesTheFileManager(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		os       core.OS
		platform string
	}{{core.OSMacOS, "macOS"}, {core.OSWindows, "Windows"}, {core.OSLinux, "Linux"}} {
		t.Run(tc.platform, func(t *testing.T) {
			a := newFakeApp()
			a.section = SectionSpeech
			app := startOn(t, a, tc.os)
			app.Press("id:settings-model-more-base")
			if got := app.PopupMenu; len(got) == 0 || got[0].Label != msgs.SettingsSpeechReveal(tc.platform) {
				t.Fatalf("menu %+v, want %q first", got, msgs.SettingsSpeechReveal(tc.platform))
			}
		})
	}
	if msgs.SettingsSpeechReveal("Windows") == msgs.SettingsSpeechReveal("macOS") {
		t.Fatal("Windows is told of Finder")
	}
}

func TestSpeechPage(t *testing.T) {
	a := newFakeApp()
	a.section = SectionSpeech
	app := start(t, a)
	msgs := testlocale.Messages()
	// The recommended model, Small and Base are listed, with the others under 更多模型 until one of them is on disk.
	if !app.Exists("id:settings-model-row-large-v3-turbo-q5") || !app.Exists("id:settings-model-row-small") || app.Exists("id:settings-model-row-tiny") || app.Exists("id:settings-model-row-medium") {
		t.Fatalf("listed: %q", app.Texts())
	}
	app.Press(msgs.SettingsSpeechMoreModels())
	app.Advance(time.Second)
	if app.Enabled("id:settings-model-tiny") || !app.Enabled("id:settings-model-base") {
		t.Fatal("a model not on disk can be chosen, or one on disk cannot")
	}
	app.Press("id:settings-model-base")
	if a.tools.WhisperModel != "/models/ggml-base.bin" {
		t.Fatalf("model %q, calls %q", a.tools.WhisperModel, a.calls)
	}
	app.Press("id:settings-model-download-tiny")
	if !slices.Contains(a.calls, "download model tiny") {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() { a.downloading, a.written, a.total = "tiny", 50, 100 })
	if app.Find("id:settings-model-progress-tiny") == nil || app.Enabled("id:settings-model-download-small") {
		t.Fatal("no progress, or another download can start meanwhile")
	}
	app.Press("id:settings-model-cancel-tiny")
	if !slices.Contains(a.calls, "cancel download") {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() { a.downloading = "" })
	// ⋯ offers the model in use no move to the Trash; another one Kando downloaded can go there once the confirmation says so, which Enter answers 取消.
	app.Press("id:settings-model-more-base")
	if got := app.PopupMenu; len(got) != 3 || !got[2].Disabled {
		t.Fatalf("menu %+v", got)
	}
	app.ClosePopup()
	change(app, a, func() { a.tools.WhisperModel = "" })
	app.Press("id:settings-model-more-base")
	app.ChoosePopup(msgs.SettingsSpeechTrash())
	if !a.trashOpen || !app.Exists(whole(msgs.SettingsSpeechTrashConfirm("Base"))) {
		t.Fatalf("no confirmation: %q", app.Texts())
	}
	app.Key("enter")
	if a.trashOpen || slices.Contains(a.calls, "trash base") {
		t.Fatalf("Enter trashed the model: open %v, calls %q", a.trashOpen, a.calls)
	}
	change(app, a, func() { a.AskTrashSpeechModel("base") })
	app.Press("id:settings-trash-ok")
	if a.trashOpen || !slices.Contains(a.calls, "trash base") {
		t.Fatalf("open %v, calls %q", a.trashOpen, a.calls)
	}
	change(app, a, func() { a.downloadErr = "连接断了。" })
	if !app.Exists(whole("连接断了。")) {
		t.Fatalf("texts %q", app.Texts())
	}
	// Once on disk, a model of 更多模型 is listed with the others, and 更多模型 goes when none is left there.
	change(app, a, func() {
		for _, id := range []string{"tiny", "medium", "large-v3-turbo", "large-v3"} {
			a.models[id] = "/models/" + id + ".bin"
		}
	})
	if !app.Exists("id:settings-model-row-medium") || app.Exists(whole(msgs.SettingsSpeechMoreModels())) {
		t.Fatalf("texts %q", app.Texts())
	}
}

// TestAIPageSaysWhatTheServiceNeeds checks the footnote and the API Key field for each kind of service: EveryAPI without a key uses the everyapi sign-in and its field says the key is optional; a cloud service keeps its key on this computer and offers 获取 API Key; a service on this computer says what it is; a custom endpoint says which ones work.
func TestAIPageSaysWhatTheServiceNeeds(t *testing.T) {
	a := newFakeApp()
	a.section = SectionAI
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.SettingsAIKeyEveryAPILogin())) || !app.Exists(whole(msgs.SettingsAIKeyOptional())) || app.Exists("id:settings-llm-get-key") {
		t.Fatalf("EveryAPI: %q", app.Texts())
	}
	change(app, a, func() { a.provider = "openai" })
	if !app.Exists(whole(msgs.SettingsAIKeyStoredLocally())) || app.Exists(whole(msgs.SettingsAIKeyOptional())) {
		t.Fatalf("OpenAI: %q", app.Texts())
	}
	app.Press("id:settings-llm-get-key")
	if !slices.Contains(a.calls, "key page openai") {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() { a.provider = "ollama" })
	if !app.Exists(whole(msgs.SettingsAINoteWithBlurb(msgs.SettingsAIBlurbOllama()))) || !app.Exists(whole(msgs.SettingsAIKeyOptional())) || app.Exists("id:settings-llm-get-key") {
		t.Fatalf("Ollama: %q", app.Texts())
	}
	change(app, a, func() { a.provider = "custom" })
	if !app.Exists(whole(msgs.SettingsAINote(msgs.SettingsAIKeyStoredLocally(), msgs.SettingsAICompatible()))) {
		t.Fatalf("custom: %q", app.Texts())
	}
}

func TestAIPage(t *testing.T) {
	a := newFakeApp()
	a.section = SectionAI
	a.everyAPI = EveryAPI{Missing: true}
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, text := range []string{msgs.SettingsAIEveryAPIMissing(), msgs.SettingsAIUntested(), msgs.SettingsAIGetEveryAPI()} {
		if !app.Exists(whole(text)) {
			t.Fatalf("missing %q in %q", text, app.Texts())
		}
	}
	app.Press("id:settings-everyapi-get")
	app.Press("id:settings-llm-test")
	if !slices.Contains(a.calls, "get everyapi") || !slices.Contains(a.calls, "test") {
		t.Fatalf("calls %q", a.calls)
	}
	app.Press("id:settings-llm-provider")
	app.Press(whole(msgs.SettingsAILocalService("Ollama")))
	if a.provider != "ollama" {
		t.Fatalf("provider %q", a.provider)
	}
	// The models the service lists, with the one chosen kept on top when it no longer lists it.
	change(app, a, func() { a.model, a.llmModels = "old-model", []string{"alpha", "beta"} })
	app.Press("id:settings-llm-model")
	if !app.Exists("id:settings-llm-model-0") || app.Find("id:settings-llm-model-0").Parent == nil || !app.Exists(whole("old-model")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press(whole("beta"))
	if a.model != "beta" {
		t.Fatalf("model %q", a.model)
	}
	change(app, a, func() { a.modelsErr = "服务不接受这个 API Key" })
	app.Press("id:settings-llm-models-retry")
	if !slices.Contains(a.calls, "retry models") {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() {
		a.result, a.resultDetail = msgs.SettingsAITestQuota(), "translation API returned 402 Payment Required: Insufficient Balance"
	})
	if !app.Exists(whole(msgs.SettingsAITestQuota())) || app.Exists(whole(a.resultDetail)) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Hover("id:settings-llm-test-detail")
	app.Advance(600 * time.Millisecond)
	app.Find(whole(a.resultDetail))
	app.HoverOut()
	change(app, a, func() { a.connected, a.result, a.resultDetail = true, msgs.SettingsAIConnected(), "" })
	if !app.Exists(whole(msgs.SettingsAIConnected())) {
		t.Fatalf("texts %q", app.Texts())
	}
}

func TestTranslateAndLookPages(t *testing.T) {
	a := newFakeApp()
	a.section = SectionTranslate
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Enabled("id:settings-translate-reset") || !app.Exists(whole(msgs.SettingsTranslatePromptDefault())) {
		t.Fatal("恢复默认 is offered for the default prompt")
	}
	app.Type("id:settings-translate-prompt", " 更多")
	if !app.Enabled("id:settings-translate-reset") {
		t.Fatal("恢复默认 is not offered for a prompt of the user's")
	}
	if !app.Exists(whole(msgs.SettingsTranslatePromptCustom())) {
		t.Fatalf("a prompt of the user's with {{language}}: %q", app.Texts())
	}
	change(app, a, func() { a.prompt = "Keep it casual." })
	if !app.Exists(whole(msgs.SettingsTranslatePromptNoLanguage())) {
		t.Fatalf("a prompt without {{language}}: %q", app.Texts())
	}
	app.Press("id:settings-translate-reset")
	if a.prompt != media.DefaultSubtitlePrompt {
		t.Fatalf("prompt %q", a.prompt)
	}
	app.Press("id:settings-translate-context")
	if a.context {
		t.Fatal("the switch did not turn")
	}
	app.Press("id:settings-section-" + SectionLook)
	if !app.Exists(whole(msgs.SettingsLookPreviewTranslation())) || !app.Exists(whole(msgs.SettingsLookPreviewOriginal())) {
		t.Fatalf("the preview is missing: %q", app.Texts())
	}
	app.Press(whole(msgs.SettingsLookModeOriginal()))
	if a.subMode != subtitle.Original || app.Exists(whole(msgs.SettingsLookPreviewTranslation())) {
		t.Fatalf("mode %q, texts %q", a.subMode, app.Texts())
	}
	app.Press("id:settings-look-yield")
	if a.yield {
		t.Fatal("the switch did not turn")
	}
}

func TestShortcutsPage(t *testing.T) {
	a := newFakeApp()
	a.section = SectionShortcuts
	app := start(t, a)
	if !app.Exists(whole("⌘,")) || !app.Exists(whole("设置…")) || !app.Exists(whole("全屏")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Type("id:settings-shortcut-search", "全屏")
	if app.Exists(whole("设置…")) || !app.Exists(whole("全屏")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Type("id:settings-shortcut-search", "没有")
	if !app.Exists(whole(testlocale.Messages().SettingsShortcutsNone())) {
		t.Fatalf("texts %q", app.Texts())
	}
}

// TestShortcutsPageWritesKeysAsThePaletteDoes checks that Escape reads Esc on the page, as on the cheat sheet and in the palette, not macOS's ⎋, that the search finds a row by its key as written there, and that a query of several words finds the rows that hold every one of them in their label, group or key.
func TestShortcutsPageWritesKeysAsThePaletteDoes(t *testing.T) {
	a := newFakeApp()
	a.section = SectionShortcuts
	app := start(t, a)
	if !app.Exists(whole("Esc")) || app.Exists(whole("⎋")) {
		t.Fatalf("Escape is not written Esc: %q", app.Texts())
	}
	app.Type("id:settings-shortcut-search", "esc")
	if !app.Exists(whole("返回资料库")) || app.Exists(whole("全屏")) || app.Exists(whole("设置…")) {
		t.Fatalf("esc: texts %q", app.Texts())
	}
	app.Clear("id:settings-shortcut-search")
	app.Type("id:settings-shortcut-search", "显示 f")
	if !app.Exists(whole("全屏")) || app.Exists(whole("返回资料库")) || app.Exists(whole("设置…")) {
		t.Fatalf("显示 f: texts %q", app.Texts())
	}
}

// TestLanguageDropDownsSearch checks that the spoken language and the target language, whose lists run past a screen's worth of names, are found by typing part of a name into the drop-down's search field, and that the choice saves whisper's code or the stored value, not the name shown.
func TestLanguageDropDownsSearch(t *testing.T) {
	msgs := testlocale.Messages()
	a := newFakeApp()
	a.section = SectionSpeech
	app := start(t, a)
	app.Press("id:settings-speech-language")
	app.Type("label:"+msgs.SettingsLanguagesSearch(), msgs.SettingsSpeechLanguagePt())
	if !app.Exists("role:option "+msgs.SettingsSpeechLanguagePt()) || app.Exists("role:option "+msgs.SettingsSpeechLanguageEn()) {
		t.Fatalf("the search did not filter the spoken languages: %q", app.Texts())
	}
	app.Press("role:option " + msgs.SettingsSpeechLanguagePt())
	if a.spoken != "pt" {
		t.Fatalf("spoken language %q", a.spoken)
	}

	a = newFakeApp()
	a.section = SectionTranslate
	app = start(t, a)
	// A Chinese user finds Vietnamese by its Chinese name, as the spoken-language list names it, not by the language's own name.
	name := msgs.SettingsLanguageVietnamese()
	app.Press("id:settings-translate-language")
	app.Type("label:"+msgs.SettingsLanguagesSearch(), "越南")
	if !app.Exists("role:option "+name) || app.Exists("role:option "+msgs.SettingsLanguageEnglish()) {
		t.Fatalf("the search did not filter the target languages: %q", app.Texts())
	}
	app.Press("role:option " + name)
	if a.language != "Tiếng Việt" {
		t.Fatalf("target language %q", a.language)
	}
}

// TestAIModelTypedWhenTheServiceListsNone checks that a service whose model list could not be fetched, or lists none, leaves a field to type the model's ID in, saying why under the row's title, and that a list it does give is searched by typing part of a model's ID.
func TestAIModelTypedWhenTheServiceListsNone(t *testing.T) {
	msgs := testlocale.Messages()
	a := newFakeApp()
	a.section = SectionAI
	a.modelsErr = msgs.SettingsAIModelsNotFound()
	app := start(t, a)
	if !app.Exists("id:settings-llm-model-id") || app.Exists("id:settings-llm-model") || !app.Exists(whole(a.modelsErr)) || !app.Exists("id:settings-llm-models-retry") {
		t.Fatalf("no field to type the model in: %q", app.Texts())
	}
	app.Type("id:settings-llm-model-id", "vendor/model-7b")
	if a.model != "vendor/model-7b" || !slices.Contains(a.calls, "apply") {
		t.Fatalf("model %q, calls %q", a.model, a.calls)
	}
	// While the list loads the field gives way to the loading line; the list, once fetched, is a drop-down that keeps the typed model on top.
	change(app, a, func() { a.modelsErr, a.loadingModels = "", true })
	if app.Exists("id:settings-llm-model-id") {
		t.Fatal("the field stays while the list loads")
	}
	change(app, a, func() { a.loadingModels, a.llmModels = false, []string{"alpha-chat", "beta-chat", "gamma-chat"} })
	if app.Exists("id:settings-llm-model-id") || !app.Exists("id:settings-llm-model") {
		t.Fatalf("no drop-down for the fetched list: %q", app.Texts())
	}
	app.Press("id:settings-llm-model")
	app.Type("label:"+msgs.SettingsAISearchModels(), "beta")
	if !app.Exists("role:option beta-chat") || app.Exists("role:option alpha-chat") || app.Exists("role:option vendor/model-7b") {
		t.Fatalf("the search did not filter the models: %q", app.Texts())
	}
	app.Press("role:option beta-chat")
	if a.model != "beta-chat" {
		t.Fatalf("model %q", a.model)
	}
	// A service that answers with an empty list leaves the field too, holding the model chosen.
	change(app, a, func() { a.llmModels = nil })
	if !app.Exists("id:settings-llm-model-id") || app.Find("id:settings-llm-model-id").Text != "beta-chat" {
		t.Fatalf("no field holding the model: %q", app.Texts())
	}
}

// TestSpeechPageTakesAModelFile checks that 选择模型文件… asks the app for a model file already on the computer, and that a model in use that is none of the catalog's, as one chosen so, shows as a row of its own, chosen, with its name and where it is, while one of the catalog's shows as its row.
func TestSpeechPageTakesAModelFile(t *testing.T) {
	msgs := testlocale.Messages()
	a := newFakeApp()
	a.section = SectionSpeech
	app := start(t, a)
	app.Press(whole(msgs.SettingsSpeechChooseFile()))
	if !slices.Contains(a.calls, "pick model") {
		t.Fatalf("calls %q", a.calls)
	}
	if app.Exists("id:settings-model-row-file") {
		t.Fatal("a row for a model file while none is in use")
	}
	change(app, a, func() { a.tools.WhisperModel = "/elsewhere/ggml-large-v2.bin" })
	if !app.Exists("id:settings-model-row-file") || !app.Exists(whole("ggml-large-v2.bin")) || !app.Exists(whole("/elsewhere/ggml-large-v2.bin")) {
		t.Fatalf("no row for the model file in use: %q", app.Texts())
	}
	if !app.Find("id:settings-model-file").Props.Checked || app.Find("id:settings-model-base").Props.Checked {
		t.Fatal("the model file in use is not the one chosen")
	}
	app.Press("id:settings-model-base")
	if a.tools.WhisperModel != "/models/ggml-base.bin" || app.Exists("id:settings-model-row-file") {
		t.Fatalf("model %q, the file's row stays: %v", a.tools.WhisperModel, app.Exists("id:settings-model-row-file"))
	}
	// A catalog model's file, wherever it is, is that model's row.
	change(app, a, func() { a.tools.WhisperModel = "/elsewhere/ggml-small.bin" })
	if app.Exists("id:settings-model-row-file") {
		t.Fatal("a catalog model's file has a row of its own")
	}
}

// TestDownloadSourceDropDown checks that the 语音识别 page offers Hugging Face, chosen by default, and hf-mirror.com as where the models download from, and that choosing one hands the app its source.
func TestDownloadSourceDropDown(t *testing.T) {
	a := newFakeApp()
	a.section = SectionSpeech
	app := start(t, a)
	if !app.Exists(whole(testlocale.Messages().SettingsSpeechSource())) || !app.Exists(whole("Hugging Face")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press("id:settings-speech-source")
	app.Press("role:option hf-mirror.com")
	if a.source != media.SpeechSourceMirror {
		t.Fatalf("source %q", a.source)
	}
	app.Press("id:settings-speech-source")
	app.Press("role:option Hugging Face")
	if a.source != "" {
		t.Fatalf("source %q after Hugging Face", a.source)
	}
}

// TestLookPageChangesTheStyle checks that the 字幕外观 page changes how subtitles look, as the player's style view does, and saves each change: a preset gives the lines its boxes and sizes, the font both lines, and each line its size, color and weight, which the preview draws; the note that sent the user to the player for them is gone.
func TestLookPageChangesTheStyle(t *testing.T) {
	a := newFakeApp()
	a.section = SectionLook
	a.cue = subtitle.Look{Preset: "outline", Shadow: true,
		Translation: subtitle.Face{Family: subtitle.FontDefault, Size: 30, Color: "#FFFFFF", Bold: true},
		Original:    subtitle.Face{Family: subtitle.FontDefault, Size: 22, Color: "#FFD60A"}}
	app := start(t, a)
	msgs := testlocale.Messages()
	saves := func() int {
		return len(slices.DeleteFunc(slices.Clone(a.calls), func(c string) bool { return c != "save" }))
	}
	app.Press("id:settings-look-preset >> " + whole(msgs.FilmPresetBox()))
	if !a.cue.Box || a.cue.Shadow || a.cue.Preset != "box" || saves() != 1 {
		t.Fatalf("Box preset: look %+v, %d saves", a.cue, saves())
	}
	app.Press("id:settings-look-preset >> " + whole(msgs.FilmPresetLarge()))
	if a.cue.Box || a.cue.Translation.Size != 42 || a.cue.Original.Size != 30 {
		t.Fatalf("Large preset: look %+v", a.cue)
	}
	app.Press("id:settings-look-font")
	app.Press(whole(msgs.FilmFontHiragino()))
	if a.cue.Translation.Family != subtitle.FontHiragino || a.cue.Original.Family != subtitle.FontHiragino {
		t.Fatalf("font: look %+v", a.cue)
	}
	if font := app.Find(whole(msgs.SettingsLookPreviewTranslation())).TextStyle.Font.Family; !strings.Contains(font, "Hiragino Sans GB") {
		t.Fatalf("the preview draws in %q", font)
	}
	app.Press("id:settings-look-color-original >> " + whole(msgs.FilmColorOrange()))
	app.Press("id:settings-look-bold-original")
	app.AccessibilityAction("id:settings-look-size-translation-thumb", core.ActionIncrement)
	if a.cue.Original.Color != "#FF9F0A" || !a.cue.Original.Bold || a.cue.Translation.Size != 43 || a.cue.Translation.Color != "#FFFFFF" {
		t.Fatalf("faces: look %+v", a.cue)
	}
	if !app.Exists(whole("43")) || saves() != 6 {
		t.Fatalf("size shown %v, %d saves", app.Exists(whole("43")), saves())
	}
}

func TestPhonePageSaysWhereTheCodeStands(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPhone
	app := start(t, a)
	msgs := testlocale.Messages()
	// Off: the switch and the firewall's note, and nothing to pair with.
	if !app.Exists(whole(msgs.SettingsPhoneFirewall("macOS"))) || app.Exists("id:settings-phone-pairing") {
		t.Fatalf("texts %q while off", app.Texts())
	}
	app.Press("id:settings-phone-sync")
	if !slices.Contains(a.calls, "phone sync true") {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() {
		a.phone = PhoneSync{On: true, Running: true, Addrs: []string{"192.168.1.20:52817"}, Code: "482913", Fingerprint: "MZXW 6YTB OJUW U3DP", QR: []byte("png"), QRModules: 49}
	})
	if got := app.Find("id:settings-phone-code").Text; got != "482 913" || !app.Exists("id:settings-phone-qr") || app.Find("id:settings-phone-fingerprint").Text != "MZXW 6YTB OJUW U3DP" {
		t.Fatalf("code %q, texts %q", got, app.Texts())
	}
	for _, c := range []struct {
		state func(*PhoneSync)
		text  string
	}{
		{func(p *PhoneSync) { p.Expired, p.QR = true, nil }, msgs.SettingsPhoneCodeExpired()},
		{func(p *PhoneSync) { p.Used, p.QR = true, nil }, msgs.SettingsPhoneCodeUsed()},
		{func(p *PhoneSync) { p.Addrs, p.QR = nil, nil }, msgs.SettingsPhoneNoNetwork()},
	} {
		change(app, a, func() {
			a.phone = PhoneSync{On: true, Running: true, Addrs: []string{"192.168.1.20:52817"}, Code: "482913"}
			c.state(&a.phone)
		})
		if !app.Exists(whole(c.text)) || app.Exists("id:settings-phone-qr") || app.Find("id:settings-phone-code").Text != "—" {
			t.Fatalf("want %q in place of the QR code and no code; texts %q", c.text, app.Texts())
		}
	}
	app.Press("id:settings-phone-new-code")
	change(app, a, func() {
		a.phone.Devices = []PhoneDevice{{ID: "d1", Name: "iPhone 17", Platform: "ios", LastSeen: time.Date(2026, 11, 17, 9, 41, 0, 0, time.Local)}}
	})
	app.Press("id:settings-phone-revoke-d1")
	if !slices.Contains(a.calls, "new phone code") || !slices.Contains(a.calls, "revoke d1") || !app.Exists(whole("iPhone 17")) {
		t.Fatalf("calls %q, texts %q", a.calls, app.Texts())
	}
	change(app, a, func() { a.phone = PhoneSync{On: true, Err: "address already in use"} })
	if !app.Exists(whole(msgs.SettingsPhoneStartFailed("address already in use"))) {
		t.Fatalf("texts %q after a failed start", app.Texts())
	}
}

// 手机发来的链接 turns receiving on and off and chooses what comes of a link; what comes of it cannot be chosen while none is received.
func TestPhonePageChoosesWhatComesOfALink(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPhone
	a.phone = PhoneSync{On: true, Running: true, Receive: true, Addrs: []string{"192.168.1.20:52817"}}
	app := startOn(t, a, core.OSMacOS)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.SettingsPhoneLinks())) || !app.Exists(whole(msgs.SettingsPhoneReceiveQueue())) || !app.Enabled("id:settings-phone-receive-then") {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press("id:settings-phone-receive-then")
	app.Press("id:settings-phone-receive-then-1")
	if !a.phone.AskFirst || !slices.Contains(a.calls, "ask first true") {
		t.Fatalf("calls %q after choosing 先问我", a.calls)
	}
	app.Press("id:settings-phone-receive")
	if a.phone.Receive || !slices.Contains(a.calls, "receive false") {
		t.Fatalf("calls %q after turning receiving off", a.calls)
	}
	change(app, a, func() {})
	if app.Enabled("id:settings-phone-receive-then") {
		t.Fatal("what comes of a link can be chosen while none is received")
	}
}

func TestPhonePageSaysNothingOfAFirewallOnLinux(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPhone
	app := startOn(t, a, core.OSLinux)
	if app.Exists(whole(testlocale.Messages().SettingsPhoneFirewall("Linux"))) {
		t.Fatal("a firewall's note on Linux, where no prompt asks")
	}
}

// TestWindowTextIsInThePlatformsFont checks that the settings window's text is in the design's font of the platform, and that it follows the interface's language as it changes (the window is built again then, see Window), for the CJK family that language's Han characters take; while the subtitle preview's 默认 lines stay in the system's font rather than taking the window's.
func TestWindowTextIsInThePlatformsFont(t *testing.T) {
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux} {
		a := newFakeApp()
		a.section = SectionLook
		a.cue = subtitle.Look{Preset: "outline", Shadow: true,
			Translation: subtitle.Face{Family: subtitle.FontDefault, Size: 30, Color: "#FFFFFF", Bold: true},
			Original:    subtitle.Face{Family: subtitle.FontDefault, Size: 22, Color: "#FFD60A"}}
		var root *core.Scope
		env := testlocale.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop, AppleKeyboard: os == core.OSMacOS})
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			root = cx
			a.Attach(cx)
			return Window{App: a, Version: "1.2.3"}
		}, nsgotest.Size(880, 640), nsgotest.Environment(env))
		title := app.Find("id:settings-title")
		if got, want := title.TextStyle.Font.Family, theme.FontFor(os, testlocale.Current()); got != want {
			t.Errorf("%v: the title in %q, want %q", os, got, want)
		}
		msgs := testlocale.Messages()
		if got := app.Find(whole(msgs.SettingsLookPreviewTranslation())).TextStyle.Font.Family; got != "system-ui" {
			t.Errorf("%v: the preview's 默认 line in %q, want the system's font", os, got)
		}
		root.SetLocale("ja")
		app.Settle()
		if got, want := app.Find("id:settings-title").TextStyle.Font.Family, theme.FontFor(os, "ja"); got != want {
			t.Errorf("%v: in Japanese the title is in %q, want %q", os, got, want)
		}
	}
}
