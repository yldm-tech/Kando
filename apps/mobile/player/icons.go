package player

import "github.com/nsgo-dev/nsgo/core"

// The transport's filled glyphs, as the design draws them: lucide's are outlines.
var (
	playIcon     = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M7 4.6v14.8a1 1 0 0 0 1.5.86l12.2-7.4a1 1 0 0 0 0-1.72L8.5 3.74A1 1 0 0 0 7 4.6z"/></svg>`))
	pauseIcon    = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><rect x="5.5" y="4" width="4.5" height="16" rx="1.2"/><rect x="14" y="4" width="4.5" height="16" rx="1.2"/></svg>`))
	skipBackIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><rect x="4" y="4" width="2.6" height="16" rx="1"/><path d="M20 5.2v13.6a1 1 0 0 1-1.55.83L8.3 12.83a1 1 0 0 1 0-1.66l10.15-6.8A1 1 0 0 1 20 5.2z"/></svg>`))
	skipNextIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><rect x="17.4" y="4" width="2.6" height="16" rx="1"/><path d="M4 5.2v13.6a1 1 0 0 0 1.55.83l10.15-6.8a1 1 0 0 0 0-1.66L5.55 4.37A1 1 0 0 0 4 5.2z"/></svg>`))
	starFillIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M11.1 2.9a1 1 0 0 1 1.8 0l2.4 4.9 5.4.8a1 1 0 0 1 .55 1.7l-3.9 3.8.92 5.37a1 1 0 0 1-1.45 1.05L12 18l-4.82 2.52a1 1 0 0 1-1.45-1.05l.92-5.37-3.9-3.8a1 1 0 0 1 .55-1.7l5.4-.8z"/></svg>`))
)

func playGlyph() *core.SVG     { return playIcon }
func pauseGlyph() *core.SVG    { return pauseIcon }
func skipBackGlyph() *core.SVG { return skipBackIcon }
func skipNextGlyph() *core.SVG { return skipNextIcon }
func starFilled() *core.SVG    { return starFillIcon }
