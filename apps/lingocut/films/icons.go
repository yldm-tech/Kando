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

// The glyphs below are the prototype's own. The monochrome ones take the color of the text around them (currentColor); the illustrations keep their colors and are drawn with ui.Image.
var (
	linkIcon     = icon(svgHead + `<path d="M6.6 9.4l-1.2 1.2a2.2 2.2 0 0 1-3.1-3.1l1.8-1.8a2.2 2.2 0 0 1 3.1 0M9.4 6.6l1.2-1.2a2.2 2.2 0 0 1 3.1 3.1l-1.8 1.8a2.2 2.2 0 0 1-3.1 0"/></svg>`)
	closeIcon    = icon(svgHead + `<path d="M5 5l6 6M11 5l-6 6"/></svg>`)
	moreIcon     = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 14 14" fill="currentColor"><circle cx="2" cy="7" r="1.4"/><circle cx="7" cy="7" r="1.4"/><circle cx="12" cy="7" r="1.4"/></svg>`)
	playIcon     = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 13"><path d="M1.5 1.2l9.5 5.3-9.5 5.3z" fill="currentColor"/></svg>`)
	downloadIcon = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M11 3v11M6.5 9.5L11 14l4.5-4.5M4 18h14"/></svg>`)
	addVideoIcon = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 18 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M1.8 3.4a1.2 1.2 0 0 1 1.2-1.2h3.8l1.6 1.7h6.6a1.2 1.2 0 0 1 1.2 1.2v7.7a1.2 1.2 0 0 1-1.2 1.2H3a1.2 1.2 0 0 1-1.2-1.2z"/><path d="M9 6.8v4.4M6.8 9h4.4"/></svg>`)
	dropIcon     = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 34 34" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 5v16M10.5 14.5L17 21l6.5-6.5M6 26h22"/></svg>`)

	// failedMark is the red circle with "!" over the thumbnail of a job that failed.
	failedMark = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 22 22"><circle cx="11" cy="11" r="9" fill="#FF453A"/><path d="M11 6.5v5.5M11 15v.1" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round"/></svg>`)
	// libraryArt is the blue folder of a library with no film yet, on the page without a library and in the new-library sheet.
	libraryArt = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="80" viewBox="0 0 96 80"><path d="M6 16a8 8 0 0 1 8-8h22l8 9h38a8 8 0 0 1 8 8v41a8 8 0 0 1-8 8H14a8 8 0 0 1-8-8z" fill="#5AA2F0"/><path d="M6 26a8 8 0 0 1 8-8h68a8 8 0 0 1 8 8v40a8 8 0 0 1-8 8H14a8 8 0 0 1-8-8z" fill="#7DB7F5"/><rect x="34" y="34" width="28" height="28" rx="7" fill="#FFFFFF" opacity="0.92"/><path d="M40 42h16M40 48h16M40 54h10" stroke="#0A66D8" stroke-width="2.4" stroke-linecap="round"/></svg>`)
	// startArt is the play tile of a library that holds no film yet.
	startArt = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="88" height="88" viewBox="0 0 88 88"><rect width="88" height="88" rx="24" fill="#F2F2F5"/><g transform="translate(22 22)" fill="none" stroke="#0A66D8" stroke-width="2"><rect x="5" y="9" width="34" height="26" rx="6"/><path d="M18.5 16.5l9 5.5-9 5.5z" fill="#0A66D8" stroke="none"/></g></svg>`)
)
