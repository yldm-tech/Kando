package quit

import (
	"testing"

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
	app.Press("id:quit-quit")
	if want := []string{"cancel", "cancel", "quit"}; len(answers) != 3 || answers[0] != want[0] || answers[1] != want[1] || answers[2] != want[2] {
		t.Fatalf("answers %q, want %q", answers, want)
	}
}
