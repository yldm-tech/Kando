package onboarding

import (
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeApp is the state the guide reads, kept as plain fields as the app keeps them, with the calls the guide made.
type fakeApp struct {
	live.Rev
	step        int
	language    string
	learning    []string
	tools       []Tool
	models      []Model
	downloading bool
	onNext      bool
	ai          AI
	provider    string
	key         string
	cliMissing  bool
	testing     bool
	connected   bool
	result      string
	link        string
	linkErr     string
	calls       []string
}

func newFakeApp() *fakeApp {
	return &fakeApp{
		step:     1,
		language: "简体中文",
		learning: []string{"en", "ja"},
		tools:    []Tool{{Name: "yt-dlp", State: ToolReady}, {Name: "ffmpeg", State: ToolReady}, {Name: "whisper-cli", State: ToolReady}},
		models:   []Model{{ID: "large-v3-turbo-q5", Name: "Large v3 Turbo", Size: "574 MB", Blurb: "best"}, {ID: "small", Name: "Small", Size: "466 MB", Blurb: "light"}},
		onNext:   true,
		ai:       AI{Text: "翻译模型：m", Ready: true},
		provider: "everyapi",
	}
}

func (f *fakeApp) OnboardingServices(*core.Scope) []Service {
	return []Service{{ID: "everyapi", Name: "EveryAPI"}, {ID: "openai", Name: "OpenAI", Key: true, KeyPage: true}, {ID: "ollama", Name: "本机 · Ollama"}}
}
func (f *fakeApp) LLMProvider() string             { return f.provider }
func (f *fakeApp) SelectLLM(id string)             { f.call("select " + id); f.provider = id }
func (f *fakeApp) LLMKey() *string                 { return &f.key }
func (f *fakeApp) ApplyLLM()                       { f.call("apply " + f.key) }
func (f *fakeApp) OpenKeyPage()                    { f.call("key page " + f.provider) }
func (f *fakeApp) OnboardingEveryAPIMissing() bool { return f.cliMissing }
func (f *fakeApp) GetEveryAPI()                    { f.call("get everyapi") }
func (f *fakeApp) Redetect()                       { f.call("redetect") }
func (f *fakeApp) TestLLM()                        { f.call("test") }
func (f *fakeApp) TestingLLM() bool                { return f.testing }
func (f *fakeApp) LLMConnected() bool              { return f.connected }
func (f *fakeApp) LLMResult() string               { return f.result }

func (f *fakeApp) call(s string)                        { f.calls = append(f.calls, s) }
func (f *fakeApp) OnboardingStep() int                  { return f.step }
func (f *fakeApp) Language() *string                    { return &f.language }
func (f *fakeApp) LanguageChoices() []string            { return []string{"简体中文", "English", "日本語"} }
func (f *fakeApp) OnboardingSave()                      { f.call("save") }
func (f *fakeApp) OnboardingLearning() []string         { return f.learning }
func (f *fakeApp) OnboardingToggle(code string)         { f.call("toggle " + code) }
func (f *fakeApp) OnboardingTools(*core.Scope) []Tool   { return slices.Clone(f.tools) }
func (f *fakeApp) OnboardingDownloadTool(n string)      { f.call("tool " + n) }
func (f *fakeApp) OnboardingModels(*core.Scope) []Model { return slices.Clone(f.models) }
func (f *fakeApp) OnboardingDownload(id string)         { f.call("download " + id) }
func (f *fakeApp) OnboardingCancelDownload()            { f.call("cancel") }
func (f *fakeApp) OnboardingUseModel(id string)         { f.call("use " + id) }
func (f *fakeApp) OnboardingDownloading() bool          { return f.downloading }
func (f *fakeApp) OnboardingDownloadsOnNext() bool      { return f.onNext }
func (f *fakeApp) OnboardingAI(*core.Scope) AI          { return f.ai }
func (f *fakeApp) OpenSettings(section string)          { f.call("settings " + section) }
func (f *fakeApp) OnboardingLink() *string              { return &f.link }
func (f *fakeApp) OnboardingLinkError() string          { return f.linkErr }
func (f *fakeApp) OnboardingBack()                      { f.call("back"); f.step-- }
func (f *fakeApp) OnboardingNext()                      { f.call("next"); f.step++ }
func (f *fakeApp) OnboardingSkip()                      { f.call("skip"); f.step = 0 }
func (f *fakeApp) OnboardingTargetName(cx *core.Scope, value string) string {
	return "target " + value
}

func (f *fakeApp) OnboardingDownloadFromMirror(id string) {
	f.call("download from mirror " + id)
}

// start shows the guide of a over a window of the app's size, on os, in testlocale's language.
func start(t *testing.T, a *fakeApp, os core.OS) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop, AppleKeyboard: os == core.OSMacOS})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		theme.Apply(cx)
		a.Attach(cx)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{View{App: a}}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
}

