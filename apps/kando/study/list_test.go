package study

import (
	"slices"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

type fakeApp struct {
	live.Rev
	entries     []Entry
	export      Export
	due         int
	calls       []string
	note, draft string
	targets     []ExportTarget
	target      string
}

func (f *fakeApp) NoteEditing() string { return f.note }
func (f *fakeApp) EditNote(id string) {
	f.note = id
	for _, e := range f.entries {
		if e.ID == id {
			f.draft = e.Note
		}
	}
}
func (f *fakeApp) NoteDraft() string        { return f.draft }
func (f *fakeApp) SetNoteDraft(text string) { f.draft = text }
func (f *fakeApp) SaveNote() {
	f.entries = slices.Clone(f.entries)
	for i, e := range f.entries {
		if e.ID == f.note {
			f.entries[i].Note = f.draft
		}
	}
	f.CancelNote()
}
func (f *fakeApp) CancelNote() { f.note, f.draft = "", "" }

func (f *fakeApp) ExportSaved()                  { f.calls = append(f.calls, "export") }
func (f *fakeApp) ExportState() Export           { return f.export }
func (f *fakeApp) RevealExport()                 { f.calls = append(f.calls, "reveal") }
func (f *fakeApp) StopExport()                   { f.calls = append(f.calls, "stop") }
func (f *fakeApp) ExportSettings()               { f.calls = append(f.calls, "settings") }
func (f *fakeApp) DueCount() int                 { return f.due }
func (f *fakeApp) ExportTargets() []ExportTarget { return f.targets }
func (f *fakeApp) ExportTarget() string          { return f.target }
func (f *fakeApp) SetExportTarget(id string)     { f.target = id }
func (f *fakeApp) StartReview()                  { f.calls = append(f.calls, "review") }

func (f *fakeApp) SavedEntries() []Entry { return f.entries }
func (f *fakeApp) OpenSaved(id string)   { f.calls = append(f.calls, "open "+id) }
func (f *fakeApp) RemoveSaved(id string) {
	f.calls = append(f.calls, "remove "+id)
	f.entries = slices.DeleteFunc(f.entries, func(e Entry) bool { return e.ID == id })
}
func (f *fakeApp) CloseSaved() { f.calls = append(f.calls, "close") }

func start(t *testing.T, a *fakeApp, root func(cx *core.Scope) core.Node) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return root(cx)
	}, nsgotest.Size(1100, 720), nsgotest.Environment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})))
}

func whole(s string) string { return `text:"` + s + `"` }

// TestSheetListsTheSavedLines checks that the sheet lists each line with its film and time over its two lines, plays one at a click, removes one with its ×, leaves a line whose video is gone unplayable, and says how to save a line once none is left.
func TestSheetListsTheSavedLines(t *testing.T) {
	a := &fakeApp{entries: []Entry{
		{ID: "a", Film: "Talk", Original: "Every river", Translation: "每条河", Start: 72400 * time.Millisecond},
		{ID: "b", Film: "Gone", Original: "Bye.", Start: time.Second, Gone: true},
	}}
	app := start(t, a, func(*core.Scope) core.Node { return Sheet{App: a} })
	msgs := testlocale.Messages()
	for _, text := range []string{msgs.SavedTitle(), msgs.SavedCount(2), msgs.SavedWhere("Talk", "1:12.4"), "Every river", "每条河", msgs.SavedWhereGone("Gone", "0:01.0"), "Bye."} {
		if !app.Exists(whole(text)) {
			t.Fatalf("no %q in %q", text, app.Texts())
		}
	}
	app.Press(whole("Every river"))
	if !slices.Equal(a.calls, []string{"open a"}) {
		t.Fatalf("calls %q", a.calls)
	}
	if !app.Find("role:button " + msgs.SavedWhereGone("Gone", "0:01.0") + " Bye.").Props.Disabled {
		t.Fatal("a line whose video is gone plays")
	}
	app.Press("id:saved-remove-a")
	a.Changed()
	app.Settle()
	app.Press("id:saved-remove-b")
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedEmpty())) || !app.Exists(whole(msgs.SavedCount(0))) {
		t.Fatalf("no note once every line went: %q", app.Texts())
	}
	app.Press("id:saved-done")
	if a.calls[len(a.calls)-1] != "close" {
		t.Fatalf("Done called %q", a.calls)
	}
}

