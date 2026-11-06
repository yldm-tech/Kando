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
	if a.player.play == nil {
		return nil
	}
	return a.player.play
}

func (a *app) PlayPath() string { return a.player.playPath }

func (a *app) PlayErr() string { return a.player.playErr }

func (a *app) Subs() film.Subs {
	return film.Subs{Key: a.player.subs.key, Ready: a.player.subs.ready, Err: a.player.subs.err, Cues: a.player.subs.cues}
}

func (a *app) SubList() *ui.ListState { return &a.player.subs.list }

func (a *app) ClipList() *ui.ListState { return &a.player.subs.clips }

func (a *app) FilmSide() string { return a.player.filmSide }

func (a *app) SetFilmSide(id string) { a.player.filmSide = id }

func (a *app) Seek(at time.Duration) { a.seekPlayback(at) }

func (a *app) Cue() *film.Look { return &a.player.look }

// ToggleFullScreen enters or leaves full screen. Without a window, as in tests, it does nothing.
func (a *app) ToggleFullScreen() {
	if a.win != nil {
		a.win.ToggleFullScreen()
	}
}

// filmFillsWindow reports whether the open film takes the whole window, without the sidebar and with less padding around it. A film opened from 全部影片 does.
func (a *app) filmFillsWindow() bool { return a.page == pageFilms && a.openKey != "" }

// closeFilm goes back from the open film to the page it was opened from, as the film's 返回 button does.
func (a *app) closeFilm() {
	a.openKey = ""
	a.stopPlayback()
}

// skip moves the picture by d from where it is, within the video.
func (a *app) skip(d time.Duration) {
	play := a.player.play
	at := play.playhead() + d
	if at < 0 {
		at = 0
	}
	if play.duration > 0 && at > play.duration {
		at = play.duration
	}
	a.seekPlayback(at)
}

// playerKeys are the player's shortcuts. They work while a film is open and its picture is loaded.
func (a *app) playerKeys() []shortcut {
	playing := func() bool { return a.openKey != "" && a.player.play != nil }
	open := func() bool { return a.openKey != "" }
	return []shortcut{
		{Group: "播放", Label: "播放/暂停", Key: ui.KeySpace, When: playing, Run: func() { a.player.play.toggle() }},
		{Group: "播放", Label: "后退 5 秒", Key: ui.KeyLeft, When: playing, Run: func() { a.skip(-5 * time.Second) }},
		{Group: "播放", Label: "前进 5 秒", Key: ui.KeyRight, When: playing, Run: func() { a.skip(5 * time.Second) }},
		{Group: "显示", Label: "全屏", Key: ui.KeyF, When: open, Run: a.ToggleFullScreen},
		{Group: "显示", Label: "返回资料库", Key: ui.KeyEscape, When: open, Run: a.closeFilm},
	}
}