// The light theme's inks the window shows them in: orange for what Kando cannot do, red for a link it cannot add and green for one it can.
var (
	lightWarnInk    = core.RGB(0xB25000)
	lightDangerInk  = core.RGB(0xC4291C)
	lightSuccessInk = core.RGB(0x1B6B30)
)

// change runs fn on the app's state as a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

// gone lets the sheet's closing motion run out.
func gone(app *nsgotest.App) { app.Advance(time.Second) }

func wantCalls(t *testing.T, a *fakeApp, want ...string) {
	t.Helper()
	if !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
	a.calls = nil
}

func TestStepsAndTheirButtons(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, core.OSMacOS)
	msgs := testlocale.Messages()
	for _, want := range []string{msgs.OnboardingWelcome(), "1 / 3", msgs.OnboardingSkip(), msgs.OnboardingTranslateInto(), msgs.OnboardingContinue()} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("no %q on the first step: %q", want, app.Texts())
		}
	}
	if app.Exists("id:onboarding-back") {
		t.Fatal("上一步 on the first step")
	}
	if got := app.Find("id:onboarding").Props.Label; got != msgs.OnboardingWelcome() {
		t.Errorf("the sheet is named %q", got)
	}
	app.Press("id:onboarding-next")
	wantCalls(t, a, "next")
	for _, want := range []string{"2 / 3", msgs.OnboardingModelsTitle(), msgs.OnboardingDownloadAndContinue()} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("no %q on the second step: %q", want, app.Texts())
		}
	}
	if got := app.Find("id:onboarding-next").Props.Label; got != msgs.OnboardingDownloadAndContinue() {
		t.Fatalf("primary button %q while it downloads on 继续", got)
	}
	change(app, a, func() { a.onNext = false })
	if got := app.Find("id:onboarding-next").Props.Label; got != msgs.OnboardingContinue() {
		t.Fatalf("primary button %q once nothing downloads on 继续", got)
	}
	app.Press("id:onboarding-back")
	wantCalls(t, a, "back")
	if !app.Exists(`text:"1 / 3"`) {
		t.Fatalf("上一步 did not go back: %q", app.Texts())
	}
	change(app, a, func() { a.step = 3 })
	for _, want := range []string{msgs.OnboardingLinkTitle(), msgs.OnboardingLookAround(), msgs.OnboardingDone(), msgs.OnboardingLinkHintEmpty()} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("no %q on the third step: %q", want, app.Texts())
		}
	}
	if !app.Find("id:onboarding-link").Focused {
		t.Error("the link field did not take the keyboard as the third step showed")
	}
	app.Press("id:onboarding-skip")
	wantCalls(t, a, "skip")
	if gone(app); app.Exists("id:onboarding") {
		t.Fatal("the guide stayed open once skipped")
	}
}

func TestEscapeSkips(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, core.OSMacOS)
	app.Key("escape")
	wantCalls(t, a, "skip")
	if gone(app); app.Exists("id:onboarding") {
		t.Fatal("the guide stayed open after Escape")
	}
}

func TestTilesToggleTheLanguages(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, core.OSMacOS)
	// The target language, 简体中文, has no tile of its own.
	if app.Exists("id:onboarding-tile-zh") {
		t.Fatal("the target's own language is offered")
	}
	if !app.Find("id:onboarding-tile-ja").Props.Checked || app.Find("id:onboarding-tile-fr").Props.Checked {
		t.Fatal("the tiles do not show the languages chosen")
	}
	app.Press("Français")
	wantCalls(t, a, "toggle fr")
	change(app, a, func() { a.learning = append(a.learning, "fr") })
	if !app.Find("id:onboarding-tile-fr").Props.Checked {
		t.Fatal("Français not checked once chosen")
	}
	if !app.Exists(`text:"` + testlocale.Messages().OnboardingPillLanguages(testlocale.Messages().OnboardingLanguageEn(), "target 简体中文") + `"`) {
		t.Fatalf("the art's pill does not name the example's languages: %q", app.Texts())
	}
	// Another target language trades its tile for the one it left.
	change(app, a, func() { a.language = "日本語" })
	if app.Exists("id:onboarding-tile-ja") || !app.Exists("id:onboarding-tile-zh") {
		t.Fatalf("tiles after the target became 日本語: %q", app.Texts())
	}
}

