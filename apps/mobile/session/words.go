package session

import (
	"slices"
	"strings"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/store"
	"kando/apps/mobile/study"
	kstudy "kando/packages/study"
	"kando/packages/sync"
)

// The vocabulary as the player reads and changes it (yldm-tech/Kando#222): the words of a film's library, which the Learn tab lists and reviews too, put in from the word looked up in a line, or taken out, and the words looked up in a film's lines on the phone. A word goes into the library's learning state as a word record (sync.KindWord), which the desktop takes into its own vocabulary, and syncs as a saved line does (changeStudy).

// Looked is a word looked up on the phone in a line of a film: the word as tapped, the line's original it was tapped in, and what the computer answered.
type Looked struct {
	Word, Line string
	Entry      sync.WordEntry
}

// Vocabulary are the words of the vocabulary of the film under key's library spoken in its language, the phone's changes not yet sent among them, the latest put in first. Read it in a binding.
func (s *Session) Vocabulary(key string) []learning.Item {
	f, ok := s.LinesFilm(key)
	if !ok || f.LibraryID == "" {
		return nil
	}
	items := learning.Items(f.Desktop, f.LibraryID, s.studies.Get()[studyKey(f.Desktop, f.LibraryID)])
	return slices.DeleteFunc(items, func(it learning.Item) bool { return it.Kind != learning.Word || !sameSpoken(it.Spoken, f.Film.Spoken) })
}

// VocabularyWord is the word text of the vocabulary of the film under key's library, in whatever case, spoken in the film's language, false for none. Read it in a binding.
func (s *Session) VocabularyWord(key, text string) (learning.Item, bool) {
	want := kstudy.WordKey(text)
	if want == "" {
		return learning.Item{}, false
	}
	for _, it := range s.Vocabulary(key) {
		if kstudy.WordKey(it.Text) == want {
			return it, true
		}
	}
	return learning.Item{}, false
}

// sameSpoken reports whether a and b name one spoken language, as the desktop's vocabulary compares them.
func sameSpoken(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// ToggleWord puts word, as e, the computer's lookup of it, gives it, looked up in c, a line of the film under key, in its library's vocabulary, or takes it out once it is in, and reports whether it is in now. A film whose library is not known yet changes nothing (Studyable).
func (s *Session) ToggleWord(key, word string, e sync.WordEntry, c study.Cue) bool {
	f, ok := s.LinesFilm(key)
	if !ok || f.LibraryID == "" {
		return false
	}
	text := strings.TrimSpace(e.Word)
	if text == "" {
		text = strings.TrimSpace(word)
	}
	if text == "" {
		return false
	}
	if it, ok := s.VocabularyWord(key, text); ok {
		s.changeStudy(f, study.RemoveWord(it.ID, it.Film, s.cfg.Now()))
		s.scheduleReminder()
		return false
	}
	s.changeStudy(f, study.AddWord(sync.NewCallID(), f.Film.ID, f.Film.Title, f.Film.Spoken, word, e, c, s.cfg.Now()))
	s.scheduleReminder()
	return true
}

// LookedUp are the words looked up on the phone in lines, the originals of the lines of the film under key, in the order the lookups were made. Read it in a binding: it changes as a word is looked up.
func (s *Session) LookedUp(key string, lines []string) []Looked {
	_ = s.lookupRev.Get()
	f, ok := s.LinesFilm(key)
	if !ok || len(lines) == 0 {
		return nil
	}
	in := map[string]bool{}
	for _, l := range lines {
		in[strings.TrimSpace(l)] = true
	}
	spoken := strings.ToLower(strings.TrimSpace(f.Film.Spoken))
	type found struct {
		Looked
		at store.Lookup
	}
	var out []found
	for k, l := range s.lookups {
		word, lang, line, ok := store.SplitLookupKey(k)
		if !ok || lang != spoken || !in[line] {
			continue
		}
		out = append(out, found{Looked: Looked{Word: word, Line: line, Entry: l.Entry}, at: l})
	}
	slices.SortStableFunc(out, func(a, b found) int {
		if c := a.at.At.Compare(b.at.At); c != 0 {
			return c
		}
		return strings.Compare(a.Word, b.Word)
	})
	looked := make([]Looked, len(out))
	for i, f := range out {
		looked[i] = f.Looked
	}
	return looked
}
