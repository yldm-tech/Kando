package subtitle

// Mark is a run of a line of a cue's original the player draws marked, by byte offsets in the line, Start before End. A mark with Ruby has its reading drawn over its characters, as furigana or pinyin; one without has them in the color of Tone and underlined with Underline. Tone is one of the tones a word list's tier names (accent, warn, success, muted), which the player maps to its theme's roles; "" keeps the line's color. A word may have both kinds of mark, the ruby mark over part of it. The app computes the marks from the plugins (package plugins/annotate, whose Mark is the same, so the app converts one into the other), and the player only draws them, so this package, and the player, import no plugin package.
type Mark struct {
	Start, End int
	Ruby       string
	Tone       string
	Underline  bool
}

// The tones a mark may be drawn in, as a word list's tiers name them.
const (
	ToneAccent  = "accent"
	ToneWarn    = "warn"
	ToneSuccess = "success"
	ToneMuted   = "muted"
)
