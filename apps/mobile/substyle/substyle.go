// Package substyle is how the subtitles over a picture look as the learner set them (store.SubStyle): their sizes and the original line's color, which the player draws and Settings › Subtitle Style previews alike.
package substyle

import (
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
)

// Color is the original line's color c, drawn on the picture's dark box: the same in both appearances.
func Color(c store.SubColor) core.Color {
	switch c {
	case store.SubWhite:
		return core.RGB(0xFFFFFF)
	case store.SubCyan:
		return core.RGB(0x64D2FF)
	case store.SubPink:
		return core.RGB(0xFF9ECD)
	}
	return theme.SubtitleYellow
}

// Sizes are the sizes of the translation and the original, tr and orig at medium, at the size st sets.
func Sizes(st store.SubStyle, tr, orig core.Points) (core.Points, core.Points) {
	k := core.Points(st.Size.Scale())
	return tr * k, orig * k
}
