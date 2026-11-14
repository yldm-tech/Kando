package feedback

import (
	"strings"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeApp is the dialog's state as the app keeps it, in plain fields, with the calls the dialog made.
type fakeApp struct {
	live.Rev
	open  bool
	form  Form
	view  View
	calls []string
}

func (f *fakeApp) FeedbackOpen() bool            { return f.open }
func (f *fakeApp) FeedbackForm() *Form           { return &f.form }
func (f *fakeApp) FeedbackView(*core.Scope) View { return f.view }
func (f *fakeApp) FeedbackCopied()               { f.calls = append(f.calls, "copied") }
func (f *fakeApp) SendFeedback()                 { f.calls = append(f.calls, "send") }
func (f *fakeApp) CloseFeedback()                { f.calls = append(f.calls, "close"); f.open = false }

func newFakeApp() *fakeApp {
	msgs := testlocale.Messages()
	return &fakeApp{open: true, form: Form{Kind: int(KindBug), Env: true, Tools: true}, view: View{
		Env:   Attachment{Detail: "看懂 0.12.1 · macOS 27.2 (arm64)", Enabled: true},
		Tools: Attachment{Detail: "yt-dlp 2026.09.14", Enabled: true},
		Film:  Attachment{Detail: msgs.FeedbackFilmNone()},
		Subs:  Attachment{Detail: msgs.FeedbackSubsNone()},
		Issue: Issue{Label: "bug", Body: "**类型**：问题", Full: "**类型**：问题\n全部", URL: NewIssue + "?labels=bug"},
	}}
}

// start shows a's dialog in a window of 1100×720 on a Mac, in testlocale's language.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return Dialog{App: a}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
}

func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func (f *fakeApp) called(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

func TestDialogShowsTheFormAndThePreview(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, want := range []string{msgs.FeedbackTitle(), msgs.FeedbackIntro(), msgs.FeedbackKindBug(), msgs.FeedbackKindIdea(), msgs.FeedbackKindSubs(), msgs.FeedbackAttachments(), msgs.FeedbackAttachEnv(), "看懂 0.12.1 · macOS 27.2 (arm64)", msgs.FeedbackFilmNone(), msgs.FeedbackPreview(), "github.com/yldm-tech/Kando/issues/new", msgs.FeedbackCancel(), msgs.FeedbackCopy(), msgs.FeedbackOpen()} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("missing %q in %q", want, app.Texts())
		}
	}
	if got, want := app.Find("id:feedback-preview").Text, a.view.Issue.Preview(msgs); got != want {
		t.Fatalf("preview %q, want %q", got, want)
	}
	if h := app.Find("id:feedback-heading"); h.Props.Role != core.RoleHeader || h.Props.Level != 1 || h.Props.Label != msgs.FeedbackTitle() {
		t.Fatalf("title read as role %v level %d %q, want a heading of level 1", h.Props.Role, h.Props.Level, h.Props.Label)
	}
	if !app.Find("id:feedback-title").Focused {
		t.Fatal("the title field does not have the keyboard as the dialog opens")
	}
	if app.Enabled("id:feedback-film") || !app.Enabled("id:feedback-env") {
		t.Fatal("a row with nothing to attach can be ticked, or one with something cannot")
	}
	if app.Enabled("id:feedback-send") {
		t.Fatal("在 GitHub 中打开 is enabled without a title")
	}
	change(app, a, func() { a.open = false })
	app.Advance(time.Second)
	if app.Exists("id:feedback") {
		t.Fatal("the dialog shows while closed")
	}
}

func TestDialogOpensWithTheKeyboardInTheDescriptionWhenTitled(t *testing.T) {
	a := newFakeApp()
	a.form.Title = "字幕问题：talk"
	app := start(t, a)
	if !app.Find("id:feedback-description").Focused {
		t.Fatal("the description field does not have the keyboard though the dialog opened titled")
	}
}

func TestDialogEditsTheForm(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Type("id:feedback-title", "导出后字幕错位")
	app.Type("id:feedback-description", "字幕晚了一秒")
	if a.form.Title != "导出后字幕错位" || a.form.Description != "字幕晚了一秒" {
		t.Fatalf("form %+v", a.form)
	}
	app.Press(msgs.FeedbackAttachEnv())
	if a.form.Env {
		t.Fatal("a click on the row left 应用与系统版本 ticked")
	}
	app.Press(msgs.FeedbackKindIdea())
	if Kind(a.form.Kind) != KindIdea {
		t.Fatalf("kind %d", a.form.Kind)
	}
	if got := app.Find("id:feedback-title").Input.Placeholder; got != msgs.FeedbackTitleHintIdea() {
		t.Fatalf("placeholder %q for a suggestion", got)
	}
	app.Press("id:feedback-send")
	if !a.called("send") {
		t.Fatalf("在 GitHub 中打开 with a title did not send: %q", a.calls)
	}
}

func TestDialogCopiesTheWholeBody(t *testing.T) {
	a := newFakeApp()
	a.view.Issue.Truncated, a.view.Issue.Cut = true, 120
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(`text:"` + msgs.FeedbackTruncated(120) + `"`) {
		t.Fatalf("no warning that the body was cut: %q", app.Texts())
	}
	app.Press("id:feedback-copy")
	if app.Clipboard != a.view.Issue.Full || !a.called("copied") {
		t.Fatalf("clipboard %q, calls %q", app.Clipboard, a.calls)
	}
	change(app, a, func() { a.view.Copied = true })
	if !app.Exists(`text:"`+msgs.FeedbackCopied()+`"`) || app.Find("id:feedback-copy").Props.Label != msgs.FeedbackCopy() {
		t.Fatalf("the copy button does not say it copied: %q", app.Texts())
	}
	change(app, a, func() { a.view.Err = "没能打开浏览器" })
	if !strings.Contains(strings.Join(app.Texts(), "\n"), "没能打开浏览器") {
		t.Fatalf("no error in %q", app.Texts())
	}
}

func TestDialogCloses(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Press("id:feedback-cancel")
	app.Advance(time.Second)
	if !a.called("close") || app.Exists("id:feedback") {
		t.Fatalf("取消 left the dialog open: %q", a.calls)
	}
	a.calls = nil
	change(app, a, func() { a.open = true })
	app.Key("escape")
	if !a.called("close") || a.open {
		t.Fatalf("Escape left the dialog open: %q", a.calls)
	}
}

// The app hands the dialog a view that follows its whole revision (live.Func), which changes on every keystroke and progress report; the fields must not be built again for those, so they keep the keyboard focus and what was typed.
func TestDialogStaysWhileTheAppChanges(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Type("id:feedback-title", "崩溃")
	field := app.Find("id:feedback-title").ID
	send := app.Find("id:feedback-send").ID
	a.Changed()
	app.Settle()
	if got := app.Find("id:feedback-title"); got.ID != field || !got.Focused || got.Text != "崩溃" {
		t.Fatalf("the title field was built again (view %d, was %d), focused %v, holds %q", got.ID, field, got.Focused, got.Text)
	}
	if got := app.Find("id:feedback-send").ID; got != send {
		t.Fatalf("在 GitHub 中打开 was built again (view %d, was %d)", got, send)
	}
}
