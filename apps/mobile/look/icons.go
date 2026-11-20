package look

import "github.com/nsgo-dev/nsgo/core"

// Plus is the design's heavy plus, 14 points square with a 2.6-point stroke, for the primary CircleButton: lucide's is drawn at 24 with a 2-point stroke, which at 14 is too thin.
func Plus() *core.SVG { return plusIcon }

// Chevron is the design's row chevron, 8 by 13 with a 2-point stroke and no room around it, so it ends at the row's padding.
func Chevron() *core.SVG { return chevronIcon }

var (
	plusIcon    = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><path d="M7 1.3v11.4"/><path d="M1.3 7h11.4"/></svg>`))
	chevronIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="13" viewBox="0 0 8 13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 1.5l5 5-5 5"/></svg>`))
)
