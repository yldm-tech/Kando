package quit

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

func TestConfirmAsksAndAnswers(t *testing.T) {
	var open, film *core.Signal[string]
	var answers []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		open, film = core.NewSignal(cx, ""), core.NewSignal(cx, "Talk")
		return Confirm{
			Open:     func() bool { return open.Get() != "" },
			Film:     film.Get,
			OnQuit:   func() { answers = append(answers, "quit"); open.Set("") },
			OnCancel: func() { answers = append(answers, "cancel"); open.Set("") },
		}
	}, nsgotest.Size(800, 600), nsgotest.Environment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})))
	msgs := testlocale.Messages()
	if app.Exists("id:quit-confirm") {
		t.Fatal("the confirmation shows while closed")
	}
	open.Set("yes")
	app.Settle()
	if !app.Exists(`text:"` + msgs.QueueQuitWhileRunningMessage("Talk") + `"`) {
		t.Fatalf("no message naming the film: %q", app.Texts())
	}
	film.Set("Next")
	app.Settle()
	if !app.Exists(`text:"` + msgs.QueueQuitWhileRunningMessage("Next") + `"`) {
		t.Fatalf("the message does not follow the job running: %q", app.Texts())
	}
	app.Press("id:quit-cancel")
	open.Set("yes")
	app.Settle()
	app.Key("escape")
	open.Set("yes")
	app.Settle()
	app.Key("enter")
	open.Set("yes")
	app.Settle()
	if role := app.Find("id:quit-confirm").Props.Role; role != core.RoleAlertDialog {
		t.Fatalf("the confirmation's role is %v, want an alert dialog", role)
	}
	app.Press("id:quit-quit")
	if want := []string{"cancel", "cancel", "cancel", "quit"}; len(answers) != 4 || answers[0] != want[0] || answers[1] != want[1] || answers[2] != want[2] || answers[3] != want[3] {
		t.Fatalf("answers %q, want %q", answers, want)
	}
}

// The app hands Confirm a Film that follows its whole revision (live.Func), which changes on every keystroke and progress report; the dialog must not be built again for those while the film stays the same.
func TestConfirmStaysWhileTheAppChanges(t *testing.T) {
	var rev live.Rev
	film := "Talk"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rev.Attach(cx)
		return Confirm{
			Open:     func() bool { return true },
			Film:     live.Func(&rev, func() string { return film }),
			OnQuit:   func() {},
			OnCancel: func() {},
		}
	}, nsgotest.Size(800, 600), nsgotest.Environment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})))
	before := app.Find("id:quit-confirm").ID
	rev.Changed()
	app.Settle()
	if after := app.Find("id:quit-confirm").ID; after != before {
		t.Fatalf("the dialog was built again (view %d, was %d) though the film stayed the same", after, before)
	}
	film = "Next"
	rev.Changed()
	app.Settle()
	if !app.Exists(`text:"` + testlocale.Messages().QueueQuitWhileRunningMessage("Next") + `"`) {
		t.Fatalf("the message does not follow the film: %q", app.Texts())
	}
}
