package study

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// LegacyFileName is the file Kando kept the saved lines in before this package, at the top of the library folder. Load takes its lines and leaves it as it is, so an older Kando opening the library still finds them.
const LegacyFileName = "saved-lines.json"

// legacyBook is LegacyFileName. Times are in seconds.
type legacyBook struct {
	Lines []legacyLine `json:"lines"`
}

type legacyLine struct {
	Video       string    `json:"video"`
	Start       float64   `json:"start"`
	End         float64   `json:"end,omitempty"`
	Original    string    `json:"original"`
	Translation string    `json:"translation,omitempty"`
	Film        string    `json:"film,omitempty"`
	SavedAt     time.Time `json:"savedAt,omitzero"`
	Box         int       `json:"box"`
	Due         time.Time `json:"due,omitzero"`
}

// importLegacy takes the lines of the library's saved-lines.json that the book lacks, unless the book took the file as it is now already (imported). found is whether the book was read from its own file: without one, a saved-lines.json that cannot be read is the error, as saving the book would leave its lines behind; with one, such a file is passed over, to be tried again on the next Load, so it does not keep the book's lines from the learner.
//
// A line the book has, at the same start of the same video, stays as it is in the book. A line removed from the book stays removed, unless the file has it saved again after its removal, as an older Kando does when the learner saves it again there. A line taken over gets an id and has its fields stamped when it was saved, on no device, as the file does not say which.
func (b *Book) importLegacy(found bool) error {
	data, err := os.ReadFile(filepath.Join(b.dir, LegacyFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		if found {
			return nil
		}
		return err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if hash == b.imported {
		return nil
	}
	var stored legacyBook
	if err := json.Unmarshal(data, &stored); err != nil {
		if found {
			return nil
		}
		return err
	}
	fresh := len(b.Lines) == 0
	for _, s := range stored.Lines {
		if s.Video == "" || s.Start < 0 {
			continue
		}
		l := Line{
			ID:    NewID(),
			Video: s.Video, Start: fromSeconds(s.Start), End: fromSeconds(s.End),
			Original: s.Original, Translation: s.Translation, Film: s.Film,
			SavedAt: s.SavedAt, Box: max(s.Box, 0), Due: s.Due,
		}
		if b.Find(l.Video, l.Start) >= 0 {
			continue
		}
		if slices.ContainsFunc(b.Removed, func(r Removed) bool {
			return r.Video == l.Video && SameStart(r.Start, l.Start) && !r.At.At.Before(l.SavedAt)
		}) {
			continue
		}
		at := Stamp{At: l.SavedAt}
		l.Stamps = Stamps{Video: at, Original: at, Translation: at, Note: at, Review: at}
		if fresh {
			b.Lines = append(b.Lines, l)
		} else {
			b.insert(l)
		}
	}
	b.imported = hash
	b.migrated = true
	return nil
}
