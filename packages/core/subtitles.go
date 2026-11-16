package core

// The subtitle model Kando's players share, on the desktop (apps/kando/film) and the phone (apps/mobile/player): which lines of a bilingual cue a subtitle mode shows, in which order, and what shows for a cue that has no translation. The values are the ones the settings save.

// Subtitle modes: which lines of a bilingual cue the player shows.
const (
	SubBilingual   = "bilingual"
	SubTranslation = "translation"
	SubOriginal    = "original"
	SubOff         = "off"
)

// Subtitle orders: which line of a bilingual cue goes on top.
const (
	SubTranslationFirst = "translation-first"
	SubOriginalFirst    = "original-first"
)

// What the picture shows, in SubTranslation, for a cue that has no translation yet: its original dimmed, or nothing.
const (
	UntranslatedDim   = "dim"
	UntranslatedBlank = "blank"
)

// CueLine is one line of a cue as a player shows it: its text, whether it is the original rather than the translation, which picks its face, and Dim for an original standing in for a missing translation.
type CueLine struct {
	Text     string
	Original bool
	Dim      bool
}

// CueLines are the lines of a cue, its translation and its original, as mode and order pick them, for the picture and the line list. SubTranslation shows the original of a cue without a translation in its place. SubOff counts as bilingual here: it hides only the lines on the picture, and the list keeps both. Empty lines are left out.
func CueLines(mode, order, translation, original string) []CueLine {
	add := func(lines []CueLine, text string, isOriginal bool) []CueLine {
		if text == "" {
			return lines
		}
		return append(lines, CueLine{Text: text, Original: isOriginal})
	}
	switch mode {
	case SubTranslation:
		lines := add(nil, translation, false)
		if len(lines) == 0 {
			lines = add(lines, original, true)
		}
		return lines
	case SubOriginal:
		return add(nil, original, true)
	}
	if order == SubOriginalFirst {
		return add(add(nil, original, true), translation, false)
	}
	return add(add(nil, translation, false), original, true)
}

// PictureLines are the lines of a cue the picture shows, as CueLines picks them; in SubTranslation a cue without a translation shows its original dimmed, or nothing when untranslated is UntranslatedBlank. SubOff is the caller's to check: it shows no lines at all.
func PictureLines(mode, order, untranslated, translation, original string) []CueLine {
	lines := CueLines(mode, order, translation, original)
	if mode != SubTranslation || translation != "" || len(lines) == 0 {
		return lines
	}
	if untranslated == UntranslatedBlank {
		return nil
	}
	lines[0].Dim = true
	return lines
}
