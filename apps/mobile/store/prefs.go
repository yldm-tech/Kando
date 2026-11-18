package store

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// prefsFile keeps the phone's own preferences.
const prefsFile = "prefs.json"

// Prefs are what the user chose on this phone: whether they went through the introduction, the languages they watch (whisper's codes, as "en") and the language they want the subtitles translated into (a BCP 47 tag, as "zh-Hans"; "" before they chose one). The languages are the phone's until the protocol carries them to the computer (yldm-tech/Kando#217).
type Prefs struct {
	Onboarded bool     `json:"onboarded"`
	Spoken    []string `json:"spoken,omitempty"`
	Target    string   `json:"target,omitempty"`
}

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
