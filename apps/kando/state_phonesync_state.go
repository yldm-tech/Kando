package main

import (
	"context"
	"errors"
	"hash/fnv"
	"log"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/saved"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/study"
	psync "kando/packages/sync"
)

// Phone Sync's learning state (packages/sync, state.go): a paired phone reads and writes a library's resume positions, saved lines with their notes and review schedule, and the corrections of its films' lines, as records merged field by field. The desktop keeps them where it kept them before: the positions with the shelf (a.player.positions), the lines in the library's study record (a.saved.book for the open library), the corrections in each film's corrections file. Each library's ledger (phonesync.LedgerFile, in the library folder) is what phones were last told: phoneState reads the desktop's state into it on the engine's goroutine at each call (Observe), which stamps what changed on the desktop, merges what a phone puts (Put) and keeps the result where the desktop keeps it. A locked library's state is neither read nor written: to a phone it is not found, as its library is not listed. phoneSyncPublishState tells the phones waiting for events when the open library's state changed on the desktop.

// phoneState is the libraries' learning state as protocol v2's calls read and write it (psync.StateService).
type phoneState struct{ a *app }

// phoneStateLib is a library's learning state as a call reads it, on the engine's goroutine: its ledger with the desktop's state taken in, its study record, and its films by their IDs and by their videos' keys in the study record.
type phoneStateLib struct {
	lib    phonesync.Library
	ledger *psync.Ledger
	book   saved.Book
	// open is set when book is the open library's (a.saved.book), which is changed in place.
	open  bool
	films map[string]phonesync.Film
	keys  map[string]string
}

// GetState answers the changes of the library's learning state after the revision the phone has.
func (s phoneState) GetState(ctx context.Context, caller psync.Caller, req psync.StateGet) (psync.StateChanges, error) {
	return phoneStateCall(ctx, s.a, req.Library, func(st *phoneStateLib) (psync.StateChanges, error) {
		out := st.ledger.Since(st.lib.ID, req.Epoch, req.Since)
		out.Devices = s.a.phoneStateDevices(caller)
		return out, nil
	})
}

// PutState merges the phone's changes into the library's ledger, records the conflicts they make, keeps the records that changed where the desktop keeps them, and tells the other phones.
func (s phoneState) PutState(ctx context.Context, _ psync.Caller, req psync.StatePut) (psync.StatePutResult, error) {
	a := s.a
	return phoneStateCall(ctx, a, req.Library, func(st *phoneStateLib) (psync.StatePutResult, error) {
		known := func(film string) bool { _, ok := st.films[film]; return ok }
		have := func(kind, id string) bool { _, ok := st.ledger.Record(kind, id); return ok }
		if why := phonesync.StateAccepts(req.Records, known, have); why != "" {
			return psync.StatePutResult{}, psync.Errorf(psync.CodeInvalid, "%s", why)
		}
		changed, conflicts := st.ledger.Put(req.Records, study.NewID)
		if err := a.phoneStateKeep(st, changed); err != nil {
			return psync.StatePutResult{}, err
		}
		a.phoneStateSettle(st)
		out := psync.StatePutResult{Epoch: st.ledger.Epoch(), Rev: st.ledger.Rev(), Records: []psync.Record{}, Conflicts: conflicts}
		for _, r := range req.Records {
			if rec, ok := st.ledger.Record(r.Kind, r.ID); ok {
				out.Records = append(out.Records, rec)
			}
		}
		if out.Conflicts == nil {
			out.Conflicts = []psync.Conflict{}
		}
		return out, nil
	})
}

// ResolveConflict keeps the value the phone chose for the conflict's field, as a change made by the phone now, and closes the conflict.
func (s phoneState) ResolveConflict(ctx context.Context, caller psync.Caller, req psync.ConflictResolve) (psync.StatePutResult, error) {
	a := s.a
	return phoneStateCall(ctx, a, req.Library, func(st *phoneStateLib) (psync.StatePutResult, error) {
		rec, c, err := st.ledger.Resolve(req.ID, req.Value, psync.Stamp{At: a.phoneStateNow(), Device: caller.DeviceID})
		if errors.Is(err, psync.ErrNoConflict) {
			return psync.StatePutResult{}, psync.Errorf(psync.CodeNotFound, "no open conflict %q", req.ID)
		}
		if err != nil {
			return psync.StatePutResult{}, err
		}
		if err := a.phoneStateKeep(st, []psync.Record{rec}); err != nil {
			return psync.StatePutResult{}, err
		}
		a.phoneStateSettle(st)
		return psync.StatePutResult{Epoch: st.ledger.Epoch(), Rev: st.ledger.Rev(), Records: []psync.Record{rec}, Conflicts: []psync.Conflict{c}}, nil
	})
}

