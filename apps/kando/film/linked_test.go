package film

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

// fakeLinker is the app's side of a film indexed from a media source: the source its file changed on, and the calls the player made.
type fakeLinker struct {
	changedOn   string
	linkedAsked []string
}

func (f *fakeLinker) LinkedChanged() string { return f.changedOn }
func (f *fakeLinker) KeepLinkedChanged()    { f.linkedAsked = append(f.linkedAsked, "keep") }
func (f *fakeLinker) RetryLinked()          { f.linkedAsked = append(f.linkedAsked, "retry") }
func (f *fakeLinker) FindLinked()           { f.linkedAsked = append(f.linkedAsked, "find") }

// TestAnIndexedFilmSaysWhyItDoesNotPlay checks what goes under the note of a film indexed from a media source that does not play: 重试 and the settings when its source cannot be reached, the settings alone when it was removed, 在来源里找 when the file is no longer at its path, and none of the file manager's buttons, the video not being on this computer.
func TestAnIndexedFilmSaysWhyItDoesNotPlay(t *testing.T) {
	a := newFakeApp()
	a.err = ErrLinkedUnreachable
	app := start(t, a)
	if !app.Exists("id:film-stage-linked-retry") || !app.Exists("id:film-stage-linked-settings") || app.Exists("id:film-stage-linked-find") || app.Exists("id:film-stage-reveal") {
		t.Fatalf("unreachable: %q", app.Texts())
	}
	app.Press("id:film-stage-linked-retry")

	change(app, a, func() { a.err = ErrLinkedGone })
	if app.Exists("id:film-stage-linked-retry") || !app.Exists("id:film-stage-linked-settings") {
		t.Fatalf("gone: %q", app.Texts())
	}

	change(app, a, func() { a.err = ErrLinkedMissing })
	if !app.Exists("id:film-stage-linked-find") || app.Exists("id:film-stage-linked-settings") {
		t.Fatalf("missing: %q", app.Texts())
	}
	app.Press("id:film-stage-linked-find")

	change(app, a, func() { a.err = ErrLinkedUnplayable })
	if app.Exists("id:film-stage-linked-find") || app.Exists("id:film-stage-reveal") || app.Exists("id:film-stage-stored-settings") {
		t.Fatalf("unplayable: %q", app.Texts())
	}
	if !slices.Equal(a.linkedAsked, []string{"retry", "find"}) {
		t.Fatalf("asked %v", a.linkedAsked)
	}
}

// TestAChangedFileSaysSo checks the bar over the picture of an indexed film whose file changed on its source: it names the source, and 知道了 takes the file as it is.
func TestAChangedFileSaysSo(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	if app.Exists("id:film-linked-changed") {
		t.Fatal("a bar over a film whose file did not change")
	}
	change(app, a, func() { a.changedOn = "客厅 NAS" })
	if got := app.Find("id:film-linked-changed"); got == nil || got.Text != testlocale.Messages().MediaSourceChanged("客厅 NAS") {
		t.Fatalf("bar %q", app.Texts())
	}
	app.Press("id:film-linked-changed-keep")
	if !slices.Equal(a.linkedAsked, []string{"keep"}) {
		t.Fatalf("asked %v", a.linkedAsked)
	}
}
