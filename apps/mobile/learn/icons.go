package learn

import "github.com/nsgo-dev/nsgo/core"

// Icon is the Learn tab's: a card of lines with another behind it, as the design draws the saved words and lines.
func Icon() *core.SVG { return learnIcon }

var learnIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="7" width="13" height="14" rx="2"/><path d="M8 3h10a2 2 0 0 1 2 2v12"/><path d="M7 12h5"/><path d="M7 16h3"/></svg>`))
