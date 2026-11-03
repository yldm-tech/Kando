package film

import "github.com/egoist/mygo/ui"

const svgHead = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">`

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

var (
	iconPlay  = icon(svgHead + `<path d="M6 4.2 L12.2 8 L6 11.8 Z" fill="currentColor" stroke="none"/></svg>`)
	iconPause = icon(svgHead + `<path d="M5 3.8h2.1v8.4H5zM8.9 3.8H11v8.4H8.9z" fill="currentColor" stroke="none"/></svg>`)
)
