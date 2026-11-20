package learn

import "github.com/nsgo-dev/nsgo/core"

// Icon is the Learn tab's: a card of lines with another behind it, as the design draws the saved words and lines.
func Icon() *core.SVG { return learnIcon }

var learnIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="7" width="13" height="14" rx="2"/><path d="M8 3h10a2 2 0 0 1 2 2v12"/><path d="M7 12h5"/><path d="M7 16h3"/></svg>`))

// playIcon is the filled triangle that plays a line, as the design draws it, where Lucide's is an outline.
func playIcon() *core.SVG { return playGlyph }

var playGlyph = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M7 4.5v15a1 1 0 0 0 1.5.86l12.5-7.5a1 1 0 0 0 0-1.72L8.5 3.64A1 1 0 0 0 7 4.5z"/></svg>`))

// closeIcon is the review's ×, drawn heavier than Lucide's, as the design's small bold glyph.
func closeIcon() *core.SVG { return closeGlyph }

var closeGlyph = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`))

// doneIcon is the check at the review's end, drawn heavier than Lucide's, as the design's.
func doneIcon() *core.SVG { return doneGlyph }

var doneGlyph = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`))
