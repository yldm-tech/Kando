package session

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"kando/apps/mobile/store"
	"kando/apps/mobile/study"
	"kando/packages/sync"
)

// The learning state of the films kept on the phone (yldm-tech/Kando#222): the lines saved from them and the notes on those lines, which the phone keeps per library of each desktop (study.Library) and syncs with the desktop through protocol v2's learning state (sync.OpStateGet, OpStatePut) whenever the player opens a film of the library or changes something, so a line saved on the phone shows on the computer and the other way round. A change made away from the computer waits on the phone until it can be sent. The words of a line are looked up, and a line explained, through the desktop too (LookUp, Explain): its AI service answers, and its key never leaves it.

// studyRun is a sync of a library's learning state running on a goroutine of its own; again asks for another once it is done, as for a change made meanwhile.
type studyRun struct{ again bool }

// studyKey names library lib of desktop.
func studyKey(desktop, lib string) string { return desktop + "/" + lib }

// studyTimeout, lookupTimeout and explainTimeout are how long a sync of the learning state, a lookup or an explanation waits for the desktop.
const (
	studyTimeout   = 15 * time.Second
	lookupTimeout  = 60 * time.Second
	explainTimeout = 100 * time.Second
)

// Studyable reports whether lines of the film under key can be saved: its library on the desktop is known, which the phone asks the desktop for the first time the film opens (OpenStudy). Read it in a binding.
func (s *Session) Studyable(key string) bool {
	f, ok := s.Film(key)
	return ok && f.LibraryID != ""
}

// Saved are the lines saved from the film under key, in the order they play, the phone's changes not yet sent among them. Read it in a binding.
func (s *Session) Saved(key string) []study.Line {
	f, ok := s.Film(key)
	if !ok || f.LibraryID == "" {
		return nil
	}
	return study.Lines(s.studies.Get()[studyKey(f.Desktop, f.LibraryID)], f.Film.ID)
}

// SavedAt is the saved line of the film under key that starts at start, false for none.
func (s *Session) SavedAt(key string, start time.Duration) (study.Line, bool) {
	f, ok := s.Film(key)
	if !ok || f.LibraryID == "" {
		return study.Line{}, false
	}
	return study.At(s.studies.Get()[studyKey(f.Desktop, f.LibraryID)], f.Film.ID, start)
}

// OpenStudy reads the learning state of the library of the film under key, and syncs it with the desktop when it can be reached; a film whose library the phone does not know yet asks the desktop for it first.
func (s *Session) OpenStudy(key string) {
	f, ok := s.Film(key)
	if !ok {
		return
	}
	if f.LibraryID == "" {
		s.findLibrary(f)
		return
	}
	s.loadStudy(f.Desktop, f.LibraryID, true)
}

// findLibrary asks f's desktop which of its libraries holds f, the one named as f's first, and keeps it with the film, then opens its learning state.
func (s *Session) findLibrary(f store.Film) {
	key := f.Key()
	if s.finding[key] {
		return
	}
	c := s.Client(f.Desktop)
	if c == nil {
		return
	}
	s.finding[key] = true
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, studyTimeout)
		defer cancel()
		id := libraryOf(ctx, c, f)
		s.cx.Post(func() {
			delete(s.finding, key)
			cur, ok := s.Film(key)
			if id == "" || !ok {
				return
			}
			cur.LibraryID = id
			s.replace(cur)
			s.saveFilm(cur)
			s.loadStudy(cur.Desktop, id, true)
		})
	})
}

// libraryOf is the ID of the library of c's desktop that holds f, the libraries named as f's first, "" when none does or the desktop cannot be reached.
func libraryOf(ctx context.Context, c *sync.Client, f store.Film) string {
	libs, err := c.Libraries(ctx)
	if err != nil {
		return ""
	}
	slices.SortStableFunc(libs, func(a, b sync.Library) int {
		switch {
		case a.Name == f.Library && b.Name != f.Library:
			return -1
		case b.Name == f.Library && a.Name != f.Library:
			return 1
		}
		return 0
	})
	for _, lib := range libs {
		films, err := c.Films(ctx, lib.ID)
		if err != nil {
			continue
		}
		if slices.ContainsFunc(films, func(x sync.Film) bool { return x.ID == f.Film.ID }) {
			return lib.ID
		}
	}
	return ""
}

// loadStudy reads library lib of desktop's learning state from the phone's disk the first time, off the engine's goroutine, and then syncs it when andSync is set.
func (s *Session) loadStudy(desktop, lib string, andSync bool) {
	k := studyKey(desktop, lib)
	if _, ok := s.studies.GetUntracked()[k]; ok {
		if andSync {
			s.syncStudy(desktop, lib)
		}
		return
	}
	if s.studyLoading[k] {
		return
	}
	s.studyLoading[k] = true
	s.work.Go(func() {
		l, _ := s.st.Learning(desktop, lib)
		s.cx.Post(func() {
			delete(s.studyLoading, k)
			if _, ok := s.studies.GetUntracked()[k]; !ok {
				m := maps.Clone(s.studies.GetUntracked())
				m[k] = l
				s.studies.Set(m)
			}
			if andSync {
				s.syncStudy(desktop, lib)
			}
		})
	})
}

