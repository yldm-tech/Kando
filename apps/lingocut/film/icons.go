package film

import "github.com/egoist/mygo/ui"

const svgHead = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">`

// svg20 is the head of the prototype's 20-unit glyphs: the skip arrows, the subtitle and panel marks.
const svg20 = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">`

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

// The player's glyphs, drawn after the prototype's (template 553-640).
var (
	iconPlay        = icon(svgHead + `<path d="M4 2.2 L13.6 8 L4 13.8 Z" fill="currentColor" stroke="none"/></svg>`)
	iconPause       = icon(svgHead + `<path d="M3.6 2.4h3.2v11.2H3.6zM9.2 2.4h3.2v11.2H9.2z" fill="currentColor" stroke="none"/></svg>`)
	iconBack5       = icon(svg20 + `<path d="M4.5 7.5A6.5 6.5 0 1 1 3.6 11"/><path d="M4.5 3.5v4h4"/></svg>`)
	iconForward5    = icon(svg20 + `<path d="M15.5 7.5A6.5 6.5 0 1 0 16.4 11"/><path d="M15.5 3.5v4h-4"/></svg>`)
	iconSubtitles   = icon(svg20 + `<rect x="1" y="3" width="18" height="13" rx="3.5"/><path d="M5 11.5h4M11 11.5h4M5 8h10"/></svg>`)
	iconPanel       = icon(svg20 + `<rect x="1.5" y="3" width="17" height="14" rx="3"/><path d="M12.5 3.5v13"/></svg>`)
	iconFullScreen  = icon(svgHead + `<path d="M2 6V2h4M14 6V2h-4M2 10v4h4M14 10v4h-4"/></svg>`)
	iconChevronLeft = icon(svgHead + `<path d="M10.5 2.5 5 8l5.5 5.5" stroke-width="1.8"/></svg>`)
	iconFolder      = icon(svgHead + `<path d="M1.6 3.8a1 1 0 0 1 1-1h3.6l1.5 1.6h5.7a1 1 0 0 1 1 1v6.8a1 1 0 0 1-1 1H2.6a1 1 0 0 1-1-1z"/></svg>`)
	iconMore        = icon(svgHead + `<circle cx="3" cy="8" r="1.4" fill="currentColor" stroke="none"/><circle cx="8" cy="8" r="1.4" fill="currentColor" stroke="none"/><circle cx="13" cy="8" r="1.4" fill="currentColor" stroke="none"/></svg>`)
	iconClose       = icon(svgHead + `<path d="M4 4l8 8M12 4l-8 8"/></svg>`)
)
