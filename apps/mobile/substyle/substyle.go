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

// OnPicture are the sizes of the translation and the original over the player's picture at the size st sets, from the design's tables: on a phone held upright, or wide, on a phone on its side or a tablet's larger picture.
func OnPicture(st store.SubStyle, wide bool) (tr, orig core.Points) {
	i := 1
	switch st.Size {
	case store.SubSmall:
		i = 0
	case store.SubLarge:
		i = 2
	case store.SubHuge:
		i = 3
	}
	if wide {
		return [4]core.Points{18, 22, 26, 30}[i], [4]core.Points{14, 16, 19, 22}[i]
	}
	return [4]core.Points{13, 15, 17, 20}[i], [4]core.Points{10, 12, 13, 15}[i]
}