// phoneStateCall runs fn on the library lib's learning state, read afresh into its ledger (phoneStateRead), on the engine's goroutine, and keeps the ledger once fn returns, when it changed. A library the desktop does not serve, a locked one among them, is not found. The library's films are listed off the engine's goroutine first, as Phone Sync lists them (phoneSource.Films).
func phoneStateCall[T any](ctx context.Context, a *app, lib string, fn func(st *phoneStateLib) (T, error)) (T, error) {
	var zero T
	src := phoneSource{a}
	libs, err := src.Libraries(ctx)
	if err != nil {
		return zero, err
	}
	i := slices.IndexFunc(libs, func(l phonesync.Library) bool { return l.ID == lib })
	if i < 0 {
		return zero, psync.Errorf(psync.CodeNotFound, "no library %q", lib)
	}
	films, err := src.Films(ctx, libs[i])
	if errors.Is(err, phonesync.ErrNotFound) {
		return zero, psync.Errorf(psync.CodeNotFound, "no library %q", lib)
	}
	if err != nil {
		return zero, err
	}
	return remote.OnUI(ctx, a.update, func() (T, error) {
		st, err := a.phoneStateRead(libs[i], films)
		if err != nil {
			return zero, err
		}
		out, err := fn(st)
		if werr := a.phoneStateKeepLedger(st.lib.Dir); werr != nil && err == nil {
			return zero, werr
		}
		return out, err
	})
}

// phoneStateRead reads the learning state of lib, whose films are films, into its ledger, which it reads the first time: the study record, the resume positions of its films and their corrections. The ledger stamps what changed on the desktop since it last read it. A library locked since it was listed is not found; one whose study record or ledger cannot be read fails, rather than tell phones its lines are gone. UI thread only.
func (a *app) phoneStateRead(lib phonesync.Library, films []phonesync.Film) (*phoneStateLib, error) {
	if a.libraryLocked(lib.Dir) {
		return nil, psync.Errorf(psync.CodeNotFound, "no library %q", lib.ID)
	}
	st := &phoneStateLib{lib: lib, films: map[string]phonesync.Film{}, keys: map[string]string{}}
	if dir := filepath.Clean(lib.Dir); filepath.Clean(a.saved.book.Dir()) == dir && filepath.Clean(a.snap().LibraryDir) == dir {
		st.book, st.open = a.saved.book, true
	} else {
		book, err := a.savedLoad(lib.Dir)
		if err != nil {
			return nil, err
		}
		st.book = book
	}
	if err := st.book.Unread(); err != nil {
		return nil, err
	}
	ledger, err := a.phoneStateLedger(lib.Dir)
	if err != nil {
		return nil, err
	}
	st.ledger = ledger
	read := map[string]bool{}
	var current []psync.Record
	for _, f := range films {
		st.films[f.ID] = f
		st.keys[st.book.Key(f.Video)] = f.ID
		if at := a.player.positions[f.Video]; at > 0 {
			current = append(current, phonesync.PositionRecord(f.ID, at))
		}
		edits, err := media.LoadCorrections(media.CorrectionsPath(f.Video))
		if err != nil {
			// A film whose corrections cannot be read keeps what the ledger has of them, rather than tell phones they are gone.
			continue
		}
		read[f.ID] = true
		current = append(current, phonesync.CorrectionRecords(f.ID, edits)...)
	}
	current = append(current, phonesync.LineRecords(st.book, func(video string) string { return st.keys[video] })...)
	covered := func(r psync.Record) bool {
		switch r.Kind {
		case psync.KindLine:
			return true
		case psync.KindPosition:
			_, ok := st.films[r.Film]
			return ok
		case psync.KindCorrection:
			return read[r.Film]
		}
		return false
	}
	st.ledger.Observe(current, covered, psync.Stamp{At: a.phoneStateNow(), Device: a.saved.device})
	return st, nil
}