// TestSecondStepProgramKandoCannotDownload checks the row of a program Kando cannot download, as on Windows: the line under it says how to install it, with Scoop's command when there is one, it links to its download page, and the card's head offers 重新检测 once it is installed.
func TestSecondStepProgramKandoCannotDownload(t *testing.T) {
	a := newFakeApp()
	a.step = 2
	a.tools[1] = Tool{Name: "ffmpeg", Use: "video", State: ToolUnavailable, Page: "https://www.gyan.dev/ffmpeg/builds/", Install: "scoop install ffmpeg"}
	app := start(t, a, core.OSWindows)
	msgs := testlocale.Messages()
	if !app.Exists(`text:"`+msgs.OnboardingToolUnavailableCommand("scoop install ffmpeg")+`"`) || !app.Exists("id:onboarding-tool-page-ffmpeg") || app.Exists("id:onboarding-tool-ffmpeg") {
		t.Fatalf("the row of a program Kando cannot download: %q", app.Texts())
	}
	app.Press("id:onboarding-tools-redetect")
	wantCalls(t, a, "redetect")
	change(app, a, func() { a.tools[1].Install = "" })
	if !app.Exists(`text:"` + msgs.OnboardingToolUnavailable() + `"`) {
		t.Fatalf("without a command to install it: %q", app.Texts())
	}
	change(app, a, func() { a.tools[1].State = ToolReady })
	if app.Exists("id:onboarding-tools-redetect") || app.Exists("id:onboarding-tool-page-ffmpeg") {
		t.Fatal("redetect or the page link stays once the program is there")
	}
}

func TestSecondStepRowsAndTheirButtons(t *testing.T) {
	a := newFakeApp()
	a.step = 2
	a.tools[1] = Tool{Name: "ffmpeg", Use: "video", State: ToolMissing, Err: "stalled"}
	a.tools[2] = Tool{Name: "whisper-cli", Use: "speech", State: ToolDownloading, Progress: 0.4}
	a.ai = AI{Text: "no model", Missing: true}
	app := start(t, a, core.OSMacOS)
	msgs := testlocale.Messages()
	if !app.Exists(`text:"`+msgs.OnboardingToolsDownloading()+`"`) || !app.Exists(`text:"`+msgs.OnboardingToolProgress(40)+`"`) {
		t.Fatalf("the programs' card: %q", app.Texts())
	}
	if app.Exists(`text:"yt-dlp"`) {
		t.Fatal("a program that is there has a row")
	}
	if got := app.Find("id:onboarding-tool-ffmpeg").Props.Label; got != msgs.OnboardingRetry() {
		t.Fatalf("a failed program's button says %q", got)
	}
	app.Press("id:onboarding-tool-ffmpeg")
	wantCalls(t, a, "tool ffmpeg")
	app.Press("id:onboarding-download-small")
	wantCalls(t, a, "download small")
	change(app, a, func() {
		a.downloading = true
		a.models[1].State, a.models[1].Progress = ModelDownloading, 0.5
	})
	if !app.Exists(`text:"`+msgs.OnboardingModelProgress(50, "466 MB")+`"`) || !app.Find("id:onboarding-download-large-v3-turbo-q5").Props.Disabled {
		t.Fatalf("while Small downloads: %q", app.Texts())
	}
	app.Press("id:onboarding-cancel-small")
	wantCalls(t, a, "cancel")
	change(app, a, func() {
		a.downloading = false
		a.models[1].State = ModelReady
		a.models[0].State = ModelInUse
	})
	app.Press("id:onboarding-use-small")
	wantCalls(t, a, "use small")
	if !app.Exists(`text:"`+msgs.OnboardingModelsReady()+`"`) || !app.Exists("label:"+msgs.OnboardingInUse()) {
		t.Fatalf("with a model in use: %q", app.Texts())
	}
	if got := app.Find(`text:"no model"`).TextStyle.Color; got != lightWarnInk {
		t.Errorf("a missing AI service reads in %v", got)
	}
	app.Press("id:onboarding-ai")
	wantCalls(t, a, "settings ai")
}

