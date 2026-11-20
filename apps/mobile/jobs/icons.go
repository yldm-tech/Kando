package jobs

import "github.com/nsgo-dev/nsgo/core"

// The design's glyphs on a job's round buttons and in its failure box: a filled pause and play, 11 by 12 points, a heavy cross, 10 by 10, and the warning's filled triangle with a white mark, which lucide draws as outlines only.
var (
	pauseSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="11" height="12" viewBox="0 0 11 12" fill="currentColor"><rect x="1" y="0.5" width="3.2" height="11" rx="1"/><rect x="6.8" y="0.5" width="3.2" height="11" rx="1"/></svg>`))
	playSVG  = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="11" height="12" viewBox="0 0 11 12" fill="currentColor"><path d="M1.5 1.3v9.4a.9.9 0 0 0 1.4.76l7.3-4.7a.9.9 0 0 0 0-1.52L2.9.54A.9.9 0 0 0 1.5 1.3z"/></svg>`))
	closeSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M1.5 1.5l7 7M8.5 1.5l-7 7"/></svg>`))
	warnSVG  = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="M6.7 1.8a1.5 1.5 0 0 1 2.6 0l6 10.6A1.5 1.5 0 0 1 14 14.6H2a1.5 1.5 0 0 1-1.3-2.2z" fill="#FF9500"/><path d="M8 5.2v4.2" stroke="#FFFFFF" stroke-width="1.8" stroke-linecap="round"/><circle cx="8" cy="11.8" r="1" fill="#FFFFFF"/></svg>`))
)

// glyph is a round button's glyph at the size the design draws it.
type glyph struct {
	svg  *core.SVG
	w, h core.Points
}

var (
	pauseGlyph = glyph{pauseSVG, 11, 12}
	playGlyph  = glyph{playSVG, 11, 12}
	closeGlyph = glyph{closeSVG, 10, 10}
)

func warnIcon() *core.SVG { return warnSVG }
