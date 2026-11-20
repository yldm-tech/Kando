package jobs

import "github.com/nsgo-dev/nsgo/core"

// The design's glyphs on a job's round buttons and in its failure box: a filled pause and play, a heavy cross, and the warning's filled triangle with a white mark, which lucide draws as outlines only.
var (
	pauseSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 12 12" fill="currentColor"><rect x="2" y="1" width="3" height="10" rx="1"/><rect x="7" y="1" width="3" height="10" rx="1"/></svg>`))
	playSVG  = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 12 12" fill="currentColor"><path d="M3 1.4v9.2a.8.8 0 0 0 1.2.7l7.3-4.6a.8.8 0 0 0 0-1.4L4.2.7A.8.8 0 0 0 3 1.4z"/></svg>`))
	closeSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M2.5 2.5l7 7M9.5 2.5l-7 7"/></svg>`))
	warnSVG  = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="M6.7 1.8a1.5 1.5 0 0 1 2.6 0l6 10.6A1.5 1.5 0 0 1 14 14.6H2a1.5 1.5 0 0 1-1.3-2.2z" fill="#FF9500"/><path d="M8 5.2v4.2" stroke="#FFFFFF" stroke-width="1.8" stroke-linecap="round"/><circle cx="8" cy="11.8" r="1" fill="#FFFFFF"/></svg>`))
)

func pauseIcon() *core.SVG { return pauseSVG }
func playIcon() *core.SVG  { return playSVG }
func closeIcon() *core.SVG { return closeSVG }
func warnIcon() *core.SVG  { return warnSVG }