// TestSheetWritesNotes checks a line's note: the pencil opens a field under the line with the note it has, Save keeps what was typed and shows it under the line, Cancel closes the field and keeps the note as it was.
func TestSheetWritesNotes(t *testing.T) {
	a := &fakeApp{entries: []Entry{{ID: "a", Film: "Talk", Original: "As water flows downhill, it picks up sand.", Translation: "水往低处流，沿途裹挟起泥沙。"}}}
	app := start(t, a, func(*core.Scope) core.Node { return Sheet{App: a} })
	if app.Exists("id:saved-note-a") || app.Exists("id:saved-note-field-a") {
		t.Fatal("a line without a note shows one")
	}
	app.Press("id:saved-note-edit-a")
	app.Settle()
	if !app.Exists("id:saved-note-field-a") {
		t.Fatalf("no note field after the pencil: %q", app.Texts())
	}
	app.Type("id:saved-note-field-a", "picks up: 裹挟")
	if a.draft != "picks up: 裹挟" {
		t.Fatalf("draft %q", a.draft)
	}
	app.Press("id:saved-note-save")
	app.Settle()
	if a.entries[0].Note != "picks up: 裹挟" || app.Exists("id:saved-note-field-a") || !app.Exists("id:saved-note-a") || !app.Exists(whole("picks up: 裹挟")) {
		t.Fatalf("after Save: note %q, texts %q", a.entries[0].Note, app.Texts())
	}

	app.Press("id:saved-note-edit-a")
	app.Settle()
	if app.Exists("id:saved-note-a") {
		t.Fatal("the note shows under its own open field")
	}
	app.Type("id:saved-note-field-a", " and more")
	app.Press("id:saved-note-cancel")
	app.Settle()
	if a.entries[0].Note != "picks up: 裹挟" || app.Exists("id:saved-note-field-a") || !app.Exists(whole("picks up: 裹挟")) {
		t.Fatalf("after Cancel: note %q, texts %q", a.entries[0].Note, app.Texts())
	}
}

// TestSheetSaysHowTheExportGoes checks that Export for Anki… asks for the export, gives way to Cancel, which stops it, while one runs, and is off with no line to export, and that the line beside it counts the lines done, then says what was written and how many lines have their audio, offering to show it, and the settings that install ffmpeg when there was none, that it was stopped, or why it failed.
func TestSheetSaysHowTheExportGoes(t *testing.T) {
	a := &fakeApp{entries: []Entry{{ID: "a", Film: "Talk", Original: "Hello."}}}
	app := start(t, a, func(*core.Scope) core.Node { return Sheet{App: a} })
	msgs := testlocale.Messages()
	if app.Exists("id:saved-export-reveal") {
		t.Fatal("Show in Finder shows before any export")
	}
	app.Press("id:saved-export")
	a.export = Export{Busy: true, Done: 1, Total: 3}
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedExporting(1, 3))) || app.Exists("id:saved-export") || !app.Exists("id:saved-export-cancel >> "+whole(msgs.SavedExportCancel())) {
		t.Fatalf("while it runs: %q", app.Texts())
	}
	app.Press("id:saved-export-cancel")
	a.export = Export{Finished: true, Stopped: true, Done: 1, Total: 3}
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedExportStopped())) || app.Exists("id:saved-export-reveal") || app.Exists("id:saved-export-cancel") || app.Find("id:saved-export").Props.Disabled {
		t.Fatalf("once stopped: %q", app.Texts())
	}
	a.export = Export{Finished: true, Lines: 3, Sounds: 3}
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedExported(3))) || app.Exists("id:saved-export-settings") {
		t.Fatalf("once done: %q", app.Texts())
	}
	app.Press("id:saved-export-reveal")
	a.export = Export{Finished: true, Lines: 3, Sounds: 2}
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedExportedSomeAudio(3, 2))) {
		t.Fatalf("done with some audio: %q", app.Texts())
	}
	a.export = Export{Finished: true, Lines: 3, NoFFmpeg: true, WithSound: true}
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedExportedNoAudio(3))) {
		t.Fatalf("done without audio: %q", app.Texts())
	}
	app.Press("id:saved-export-settings")
	// A plugin's template with a column for the audio warns as the Anki notes do, and one without does not, nor offers ffmpeg's settings.
	for _, c := range []struct {
		export Export
		want   string
	}{
		{Export{Finished: true, Lines: 3, NoFFmpeg: true, WithSound: true, Template: "Cards", File: "cards.csv"}, msgs.SavedExportedTemplateNoAudio(3, "cards.csv")},
		{Export{Finished: true, Lines: 3, Sounds: 1, WithSound: true, Template: "Cards", File: "cards.csv"}, msgs.SavedExportedTemplateSomeAudio(3, 1, "cards.csv")},
		{Export{Finished: true, Lines: 3, NoFFmpeg: true, Template: "Cards", File: "cards.csv"}, msgs.SavedExportedTemplate(3, "cards.csv")},
	} {
		a.export = c.export
		a.Changed()
		app.Settle()
		if !app.Exists(whole(c.want)) || app.Exists("id:saved-export-settings") != (c.export.NoFFmpeg && c.export.WithSound) {
			t.Fatalf("%+v: %q", c.export, app.Texts())
		}
	}
	a.export = Export{Finished: true, Err: "permission denied"}
	a.Changed()
	app.Settle()
	if !app.Exists(whole(msgs.SavedExportFailed("permission denied"))) || app.Exists("id:saved-export-reveal") {
		t.Fatalf("once failed: %q", app.Texts())
	}
	if !slices.Equal(a.calls, []string{"export", "stop", "reveal", "settings"}) {
		t.Fatalf("calls %q", a.calls)
	}
	a.entries = nil
	a.Changed()
	app.Settle()
	if !app.Find("id:saved-export").Props.Disabled {
		t.Fatal("Export for Anki… is on with no saved line")
	}
}