// phoneLedger is a library's ledger as read, and the revision at which it was last kept on disk.
type phoneLedger struct {
	ledger *psync.Ledger
	kept   int64
}

// phoneStateLedger is the ledger of the library at dir, read the first time it is asked for. One that cannot be read fails, and is not written over. UI thread only.
func (a *app) phoneStateLedger(dir string) (*psync.Ledger, error) {
	dir = filepath.Clean(dir)
	if l, ok := a.phone.ledgers[dir]; ok {
		return l.ledger, nil
	}
	l, err := psync.ReadLedger(phonesync.LedgerPath(dir))
	if err != nil {
		return nil, err
	}
	if a.phone.ledgers == nil {
		a.phone.ledgers = map[string]*phoneLedger{}
	}
	a.phone.ledgers[dir] = &phoneLedger{ledger: l, kept: l.Rev()}
	return l, nil
}

// phoneStateKeepLedger keeps the ledger of the library at dir on disk when it changed since it was last kept. UI thread only.
func (a *app) phoneStateKeepLedger(dir string) error {
	l, ok := a.phone.ledgers[filepath.Clean(dir)]
	if !ok || l.ledger.Rev() == l.kept {
		return nil
	}
	if err := psync.WriteLedger(phonesync.LedgerPath(dir), l.ledger); err != nil {
		return err
	}
	l.kept = l.ledger.Rev()
	return nil
}

// phoneStateKeep keeps records, as the ledger has them once merged, where the desktop keeps them: the lines in the study record, the positions with the shelf, the corrections in their films' files, the open film's through the player so it shows them. UI thread only.
func (a *app) phoneStateKeep(st *phoneStateLib, records []psync.Record) error {
	var lines []psync.Record
	moved := false
	corrections := map[string][]psync.Record{}
	for _, r := range records {
		switch r.Kind {
		case psync.KindLine:
			lines = append(lines, r)
		case psync.KindPosition:
			f, ok := st.films[r.Film]
			if !ok {
				continue
			}
			a.phoneStateSetPosition(f.Video, phonesync.PositionOf(r), r.Fields[psync.FieldPositionMS].At)
			moved = true
		case psync.KindCorrection:
			corrections[r.Film] = append(corrections[r.Film], r)
		}
	}
	if moved {
		a.playerPositionsChanged()
	}
	if len(lines) > 0 {
		in, removed := phonesync.LinesOf(st.book, lines, func(film string) (string, bool) {
			f, ok := st.films[film]
			if !ok {
				return "", false
			}
			return st.book.Key(f.Video), true
		})
		if st.book.Merge(in, removed) {
			if st.open {
				a.saved.book = st.book
				a.savedWrite()
			} else if err := st.book.Save(); err != nil {
				return err
			}
		}
	}
	for film, recs := range corrections {
		f, ok := st.films[film]
		if !ok {
			continue
		}
		if err := a.phoneStateCorrect(f.Video, recs); err != nil {
			return err
		}
	}
	return nil
}

// phoneStateSetPosition keeps at as where the video at path was left, played at played, as the player keeps it (rememberPosition); 0 starts it from its beginning. The caller has it saved (playerPositionsChanged).
func (a *app) phoneStateSetPosition(path string, at time.Duration, played time.Time) {
	if a.player.positions == nil {
		a.player.positions = map[string]time.Duration{}
	}
	if a.player.played == nil {
		a.player.played = map[string]time.Time{}
	}
	if at > 0 {
		a.player.positions[path] = at
	} else {
		delete(a.player.positions, path)
	}
	if played.After(a.player.played[path]) {
		a.player.played[path] = played
	}
}

// phoneStateCorrect lays records, corrections of the film whose video is at video, over its corrections file. The open film's go through the player (correctWrite), which shows them, and stay out of its Undo, as they are not the user's own steps there; another film's are written to its file.
func (a *app) phoneStateCorrect(video string, records []psync.Record) error {
	path := media.CorrectionsPath(video)
	open := a.correctLoaded() && media.CorrectionsPath(a.player.subs.path) == path
	var edits []core.Correction
	if open {
		edits = a.player.subs.edits
	} else {
		var err error
		if edits, err = media.LoadCorrections(path); err != nil {
			return err
		}
	}
	for _, r := range records {
		if c, ok := phonesync.CorrectionOf(r); ok {
			edits = phonesync.SetCorrection(edits, c)
		}
	}
	if open {
		if !a.correctWrite(edits) {
			return errors.New("the open film's corrections could not be written")
		}
		return nil
	}
	return media.SaveCorrections(path, edits)
}

