package store

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// prefsFile keeps the phone's own preferences.
const prefsFile = "prefs.json"

// Prefs are what the user chose on this phone: whether they went through the introduction, the languages they watch (whisper's codes, as "en") and the language they want the subtitles translated into (a BCP 47 tag, as "zh-Hans"; "" before they chose one), which the paired computers translate into too (TargetPending are the paired computers, by ID, still to take it: each one paired while it was set, all of them once the user chose it, and the others once one of them changed it); the daily goal in minutes of films played (0 for the default), whether to be reminded to review, how the player draws subtitles, what the Learn tab's export writes, the app's appearance and interface language, and the computer new links go to.
type Prefs struct {
	Onboarded     bool     `json:"onboarded"`
	Spoken        []string `json:"spoken,omitempty"`
	Target        string   `json:"target,omitempty"`
	TargetPending []string `json:"target_pending,omitempty"`
	Goal          int      `json:"goal,omitempty"`
	Remind        bool     `json:"remind,omitempty"`
	Subtitles     SubStyle `json:"subtitles,omitzero"`
	Export        Export   `json:"export,omitzero"`
	// Review is how the Learn tab reviews: "dictation", or "" for cards.
	Review string `json:"review,omitempty"`
	// Appearance is the app's look: AppearanceLight or AppearanceDark, or "" to follow the system's.
	Appearance string `json:"appearance,omitempty"`
	// Language is the language the app's interface shows, one of locales.Interface, or "" to follow the system's.
	Language string `json:"language,omitempty"`
	// SendTo is the paired computer new links go to and Settings shows as the one that processes them, by its ID; "" for the Add sheet's own pick.
	SendTo string `json:"send_to,omitempty"`
}

// The appearances the user can choose over the system's.
const (
	AppearanceLight = "light"
	AppearanceDark  = "dark"
)

// Export is what the Learn tab's export writes, as its page last chose it: the format, one of study's Vocab formats ("" for Anki), and what it leaves out of what it writes by default: the line each word was looked up in, that line's translation, and the saved lines after the words.
type Export struct {
	Format        string `json:"format,omitempty"`
	NoSentence    bool   `json:"no_sentence,omitempty"`
	NoTranslation bool   `json:"no_translation,omitempty"`
	NoLines       bool   `json:"no_lines,omitempty"`
}

// SubStyle is how the player draws subtitles: their size, from SubSmall to SubHuge, the color of the original line, from SubYellow on, whether they show at the top of the picture rather than at its foot, and whether the original's words show no marks of the computer's (NoMarks: readings over them and lines under them, docs/plugin-system.md E2). The zero SubStyle is the player's own: medium, yellow, at the foot, with the marks.
type SubStyle struct {
	Size    SubSize  `json:"size,omitempty"`
	Color   SubColor `json:"color,omitempty"`
	Top     bool     `json:"top,omitempty"`
	NoMarks bool     `json:"no_marks,omitempty"`
}

// SubSize is a subtitle size; the zero one is medium.
type SubSize int8

const (
	SubSmall  SubSize = -1
	SubMedium SubSize = 0
	SubLarge  SubSize = 1
	SubHuge   SubSize = 2
)

// SubSizes are the sizes in the order the settings offer them.
var SubSizes = []SubSize{SubSmall, SubMedium, SubLarge, SubHuge}

// Scale is how much larger than medium lines of size z are drawn.
func (z SubSize) Scale() float32 {
	switch z {
	case SubSmall:
		return 0.82
	case SubLarge:
		return 1.18
	case SubHuge:
		return 1.4
	}
	return 1
}

// SubColor is the color of the original line; the zero one is yellow.
type SubColor uint8

const (
	SubYellow SubColor = iota
	SubWhite
	SubCyan
	SubPink
)

// SubColors are the colors in the order the settings offer them.
var SubColors = []SubColor{SubYellow, SubWhite, SubCyan, SubPink}

// Prefs are the kept preferences, the zero Prefs before any were kept.
func (s *Store) Prefs() (Prefs, error) {
	var p Prefs
	err := readJSON(filepath.Join(s.dir, prefsFile), &p)
	if errors.Is(err, fs.ErrNotExist) {
		return Prefs{}, nil
	}
	return p, err
}

// SavePrefs keeps p.
func (s *Store) SavePrefs(p Prefs) error { return writeJSON(filepath.Join(s.dir, prefsFile), p) }