// TestSecondStepSetsUpTranslation checks the translation card of the second step: EveryAPI without its everyapi program offers 获取 EveryAPI and 重新检测 rather than a test; another service is chosen in place, a cloud one takes its API Key there with 获取 API Key, and 测试连接 shows its result, while one on this computer asks for no key.
func TestSecondStepSetsUpTranslation(t *testing.T) {
	a := newFakeApp()
	a.step = 2
	a.cliMissing = true
	a.ai = AI{Text: "install everyapi", Missing: true}
	app := start(t, a, core.OSMacOS)
	msgs := testlocale.Messages()
	if app.Exists("id:onboarding-ai-test") || app.Exists("id:onboarding-ai-key") {
		t.Fatalf("EveryAPI without its program: %q", app.Texts())
	}
	app.Press("id:onboarding-everyapi-get")
	app.Press("id:onboarding-everyapi-redetect")
	wantCalls(t, a, "get everyapi", "redetect")
	app.Press("id:onboarding-ai-service")
	app.Press(`text:"OpenAI"`)
	wantCalls(t, a, "select openai")
	change(app, a, func() { a.cliMissing, a.ai = false, AI{Text: msgs.OnboardingAINeedsKey(), Missing: true} })
	if !app.Exists(`text:"`+msgs.OnboardingAINeedsKey()+`"`) || app.Exists("id:onboarding-everyapi-get") {
		t.Fatalf("OpenAI without a key: %q", app.Texts())
	}
	app.Focus("id:onboarding-ai-key")
	app.Type("id:onboarding-ai-key", "sk")
	field := app.Find("id:onboarding-ai-key")
	if a.key != "sk" || !field.Focused {
		t.Fatalf("key %q, focused %v", a.key, field.Focused)
	}
	change(app, a, func() { a.ai = AI{Text: "翻译模型：gpt", Ready: true} })
	if again := app.Find("id:onboarding-ai-key"); again.ID != field.ID || !again.Focused {
		t.Fatal("the key field was built again as the app changed")
	}
	app.Press("id:onboarding-ai-get-key")
	app.Press("id:onboarding-ai-test")
	wantCalls(t, a, "apply s", "apply sk", "key page openai", "test")
	change(app, a, func() { a.testing = true })
	if !app.Exists(`text:"`+msgs.SettingsAITesting()+`"`) || !app.Find("id:onboarding-ai-test").Props.Disabled {
		t.Fatalf("while testing: %q", app.Texts())
	}
	change(app, a, func() { a.testing, a.result = false, "服务不接受这个 API Key" })
	if got := app.Find(`text:"服务不接受这个 API Key"`).TextStyle.Color; got == lightSuccessInk {
		t.Errorf("a failed test reads in %v", got)
	}
	change(app, a, func() { a.connected, a.result = true, msgs.SettingsAIConnected() })
	if got := app.Find("id:onboarding-ai-result").TextStyle.Color; got != lightSuccessInk || !app.Exists(`text:"`+msgs.SettingsAIConnected()+`"`) {
		t.Fatalf("a passed test reads %q in %v", app.Texts(), got)
	}
	change(app, a, func() { a.provider = "ollama" })
	if app.Exists("id:onboarding-ai-key") {
		t.Fatal("a service on this computer asks for a key")
	}
}

// On a first launch every program is missing, and the second step's body is taller than the sheet: it scrolls, and the buttons stay inside the sheet.
func TestSecondStepFitsWithEveryProgramMissing(t *testing.T) {
	a := newFakeApp()
	a.step = 2
	a.tools = []Tool{{Name: "yt-dlp", Use: "links", State: ToolMissing}, {Name: "ffmpeg", Use: "video", State: ToolMissing}, {Name: "whisper-cli", Use: "speech", State: ToolMissing}}
	a.ai = AI{Text: "no model", Missing: true}
	app := start(t, a, core.OSMacOS)
	if !app.Exists("id:onboarding-next") || !app.Exists("id:onboarding-back") {
		t.Fatalf("the buttons are not shown: %q", app.Texts())
	}
	sheet := windowRect(app.FindMounted("id:onboarding"))
	for _, id := range []string{"id:onboarding-next", "id:onboarding-back", "id:onboarding-skip"} {
		if b := windowRect(app.FindMounted(id)); b[0] < sheet[0] || b[1] < sheet[1] || b[2] > sheet[2] || b[3] > sheet[3] {
			t.Errorf("%s at %v lies outside the sheet at %v", id, b, sheet)
		}
	}
	app.Press("id:onboarding-next")
	wantCalls(t, a, "next")
}

