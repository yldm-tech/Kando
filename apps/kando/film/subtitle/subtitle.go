// Package subtitle is how the player draws subtitles and the values of the settings that pick which lines it shows. It imports no interface package, only the message catalogs, Kando's shared subtitle model (kando/packages/core), NSGo's core for the scope they speak under and NSGo's desktop for the menu of target languages (TranslateIntoMenu), so the settings can show and change the look without importing the player (package film), which links to the settings by section id (package sectionid). film keeps aliases of these names, so the player's code and the main package read film.Look, film.SubOriginal and the rest as before.
package subtitle

import (
	"slices"
	"strings"

	"kando/apps/kando/locales"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

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
	// High lifts the subtitles to about a third of the picture's height, from the bottom. Where a film's own subtitles were moved from there is the film's, not the look's: the player keeps it with the film (film.App.SubLift).
	High        bool
	Translation Face
	Original    Face
}

// Subtitle modes: which lines of a bilingual cue the player shows. S steps through the three that show lines, in the order original, translation, bilingual. The values are the subtitle model Kando's players share (kando/packages/core), the phone's too.
const (
	Bilingual   = kcore.SubBilingual
	Translation = kcore.SubTranslation
	Original    = kcore.SubOriginal
	Off         = kcore.SubOff
)

// ModeName is what a subtitle mode is called in the language the interface shows under cx, for a binding; the mode itself is the value saved in the settings. An unknown mode is called bilingual, as the player shows it.
func ModeName(cx *core.Scope, mode string) string { return ModeNameIn(locales.For(cx), mode) }

// ModeNameIn is ModeName in the language of m, for words said outside a scope and kept as they were said, as a message on the picture.
func ModeNameIn(m locales.Messages, mode string) string {
	switch mode {
	case Off:
		return m.FilmModeOff()
	case Original:
		return m.FilmModeOriginal()
	case Translation:
		return m.FilmModeTranslation()
	default:
		return m.FilmModeBilingual()
	}
}

// OrderName is what a subtitle order is called in the language the interface shows under cx.
func OrderName(cx *core.Scope, order string) string {
	if order == OriginalFirst {
		return locales.FilmOrderOriginalFirst(cx)
	}
	return locales.FilmOrderTranslationFirst(cx)
}

// Subtitle orders: which line of a bilingual cue goes on top.
const (
	TranslationFirst = kcore.SubTranslationFirst
	OriginalFirst    = kcore.SubOriginalFirst
)

// What the player shows for a cue that has no translation yet.
const (
	UntranslatedDim   = kcore.UntranslatedDim
	UntranslatedBlank = kcore.UntranslatedBlank
)

// PreviewInset is how far above a preview tile's bottom edge the settings' sample cue sits, so the preview shows the subtitles as high as the player draws them. The player owns where subtitles sit; the settings read it only through this.
func (l Look) PreviewInset() float32 {
	// The player draws subtitles 44 points above the window's bottom, or at about 30% of its height when High; a preview tile is about a third of that scale.
	if l.High {
		return 40
	}
	return 15
}

// TargetLanguages are the target languages Kando offers, as the settings, the jobs and the translation prompt keep them, such as 简体中文; TargetLanguageName is what the interface shows for each.
func TargetLanguages() []string {
	return []string{"简体中文", "繁体中文", "English", "日本語", "한국어", "Español", "Français", "Deutsch",
		"Português", "Italiano", "Русский", "Nederlands", "Polski", "Українська", "Türkçe", "العربية", "हिन्दी", "Bahasa Indonesia", "Tiếng Việt", "ไทย"}
}

// TranslateIntoMenu is the items of 重新翻译为 ▸, the submenu of a done film's menus in the library and the player: preferred, the target language the settings name, first, then the others Kando offers, and current, the film's own, checked, and added when it is none of them, as one typed into an older version. choose translates the film into the language picked. With no preferred language set, current comes first.
func TranslateIntoMenu(cx *core.Scope, preferred, current string, choose func(language string)) []nsdesktop.MenuItem {
	preferred, current = strings.TrimSpace(preferred), strings.TrimSpace(current)
	var order []string
	for _, language := range append([]string{preferred, current}, TargetLanguages()...) {
		if language != "" && !slices.Contains(order, language) {
			order = append(order, language)
		}
	}
	items := make([]nsdesktop.MenuItem, len(order))
	for i, language := range order {
		items[i] = nsdesktop.MenuItem{Label: TargetLanguageName(cx, language), Checkable: true, Checked: language == current, OnSelect: func() { choose(language) }}
	}
	return items
}

