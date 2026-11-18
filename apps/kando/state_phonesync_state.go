package main

import (
	"context"
	"errors"
	"hash/fnv"
	"log"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/study"
	psync "kando/packages/sync"
)

// Phone Sync's learning state (packages/sync, state.go): a paired phone reads and writes a library's resume positions, saved lines with their notes and review schedule, and the corrections of its films' lines, as records merged field by field. The desktop keeps them where it kept them before: the positions with the shelf (a.player.positions), the lines in the library's study record (a.saved.book for the open library), the corrections in each film's corrections file. Each library's ledger (phonesync.LedgerFile, in the library folder) is what phones were last told: at each call phoneState reads the library's files off the engine's goroutine, then on it takes the desktop's state into the ledger (Observe), which stamps what changed on the desktop, merges what a phone puts (Put) and keeps the result where the desktop keeps it. A put that cannot all be kept is not kept in the ledger either, so the phone puts it again. A locked library's state, or one whose files are being moved into or out of its encrypted image, is neither read nor written: to a phone it is not found. phoneSyncPublishState takes the open library's changes into its ledger as they are made, so they are stamped when they were made, and tells the phones waiting for events.

// phoneState is the libraries' learning state as protocol v2's calls read and write it (psync.StateService).
type phoneState struct{ a *app }

// phoneStateLib is a library's learning state as a call reads it, on the engine's goroutine: its ledger with the desktop's state taken in, its study record, its films by their IDs (list, in the order they are listed) and by their videos' keys in the study record, and their corrections.
type phoneStateLib struct {
	lib    phonesync.Library
	ledger *psync.Ledger
	book   saved.Book
	// open is set when book is the open library's (a.saved.book).
	open bool
	// lines is set when book's lines are kept with the IDs they have, so the ledger takes them in: not while lines taken from saved-lines.json could not be saved, as they get other IDs each time the book is read.
	lines bool
	list  []phonesync.Film
	films map[string]phonesync.Film
	keys  map[string]string
	// edits are the films' corrections by film ID; a film whose corrections file could not be read has none, and keeps what the ledger has of them.
	edits map[string][]core.Correction
	// failed is set when what a phone put could not all be kept where the desktop keeps it, so the ledger is not kept with it either (phoneStateCall).
	failed bool
}

// phoneStateDisk is what a call reads of a library's files off the engine's goroutine: its study record, used unless it is the open library's, and its films' corrections by film ID, those that could be read.
type phoneStateDisk struct {
	book    saved.Book
	bookErr error
	edits   map[string][]core.Correction
}

