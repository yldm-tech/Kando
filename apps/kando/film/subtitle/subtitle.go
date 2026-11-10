// Package subtitle is how the player draws subtitles and the values of the settings that pick which lines it shows. It imports no interface package, only the message catalogs, so the settings can show and change the look without importing the player (package film), which links to the settings by section id (package sectionid). film keeps aliases of these names, so the player's code and the main package read film.Look, film.SubOriginal and the rest as before.
package subtitle

import "kando/apps/kando/locales"

// Face is how one line of a cue is drawn: the translation's or the original's.
type Face struct {
	Family string
	Size   float32
	Color  string
	Bold   bool
}

// Look is how subtitles are drawn. Which lines show and which goes on top are not part of it: they are the app's SubMode and SubOrder.
type Look struct {
	// Preset is the style view's preset last picked: "outline", "box" or "large".
	Preset string
	// Box puts each line on a dark box; Shadow draws an outline around the glyphs.
	Box, Shadow bool
	// High lifts the subtitles to about a third of the picture's height, from the bottom.
	High bool
	// Lift is how far the pointer dragged the subtitles up from where High puts them, as a share of the picture's height, so they keep their place over the picture as the window changes size; a negative Lift moves them down. It is for dragging them off a film's own burned-in subtitles. Subtitles dragged anywhere stay there while the controls show, rather than yield to them.
	Lift        float32
	Translation Face
	Original    Face
}

// Subtitle modes: which lines of a bilingual cue the player shows. S steps through the three that show lines, in the order original, translation, bilingual.
const (
	Bilingual   = "bilingual"
	Translation = "translation"
	Original    = "original"
	Off         = "off"
)

// ModeName is what a subtitle mode is called in the interface's language; the mode itself is the value saved in the settings. An unknown mode is called bilingual, as the player shows it.
func ModeName(mode string) string {
	switch mode {
	case Off:
		return locales.FilmModeOff()
	case Original:
		return locales.FilmModeOriginal()
	case Translation:
		return locales.FilmModeTranslation()
	default:
		return locales.FilmModeBilingual()
	}
}

// OrderName is what a subtitle order is called in the interface's language.
func OrderName(order string) string {
	if order == OriginalFirst {
		return locales.FilmOrderOriginalFirst()
	}
	return locales.FilmOrderTranslationFirst()
}

// Subtitle orders: which line of a bilingual cue goes on top.
const (
	TranslationFirst = "translation-first"
	OriginalFirst    = "original-first"
)

// What the player shows for a cue that has no translation yet.
const (
	UntranslatedDim   = "dim"
	UntranslatedBlank = "blank"
)

// PreviewInset is how far above a preview tile's bottom edge the settings' sample cue sits, so the preview shows the subtitles as high as the player draws them. The player owns where subtitles sit; the settings read it only through this.
func (l Look) PreviewInset() float32 {
	// The player draws subtitles 44 points above the window's bottom, or at about 30% of its height when High; a preview tile is about a third of that scale.
	if l.High {
		return 40
	}
	return 15
}

// TargetLanguageName is how the interface names a target language as the settings, the jobs and the translation prompt keep it, such as 简体中文 or 日本語, in the interface's language: Simplified Chinese and Japanese in English. Every screen that names a film's or the settings' target language calls it, so they name it alike. A value typed into an older version is shown as it is.
func TargetLanguageName(value string) string {
	switch value {
	case "简体中文":
		return locales.SettingsLanguageSimplifiedChinese()
	case "繁体中文":
		return locales.SettingsLanguageTraditionalChinese()
	case "English":
		return locales.SettingsLanguageEnglish()
	case "日本語":
		return locales.SettingsLanguageJapanese()
	case "한국어":
		return locales.SettingsLanguageKorean()
	case "Español":
		return locales.SettingsLanguageSpanish()
	case "Français":
		return locales.SettingsLanguageFrench()
	case "Deutsch":
		return locales.SettingsLanguageGerman()
	}
	return value
}
