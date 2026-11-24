package film

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Linker is what the page reads and does of a film indexed from a media source, whose video plays from the source (docs/media-sources.md 6.4).
type Linker interface {
	// LinkedChanged is the name of the source the open film's file changed on since it was indexed, which the bar over the picture says, "" while it did not; KeepLinkedChanged takes the file as it is now, as the bar's 知道了 does.
	LinkedChanged() string
	KeepLinkedChanged()
	// RetryLinked opens the video from its source again, once it could not be reached; FindLinked shows the source's folder the video was in, to choose where it is now.
	RetryLinked()
	FindLinked()
}

// linkedRemedy is what goes under the note of an indexed film whose video does not play: the settings, where the sources are, once its source was removed; 重试 and the settings when it could not be reached; 在来源里找 when it is no longer at its path there; the privacy settings, where offline mode is, when offline mode keeps the source closed. Nothing when not even the copy made of it from there plays.
func (s *screen) linkedRemedy(note func() stageNoteState) core.Node {
	a := s.a
	err := func() string { return note().err }
	return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8, JustifyContent: core.JustifyCenter}, Children: []core.Node{
		core.Show{When: func() bool { return err() == ErrLinkedUnreachable }, Then: func(cx *core.Scope) core.Node {
			return ui.Button{TestID: "film-stage-linked-retry", TextFunc: func() string { return locales.MediaSourceRetry(cx) }, Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
				OnPress: live.Do(a, a.RetryLinked)}
		}},
		core.Show{When: func() bool { return err() == ErrLinkedGone || err() == ErrLinkedUnreachable }, Then: func(cx *core.Scope) core.Node {
			return ui.Button{TestID: "film-stage-linked-settings", TextFunc: func() string { return locales.FilmsOpenSettings(cx) }, Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
				OnPress: live.Do(a, func() { a.OpenSettings(sectionid.MediaSources) })}
		}},
		core.Show{When: func() bool { return err() == ErrLinkedMissing }, Then: func(cx *core.Scope) core.Node {
			return ui.Button{TestID: "film-stage-linked-find", TextFunc: func() string { return locales.MediaSourceFind(cx) }, Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
				OnPress: live.Do(a, a.FindLinked)}
		}},
		core.Show{When: func() bool { return err() == ErrLinkedOffline }, Then: func(cx *core.Scope) core.Node {
			return ui.Button{TestID: "film-stage-linked-privacy", TextFunc: func() string { return locales.FilmsOpenSettings(cx) }, Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
				OnPress: live.Do(a, func() { a.OpenSettings(sectionid.Privacy) })}
		}},
	}}
}

// linkedErr reports whether id is one of the PlayErr IDs of an indexed film, whose remedies linkedRemedy gives rather than unplayableRemedy.
func linkedErr(id string) bool {
	switch id {
	case ErrLinkedGone, ErrLinkedUnreachable, ErrLinkedMissing, ErrLinkedUnplayable, ErrLinkedOffline:
		return true
	}
	return false
}

// changedBar is the bar at the top of the picture of an indexed film whose file changed on its source since its lines were made: that they may not match, with 知道了.
func (s *screen) changedBar() core.Node {
	a, cx := s.a, s.cx
	source := live.Memo(cx, a, a.LinkedChanged)
	return core.Show{When: func() bool { return source() != "" }, Then: func(cx *core.Scope) core.Node {
		text := core.Text{TestID: "film-linked-changed", ContentFunc: func() string { return locales.MediaSourceChanged(cx, source()) },
			Style: core.Style{FontSize: 12, LineHeight: 17, Color: onGlass, FlexShrink: 1}}
		keep := lookupButton(cx, func() string { return locales.MediaSourceChangedKeep(cx) }, "film-linked-changed-keep", live.Do(a, a.KeepLinkedChanged), nil)
		bar := core.View{Role: core.RoleStatus, Style: streamGlass.style().Merge(core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, Padding: core.Symmetric(8, 14), MaxWidth: core.Pt(560)}),
			Children: []core.Node{text, keep}}
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(24), Top: core.Pt(64), AlignItems: core.AlignCenter},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight() + 24), Transition: roomTransition}
			},
			Children: []core.Node{bar}}
	}}
}