// phoneStateReadDisk reads the files of lib, whose films are films, that a call takes in (phoneStateDisk). It runs off the engine's goroutine.
func phoneStateReadDisk(lib phonesync.Library, films []phonesync.Film) phoneStateDisk {
	d := phoneStateDisk{edits: map[string][]core.Correction{}}
	d.book, d.bookErr = saved.Load(lib.Dir)
	for _, f := range films {
		if edits, err := media.LoadCorrections(media.CorrectionsPath(f.Video)); err == nil {
			d.edits[f.ID] = edits
		}
	}
	return d
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
func (s phoneState) PutState(ctx context.Context, caller psync.Caller, req psync.StatePut) (psync.StatePutResult, error) {
	a := s.a
	return phoneStateCall(ctx, a, req.Library, func(st *phoneStateLib) (psync.StatePutResult, error) {
		known := func(film string) bool { _, ok := st.films[film]; return ok }
		have := func(kind, id string) bool { _, ok := st.ledger.Record(kind, id); return ok }
		if why := phonesync.StateAccepts(req.Records, known, have); why != "" {
			return psync.StatePutResult{}, psync.Errorf(psync.CodeInvalid, "%s", why)
		}
		changed, conflicts := st.ledger.Put(req.Records, study.NewID)
		if err := a.phoneStateKeep(st, changed); err != nil {
			st.failed = true
			return psync.StatePutResult{}, err
		}
		a.phoneStateSettle(st)
		a.phoneLogPut(caller.Name, changed, st.films)
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
		if i := slices.IndexFunc(st.ledger.Conflicts(), func(c psync.Conflict) bool { return c.ID == req.ID }); i >= 0 {
			if c := st.ledger.Conflicts()[i]; !phonesync.FieldValid(c.Kind, c.Field, req.Value) {
				return psync.StatePutResult{}, psync.Errorf(psync.CodeInvalid, "the value kept is not of field %s's type", c.Field)
			}
		}
		rec, c, err := st.ledger.Resolve(req.ID, req.Value, psync.Stamp{At: a.phoneStateNow(), Device: caller.DeviceID})
		if errors.Is(err, psync.ErrNoConflict) {
			return psync.StatePutResult{}, psync.Errorf(psync.CodeNotFound, "no open conflict %q", req.ID)
		}
		if err != nil {
			return psync.StatePutResult{}, err
		}
		if err := a.phoneStateKeep(st, []psync.Record{rec}); err != nil {
			st.failed = true
			return psync.StatePutResult{}, err
		}
		a.phoneStateSettle(st)
		// This computer settles a conflict as a phone does, on the page (phoneSyncResolve).
		device := caller.Name
		if caller.DeviceID == a.saved.device {
			device = ""
		}
		a.phoneLog(device, settings.ActivityResolved, "", 1, 0)
		return psync.StatePutResult{Epoch: st.ledger.Epoch(), Rev: st.ledger.Rev(), Records: []psync.Record{rec}, Conflicts: []psync.Conflict{c}}, nil
	})
}

// phoneStateCall runs fn on the library lib's learning state, read afresh into its ledger (phoneStateRead), on the engine's goroutine, and keeps the ledger once fn returns, when it changed. A library the desktop does not serve, a locked one among them, is not found. The library's films are listed, and its files read (phoneStateReadDisk), off the engine's goroutine first, as Phone Sync lists them (phoneSource.Films). When fn could not keep what a phone put (failed), the ledger, which has it, is not kept but read again from its file at the next call, so what was not kept is not taken as removed on the desktop then, and the phone puts it again.
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
	disk := phoneStateReadDisk(libs[i], films)
	return remote.OnUI(ctx, a.update, func() (T, error) {
		st, err := a.phoneStateRead(libs[i], films, disk)
		if err != nil {
			return zero, err
		}
		out, err := fn(st)
		if st.failed {
			delete(a.phone.ledgers, filepath.Clean(st.lib.Dir))
			return out, err
		}
		if werr := a.phoneStateKeepLedger(st.lib.Dir); werr != nil && err == nil {
			return zero, werr
		}
		return out, err
	})
}

// phoneStateRead takes the learning state of lib, whose films are films and whose files disk holds as read just before, into its ledger, which it reads the first time: the study record (the open library's as the app holds it), the resume positions of its films and their corrections (the open film's as the player holds them). The ledger stamps what changed on the desktop since it last took it in. A library locked since it was listed, or whose files are being moved into or out of its encrypted image, is not found, so nothing is read from or written into its folder while they are; one whose study record or ledger cannot be read fails, rather than tell phones its lines are gone. A study record that took lines from saved-lines.json is saved first, so the lines keep their IDs; while it cannot be, its lines are left as the ledger has them. UI thread only.
func (a *app) phoneStateRead(lib phonesync.Library, films []phonesync.Film, disk phoneStateDisk) (*phoneStateLib, error) {
	if a.libraryLocked(lib.Dir) || a.lockFilesAway(lib.Dir) {
		return nil, psync.Errorf(psync.CodeNotFound, "no library %q", lib.ID)
	}
	st := &phoneStateLib{lib: lib, list: films, films: map[string]phonesync.Film{}, keys: map[string]string{}, edits: maps.Clone(disk.edits)}
	if dir := filepath.Clean(lib.Dir); filepath.Clean(a.saved.book.Dir()) == dir && filepath.Clean(a.snap().LibraryDir) == dir {
		st.book, st.open = a.saved.book, true
	} else {
		if disk.bookErr != nil {
			return nil, disk.bookErr
		}
		st.book = a.savedOwn(disk.book)
	}
	if err := st.book.Unread(); err != nil {
		return nil, err
	}
	st.lines = true
	if st.book.Migrated() {
		if err := st.book.Save(); err != nil {
			log.Printf("phonesync: keep the lines taken from %s: %v", study.LegacyFileName, err)
			st.lines = false
		} else if st.open {
			a.saved.book = st.book
		}
	}
	ledger, err := a.phoneStateLedger(lib.Dir)
	if err != nil {
		return nil, err
	}
	st.ledger = ledger
	for _, f := range films {
		st.films[f.ID] = f
		st.keys[st.book.Key(f.Video)] = f.ID
	}
	if id, ok := a.phoneStateOpenFilm(st.films); ok {
		st.edits[id] = a.player.subs.edits
	}
	a.phoneStateObserve(st)
	a.phone.ledgers[filepath.Clean(lib.Dir)].films, a.phone.ledgers[filepath.Clean(lib.Dir)].listed = films, true
	return st, nil
}

