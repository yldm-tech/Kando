package chrome

import "github.com/nsgo-dev/nsgo/core"

func icon(src string) *core.SVG { return core.MustParseSVG([]byte(src)) }

// svg is icon as the accessor NSGo's components take (ui.Icon, ui.IconOf).
func svg(icon *core.SVG) func() *core.SVG { return func() *core.SVG { return icon } }

// The design's glyphs, in currentColor so the buttons tint them: the search button's magnifier on a 12-point grid, and the header bar's minimize, maximize and close on a 10-point one, stroked as GNOME's; restore is maximize's square with another behind it, as the platforms draw it.
var (
	iconSearch   = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"><circle cx="5" cy="5" r="3.6"/><path d="M7.8 7.8L11 11"/></svg>`)
	iconMinimize = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M1.5 7.5h7"/></svg>`)
	iconMaximize = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="1.5" y="1.5" width="7" height="7" rx="1"/></svg>`)
	iconRestore  = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.3"><rect x="1" y="3" width="6" height="6" rx="1"/><path d="M3 1.6h4.4a1 1 0 0 1 1 1V7"/></svg>`)
	iconClose    = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M2 2l6 6M8 2l-6 6"/></svg>`)
)
