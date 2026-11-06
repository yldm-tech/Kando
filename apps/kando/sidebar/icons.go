package sidebar

// The sidebar's glyphs are the Kando prototype's own, drawn in currentColor so the sidebar tints them. The audio-lines, sparkles, languages and user icons are Lucide's (https://lucide.dev), unchanged.
//
// ISC License. Copyright (c) 2026 Lucide Icons and Contributors. Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted, provided that the above copyright notice and this permission notice appear in all copies. THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

import "github.com/egoist/mygo/ui"

func icon(src string) *ui.SVG {
	parsed, err := ui.ParseSVG([]byte(src))
	if err != nil {
		panic(err)
	}
	return parsed
}

// The prototype's glyphs. The sources' are drawn on an 18-point grid; a source list gives its icons 16 points, so their view boxes keep the middle 16 of it, where the glyphs lie, and they show at the prototype's size and stroke.
var (
	iconFilms  = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="1 1 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="2.5" y="3.5" width="13" height="11" rx="2.6"/><path d="M7.4 6.9l4 2.1-4 2.1z" fill="currentColor" stroke="none"/></svg>`)
	iconGlobe  = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="1 1 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="9" cy="9" r="6.5"/><path d="M2.6 9h12.8M9 2.5c1.9 2 2.8 4.1 2.8 6.5S10.9 13.5 9 15.5M9 2.5C7.1 4.5 6.2 6.6 6.2 9s.9 4.5 2.8 6.5"/></svg>`)
	iconTV     = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="1 1 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="2" y="5" width="14" height="10" rx="2.6"/><path d="M6 2.5l2 2.3M12 2.5l-2 2.3M6.5 9.2v1.6M11.5 9.2v1.6"/></svg>`)
	iconLink   = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="1 1 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M7.5 10.5l3-3M8 5.5l1.4-1.4a3 3 0 0 1 4.3 4.3L12.3 9.8M10 12.5l-1.4 1.4a3 3 0 0 1-4.3-4.3l1.4-1.4"/></svg>`)
	iconDrive  = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="1 1 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="2" y="5" width="14" height="8.5" rx="2.2"/><circle cx="12.6" cy="9.25" r="1" fill="currentColor" stroke="none"/></svg>`)
	iconFolder = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2.3 5.3a1.3 1.3 0 0 1 1.3-1.3h3.6l1.6 1.7h5.6a1.3 1.3 0 0 1 1.3 1.3v6.4a1.3 1.3 0 0 1-1.3 1.3H3.6a1.3 1.3 0 0 1-1.3-1.3z"/></svg>`)
	iconUpDown = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 4l2.5-2.5L7.5 4M2.5 6l2.5 2.5L7.5 6"/></svg>`)
	iconGear   = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><circle cx="8" cy="8" r="2.2"/><path d="M8 1.6v1.8M8 12.6v1.8M1.6 8h1.8M12.6 8h1.8M3.5 3.5l1.3 1.3M11.2 11.2l1.3 1.3M3.5 12.5l1.3-1.3M11.2 4.8l1.3-1.3"/></svg>`)
)

// Lucide icons the prototype's glyphs did not replace.
var (
	iconAudioLines = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 10v3"/><path d="M6 6v11"/><path d="M10 3v18"/><path d="M14 8v7"/><path d="M18 5v13"/><path d="M22 10v3"/></svg>`)
	iconSparkles   = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/><path d="M20 2v4"/><path d="M22 4h-4"/><circle cx="4" cy="20" r="2"/></svg>`)
	iconLanguages  = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 8 6 6"/><path d="m4 14 6-6 2-3"/><path d="M2 5h12"/><path d="M7 2h1"/><path d="m22 22-5-10-5 10"/><path d="M14 18h6"/></svg>`)
	iconUser       = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>`)
)
