package main

import (
	"maps"
	"slices"
	"sync/atomic"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/annotate"
)

// The marks of the films the paired phones play (docs/plugin-system.md, E2, and the phones' OpFilmMarks, state_phonesync_marks.go): a phone runs no plugin, so the desktop computes the marks of a film's lines for it, as its own player draws them, with the switches of 逐词标注 and the annotators of the script plugins running, for a film that is not open as for the one that is. They are computed on demand, off the engine's goroutine, once for each key (pluginsMarkKey, with subs the hash of the lines' originals alone, annotate.LinesHash, as a phone's lines come without the player's times), and kept for the films last asked for, one set per film marked a way; the open film's marks answer when they are of the same lines marked the same way. A line's marks depend on its original alone, so the originals marked before for the same film marked the same way, the open film's or a phone's, are not marked again (pluginsMarksHad): a film whose subtitles are made while it plays (media.Make) has more lines at each ask, and only those are marked; the phone draws each line's marks only over the original they were computed for (psync.LineHash).

// pluginsPhoneMarks are the marks computed for the phones' films: kept, those of the films last asked for, the latest last, at most pluginsPhoneKept, one per film marked a way; and running, the computation running for each film, by its video, at most one: a newer ask of the film for another key (more lines, or marked another way) stops it (pluginsMarkWait.stop), so a run that ends late never replaces the newer marks.
type pluginsPhoneMarks struct {
	kept    []pluginsMarks
	running map[string]*pluginsMarkWait
}

// pluginsMarkWait is the marks of a film being computed for the phones, for key: done closes once marks is set, on the engine's goroutine, or once the run is stopped (stop), with stopped set and no marks, as a newer ask of the film, a change of how the lines are marked or the app quitting stops it.
type pluginsMarkWait struct {
	key     pluginsMarkKey
	stop    atomic.Bool
	done    chan struct{}
	marks   pluginsMarks
	stopped bool
}

// pluginsPhoneKept is how many films' marks the desktop keeps for the phones: a phone asks for a film's as it plays it and as it downloads it, so a few films' cover a phone going back and forth between them.
const pluginsPhoneKept = 8

// pluginsMarkState is what pluginsMarksFor found: no mark turned on, what the marks are computed from not read yet, or marks (kept, or being computed).
type pluginsMarkState int

const (
	pluginsMarksOff pluginsMarkState = iota
	pluginsMarksNotReady
	pluginsMarksFound
)

// pluginsMarksFor are the marks of the lines of video whose originals are originals, heard in spoken, on the engine's goroutine, the words of the vocabulary underlined only with vocab, for a film of the open library (pluginsMarkKeyFor): those kept, or the open film's when it is of the same lines marked the same way, else wait, whose done closes once they are computed, which starts them unless they are being computed already, stopping a run of the film for another key. Before the plugins folder is read and the dictionaries turned on are open (pluginsMarksReady) it computes nothing and answers pluginsMarksNotReady, as marks computed then would lack the word list or the readings, and a phone would keep them in place of its own.
func (a *app) pluginsMarksFor(video, spoken string, vocab bool, originals []string) (pluginsMarks, *pluginsMarkWait, pluginsMarkState) {
	if !a.pluginsMarksReady() {
		return pluginsMarks{}, nil, pluginsMarksNotReady
	}
	key, ok := a.pluginsMarkKeyFor(video, annotate.LinesHash(originals), spoken, vocab)
	if !ok {
		return pluginsMarks{}, nil, pluginsMarksOff
	}
	p := &a.plugins
	if open := p.marks; open.lines != nil && open.key.sameMarking(key) && slices.Equal(open.originals, originals) {
		return open, nil, pluginsMarksFound
	}
	ph := &p.phone
	if i := slices.IndexFunc(ph.kept, func(m pluginsMarks) bool { return m.key == key }); i >= 0 {
		m := ph.kept[i]
		ph.kept = append(slices.Delete(ph.kept, i, i+1), m)
		return m, nil, pluginsMarksFound
	}
	if w := ph.running[video]; w != nil {
		if w.key == key {
			return pluginsMarks{}, w, pluginsMarksFound
		}
		a.pluginsPhoneMarkStop(w)
	}
	w := &pluginsMarkWait{key: key, done: make(chan struct{})}
	if ph.running == nil {
		ph.running = map[string]*pluginsMarkWait{}
	}
	ph.running[video] = w
	job := a.pluginsMarkJobFor(key)
	job.had = a.pluginsMarksHad(key)
	job.src.Stop = w.stop.Load
	lines := slices.Clone(originals)
	update := a.update
	go func() {
		marks, byOriginal, list := job.run(lines)
		update(func() {
			a.pluginsKeepList(list, job.cached)
			if w.stopped || w.stop.Load() {
				return
			}
			// The script plugins' annotators mark the phone's film too, as they mark the open one (pluginsAnnotate).
			a.pluginsAnnotate(key, func() bool { return w.stopped || w.stop.Load() }, lines, job.had, marks, byOriginal, func() {
				if w.stopped || w.stop.Load() {
					return
				}
				m := pluginsMarks{key: key, lines: marks, originals: lines, byOriginal: byOriginal}
				delete(ph.running, video)
				// The film's marks before, of fewer lines as while its subtitles are made, give way to these, which hold theirs.
				ph.kept = slices.DeleteFunc(ph.kept, func(o pluginsMarks) bool { return o.key.sameMarking(key) })
				ph.kept = append(ph.kept, m)
				if n := len(ph.kept) - pluginsPhoneKept; n > 0 {
					ph.kept = slices.Delete(ph.kept, 0, n)
				}
				w.marks = m
				close(w.done)
			})
		})
	}()
	return pluginsMarks{}, w, pluginsMarksFound
}

