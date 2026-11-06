package cheatsheet

import "github.com/egoist/mygo/ui"

const svgHead = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">`

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

// iconClose is the sheet's close button's ×.
var iconClose = icon(svgHead + `<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/></svg>`)
