package main

import "github.com/egoist/mygo/ui"

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

const svgHead = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">`

var (
	iconMark  = icon(svgHead + `<rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M7 6.2 L11 8 L7 9.8 Z" fill="currentColor" stroke="none"/></svg>`)
	iconFilm  = icon(svgHead + `<rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M6.5 6.2v3.6L10.5 8z"/></svg>`)
	iconTube  = icon(svgHead + `<rect x="2" y="4" width="12" height="8" rx="2"/><path d="M7 6.5v3l2.6-1.5z"/></svg>`)
	iconGear  = icon(svgHead + `<circle cx="8" cy="8" r="2.2"/><path d="M8 2.2v1.6M8 12.2v1.6M2.2 8h1.6M12.2 8h1.6M3.8 3.8l1.1 1.1M11.1 11.1l1.1 1.1M12.2 3.8l-1.1 1.1M4.9 11.1l-1.1 1.1"/></svg>`)
	iconWave  = icon(svgHead + `<path d="M2 8h1.2M4 5.5v5M6.2 3.5v9M8.4 6v4M10.6 4.5v7M12.8 6.5v3M14 8h.01"/></svg>`)
	iconSpark = icon(svgHead + `<path d="M8 2.2l1.1 3.6L12.8 8 9.1 9.2 8 13.8 6.9 9.2 3.2 8l3.7-1.2z"/></svg>`)
	iconLang  = icon(svgHead + `<path d="M3 4.5h6M6 4.5c0 3.2-2.2 5.4-4 6.5M4.2 7.2c.7 1.3 2 2.6 3.8 3.3M9 13.2l2.2-5.4 2.2 5.4M9.8 11.4h2.8"/></svg>`)
	iconLink  = icon(svgHead + `<path d="M6.6 9.4l-1.2 1.2a2.2 2.2 0 0 1-3.1-3.1l1.8-1.8a2.2 2.2 0 0 1 3.1 0M9.4 6.6l1.2-1.2a2.2 2.2 0 0 1 3.1 3.1l-1.8 1.8a2.2 2.2 0 0 1-3.1 0"/></svg>`)
	iconPlay  = icon(svgHead + `<path d="M6 4.2 L12.2 8 L6 11.8 Z" fill="currentColor" stroke="none"/></svg>`)
	iconPause = icon(svgHead + `<path d="M5 3.8h2.1v8.4H5zM8.9 3.8H11v8.4H8.9z" fill="currentColor" stroke="none"/></svg>`)
)

func navIcon(page string) *ui.SVG {
	switch page {
	case pageYouTube:
		return iconTube
	case pageGeneral:
		return iconGear
	case pageSpeech:
		return iconWave
	case pageAI:
		return iconSpark
	case pageTranslate:
		return iconLang
	default:
		return iconFilm
	}
}