// pluginsMarksReady reports whether what the marks are computed from is read, on the engine's goroutine: the script plugins turned on verified and loaded; with a mark turned on, the plugins folder (scanned) and, for the readings, each dictionary turned on opened or failed to open. Right after the app starts it is not, and marks computed then would lack the word list or the readings.
func (a *app) pluginsMarksReady() bool {
	p := &a.plugins
	// The script plugins turned on are verified and loaded first, as their annotators may mark the lines too.
	if p.scanned && slices.ContainsFunc(a.pluginsScriptPacks(), p.state.EnabledPack) && (p.script.verified == nil || len(p.script.loading) > 0) {
		return false
	}
	if !p.prefs.readings && p.prefs.list == "" && !p.prefs.known {
		return true
	}
	if !p.scanned {
		return false
	}
	if p.prefs.readings {
		for _, pack := range a.pluginsOn(plugins.KindDictionary) {
			if open := p.dicts[pack.ID]; open == nil || open.hash != pack.Hash || (open.d == nil && open.err == nil) {
				return false
			}
		}
	}
	return true
}

// pluginsPhoneMarkStop stops w at its next line and lets those waiting for it go, without marks (stopped).
func (a *app) pluginsPhoneMarkStop(w *pluginsMarkWait) {
	w.stop.Store(true)
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.done)
	ph := &a.plugins.phone
	for video, r := range ph.running {
		if r == w {
			delete(ph.running, video)
		}
	}
}

// pluginsPhoneMarksStop stops every run of marks for the phones, as when how the lines are marked changes, which they were computed for, or the app quits.
func (a *app) pluginsPhoneMarksStop() {
	for _, w := range a.plugins.phone.running {
		a.pluginsPhoneMarkStop(w)
	}
}

// pluginsMarksHad are the marks by original computed before for the film of key marked the same way (pluginsMarkKey.sameMarking), the open film's or a phone's, whatever lines it had then: a film whose subtitles are being made while it plays has more lines at each ask, and only the new ones are marked.
func (a *app) pluginsMarksHad(key pluginsMarkKey) map[string][]subtitle.Mark {
	p := &a.plugins
	had := map[string][]subtitle.Mark{}
	if p.marks.key.sameMarking(key) {
		maps.Copy(had, p.marks.byOriginal)
	}
	for _, m := range p.phone.kept {
		if m.key.sameMarking(key) {
			maps.Copy(had, m.byOriginal)
		}
	}
	return had
}