// TargetLanguageName is how the interface names a target language as the settings, the jobs and the translation prompt keep it, such as 简体中文 or 日本語, in the language the interface shows under cx: Simplified Chinese and Japanese in English. Every screen that names a film's or the settings' target language calls it, so they name it alike. A value typed into an older version is shown as it is.
func TargetLanguageName(cx *core.Scope, value string) string {
	return TargetLanguageNameIn(locales.For(cx), value)
}

// TargetLanguageNameIn is TargetLanguageName in the language of m, for words said outside a scope.
func TargetLanguageNameIn(m locales.Messages, value string) string {
	switch value {
	case "简体中文":
		return m.SettingsLanguageSimplifiedChinese()
	case "繁体中文":
		return m.SettingsLanguageTraditionalChinese()
	case "English":
		return m.SettingsLanguageEnglish()
	case "日本語":
		return m.SettingsLanguageJapanese()
	case "한국어":
		return m.SettingsLanguageKorean()
	case "Español":
		return m.SettingsLanguageSpanish()
	case "Français":
		return m.SettingsLanguageFrench()
	case "Deutsch":
		return m.SettingsLanguageGerman()
	case "Português":
		return m.SettingsLanguagePortuguese()
	case "Italiano":
		return m.SettingsLanguageItalian()
	case "Русский":
		return m.SettingsLanguageRussian()
	case "Nederlands":
		return m.SettingsLanguageDutch()
	case "Polski":
		return m.SettingsLanguagePolish()
	case "Українська":
		return m.SettingsLanguageUkrainian()
	case "Türkçe":
		return m.SettingsLanguageTurkish()
	case "العربية":
		return m.SettingsLanguageArabic()
	case "हिन्दी":
		return m.SettingsLanguageHindi()
	case "Bahasa Indonesia":
		return m.SettingsLanguageIndonesian()
	case "Tiếng Việt":
		return m.SettingsLanguageVietnamese()
	case "ไทย":
		return m.SettingsLanguageThai()
	}
	return value
}

// SpokenLanguageName is how the interface names a language the speech recognition hears, as whisper's code keeps it (ja, en), in the language the interface shows under cx: the settings' spoken-language drop-down and a film's information name it alike. A code the settings do not offer is shown in capitals, as JA would be.
func SpokenLanguageName(cx *core.Scope, code string) string {
	return SpokenLanguageNameIn(locales.For(cx), code)
}

// SpokenLanguageNameIn is SpokenLanguageName in the language of m, for words said outside a scope.
func SpokenLanguageNameIn(m locales.Messages, code string) string {
	switch code {
	case "en":
		return m.SettingsSpeechLanguageEn()
	case "zh":
		return m.SettingsSpeechLanguageZh()
	case "ja":
		return m.SettingsSpeechLanguageJa()
	case "ko":
		return m.SettingsSpeechLanguageKo()
	case "fr":
		return m.SettingsSpeechLanguageFr()
	case "es":
		return m.SettingsSpeechLanguageEs()
	case "de":
		return m.SettingsSpeechLanguageDe()
	case "pt":
		return m.SettingsSpeechLanguagePt()
	case "it":
		return m.SettingsSpeechLanguageIt()
	case "ru":
		return m.SettingsSpeechLanguageRu()
	case "nl":
		return m.SettingsSpeechLanguageNl()
	case "pl":
		return m.SettingsSpeechLanguagePl()
	case "uk":
		return m.SettingsSpeechLanguageUk()
	case "tr":
		return m.SettingsSpeechLanguageTr()
	case "ar":
		return m.SettingsSpeechLanguageAr()
	case "hi":
		return m.SettingsSpeechLanguageHi()
	case "id":
		return m.SettingsSpeechLanguageId()
	case "vi":
		return m.SettingsSpeechLanguageVi()
	case "th":
		return m.SettingsSpeechLanguageTh()
	}
	return strings.ToUpper(code)
}