// TestSheetStartsTheReview checks that Review counts the lines due, is off when none is, and starts the review.
func TestSheetStartsTheReview(t *testing.T) {
	a := &fakeApp{entries: []Entry{{ID: "a", Film: "Talk", Original: "Hello."}}}
	app := start(t, a, func(*core.Scope) core.Node { return Sheet{App: a} })
	msgs := testlocale.Messages()
	if b := app.Find("id:saved-review"); !b.Props.Disabled || b.Props.Label != msgs.SavedReviewDue(0) {
		t.Fatalf("with none due Review is %q, disabled %v", b.Props.Label, b.Props.Disabled)
	}
	a.due = 3
	a.Changed()
	app.Settle()
	app.Press("role:button " + msgs.SavedReviewDue(3))
	if !slices.Equal(a.calls, []string{"review"}) {
		t.Fatalf("calls %q", a.calls)
	}
}

// TestSheetFollowsTheAppearance checks the saved-lines sheet in the window's theme as the system turns dark: a line's text turns light, and the wash under a line the pointer rests on is white over the dark sheet, as black is over the light one, rather than black on both.
func TestSheetFollowsTheAppearance(t *testing.T) {
	a := &fakeApp{entries: []Entry{{ID: "a", Film: "Talk", Original: "Every river", Translation: "每条河"}}}
	app := start(t, a, func(cx *core.Scope) core.Node {
		theme.Apply(cx)
		return Sheet{App: a}
	})
	line := "role:button " + testlocale.Messages().SavedWhere("Talk", "0:00.0") + " Every river 每条河"
	row := func() *nsgotest.View { return app.Find(line) }
	app.Hover(line)
	if got := row().Style.Background; got != core.RGBA(0x000000, 0.05) {
		t.Fatalf("a line under the pointer is washed in %#x on the light sheet, want black at 5%%", uint32(got))
	}
	app.SetEnvironment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Dark: true}))
	if got := row().Style.Background; got != core.RGBA(0xFFFFFF, 0.05) {
		t.Fatalf("a line under the pointer is washed in %#x on the dark sheet, want white at 5%%", uint32(got))
	}
	if r, g, b, _ := app.Find(whole("Every river")).TextStyle.Color.Components(); r < 0.7 || g < 0.7 || b < 0.7 {
		t.Fatalf("a line's original is %#x on the dark sheet, want a light ink", uint32(app.Find(whole("Every river")).TextStyle.Color))
	}
}
