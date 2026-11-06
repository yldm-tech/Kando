package settings

import "lingocut/apps/lingocut/settings/sectionid"

// Sections of the settings, as App.OpenSettings in the main package takes them. The values are declared in the leaf package sectionid, which pages outside the settings use instead of importing this package. The first five are the sidebar pages the settings live on until they get a window of their own (sections_test.go checks that they match).
const (
	SectionGeneral   = sectionid.General
	SectionSpeech    = sectionid.Speech
	SectionAI        = sectionid.AI
	SectionTranslate = sectionid.Translate
	SectionLook      = sectionid.Look
	SectionShortcuts = sectionid.Shortcuts
	SectionAccount   = sectionid.Account
)
