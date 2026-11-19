package films

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// storedLook is the mark of a film whose video was moved to a storage location (Film.Stored, App.StoredMark): the location's name, whether it is an object store, whose mark is a cloud rather than a drive, and whether it is no longer in the settings, which draws the mark as a warning. on is false for a video in the library's folder, which has no mark.
type storedLook struct {
	place           string
	cloud, gone, on bool
}

// storedLookOf is item's mark in m's language, as the settings know its location now.
func storedLookOf(a App, m locales.Messages, item Film) storedLook {
	if item.Stored == "" {
		return storedLook{}
	}
	place, cloud, gone := a.StoredMark(m, item)
	return storedLook{place: place, cloud: cloud, gone: gone, on: true}
}

// storedMemo is item's mark where cx is, which follows the shelf, the settings and the interface language.
func storedMemo(cx *core.Scope, a App, item func() Film) func() storedLook {
	return live.Memo(cx, a, func() storedLook { return storedLookOf(a, locales.For(cx), item()) })
}

// label is the mark's accessible name and tooltip, in m's language: where the video is, or that its location is no longer in the settings.
func (l storedLook) label(m locales.Messages) string {
	if !l.on {
		return ""
	}
	if l.gone {
		return m.FilmsStoredGone()
	}
	return m.FilmsStoredOn(l.place)
}

// icon is the mark's glyph: a cloud for an object store, a drive for a NAS location.
func (l storedLook) icon() *core.SVG {
	if l.cloud {
		return lucide.Cloud()
	}
	return lucide.HardDrive()
}

// storedChip is the mark at the bottom start of the poster of a film whose video is on a storage location, a glyph on the frosted glass of the poster's other chips, or on the warning's fill when the location is no longer in the settings. The poster's tooltip says what it means (storedTip), as the chip lets the pointer through to the poster.
func storedChip(cx *core.Scope, look func() storedLook) core.Node {
	k := theme.Of(cx)
	return core.Show{When: func() bool { return look().on }, Then: func(cx *core.Scope) core.Node {
		place := core.Style{Position: core.Absolute, Left: core.Pt(9), Bottom: core.Pt(9), CornerRadius: 6, Padding: core.Symmetric(3, 6), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
		return core.View{TestID: "film-stored", Role: core.RoleImage, LabelFunc: func() string { return look().label(locales.For(cx)) }, PointerEvents: core.PointerEventsNone,
			Style: place.Merge(frosted(10, chipShade)),
			StyleFunc: func() core.Style {
				if look().gone {
					return core.Style{Background: k.WarnInk()}
				}
				return core.Style{}
			},
			Children: []core.Node{core.Icon{SVGFunc: func() *core.SVG { return look().icon() }, Style: core.Style{Width: core.Pt(11), Height: core.Pt(11)},
				StyleFunc: func() core.Style {
					if look().gone {
						return core.Style{Color: k.OnWarn()}
					}
					return core.Style{Color: onPicture}
				}}}}
	}}
}

// storedTip is the poster's tooltip: the film's name, and under it what its storage mark means when it has one.
func storedTip(cx *core.Scope, name func() string, look func() storedLook) func() string {
	return func() string {
		if l := look(); l.on {
			return name() + "\n" + l.label(locales.For(cx))
		}
		return name()
	}
}

// storedIcon is the mark after a film's name in the list, in the muted ink of the row's other details, or the warning's when its location is no longer in the settings, named for assistive technology by what it means.
// TODO(nsgo): DataTable's body takes no pointer events (base/datatable/view.go body), so a view drawn in a cell (RenderCell) never hears the pointer and a ui.Tooltip there never shows; once cells can carry a tooltip, give this mark its label as one, as the poster's tooltip says it
func storedIcon(cx *core.Scope, look func() storedLook) core.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	return core.Show{When: func() bool { return look().on }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "film-stored", Role: core.RoleImage, LabelFunc: func() string { return look().label(locales.For(cx)) }, Style: core.Style{FlexShrink: 0},
			Children: []core.Node{core.Icon{SVGFunc: func() *core.SVG { return look().icon() }, Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)},
				StyleFunc: func() core.Style {
					if look().gone {
						return core.Style{Color: k.WarnInk()}
					}
					return core.Style{Color: colors.Muted()}
				}}}}
	}}
}
