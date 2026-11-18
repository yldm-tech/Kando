package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	psync "kando/packages/sync"
)

// What the desktop shows of what the paired phones do (yldm-tech/Kando#218): where a phone last left a film of the open library, which the library offers to continue here (phoneHandoff) and the hero names (phoneLeftOn); and on Settings › 手机同步, each phone online or not with its read-only switch, how much of the open library's learning state the phones were told, its sync conflicts, which the user settles here as a phone does, and what the phones did since Kando opened (phoneLog). All of it is read from the open library's ledger as the last call left it (state_phonesync_state.go), never from disk; a locked library shows none of it.

// phoneActivityKept is how many of the phones' latest actions the page lists.
const phoneActivityKept = 30

// handoffWithin is how long after a phone left a film the library offers to continue it here.
const handoffWithin = 24 * time.Hour

// phoneClockEvery is how often the relative times on Settings › 手机同步 and the handoff bar are told again while they show.
const phoneClockEvery = time.Minute

// phoneClockNext is the next time what Settings › 手机同步 or the handoff bar shows changes with time alone, after now, or zero for none: a phone seen lately going offline, and, while the page or the bar shows, their relative times moving on and the offer to continue running out. UI thread only.
func (a *app) phoneClockNext(now time.Time) time.Time {
	var next time.Time
	soonest := func(t time.Time) {
		if t.After(now) && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	if !a.phone.running {
		return next
	}
	for _, d := range a.phone.devices {
		if d.Online(now) {
			soonest(d.LastSeen.Add(phonesync.OnlineWithin))
		}
	}
	if (a.prefs.open && a.SettingsSection() == settings.SectionPhone) || a.phoneHandoff().Film.Key != "" {
		// On the minute rather than a minute from now, so the changes in between leave the timer as it is.
		soonest(now.Truncate(phoneClockEvery).Add(phoneClockEvery))
	}
	return next
}

// phoneClockArm sets the timer that runs the bindings again at phoneClockNext, so a phone that stopped calling shows offline and the relative times move on while nothing else changes; the run's afterChange arms it again. A timer already set for that time is left running. UI thread only.
func (a *app) phoneClockArm() {
	now := a.phoneStateNow()
	next := a.phoneClockNext(now)
	if a.phone.clockStop != nil && next.Equal(a.phone.clockAt) {
		return
	}
	if a.phone.clockStop != nil {
		a.phone.clockStop()
		a.phone.clockStop = nil
	}
	a.phone.clockAt = next
	if next.IsZero() || a.cx == nil {
		return
	}
	a.phone.clockStop = a.cx.AfterFunc(next.Sub(now), live.Do(a, func() { a.phone.clockStop, a.phone.clockAt = nil, time.Time{} }))
}

// phoneLog notes what a phone, or this computer for one (device ""), did now, for the page's list of what the phones did lately, the latest first. Each call makes a new list rather than shifting the old one in place, as the page compares the list it last drew with the one it reads (settings.activity), and an in-place shift would change the one it drew too. UI thread only.
func (a *app) phoneLog(device, kind, title string, count int, at time.Duration) {
	entry := settings.PhoneActivity{At: a.phoneStateNow(), Device: device, Kind: kind, Title: title, Count: count, Position: at}
	a.phone.activity = slices.Concat([]settings.PhoneActivity{entry}, a.phone.activity[:min(len(a.phone.activity), phoneActivityKept-1)])
}

// phoneLogPut notes what a phone's put changed: where it left each film, and how many lines, words and corrections of each film it changed. changed is the records as they are once merged, films the library's films by ID. UI thread only.
func (a *app) phoneLogPut(device string, changed []psync.Record, films map[string]phonesync.Film) {
	lines, words := 0, 0
	edits := map[string]int{}
	for _, r := range changed {
		switch r.Kind {
		case psync.KindPosition:
			if at := phonesync.PositionOf(r); at > 0 {
				a.phoneLog(device, settings.ActivityPosition, films[r.Film].Title, 0, at)
			}
		case psync.KindLine:
			lines++
		case psync.KindWord:
			words++
		case psync.KindCorrection:
			edits[r.Film]++
		}
	}
	if lines > 0 {
		a.phoneLog(device, settings.ActivityLines, "", lines, 0)
	}
	if words > 0 {
		a.phoneLog(device, settings.ActivityWords, "", words, 0)
	}
	for film, n := range edits {
		a.phoneLog(device, settings.ActivityEdits, films[film].Title, n, 0)
	}
}

// phoneOpenLedger is the open library's ledger as the last call left it, false while there is none: no library open, a locked one, or none read since Phone Sync started. UI thread only.
func (a *app) phoneOpenLedger() (*phoneLedger, bool) {
	dir := a.snap().LibraryDir
	if !a.phone.running || dir == "" || a.lockShut() || a.lockFilesAway(dir) {
		return nil, false
	}
	l, ok := a.phone.ledgers[filepath.Clean(dir)]
	if !ok || !l.listed {
		return nil, false
	}
	return l, true
}

// phoneDeviceName is the name of the device id names in the stamps: "" for this computer, the paired phone's name, or "" for one no longer paired, which the page cannot name. UI thread only.
func (a *app) phoneDeviceName(id string) (string, bool) {
	if id == "" || id == a.saved.device {
		return "", true
	}
	for _, d := range a.phone.devices {
		if d.ID == id {
			return d.Name, true
		}
	}
	return "", false
}

// phoneFilmOf is the open library's film whose video is f's, as the library lists it.
func (a *app) phoneFilmOf(f phonesync.Film) (films.Film, bool) {
	for _, item := range a.films() {
		if item.VideoPath != "" && filepath.Clean(item.VideoPath) == filepath.Clean(f.Video) {
			return item, true
		}
	}
	return films.Film{}, false
}

// phoneHandoff is where a paired phone last left a film of the open library within handoffWithin, which the library offers to continue here, unless the user closed the offer, the desktop played a film since, or the phone is no longer paired. UI thread only.
func (a *app) phoneHandoff() films.Handoff {
	l, ok := a.phoneOpenLedger()
	if !ok {
		return films.Handoff{}
	}
	left, ok := phonesync.Handoff(l.ledger, a.saved.device)
	if !ok || a.phone.handoffClosed == phoneHandoffKey(left) || a.phoneStateNow().Sub(left.Stamp.At) > handoffWithin {
		return films.Handoff{}
	}
	device, known := a.phoneDeviceName(left.Stamp.Device)
	if !known || device == "" {
		return films.Handoff{}
	}
	i := slices.IndexFunc(l.films, func(f phonesync.Film) bool { return f.ID == left.Film })
	if i < 0 {
		return films.Handoff{}
	}
	item, ok := a.phoneFilmOf(l.films[i])
	if !ok || a.player.positions[item.VideoPath] != left.At {
		return films.Handoff{}
	}
	return films.Handoff{Film: item, Device: device, At: left.At, When: left.Stamp.At}
}

// phoneHandoffKey names a place a phone left a film, so a closed offer stays closed until a phone leaves a film again.
func phoneHandoffKey(left phonesync.Left) string {
	return left.Film + "@" + left.Stamp.At.UTC().Format(time.RFC3339Nano)
}

// phoneHandoffClose closes the offer to continue where a phone left a film, until a phone leaves one again. UI thread only.
func (a *app) phoneHandoffClose() {
	l, ok := a.phoneOpenLedger()
	if !ok {
		return
	}
	if left, ok := phonesync.Handoff(l.ledger, a.saved.device); ok {
		a.phone.handoffClosed = phoneHandoffKey(left)
	}
}

// phoneLeftOn is the paired phone that last left the open library's video at path where it resumes, "" when it was this computer, or none says. UI thread only.
func (a *app) phoneLeftOn(path string) string {
	l, ok := a.phoneOpenLedger()
	if !ok || path == "" {
		return ""
	}
	i := slices.IndexFunc(l.films, func(f phonesync.Film) bool { return filepath.Clean(f.Video) == filepath.Clean(path) })
	if i < 0 {
		return ""
	}
	r, ok := l.ledger.Record(psync.KindPosition, l.films[i].ID)
	if !ok || phonesync.PositionOf(r) != a.player.positions[path] {
		return ""
	}
	name, _ := a.phoneDeviceName(phonesync.LeftOn(l.ledger, l.films[i].ID, a.saved.device))
	return name
}

// phoneSyncLinked fills in what the page shows of the open library's learning state: its counts, its conflicts and what the phones did. UI thread only.
func (a *app) phoneSyncLinked(st *settings.PhoneSync) {
	st.ReadOnlyErr, st.ResolveErr = a.phone.readOnlyErr, a.phone.resolveErr
	st.Activity = a.phone.activity
	l, ok := a.phoneOpenLedger()
	if !ok {
		return
	}
	st.Stats.Shown = true
	st.Stats.Words, st.Stats.Lines, st.Stats.Positions = phonesync.Counts(l.ledger)
	titles := map[string]string{}
	for _, f := range l.films {
		titles[f.ID] = f.Title
	}
	for _, c := range phonesync.OpenConflicts(l.ledger) {
		out := settings.PhoneConflict{ID: c.ID, Field: c.Field, Film: titles[c.Film], At: -1, Both: phonesync.Joinable(c.Kind, c.Field)}
		if r, ok := l.ledger.Record(c.Kind, c.Record); ok {
			switch c.Kind {
			case psync.KindLine:
				if start, ok := r.Fields[psync.FieldStartMS]; ok {
					out.At = phonesync.PositionOf(psync.Record{Fields: map[string]psync.Field{psync.FieldPositionMS: start}})
				}
				if c.Field != psync.FieldOriginal {
					out.Line = phonesync.Text(r.Fields[psync.FieldOriginal].Value)
				}
			case psync.KindWord:
				out.Word = phonesync.Text(r.Fields[psync.FieldWord].Value)
			}
		}
		if c.Kind == psync.KindCorrection {
			if _, start, _, ok := psync.ParseCorrectionID(c.Record); ok {
				out.At = time.Duration(start) * time.Millisecond
			}
		}
		version := func(f psync.Field) settings.PhoneVersion {
			name, _ := a.phoneDeviceName(f.Device)
			return settings.PhoneVersion{Device: name, At: f.At, Text: phonesync.Text(f.Value)}
		}
		out.Kept, out.Lost = version(c.Kept), version(c.Lost)
		st.Conflicts = append(st.Conflicts, out)
	}
}

// phoneSyncSetReadOnly makes paired phone id read-only (on), or lets it change things again, from its next call on, and notes it. UI thread only.
func (a *app) phoneSyncSetReadOnly(id string, on bool) {
	if a.phone.server == nil {
		return
	}
	a.phone.readOnlyErr = ""
	if err := a.phone.server.SetReadOnly(id, on); err != nil {
		a.phone.readOnlyErr = err.Error()
		return
	}
	name, _ := a.phoneDeviceName(id)
	kind := settings.ActivityReadWrite
	if on {
		kind = settings.ActivityReadOnly
	}
	a.phoneLog("", kind, name, 0, 0)
	a.phoneSyncPull()
}

// phoneSyncResolve settles conflict id of the open library with the version keep names, as a phone settles one (phoneState.ResolveConflict), as a change made on this computer now, which the phones are told of. The library's state is read and kept off the engine's goroutine as a phone's call does, so this returns at once and the page follows once it is done.
func (a *app) phoneSyncResolve(id string, keep settings.PhoneKeep) {
	l, ok := a.phoneOpenLedger()
	if !ok {
		return
	}
	i := slices.IndexFunc(l.ledger.Conflicts(), func(c psync.Conflict) bool { return c.ID == id })
	if i < 0 {
		return
	}
	c := l.ledger.Conflicts()[i]
	value := c.Kept.Value
	switch keep {
	case settings.KeepLost:
		value = c.Lost.Value
	case settings.KeepBoth:
		value = phonesync.Joined(c)
	}
	a.phone.resolveErr = ""
	req := psync.ConflictResolve{Library: phoneLibraryID(a.snap().LibraryDir), ID: id, Value: value}
	caller := psync.Caller{DeviceID: a.saved.device, Name: phoneSyncName()}
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_, err := phoneState{a}.ResolveConflict(ctx, caller, req)
		update(func() {
			var ce *psync.CallError
			// A conflict a phone settled meanwhile is not found, and nothing is left to do.
			if err != nil && !(errors.As(err, &ce) && ce.Code == psync.CodeNotFound) {
				a.phone.resolveErr = err.Error()
			}
		})
	}()
}