// changeStudy lays change over the learning state of the library of the film under key, keeps it and sends it to the desktop.
func (s *Session) changeStudy(f store.Film, change sync.Record) {
	k := studyKey(f.Desktop, f.LibraryID)
	s.setStudy(f.Desktop, f.LibraryID, study.Change(s.studies.GetUntracked()[k], change))
	s.syncStudy(f.Desktop, f.LibraryID)
}

// setStudy keeps l as library lib of desktop's learning state, in the session at once and on disk after the saves of it handed before, the latest last.
func (s *Session) setStudy(desktop, lib string, l study.Library) {
	k := studyKey(desktop, lib)
	m := maps.Clone(s.studies.GetUntracked())
	m[k] = l
	s.studies.Set(m)
	s.studyMu.Lock()
	s.studySaving[k]++
	gen := s.studySaving[k]
	s.studyMu.Unlock()
	s.work.Go(func() {
		s.studyMu.Lock()
		defer s.studyMu.Unlock()
		if gen > s.studySaved[k] && s.st.SaveLearning(desktop, lib, l) == nil {
			s.studySaved[k] = gen
		}
	})
}

// ToggleSave saves c, a line of the film under key, or takes it back once it is saved, and reports whether it is saved now. A film whose library is not known yet saves nothing (Studyable).
func (s *Session) ToggleSave(key string, c study.Cue) bool {
	f, ok := s.Film(key)
	if !ok || f.LibraryID == "" {
		return false
	}
	if line, ok := s.SavedAt(key, c.Start); ok {
		s.changeStudy(f, study.Remove(line.ID, f.Film.ID, s.cfg.Now()))
		return false
	}
	s.changeStudy(f, study.Save(sync.NewCallID(), f.Film.ID, f.Film.Title, c, "", s.cfg.Now()))
	return true
}

// SetNote writes note as the note of c, a line of the film under key, saving the line when it is not saved yet. A film whose library is not known yet keeps nothing (Studyable).
func (s *Session) SetNote(key string, c study.Cue, note string) {
	f, ok := s.Film(key)
	if !ok || f.LibraryID == "" {
		return
	}
	if line, ok := s.SavedAt(key, c.Start); ok {
		l := s.studies.GetUntracked()[studyKey(f.Desktop, f.LibraryID)]
		s.changeStudy(f, study.Note(l, line.ID, f.Film.ID, note, s.cfg.Now()))
		return
	}
	s.changeStudy(f, study.Save(sync.NewCallID(), f.Film.ID, f.Film.Title, c, note, s.cfg.Now()))
}

// putBudget is how many bytes of records a put of the learning state carries at most, below sync.MaxCall by room for the call around them.
const putBudget = sync.MaxCall - 4<<10

// studyPut is a put of the learning state as the desktop answered it: the records sent, what it answered, and whether it refused them for good.
type studyPut struct {
	sent     []sync.Record
	res      sync.StatePutResult
	rejected bool
}

// syncStudy sends library lib of desktop's pending changes and takes in the desktop's since the revision the phone has, on a goroutine of its own, one sync of a library at a time. The changes go in puts the desktop takes (study.Batches); a put it refuses for good, as one with a change of a film it no longer serves, is put again a change at a time, so only the changes it refuses are dropped. A desktop out of reach leaves the changes not taken pending for the next sync.
func (s *Session) syncStudy(desktop, lib string) {
	k := studyKey(desktop, lib)
	if run, ok := s.studyRuns[k]; ok {
		run.again = true
		return
	}
	c := s.Client(desktop)
	if c == nil {
		return
	}
	run := &studyRun{}
	s.studyRuns[k] = run
	l := s.studies.GetUntracked()[k]
	sent := slices.Clone(l.Pending)
	if len(sent) > 0 {
		// From here the desktop may have them, though it does not answer.
		l = study.Sending(l, sent)
		s.setStudy(desktop, lib, l)
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, studyTimeout)
		defer cancel()
		puts, reached := putStudy(ctx, c, lib, sent)
		var ch sync.StateChanges
		var getErr error
		if reached {
			ch, getErr = c.State(ctx, lib, l.Epoch, l.Rev)
		}
		s.cx.Post(func() {
			delete(s.studyRuns, k)
			cur := s.studies.GetUntracked()[k]
			changed := false
			for _, p := range puts {
				cur, changed = study.Took(cur, p.sent, p.res, p.rejected), true
			}
			if reached && getErr == nil {
				cur, changed = study.Apply(cur, ch), true
			}
			if changed {
				s.setStudy(desktop, lib, cur)
			}
			if run.again {
				s.syncStudy(desktop, lib)
			}
		})
	})
}

