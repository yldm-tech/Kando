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
	// Preset is the style view's preset last picked: "outline", "box" or "large".
	Preset string
	// Box puts each line on a dark box; Shadow draws an outline around the glyphs.
	Box, Shadow bool
	// High lifts the subtitles to about a third of the picture's height, from the bottom.
	High        bool
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
func (l Look) PreviewInset() float32 { return l.Bottom / 3 }
