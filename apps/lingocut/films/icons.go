package films

import "github.com/egoist/mygo/ui"

const svgHead = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">`

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

var linkIcon = icon(svgHead + `<path d="M6.6 9.4l-1.2 1.2a2.2 2.2 0 0 1-3.1-3.1l1.8-1.8a2.2 2.2 0 0 1 3.1 0M9.4 6.6l1.2-1.2a2.2 2.2 0 0 1 3.1 3.1l-1.8 1.8a2.2 2.2 0 0 1-3.1 0"/></svg>`)