// windowRect is v's left, top, right and bottom edges in window points, as its ancestors and their scroll offsets place it.
func windowRect(v *nsgotest.View) [4]float32 {
	x, y := float32(v.Frame.Origin.X), float32(v.Frame.Origin.Y)
	for p := v.Parent; p != nil; p = p.Parent {
		x += float32(p.Frame.Origin.X - p.Offset.X)
		y += float32(p.Frame.Origin.Y - p.Offset.Y)
	}
	return [4]float32{x, y, x + float32(v.Frame.Size.Width), y + float32(v.Frame.Size.Height)}
}

func TestLinkFieldSaysWhatALinkIs(t *testing.T) {
	a := newFakeApp()
	a.step = 3
	app := start(t, a, core.OSMacOS)
	msgs := testlocale.Messages()
	app.Type("id:onboarding-link", "https://www.youtube.com/")
	if a.link != "https://www.youtube.com/" {
		t.Fatalf("link %q after typing", a.link)
	}
	hint := app.Find("id:onboarding-link-hint")
	if want, _ := linkHint(msgs, a.link); hint.Text != want || hint.TextStyle.Color != lightDangerInk {
		t.Fatalf("hint %q in %v, want %q in red", hint.Text, hint.TextStyle.Color, want)
	}
	next := app.Find("id:onboarding-next")
	if !next.Props.Disabled || next.Props.Label != msgs.OnboardingAddAndStart() {
		t.Fatalf("primary button %q disabled %v for a site's home page", next.Props.Label, next.Props.Disabled)
	}
	app.Submit("id:onboarding-link")
	if len(a.calls) != 0 {
		t.Fatalf("Return added a site's home page: %q", a.calls)
	}
	// A channel is taken: the guide's button lists its videos, as the library's link field does.
	app.Clear("id:onboarding-link")
	app.Type("id:onboarding-link", "https://www.youtube.com/@SomeChannel")
	if hint := app.Find("id:onboarding-link-hint"); hint.Text != msgs.OnboardingLinkChannel("YouTube") || hint.TextStyle.Color != lightSuccessInk {
		t.Fatalf("hint %q in %v for a channel", hint.Text, hint.TextStyle.Color)
	}
	if app.Find("id:onboarding-next").Props.Disabled {
		t.Fatal("primary button disabled for a channel")
	}
	app.Clear("id:onboarding-link")
	app.Type("id:onboarding-link", "https://youtu.be/k9Yt2LmQ0pX")
	if hint := app.Find("id:onboarding-link-hint"); hint.TextStyle.Color != lightSuccessInk {
		t.Fatalf("hint %q in %v for a video", hint.Text, hint.TextStyle.Color)
	}
	app.Submit("id:onboarding-link")
	wantCalls(t, a, "next")
	change(app, a, func() { a.step, a.linkErr = 3, "no library" })
	if got := app.Find("id:onboarding-link-hint").Text; got != "no library" {
		t.Fatalf("hint %q, want the app's error", got)
	}
}

// The field keeps its view, what it holds and the keyboard while the app changes around it, as the app's revision changes on every report of a download.
func TestLinkFieldStaysWhileTheAppChanges(t *testing.T) {
	a := newFakeApp()
	a.step = 3
	app := start(t, a, core.OSMacOS)
	app.Type("id:onboarding-link", "https://youtu.be/k9Yt2LmQ0pX")
	field := app.Find("id:onboarding-link")
	next := app.Find("id:onboarding-next").ID
	for range 3 {
		change(app, a, func() {})
	}
	if v := app.Find("id:onboarding-link"); v.ID != field.ID || !v.Focused || v.Text != "https://youtu.be/k9Yt2LmQ0pX" {
		t.Fatalf("the field was built again: view %d (was %d), focused %v, holding %q", v.ID, field.ID, v.Focused, v.Text)
	}
	if app.Find("id:onboarding-next").ID != next {
		t.Fatal("the primary button was built again though its words stayed")
	}
}

