package film

import (
	"testing"

	ns "github.com/nsgo-dev/nsgo/core"
)

// TestThePanelShowsATasksRun checks the side panel's run of a task pack: in place of the tabs' content, the pack's name, what it ran on and the reply as sections under their headings, a list and cards, the footnote, and ‹ 字幕 going back to the app; while the AI service is asked it says so, and when the run could not go it says why and offers the settings that fix it.
func TestThePanelShowsATasksRun(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	say := func(s string) func(*ns.Scope) string { return func(*ns.Scope) string { return s } }
	labels := func(*ns.Scope) []string { return []string{"Question", "Answer"} }
	change(app, a, func() {
		a.side = SideTask
		a.task = TaskResult{Open: true, Title: say("Quiz me"), Subject: say("Hello there."), Render: TaskSections, Labels: labels,
			Items: [][]string{{"Who says it?", ""}}, Note: say("Written by m1")}
	})
	for _, want := range []string{"Quiz me", "Hello there.", "Question", "Who says it?", "Written by m1"} {
		if !app.Exists("id:film-task >> " + whole(want)) {
			t.Fatalf("the run has no %q", want)
		}
	}
	if app.Exists("id:film-task >> " + whole("Answer")) {
		t.Fatal("a field the reply left empty shows its heading")
	}
	change(app, a, func() {
		a.task.Render, a.task.Items = TaskList, [][]string{{"one", "1"}, {"two", ""}}
	})
	if !app.Exists("id:film-task >> "+whole("one · 1")) || !app.Exists("id:film-task >> "+whole("two")) {
		t.Fatalf("the list: %q", app.Texts())
	}
	change(app, a, func() {
		a.task.Render, a.task.Items = TaskCards, [][]string{{"Front", "Back"}}
	})
	if !app.Exists("id:film-task-card >> "+whole("Front")) || !app.Exists("id:film-task-card >> "+whole("Answer")) || !app.Exists("id:film-task-card >> "+whole("Back")) {
		t.Fatalf("the cards: %q", app.Texts())
	}
	app.Press("id:film-task-back")
	if a.lastAsk() != "close task" {
		t.Fatalf("‹ 字幕 asked %q", a.asks)
	}

	change(app, a, func() {
		a.task = TaskResult{Open: true, Title: say("Quiz me"), Subject: say("Hello there."), Busy: true, Say: say("asking m1")}
	})
	if !app.Exists(whole("asking m1")) || app.Exists("id:film-task-settings") || app.Exists("id:film-task-note") {
		t.Fatal("the panel while the task runs")
	}
	change(app, a, func() {
		a.task = TaskResult{Open: true, Title: say("Quiz me"), Failed: true, Say: say("over budget"), Settings: say("Plugin settings")}
	})
	if !app.Exists(whole("over budget")) || !app.Exists("id:film-task-settings >> "+whole("Plugin settings")) {
		t.Fatalf("the panel does not say why: %q", app.Texts())
	}
	app.Press("id:film-task-settings")
	if a.lastAsk() != "task settings" {
		t.Fatalf("the settings button asked %q", a.asks)
	}
}
