// Package subtitle is how the player draws subtitles and the values of the settings that pick which lines it shows. It imports nothing, so the settings can show and change the look without importing the player (package film), which links to the settings by section id (package sectionid). film keeps aliases of these names, so the player's code and the main package read film.Look, film.SubOriginal and the rest as before.
package subtitle

// Face is how one line of a cue is drawn: the translation's or the original's.
type Face struct {
	Family string
	Size   float32
	Color  string
	Bold   bool
}

// Look is how subtitles are drawn. Which lines show and which goes on top are not part of it: they are the app's SubMode and SubOrder.
type Look struct {
	Preset, Edit string
	Bottom       float32
	Box, Shadow  bool
	Translation  Face
	Original     Face
}

// Subtitle modes: which lines of a bilingual cue the player shows. V steps through them in this order.
const (
	Bilingual   = "bilingual"
	Translation = "translation"
	Original    = "original"
	Off         = "off"
)

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