// phoneStateOpenFilm is the ID of the film among films whose corrections the player holds, read (correctLoaded) and readable, so they are taken as it holds them.
func (a *app) phoneStateOpenFilm(films map[string]phonesync.Film) (string, bool) {
	if !a.correctLoaded() || a.player.subs.editsErr {
		return "", false
	}
	open := media.CorrectionsPath(a.player.subs.path)
	for id, f := range films {
		if media.CorrectionsPath(f.Video) == open {
			return id, true
		}
	}
	return "", false
}

// phoneStateObserve takes st, the library's state as the desktop holds it, into its ledger, stamping what changed now on this computer: the resume positions of its films, its corrections and, unless their IDs are not kept yet (lines), its lines. What it does not hold, as a film's corrections that could not be read, stays as the ledger has it. UI thread only.
func (a *app) phoneStateObserve(st *phoneStateLib) {
	var current []psync.Record
	for _, f := range st.list {
		if at := a.player.positions[f.Video]; at > 0 {
			current = append(current, phonesync.PositionRecord(f.ID, at))
		}
		if edits, ok := st.edits[f.ID]; ok {
			current = append(current, phonesync.CorrectionRecords(f.ID, edits)...)
		}
	}
	if st.lines {
		current = append(current, phonesync.LineRecords(st.book, func(video string) string { return st.keys[video] })...)
	}
	covered := func(r psync.Record) bool {
		switch r.Kind {
		case psync.KindLine:
			return st.lines
		case psync.KindPosition:
			_, ok := st.films[r.Film]
			return ok
		case psync.KindCorrection:
			_, ok := st.edits[r.Film]
			return ok
		}
		return false
	}
	st.ledger.Observe(current, covered, psync.Stamp{At: a.phoneStateNow(), Device: a.saved.device})
}

