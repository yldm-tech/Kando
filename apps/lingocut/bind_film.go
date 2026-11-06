package main

import (
	"time"

	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"

	"github.com/egoist/mygo/ui"
)

// The open film's reads and clicks (film.App). The upload row is in cloud_app.go.

func (a *app) StartFilm(item films.Film) { a.startFilm(item) }

func (a *app) RetryOpen(id string) {
	if job, ok := a.library.Find(id); ok {
		a.retry(job)
		if a.job != nil {
			a.openKey = a.job.ID
			a.loadSubtitles(a.job.ID, a.job.SubtitlePath)
		}
	}
}

func (a *app) Play() film.Player {
	if a.play == nil {
		return nil
	}
	return a.play
}

func (a *app) PlayPath() string { return a.playPath }

func (a *app) PlayErr() string { return a.playErr }

func (a *app) Subs() film.Subs {
	return film.Subs{Key: a.subs.key, Ready: a.subs.ready, Err: a.subs.err, Cues: a.subs.cues}
}

func (a *app) SubList() *ui.ListState { return &a.subs.list }

func (a *app) ClipList() *ui.ListState { return &a.subs.clips }

func (a *app) FilmSide() string { return a.filmSide }

func (a *app) SetFilmSide(id string) { a.filmSide = id }

func (a *app) Seek(at time.Duration) { a.seekPlayback(at) }

func (a *app) Cue() *film.Look { return &a.cue }
