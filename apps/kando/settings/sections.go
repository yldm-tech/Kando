package settings

import (
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"

	"github.com/nsgo-dev/nsgo/core"
)

// Sections of the settings window, as App.OpenSettings in the main package takes them and the window's sidebar lists them. The values are declared in the leaf package sectionid, which pages outside the settings use instead of importing this package.
const (
	SectionGeneral   = sectionid.General
	SectionSpeech    = sectionid.Speech
	SectionAI        = sectionid.AI
	SectionTranslate = sectionid.Translate
	SectionLook      = sectionid.Look
	SectionShortcuts = sectionid.Shortcuts
	SectionAccount   = sectionid.Account
)

// section is one category of the settings window: its title, its tile in the sidebar and the words the search field finds it by besides its title. The title and the keywords are read in the language the app shows under the scope they are given.
type section struct {
	id              string
	title, keywords func(cx *core.Scope) string
	icon            *core.SVG
}

var sections = []section{
	{SectionGeneral, locales.SettingsSectionGeneral, locales.SettingsKeywordsGeneral, iconGeneral},
	{SectionSpeech, locales.SettingsSectionSpeech, locales.SettingsKeywordsSpeech, iconSpeech},
	{SectionAI, locales.SettingsSectionAI, locales.SettingsKeywordsAI, iconAI},
	{SectionTranslate, locales.SettingsSectionTranslate, locales.SettingsKeywordsTranslate, iconTranslate},
	{SectionLook, locales.SettingsSectionLook, locales.SettingsKeywordsLook, iconLook},
	{SectionShortcuts, locales.SettingsSectionShortcuts, locales.SettingsKeywordsShortcuts, iconShortcuts},
	{SectionAccount, locales.SettingsSectionAccount, locales.SettingsKeywordsAccount, iconAccount},
}

// Known reports whether id is one of the Section values.
func Known(id string) bool {
	for _, s := range sections {
		if s.id == id {
			return true
		}
	}
	return false
}

// Title is the heading of section id in the language the app shows under cx, as the settings window's sidebar lists it, or "" for an id that is not a section. The command palette names the sections it opens by it.
func Title(cx *core.Scope, id string) string {
	for _, s := range sections {
		if s.id == id {
			return s.title(cx)
		}
	}
	return ""
}

// IDs are the sections in the order the settings window's sidebar lists them.
func IDs() []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.id
	}
	return out
}

// matches reports whether the search query finds s, by its title or its keywords in the language the app shows under cx, ignoring case. An empty query finds every section.
func (s section) matches(cx *core.Scope, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s.title(cx)+" "+s.keywords(cx)), q)
}