// phoneLedger is a library's ledger as read, the revision at which it was last kept on disk, and the library's films as the last call listed them (listed), which phoneStateObserveOpen takes the open library's changes of in between.
type phoneLedger struct {
	ledger *psync.Ledger
	kept   int64
	films  []phonesync.Film
	listed bool
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
		// The lines are merged into a copy, so a book that cannot be saved is left as it was: the app's, for the open library, keeps nothing the study record on disk lacks.
		next := st.book
		next.Lines, next.Removed = slices.Clone(st.book.Lines), slices.Clone(st.book.Removed)
		if next.Merge(in, removed) {
			if err := next.Save(); err != nil {
				if st.open {
					a.flash(a.messages().SavedWriteFailed())
				}
				return err
			}
			// Saved, the lines taken from saved-lines.json keep their IDs too.
			st.book, st.lines = next, true
			if st.open {
				a.saved.book = next
			}
		}
	}
	for film, recs := range corrections {
		f, ok := st.films[film]
		if !ok {
			continue
		}
		have, read := st.edits[film]
		edits, err := a.phoneStateCorrect(f.Video, recs, have, read)
		if err != nil {
			return err
		}
		st.edits[film] = edits
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

// phoneStateCorrect lays records, corrections of the film whose video is at video, over its corrections, have as the call read them (read false for a file it could not read, which is read again), and returns them as kept. The open film's go through the player (correctWrite), which shows them, and stay out of its Undo, as they are not the user's own steps there; another film's are written to its file.
func (a *app) phoneStateCorrect(video string, records []psync.Record, have []core.Correction, read bool) ([]core.Correction, error) {
	path := media.CorrectionsPath(video)
	open := a.correctLoaded() && media.CorrectionsPath(a.player.subs.path) == path
	edits := have
	switch {
	case open:
		edits = a.player.subs.edits
	case !read:
		var err error
		if edits, err = media.LoadCorrections(path); err != nil {
			return nil, err
		}
	}
	for _, r := range records {
		if c, ok := phonesync.CorrectionOf(r); ok {
			edits = phonesync.SetCorrection(edits, c)
		}
	}
	if open {
		if !a.correctWrite(edits) {
			return nil, errors.New("the open film's corrections could not be written")
		}
		return edits, nil
	}
	if err := media.SaveCorrections(path, edits); err != nil {
		return nil, err
	}
	return edits, nil
}

// phoneStateSettle has the ledger take in what keeping the records changed on the desktop, as the study record making one line of two saved at the same start, from what the call holds now, without reading the files again, and tells the phones waiting for events that the library's state changed. UI thread only.
func (a *app) phoneStateSettle(st *phoneStateLib) {
	if st.open {
		st.book = a.saved.book
	}
	a.phoneStateObserve(st)
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

// phoneSyncPublishState takes the open library's learning state into its ledger when it changed on the desktop, its resume positions, its saved lines or the open film's corrections (phoneStateSign), so the change is stamped when it was made, and tells the phones waiting for events. It runs after every change of the app's state, and changes nothing the window reads.
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
	if dir := a.snap().LibraryDir; dir != "" && !a.lockShut() && !a.lockFilesAway(dir) {
		a.phoneStateObserveOpen(dir)
		srv.PublishState(phoneLibraryID(dir))
	}
}

// phoneStateObserveOpen takes the open library at dir's learning state, as the app holds it, into its ledger: the resume positions of the films the last call listed, its saved lines once read, and the open film's corrections. Nothing is read from disk here; the ledger is kept at the next call, or as Phone Sync stops. Until a call has read the library since Phone Sync started, one is made for it (phoneStatePrime). UI thread only.
func (a *app) phoneStateObserveOpen(dir string) {
	l, ok := a.phone.ledgers[filepath.Clean(dir)]
	if !ok || !l.listed {
		a.phoneStatePrime(dir)
		return
	}
	st := &phoneStateLib{ledger: l.ledger, list: l.films, films: map[string]phonesync.Film{}, keys: map[string]string{}, edits: map[string][]core.Correction{}}
	book := a.saved.book
	if filepath.Clean(book.Dir()) == filepath.Clean(dir) && book.Unread() == nil && !book.Migrated() {
		st.book, st.lines = book, true
	}
	for _, f := range l.films {
		st.films[f.ID] = f
		st.keys[st.book.Key(f.Video)] = f.ID
	}
	if id, ok := a.phoneStateOpenFilm(st.films); ok {
		st.edits[id] = a.player.subs.edits
	}
	a.phoneStateObserve(st)
}

// phoneStatePrime reads the library at dir's learning state into its ledger as a call does, off the engine's goroutine but for taking it in, once at a time, so its later changes are taken in as they are made (phoneStateObserveOpen). UI thread only.
func (a *app) phoneStatePrime(dir string) {
	dir = filepath.Clean(dir)
	if a.phone.priming[dir] {
		return
	}
	if a.phone.priming == nil {
		a.phone.priming = map[string]bool{}
	}
	a.phone.priming[dir] = true
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_, err := phoneStateCall(ctx, a, phoneLibraryID(dir), func(*phoneStateLib) (struct{}, error) { return struct{}{}, nil })
		var ce *psync.CallError
		if err != nil && !(errors.As(err, &ce) && ce.Code == psync.CodeNotFound) {
			log.Printf("phonesync: read the learning state of %s: %v", dir, err)
		}
		update(func() { delete(a.phone.priming, dir) })
	}()
}

// phoneStateKeepAll keeps every ledger read, but those of libraries locked or being moved into or out of their encrypted image, and forgets them, as Phone Sync stops: the open library's changes taken in since the last call are kept, and a start reads them again. UI thread only.
func (a *app) phoneStateKeepAll() {
	for dir := range a.phone.ledgers {
		if a.libraryLocked(dir) || a.lockFilesAway(dir) {
			continue
		}
		if err := a.phoneStateKeepLedger(dir); err != nil {
			log.Printf("phonesync: keep the learning state of %s: %v", dir, err)
		}
	}
	a.phone.ledgers, a.phone.stateSign = nil, 0
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
		add(append([]string{"l"}, phonesync.LineSign(l)...)...)
	}
	if a.correctLoaded() {
		add("c", a.player.subs.path, strconv.Itoa(len(a.player.subs.edits)))
		for _, c := range a.player.subs.edits {
			add(append([]string{"e"}, phonesync.CorrectionSign(c)...)...)
		}
	}
	return sum
}
