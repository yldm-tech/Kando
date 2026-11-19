package film

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
)

// Referencer is the open film's reference layer: the captions its site made itself, YouTube's automatic captions or bilibili's AI subtitles, which its job kept beside the video when the Add sheet asked for them (core.SubtitlesAuto). They are machine-made, so they are never the film's lines: the picture shows the one playing above its own lines, smaller and dimmed, while ShowReference holds, and the subtitle menu turns it on and off.
type Referencer interface {
	// Reference is the open film's reference layer, nil when it has none.
	Reference() []kcore.Segment
	ShowReference() bool
	SetShowReference(on bool)
}

// referenceScale is how big the reference layer's line is against the original's.
const referenceScale = 0.8

// referenceLine is the line of the reference layer playing at at, in the subtitles' time, as the picture shows it above the film's lines in look: in the original's face, smaller, dimmed, a line no click looks up; ok is false while none plays.
func referenceLine(look *Look, reference []kcore.Segment, at time.Duration) (styledLine, bool) {
	i := cueIndex(reference, at)
	if i < 0 || reference[i].Text == "" {
		return styledLine{}, false
	}
	face := look.Original
	face.Size *= referenceScale
	face.Bold = false
	return styledLine{text: reference[i].Text, face: face, dim: true, cue: -1}, true
}

// referenceRow is the subtitle menu's row that shows or hides the reference layer, there while the open film has one.
func (s *screen) referenceRow(cx *core.Scope) core.Node {
	a := s.a
	has := live.Func(a, func() bool { return len(a.Reference()) > 0 })
	return core.Show{When: has, Then: func(cx *core.Scope) core.Node {
		on := live.Func(a, a.ShowReference)
		name := func() string { return locales.FilmReference(cx) }
		text := core.View{Style: core.Style{FlexGrow: 1, Gap: 1}, Children: []core.Node{
			core.Text{ContentFunc: name, Style: core.Style{FontSize: 13, LineHeight: 18, Color: onGlass}},
			core.Text{ContentFunc: func() string { return locales.FilmReferenceNote(cx) }, Style: core.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: core.Alpha(0.6)}},
		}}
		return menuRow(cx, "film-reference", core.RoleCheckbox, name, on, live.Do(a, func() {
			a.SetShowReference(!a.ShowReference())
		}), core.Style{}, check(on), text)
	}}
}