// putStudy puts records into library lib through c in batches the desktop takes, a batch it refuses (sync.CodeInvalid) again a record at a time, and reports the puts it answered, and whether it answered them all; it stops at the first it does not.
func putStudy(ctx context.Context, c *sync.Client, lib string, records []sync.Record) ([]studyPut, bool) {
	var out []studyPut
	put := func(batch []sync.Record) (refused, ok bool) {
		res, err := c.PutState(ctx, lib, batch)
		var e *sync.Error
		switch {
		case err == nil:
			out = append(out, studyPut{sent: batch, res: res})
			return false, true
		case errors.As(err, &e) && e.Code == sync.CodeInvalid:
			return true, true
		}
		return false, false
	}
	for _, batch := range study.Batches(records, putBudget) {
		refused, ok := put(batch)
		if !ok {
			return out, false
		}
		if !refused {
			continue
		}
		if len(batch) == 1 {
			out = append(out, studyPut{sent: batch, rejected: true})
			continue
		}
		for _, r := range batch {
			one := []sync.Record{r}
			refused, ok := put(one)
			if !ok {
				return out, false
			}
			if refused {
				out = append(out, studyPut{sent: one, rejected: true})
			}
		}
	}
	return out, true
}

// StudyProblem is why a lookup or an explanation failed, as the player tells it.
type StudyProblem uint8

const (
	// StudyUnreachable is a desktop the phone cannot reach.
	StudyUnreachable StudyProblem = iota
	// StudyUnpaired is a film whose desktop is no longer paired, or that revoked the phone.
	StudyUnpaired
	// StudyNoAI is a desktop with no AI service that can answer (sync.CodeNoAI).
	StudyNoAI
	// StudyFailed is a desktop whose AI service failed to answer, or a desktop too old to ask.
	StudyFailed
)

// StudyProblemOf is why err happened, as the player tells it.
func StudyProblemOf(err error) StudyProblem {
	var e *sync.Error
	switch {
	case errors.As(err, &e) && e.Code == sync.CodeNoAI:
		return StudyNoAI
	case errors.As(err, &e) && e.Status == 401:
		return StudyUnpaired
	case errors.As(err, &e) && (e.Code == sync.CodeFailed || e.Code == sync.CodeUnsupported || e.Status == 404):
		return StudyFailed
	case errors.Is(err, errUnpaired) || errors.Is(err, sync.ErrFingerprint):
		return StudyUnpaired
	}
	return StudyUnreachable
}

// errUnpaired is the error of a lookup or an explanation of a film whose desktop is not paired any more.
var errUnpaired = errors.New("the film's desktop is not paired")

// LookUp looks word up, as it reads in line, a line of the film under key, through the film's desktop, and calls done on the engine's goroutine with what it means: from the words looked up before on the phone when it is among them, else as the desktop answers, which the phone then keeps.
func (s *Session) LookUp(key, word, line string, done func(sync.WordEntry, error)) {
	f, ok := s.Film(key)
	word = strings.TrimSpace(word)
	if !ok || word == "" {
		return
	}
	lk := store.LookupKey(word, f.Film.Spoken, line)
	if l, ok := s.lookups[lk]; ok {
		done(l.Entry, nil)
		return
	}
	c := s.Client(f.Desktop)
	if c == nil {
		done(sync.WordEntry{}, errUnpaired)
		return
	}
	req := sync.WordLookup{Library: f.LibraryID, Film: f.Film.ID, Word: word, Line: strings.TrimSpace(line)}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, lookupTimeout)
		defer cancel()
		e, err := c.LookUp(ctx, req)
		s.cx.Post(func() {
			if err == nil {
				s.keepLookup(lk, e)
			}
			done(e, err)
		})
	})
}

// keepLookup keeps e as the lookup under lk, in the session at once and on disk after the saves handed before.
func (s *Session) keepLookup(lk string, e sync.WordEntry) {
	s.lookups[lk] = store.Lookup{Entry: e, At: s.cfg.Now()}
	m := maps.Clone(s.lookups)
	s.lookupMu.Lock()
	s.lookupSaving++
	gen := s.lookupSaving
	s.lookupMu.Unlock()
	s.work.Go(func() {
		s.lookupMu.Lock()
		defer s.lookupMu.Unlock()
		if gen > s.lookupSaved && s.st.SaveLookups(m) == nil {
			s.lookupSaved = gen
		}
	})
}

// Explain has the film's desktop explain line i of lines, the film under key's lines around it, and calls done on the engine's goroutine with the explanation.
func (s *Session) Explain(key string, lines []sync.Cue, i int, done func(sync.Explanation, error)) {
	f, ok := s.Film(key)
	if !ok || i < 0 || i >= len(lines) {
		return
	}
	c := s.Client(f.Desktop)
	if c == nil {
		done(sync.Explanation{}, errUnpaired)
		return
	}
	req := sync.LineExplain{Library: f.LibraryID, Film: f.Film.ID, Lines: lines, Index: i}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, explainTimeout)
		defer cancel()
		x, err := c.Explain(ctx, req)
		s.cx.Post(func() { done(x, err) })
	})
}

// SetDelay keeps d as how far the subtitles of the film under key are moved against its picture.
func (s *Session) SetDelay(key string, d time.Duration) {
	f, ok := s.Film(key)
	if !ok || f.DelayMS == d.Milliseconds() {
		return
	}
	f.DelayMS = d.Milliseconds()
	s.replace(f)
	s.saveFilm(f)
}
