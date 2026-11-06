package cheatsheet

import "github.com/egoist/mygo/ui"

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

// iconClose is the sheet's close button's ×, as the prototype's × of 15 draws it in the 16 the button gives its icon: arms 7 across, a stroke of 1.2.
var iconClose = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"><path d="M4.75 4.75l6.5 6.5M11.25 4.75l-6.5 6.5"/></svg>`)
