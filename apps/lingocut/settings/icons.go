package settings

// iconFolder below is Lucide's (https://lucide.dev), unchanged.
//
// ISC License. Copyright (c) 2026 Lucide Icons and Contributors. Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted, provided that the above copyright notice and this permission notice appear in all copies. THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
//
// The category glyphs (iconGeneral to iconAccount) are LingoCut's own, drawn for the prototype's settings sidebar on a 22-point grid.

import "github.com/egoist/mygo/ui"

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

// glyph parses one of the settings sidebar's 22-point line glyphs, drawn in the current color.
func glyph(body string) *ui.SVG {
	return icon(`<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`)
}

var (
	iconFolder = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>`)

	iconGeneral   = glyph(`<circle cx="11" cy="11" r="3"/><path d="M11 2.5v2.6M11 16.9v2.6M2.5 11h2.6M16.9 11h2.6M5 5l1.8 1.8M15.2 15.2L17 17M5 17l1.8-1.8M15.2 6.8L17 5"/>`)
	iconSpeech    = glyph(`<path d="M3 9v4M7 6v10M11 3v16M15 7v8M19 10v2"/>`)
	iconAI        = glyph(`<path d="M11 3l1.8 5.2L18 10l-5.2 1.8L11 17l-1.8-5.2L4 10l5.2-1.8z"/><path d="M17.5 15.5l.7 1.8 1.8.7-1.8.7-.7 1.8-.7-1.8-1.8-.7 1.8-.7z"/>`)
	iconTranslate = glyph(`<path d="M2.5 5h8M6.5 3v2M4 5c.6 3 2.5 5 5 6M9 5c-.6 3-2.5 5-5.5 6.5"/><path d="M11.5 19l4-9 4 9M13 15.8h5"/>`)
	iconLook      = glyph(`<rect x="2.5" y="4" width="17" height="14" rx="3.5"/><path d="M6 13h4M12 13h4M6 9.5h10"/>`)
	iconShortcuts = glyph(`<rect x="2" y="5.5" width="18" height="11" rx="2.5"/><path d="M5.5 9h1M9 9h1M12.5 9h1M16 9h.5M7 13h8"/>`)
	iconAccount   = glyph(`<circle cx="11" cy="11" r="8.5"/><circle cx="11" cy="9" r="3"/><path d="M5.5 17.3c1.3-2 3.2-3 5.5-3s4.2 1 5.5 3"/>`)
)
