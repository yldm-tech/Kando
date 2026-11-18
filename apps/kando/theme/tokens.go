package theme

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
)

// The design's fixed colors: the same in both appearances, because each is drawn on a picture, on a dark subtree or as a mark whose meaning is its color. The phone app has the same ones (apps/mobile/theme); a change here goes there too.
var (
	// The source marks: a film's platform tag in the link field, its letter badge, its dot. SourceOther is a web page that is neither, SourceLocal a file on this computer.
	SourceYouTube  = core.RGB(0xE62117)
	SourceBilibili = core.RGB(0x00A1D6)
	SourceOther    = core.RGB(0x5E5CE6)
	SourceLocal    = core.RGB(0x8E8E93)
	// SubtitleYellow is the original line's default color under the translation, and what marks a saved line on the dark player. The look saved in settings.json keeps its own "#FFD60A"; this is the color for drawing.
	SubtitleYellow = core.RGB(0xFFD60A)
	// AI is the accent of what an AI service does: its tile, its share of a bar, its badge.
	AI = core.RGB(0x5E5CE6)
	// The status dots: ready or done (StatusOK), something missing or waiting (StatusWarn), failed (StatusFailed).
	StatusOK     = core.RGB(0x30D158)
	StatusWarn   = core.RGB(0xFF9F0A)
	StatusFailed = core.RGB(0xFF453A)
)

// Radii are a platform's corner radii in the design: Dialog the window's and a sheet's or dialog's drawn in it, Card a card's on the page, as the sidebar's and the hero's.
type Radii struct {
	Dialog, Card core.Points
}

// RadiiFor is the design's radii on os: macOS rounds its sheets 20 and its cards 14, Windows rounds both 8 and Linux (GNOME's libadwaita) 12. Another system gets macOS's.
func RadiiFor(os core.OS) Radii {
	switch os {
	case core.OSWindows:
		return Radii{Dialog: 8, Card: 8}
	case core.OSLinux:
		return Radii{Dialog: 12, Card: 12}
	}
	return Radii{Dialog: 20, Card: 14}
}

// RadiiOf is the radii of the system cx runs on.
func RadiiOf(cx *core.Scope) Radii { return RadiiFor(cx.Env().OS()) }

// FontFor is the design's font family list for the interface on os in the language tag lang, as core.Style's FontFamily takes it: "" on macOS, whose system font is the design's (SF Pro and PingFang); Segoe UI Variable on Windows and Cantarell on Linux, each followed by the platform's CJK family for the interface's language, so Han characters take that language's forms.
func FontFor(os core.OS, lang string) string {
	switch os {
	case core.OSWindows:
		// TODO(nsgo#668): DirectWrite is asked for the first family of the list that is installed (internal/platform/windows/text.go firstInstalled), not each in turn for the characters those before it lack as core.Style.FontFamily says, so the CJK family here is used only where Segoe UI is missing; the system's fallback for Language picks the CJK font meanwhile https://github.com/nsgo-dev/nsgo/issues/668
		return "Segoe UI Variable Text, Segoe UI, " + windowsCJK(lang) + ", sans-serif"
	case core.OSLinux:
		cjk := linuxCJK(lang)
		return "Cantarell, Noto Sans CJK " + cjk + ", Noto Sans " + cjk + ", Noto Sans, Ubuntu, sans-serif"
	}
	return ""
}

// FontOf is FontFor the system cx runs on and the language its interface shows, read in a binding so it follows the language as it changes.
func FontOf(cx *core.Scope) string { return FontFor(cx.Env().OS(), i18n.Language(cx)) }

// windowsCJK is Windows' UI family for Han, kana or hangul in lang: Microsoft YaHei UI for Simplified Chinese and for a language without them.
func windowsCJK(lang string) string {
	switch script(lang) {
	case "TC":
		return "Microsoft JhengHei UI"
	case "JP":
		return "Yu Gothic UI"
	case "KR":
		return "Malgun Gothic"
	}
	return "Microsoft YaHei UI"
}

// linuxCJK is the suffix of the Noto Sans CJK family for lang: SC for Simplified Chinese and for a language without Han.
func linuxCJK(lang string) string { return script(lang) }

// script is the CJK region lang's Han characters are drawn in: TC, JP, KR or SC.
func script(lang string) string {
	switch {
	case lang == "zh-Hant" || lang == "zh-TW" || lang == "zh-HK":
		return "TC"
	case lang == "ja" || len(lang) > 3 && lang[:3] == "ja-":
		return "JP"
	case lang == "ko" || len(lang) > 3 && lang[:3] == "ko-":
		return "KR"
	}
	return "SC"
}
