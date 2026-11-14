package settings

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
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
	notify                  bool
	models                  map[string]string
	owned                   map[string]bool
	downloading             string
	written, total          int64
	downloadErr             string
	trashOpen               bool
	trashID                 string
	spoken                  string
	provider                string
	base, key, model        string
	testing, connected      bool
	result                  string
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
func (f *fakeApp) CheckForUpdates()                      { f.call("check updates") }
func (f *fakeApp) JobNotifications() bool                { return f.notify }
func (f *fakeApp) SetJobNotifications(on bool)           { f.notify = on }
func (f *fakeApp) InstalledSpeechModel(id string) string { return f.models[id] }
func (f *fakeApp) UseSpeechModel(path string)            { f.call("use " + path); f.tools.WhisperModel = path }
func (f *fakeApp) Downloading() string                   { return f.downloading }
func (f *fakeApp) DownloadWritten() int64                { return f.written }
func (f *fakeApp) DownloadTotal() int64                  { return f.total }
func (f *fakeApp) DownloadErr() string                   { return f.downloadErr }
func (f *fakeApp) CancelDownload()                       { f.call("cancel download") }
func (f *fakeApp) DownloadModel(id string)               { f.call("download model " + id) }
func (f *fakeApp) OwnsSpeechModel(id string) bool        { return f.owned[id] }
func (f *fakeApp) SpeechModelInUse(path string) bool     { return path == f.tools.WhisperModel }
func (f *fakeApp) AskTrashSpeechModel(id string)         { f.trashOpen, f.trashID = true, id }
func (f *fakeApp) TrashingSpeechModel() (*bool, string)  { return &f.trashOpen, f.trashID }
func (f *fakeApp) TrashSpeechModel(id string)            { f.call("trash " + id) }
func (f *fakeApp) SpeechLanguage() string                { return f.spoken }
func (f *fakeApp) SetSpeechLanguage(code string)         { f.spoken = code }
func (f *fakeApp) CurrentPreset() (Preset, bool) {
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
func (f *fakeApp) SelectLLM(id string)             { f.call("select " + id); f.provider = id }
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
func (f *fakeApp) Shortcuts(cx *core.Scope) []Shortcut {
	return []Shortcut{{Group: "看懂", Label: "设置…", Key: "mod+,"}, {Group: "显示", Label: "全屏", Key: "f"}}
}

// start shows the settings window of a, 880 by 640 as it opens, on a Mac, in testlocale's language. account stands for the 账号与上传 page.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	account := core.Text{TestID: "account-page", Content: "account"}
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{Window{App: a, Version: "1.2.3", Account: account}}}
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

func TestSpeechPage(t *testing.T) {
	a := newFakeApp()
	a.section = SectionSpeech
	app := start(t, a)
	msgs := testlocale.Messages()
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
	change(app, a, func() { a.connected = true })
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
