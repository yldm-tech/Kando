package store

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// prefsFile keeps the phone's own preferences.
const prefsFile = "prefs.json"

// Prefs are what the user chose on this phone: whether they went through the introduction, the languages they watch (whisper's codes, as "en") and the language they want the subtitles translated into (a BCP 47 tag, as "zh-Hans"; "" before they chose one), which the paired computers translate into too (TargetPending are the paired computers, by ID, still to take it: each one paired while it was set, all of them once the user chose it, and the others once one of them changed it); the daily goal in minutes of films played (0 for the default), whether to be reminded to review, how the player draws subtitles, what the Learn tab's export writes, and the videos last looked at in Browse, newest first.
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
	Review  string  `json:"review,omitempty"`
	Visited []Visit `json:"visited,omitempty"`
}

// Export is what the Learn tab's export writes, as its page last chose it: the format, one of study's Vocab formats ("" for Anki), and what it leaves out of what it writes by default: the line each word was looked up in, that line's translation, and the saved lines after the words.
type Export struct {
	Format        string `json:"format,omitempty"`
	NoSentence    bool   `json:"no_sentence,omitempty"`
	NoTranslation bool   `json:"no_translation,omitempty"`
	NoLines       bool   `json:"no_lines,omitempty"`
}

// Visit is a page of one video looked at in Browse: its address and the title the page gave itself, "" while it had none.
type Visit struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// MaxVisits is how many videos Browse keeps of those last looked at.
const MaxVisits = 5

// Visited is visits with v first, in place of an earlier visit of its address, which keeps its title when v has none, and at most MaxVisits of them.
func Visited(visits []Visit, v Visit) []Visit {
	out := []Visit{v}
	for _, old := range visits {
		if old.URL == v.URL {
			if v.Title == "" {
				out[0].Title = old.Title
			}
			continue
		}
		// The rest are still looked through, for an earlier visit of v's address past the last kept, whose title v takes.
		if len(out) < MaxVisits {
			out = append(out, old)
		}
	}
	return out
}

// SubStyle is how the player draws subtitles: their size, from SubSmall to SubHuge, the color of the original line, from SubYellow on, and whether they show at the top of the picture rather than at its foot. The zero SubStyle is the player's own: medium, yellow, at the foot.
type SubStyle struct {
	Size  SubSize  `json:"size,omitempty"`
	Color SubColor `json:"color,omitempty"`
	Top   bool     `json:"top,omitempty"`
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
