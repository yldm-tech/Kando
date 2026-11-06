package film

import "lingocut/apps/lingocut/film/subtitle"

// The subtitle look and the values of the display settings are declared in the leaf package film/subtitle, so the settings can use them without importing this package. These are the player's names for them.
type (
	Face = subtitle.Face
	Look = subtitle.Look
)

const (
	SubBilingual   = subtitle.Bilingual
	SubTranslation = subtitle.Translation
	SubOriginal    = subtitle.Original
	SubOff         = subtitle.Off

	SubTranslationFirst = subtitle.TranslationFirst
	SubOriginalFirst    = subtitle.OriginalFirst

	UntranslatedDim   = subtitle.UntranslatedDim
	UntranslatedBlank = subtitle.UntranslatedBlank
)
