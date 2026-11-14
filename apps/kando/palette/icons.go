package palette

import "github.com/nsgo-dev/nsgo/core"

func icon(src string) *core.SVG { return core.MustParseSVG([]byte(src)) }

// svg is icon as the accessor NSGo's components take (ui.Icon).
func svg(icon *core.SVG) func() *core.SVG { return func() *core.SVG { return icon } }

// iconSearch is the magnifier before the field, the prototype's: a ring of 4.3 at (6, 6) and its handle to (12.5, 12.5) in a box of 14, stroked 1.5 with round ends.
var iconSearch = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="6" cy="6" r="4.3"/><path d="M9.3 9.3l3.2 3.2"/></svg>`)