// The second step names the machine as the platform calls it, so Linux and Windows do not read 这台 Mac.
func TestStepsNameThisComputer(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		os                   core.OS
		caption, description string
	}{
		{core.OSMacOS, msgs.OnboardingCaptionSpeechMac(), msgs.OnboardingModelsDescriptionMac()},
		{core.OSLinux, msgs.OnboardingCaptionSpeechComputer(), msgs.OnboardingModelsDescriptionComputer()},
		{core.OSWindows, msgs.OnboardingCaptionSpeechComputer(), msgs.OnboardingModelsDescriptionComputer()},
	} {
		a := newFakeApp()
		a.step = 2
		app := start(t, a, tc.os)
		for _, want := range []string{tc.caption, tc.description} {
			if !app.Exists(`text:"` + want + `"`) {
				t.Errorf("%v: no %q", tc.os, want)
			}
		}
		if tc.os != core.OSMacOS && slices.ContainsFunc(app.Texts(), func(s string) bool { return strings.Contains(s, "Mac") }) {
			t.Errorf("%v says Mac: %q", tc.os, app.Texts())
		}
	}
}

// The guide follows the interface language as the app changes it, keeping what the link field holds.
func TestGuideFollowsTheLanguage(t *testing.T) {
	testlocale.Use(t, "en")
	a := newFakeApp()
	a.step = 3
	app := start(t, a, core.OSMacOS)
	if !app.Exists(`text:"Paste Your First Link"`) {
		t.Fatalf("not in English: %q", app.Texts())
	}
	app.Type("id:onboarding-link", "talk")
	field := app.Find("id:onboarding-link").ID
	ja := locales.In("ja")
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true, Locale: "ja"})
	for _, want := range []string{ja.OnboardingLinkTitle(), ja.OnboardingLookAround(), ja.OnboardingAddAndStart()} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("no %q after the switch to Japanese: %q", want, app.Texts())
		}
	}
	if v := app.Find("id:onboarding-link"); v.ID != field || v.Text != "talk" {
		t.Fatalf("the link field was built again: view %d holding %q", v.ID, v.Text)
	}
}

// TestSheetCornersLineUp checks that the corners one sees are the sheet's on each platform: the clear box that casts the shadow is rounded as the design's dialogs on the platform are, up to ui.RadiusDialog, and the face, every view that clips it and the rim inside are all rounded one point tighter, so the face is not cut off inside a rounder shadow or the rim's arcs left outside it.
func TestSheetCornersLineUp(t *testing.T) {
	for _, c := range []struct {
		os   core.OS
		want core.Points
	}{{core.OSMacOS, 16}, {core.OSWindows, 8}, {core.OSLinux, 12}} {
		app := start(t, newFakeApp(), c.os)
		box := app.Find("id:onboarding")
		if got := box.Style.CornerRadius; got != c.want {
			t.Errorf("%v: the sheet is rounded %v, want %v", c.os, got, c.want)
		}
		face := app.Find("id:onboarding-face")
		// The face shows the tightest of its own corners and those of every view between it and the sheet's box that clips it.
		shown := face.Style.CornerRadius
		for v := face.Parent; v != nil && v != box; v = v.Parent {
			if v.Style.Clip && v.Style.CornerRadius < shown {
				shown = v.Style.CornerRadius
			}
		}
		if !face.Style.Clip || shown != c.want-1 {
			t.Errorf("%v: the face clips %v and shows corners of %v, want %v", c.os, face.Style.Clip, shown, c.want-1)
		}
		if got := app.Find("id:onboarding-rim").Style.CornerRadius; got != c.want-1 {
			t.Errorf("%v: the rim is rounded %v, want %v", c.os, got, c.want-1)
		}
	}
}

// TestTextsSayTheirLanguage checks that a tile's own name and the art's example lines are tagged with the languages they are written in, so Han characters take that language's glyph forms.
func TestTextsSayTheirLanguage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, core.OSMacOS)
	if got := app.Find(`text:"日本語"`).TextStyle.Font.Language; got != "ja" {
		t.Fatalf("日本語 on its tile is in %q", got)
	}
	code, original, translation := demo(a.learning, a.language)
	if got := app.Find(`text:"` + original + `"`).TextStyle.Font.Language; got != code {
		t.Fatalf("the example's line is in %q, want %q", got, code)
	}
	if got := app.Find(`text:"` + translation + `"`).TextStyle.Font.Language; got != "zh-Hans" {
		t.Fatalf("the example's translation is in %q, want zh-Hans", got)
	}
}
