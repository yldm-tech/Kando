package settings

import "lingocut/apps/lingocut/sidebar"

// Sections of the settings, as App.OpenSettings in the main package takes them. The first five are the sidebar pages the settings live on until they get a window of their own.
const (
	SectionGeneral   = sidebar.PageGeneral
	SectionSpeech    = sidebar.PageSpeech
	SectionAI        = sidebar.PageAI
	SectionTranslate = sidebar.PageTranslate
	SectionLook      = "look"
	SectionShortcuts = "shortcuts"
	SectionAccount   = sidebar.PageAccount
)
