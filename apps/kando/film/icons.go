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

// svgBox is the head of a glyph of the prototype's drawn in a w×h viewBox, stroked 1.5 wide unless its paths say otherwise.
func svgBox(w, h string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + w + ` ` + h + `" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">`
}

// The player's glyphs, the prototype's (template 556-640), each drawn at the size the prototype draws it.
var (
	iconPlay        = icon(svgHead + `<path d="M3.5 1.8l10.5 6.2-10.5 6.2z" fill="currentColor" stroke="none"/></svg>`)
	iconPause       = icon(svgHead + `<rect x="3" y="2" width="3.4" height="12" rx="1" fill="currentColor" stroke="none"/><rect x="9.6" y="2" width="3.4" height="12" rx="1" fill="currentColor" stroke="none"/></svg>`)
	iconBigPlay     = icon(svgBox("28", "30") + `<path d="M7 4l18 11-18 11z" fill="currentColor" stroke="none"/></svg>`)
	iconBack5       = icon(svg20 + `<path d="M4.5 7.5A6.5 6.5 0 1 1 3.6 11"/><path d="M4.5 3.5v4h4"/></svg>`)
	iconForward5    = icon(svg20 + `<path d="M15.5 7.5A6.5 6.5 0 1 0 16.4 11"/><path d="M15.5 3.5v4h-4"/></svg>`)
	iconSubtitles   = icon(svgBox("20", "15") + `<rect x="1" y="1" width="18" height="13" rx="3.5"/><path d="M5 9.5h4M11 9.5h4M5 6h10"/></svg>`)
	iconPanel       = icon(svgBox("18", "15") + `<rect x="1" y="1" width="16" height="13" rx="3" stroke-width="1.4"/><path d="M11.5 1.5v12" stroke-width="1.4"/></svg>`)
	iconFullScreen  = icon(svgBox("16", "16") + `<path d="M2 6V2h4M14 6V2h-4M2 10v4h4M14 10v4h-4"/></svg>`)
	iconChevronLeft = icon(svgBox("10", "16") + `<path d="M8 2L2 8l6 6" stroke-width="1.9"/></svg>`)
	iconFolder      = icon(svgBox("18", "16") + `<path d="M1.8 3.6a1 1 0 0 1 1-1h4l1.6 1.7h6.8a1 1 0 0 1 1 1v7.9a1 1 0 0 1-1 1H2.8a1 1 0 0 1-1-1z" stroke-width="1.4" stroke-linecap="butt" stroke-linejoin="miter"/></svg>`)
	iconUpload      = icon(svgBox("16", "18") + `<path d="M8 11V1.8M4.8 4.8L8 1.6l3.2 3.2M5 7.5H3.2a1 1 0 0 0-1 1v7.2a1 1 0 0 0 1 1h9.6a1 1 0 0 0 1-1V8.5a1 1 0 0 0-1-1H11"/></svg>`)
	iconMore        = icon(svgBox("14", "4") + `<circle cx="2" cy="2" r="1.5" fill="currentColor" stroke="none"/><circle cx="7" cy="2" r="1.5" fill="currentColor" stroke="none"/><circle cx="12" cy="2" r="1.5" fill="currentColor" stroke="none"/></svg>`)
	iconSearch      = icon(svgBox("14", "14") + `<circle cx="6" cy="6" r="4.3"/><path d="M9.3 9.3l3.2 3.2"/></svg>`)
	iconClose       = icon(svgHead + `<path d="M4 4l8 8M12 4l-8 8"/></svg>`)
)
