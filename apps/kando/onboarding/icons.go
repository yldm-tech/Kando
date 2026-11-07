package onboarding

import "github.com/egoist/mygo/ui"

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

// The prototype's own glyphs. The monochrome ones take the color of the text around them (currentColor); the marks keep their colors and are drawn with ui.Image.
var (
	linkIcon = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M6.5 9.5l3-3M7 4.5l1.3-1.3a2.8 2.8 0 0 1 4 4L11 8.5M9 11.5l-1.3 1.3a2.8 2.8 0 0 1-4-4L5 7.5"/></svg>`)
	lockIcon = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.3"><rect x="2" y="5.5" width="8" height="5" rx="1.2"/><path d="M4 5.5V4a2 2 0 0 1 4 0v1.5"/></svg>`)
	// checkOn is the filled blue disc with a white tick of a chosen language tile, checkOff the empty ring of one that is not chosen.
	checkOn  = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="8" fill="#0A66D8"/><path d="M5.3 8.2l1.8 1.8 3.6-3.9" fill="none" stroke="#FFFFFF" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	checkOff = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="7.5" fill="none" stroke="#000000" stroke-opacity="0.2" stroke-width="1"/></svg>`)
	// readyMark is the green disc with a tick of a speech model that is on this Mac and in use.
	readyMark = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="8" fill="#34C759"/><path d="M4.8 8.2l2.1 2.1 4.3-4.6" fill="none" stroke="#FFFFFF" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
)
