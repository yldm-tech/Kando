package film

import (
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// fakeReferencer is the fake app's reference layer: the captions as a test sets them, shown while show holds.
type fakeReferencer struct {
	reference []core.Segment
	hidden    bool
}

func (f *fakeReferencer) Reference() []core.Segment { return f.reference }
func (f *fakeReferencer) ShowReference() bool       { return !f.hidden }
func (f *fakeReferencer) SetShowReference(on bool)  { f.hidden = !on }

// The reference layer's line shows above the film's own, smaller and dimmed, and alone where the film has no line; it is no line a click looks up.
func TestReferenceLineGoesAboveTheFilmsLines(t *testing.T) {
	look := DefaultLook()
	reference := []core.Segment{{Start: time.Second, End: 3 * time.Second, Text: "hello every one"}}
	line, ok := referenceLine(&look, reference, 2*time.Second)
	if !ok || line.text != "hello every one" || !line.dim || line.original || line.cue != -1 || line.face.Size != look.Original.Size*referenceScale || line.face.Bold {
		t.Fatalf("line %+v, ok %v", line, ok)
	}
	if _, ok := referenceLine(&look, reference, 4*time.Second); ok {
		t.Fatal("a line shows where the reference layer has none")
	}
	if _, ok := referenceLine(&look, nil, 2*time.Second); ok {
		t.Fatal("a film without a reference layer shows one")
	}
}

// The subtitle menu offers the reference layer only for a film that has one, and its row turns it off and on again.
func TestModeMenuTurnsTheReferenceLayerOff(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	change(app, a, func() { a.modeMenu = true })
	if app.Exists("id:film-reference") {
		t.Fatal("a film without a reference layer offers one")
	}
	change(app, a, func() {
		a.reference = []core.Segment{{Start: time.Second, End: 3 * time.Second, Text: "hello every one"}}
		a.modeMenu = true
	})
	if !app.Exists(`text:"` + msgs.FilmReference() + `"`) {
		t.Fatalf("no reference row in %q", app.Texts())
	}
	app.Press("id:film-reference")
	if a.ShowReference() {
		t.Fatal("the row did not hide the reference layer")
	}
	app.Press("id:film-reference")
	if !a.ShowReference() {
		t.Fatal("the row did not show the reference layer again")
	}
}

// The reference layer's line shows alone between the film's own lines, and turning the subtitles off hides it there as on them.
func TestReferenceLineHidesWithTheSubtitles(t *testing.T) {
	a := newFakeApp()
	a.reference = []core.Segment{{Start: 3 * time.Second, End: 4 * time.Second, Text: "between the lines"}}
	app := start(t, a)
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second})
	a.play.playing = true
	app.Progress("id:film-video", 3500*time.Millisecond, 10*time.Second)
	if !app.Exists(`text:"between the lines"`) {
		t.Fatalf("between the film's lines the reference line does not show: %q", app.Texts())
	}
	change(app, a, func() { a.mode = SubOff })
	if app.Exists(`text:"between the lines"`) {
		t.Fatal("with the subtitles off the reference line shows")
	}
}
