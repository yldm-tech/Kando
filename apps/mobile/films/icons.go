package films

import "github.com/nsgo-dev/nsgo/core"

// moreIcon is the design's three dots of a film's options, 16 by 4: lucide's ellipsis is drawn at 24 with thin dots.
func moreIcon() *core.SVG { return moreSVG }

var moreSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="4" viewBox="0 0 16 4" fill="currentColor"><circle cx="2" cy="2" r="1.6"/><circle cx="8" cy="2" r="1.6"/><circle cx="14" cy="2" r="1.6"/></svg>`))