// phoneStateSettle has the ledger take in what keeping the records changed on the desktop, as the study record making one line of two saved at the same start, and tells the phones waiting for events that the library's state changed. UI thread only.
func (a *app) phoneStateSettle(st *phoneStateLib) {
	if fresh, err := a.phoneStateRead(st.lib, slices.Collect(func(yield func(phonesync.Film) bool) {
		for _, f := range st.films {
			if !yield(f) {
				return
			}
		}
	})); err == nil {
		*st = *fresh
	} else {
		log.Printf("phonesync: read the learning state again: %v", err)
	}
	if srv := a.phone.server; srv != nil {
		srv.PublishState(st.lib.ID)
	}
	a.phone.stateSign = a.phoneStateSign()
}

// phoneStateDevices names the devices the stamps name: this computer, by its id in the study record, and the paired phones. UI thread only.
func (a *app) phoneStateDevices(caller psync.Caller) map[string]string {
	out := map[string]string{}
	if a.saved.device != "" {
		out[a.saved.device] = phoneSyncName()
	}
	for _, d := range a.phone.devices {
		out[d.ID] = d.Name
	}
	if caller.DeviceID != "" {
		out[caller.DeviceID] = caller.Name
	}
	return out
}

func (a *app) phoneStateNow() time.Time {
	if a.phone.now != nil {
		return a.phone.now()
	}
	return time.Now()
}

// phoneSyncPublishState tells the phones waiting for events when the open library's learning state changed on the desktop: its resume positions, its saved lines or the open film's corrections (phoneStateSign). It runs after every change of the app's state, and changes nothing the window reads.
func (a *app) phoneSyncPublishState() {
	srv := a.phone.server
	if srv == nil {
		return
	}
	sign := a.phoneStateSign()
	if sign == a.phone.stateSign {
		return
	}
	a.phone.stateSign = sign
	if dir := a.snap().LibraryDir; dir != "" && !a.lockShut() {
		srv.PublishState(phoneLibraryID(dir))
	}
}

// phoneStateSign sums up what a phone is told of the open library's learning state that the desktop changes: the resume positions, the saved lines and their stamps, and the open film's corrections, so a change of any shows as a change of the sum. UI thread only.
func (a *app) phoneStateSign() uint64 {
	var sum uint64
	add := func(parts ...string) {
		h := fnv.New64a()
		for _, p := range parts {
			_, _ = h.Write([]byte(p))
			_, _ = h.Write([]byte{0})
		}
		// Each entry is summed apart, so the maps' order does not count.
		sum += h.Sum64()
	}
	for path, at := range a.player.positions {
		add("p", path, strconv.FormatInt(int64(at), 10))
	}
	book := a.savedBook()
	add("b", book.Dir(), strconv.Itoa(len(book.Lines)), strconv.Itoa(len(book.Removed)))
	for _, l := range book.Lines {
		s := l.Stamps
		add("l", l.ID, l.Video, phoneStateWhen(s.Original), phoneStateWhen(s.Translation), phoneStateWhen(s.Note), phoneStateWhen(s.Review), phoneStateWhen(s.Video))
	}
	if a.correctLoaded() {
		add("c", a.player.subs.path, strconv.Itoa(len(a.player.subs.edits)))
		for _, c := range a.player.subs.edits {
			add("e", strconv.FormatInt(int64(c.Span.Start), 10), strconv.FormatInt(int64(c.Span.End), 10), phoneStateText(c.Original), phoneStateText(c.Translation), strconv.FormatInt(int64(c.StartShift), 10), strconv.FormatInt(int64(c.EndShift), 10), strconv.FormatBool(c.NoSpeech))
		}
	}
	return sum
}

// phoneStateWhen is when and where s was stamped, for phoneStateSign.
func phoneStateWhen(s study.Stamp) string { return strconv.FormatInt(s.At.UnixNano(), 36) + s.Device }

// phoneStateText is s, or a mark of none for nil, for phoneStateSign.
func phoneStateText(s *string) string {
	if s == nil {
		return "\x00nil"
	}
	return *s
}
