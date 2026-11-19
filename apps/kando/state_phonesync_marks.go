package main

import (
	"cmp"
	"context"
	"errors"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	psync "kando/packages/sync"
)

// phoneMarks answers a paired phone's ask for the word marks of a film's lines (psync.MarkService, OpFilmMarks): readings over the words of the originals, a line in a level's tone under the words of the word list chosen and a dashed one under those in the vocabulary, as the player draws them with 字幕外观's 逐词标注 switches, computed by the plugins (pluginsMarksFor), for any film Phone Sync lists, open or not; the dashed lines only on a film of the open library, whose vocabulary the app holds. The lines are those a phone reads (phoneSource.Cues), each line's marks with the hash of the original they were computed for, so a phone draws none over a line it has otherwise. With every switch off a film has no marks, which the phone drops.
type phoneMarks struct{ a *app }

// phoneMarksWait is how long a phone's ask on the local network waits for marks still being computed before it answers them pending, for the phone to ask again; through the storage relay it does not wait, as a call waiting there holds up those behind it.
const phoneMarksWait = 10 * time.Second

func (p phoneMarks) FilmMarks(ctx context.Context, caller psync.Caller, ref psync.FilmRef) (psync.FilmMarks, error) {
	a := p.a
	f, err := a.phoneFilmFind(ctx, ref.ID)
	if err != nil {
		return psync.FilmMarks{}, err
	}
	cues, err := phoneSource{a}.Cues(ctx, f.film)
	if errors.Is(err, phonesync.ErrNotFound) {
		// A film without lines has nothing to mark.
		return psync.FilmMarks{}, nil
	}
	if err != nil {
		return psync.FilmMarks{}, err
	}
	originals := make([]string, len(cues))
	for i, c := range cues {
		originals[i] = c.Original
	}
	type found struct {
		marks pluginsMarks
		wait  *pluginsMarkWait
		state pluginsMarkState
	}
	got, err := remote.OnUI(ctx, a.update, func() (found, error) {
		if a.libraryLocked(f.lib.Dir) {
			return found{}, psync.Errorf(psync.CodeNotFound, "no film %q", f.film.ID)
		}
		// The vocabulary the app holds is the open library's, so the words of a film of another library are not underlined.
		vocab := f.lib.Dir == a.snap().LibraryDir
		m, w, state := a.pluginsMarksFor(f.item.VideoPath, a.phoneMarksSpoken(f.item), vocab, originals)
		return found{marks: m, wait: w, state: state}, nil
	})
	switch {
	case err != nil || got.state == pluginsMarksOff:
		return psync.FilmMarks{}, err
	case got.state == pluginsMarksNotReady:
		// Marks computed before the plugins are read would lack the word list or the readings, and the phone would keep them in place of its own.
		return psync.FilmMarks{Pending: true}, nil
	}
	if w := got.wait; w != nil {
		wait := phoneMarksWait
		if caller.Relay {
			wait = 0
		}
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-w.done:
			// A run stopped by a newer ask of the film, or a change of how it is marked, has no marks to answer.
			if w.stopped {
				return psync.FilmMarks{Pending: true}, nil
			}
			got.marks = w.marks
		case <-t.C:
			return psync.FilmMarks{Pending: true}, nil
		case <-ctx.Done():
			return psync.FilmMarks{}, ctx.Err()
		}
	}
	return phoneMarksOf(got.marks), nil
}

// phoneMarksSpoken is the language spoken in item, as the player's key takes it when item is open (pluginsMarkKey): from the job its key names, "" for a film found in the library's folder, whose key is its video's path, so the phone's marks are the player's.
func (a *app) phoneMarksSpoken(item films.Film) string {
	job, _ := a.library.Find(item.Key)
	return cmp.Or(job.SpeechLanguage, job.SpokenLanguage)
}

// phoneMarksOf is m as the protocol carries it: the lines with marks, each with the hash of the original they were computed for.
func phoneMarksOf(m pluginsMarks) psync.FilmMarks {
	out := psync.FilmMarks{Lines: []psync.LineMarks{}}
	for i, line := range m.lines {
		if len(line) == 0 {
			continue
		}
		marks := make([]psync.Mark, len(line))
		for k, mk := range line {
			marks[k] = psync.Mark(mk)
		}
		out.Lines = append(out.Lines, psync.LineMarks{Line: i, Hash: psync.LineHash(m.originals[i]), Marks: marks})
	}
	return out
}
