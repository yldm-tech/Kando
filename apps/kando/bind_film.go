package main

import (
	"math"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
)

// The open film's reads and clicks (film.App). The upload row is in cloud_app.go.

func (a *app) StartFilm(item films.Film) { a.startFilm(item) }

func (a *app) RetryOpen(id string) {
	if job, ok := a.library.Find(id); ok {
		a.retry(job)
		if a.queue.job != nil {
			a.openKey = a.queue.job.ID
			a.loadSubtitles(a.queue.job.ID, a.queue.job.SubtitlePath)
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

func (a *app) PlayErr() string { return playerErrText(a.player.playErr) }

func (a *app) PlayHead() time.Duration {
	if a.player.play == nil {
		return a.player.opening
	}
	return a.player.play.playhead()
}

func (a *app) PlayLength() time.Duration {
	if a.player.play != nil && a.player.play.duration > 0 {
		return a.player.play.duration
	}
	return a.player.length
}

func (a *app) Subs() film.Subs {
	return film.Subs{Key: a.player.subs.key, Ready: a.player.subs.ready, Err: playerErrText(a.player.subs.err), Cues: a.player.subs.cues}
}

func (a *app) SubList() *ui.ListState { return &a.player.subs.list }

func (a *app) ClipList() *ui.ListState { return &a.player.subs.clips }

func (a *app) FilmSide() string { return a.player.filmSide }

func (a *app) SetFilmSide(id string) { a.player.filmSide = id }

// Seek plays from at, where the scrubber or a line of the side panel was clicked; a looped line moves with it (playerSeek).
func (a *app) Seek(at time.Duration) { a.playerSeek(at) }

func (a *app) Skip(d time.Duration) {
	if a.player.play == nil {
		return
	}
	a.skip(d)
	switch d {
	case -5 * time.Second:
		a.flash(locales.FilmSeconds("−5"))
	case 5 * time.Second:
		a.flash(locales.FilmSeconds("+5"))
	case -30 * time.Second:
		a.flash(locales.FilmSeconds("−30"))
	case 30 * time.Second:
		a.flash(locales.FilmSeconds("+30"))
	}
}

func (a *app) Cue() *film.Look { return &a.player.look }

// LookChanged saves the subtitle settings after the style view changed them.
func (a *app) LookChanged() { a.playerSaveSettings() }

func (a *app) SubMode() string { return a.player.subMode }

// SetSubMode shows mode's lines and keeps the subtitle settings. A mode that shows lines is the one V turns them back on to.
func (a *app) SetSubMode(mode string) {
	a.player.subMode = mode
	if mode != film.SubOff {
		a.player.lastMode = mode
	}
	a.playerSaveSettings()
}

func (a *app) SubOrder() string { return a.player.subOrder }

func (a *app) SetSubOrder(order string) { a.player.subOrder = order }

// Untranslated and YieldSubs are the player's settings, which the settings pages edit too. Both areas use these, so they are declared once, here, with the state they read.

func (a *app) Untranslated() string { return a.player.untranslated }

func (a *app) SetUntranslated(mode string) { a.player.untranslated = mode }

func (a *app) YieldSubs() bool { return a.player.yieldSubs }

func (a *app) SetYieldSubs(on bool) { a.player.yieldSubs = on }

func (a *app) Delay() time.Duration { return a.player.film.delay }

// SetDelay moves the subtitles by d against the picture and keeps it for the film's video, so they are where they were left when it opens again.
func (a *app) SetDelay(d time.Duration) {
	a.player.film.delay = d
	a.playerNoteDelay(d)
	a.flash(locales.PlayerOsdDelay(film.DelayText(d)))
}

func (a *app) Panel() bool { return a.player.panel }

func (a *app) TogglePanel() { a.player.panel = !a.player.panel }

func (a *app) StyleView() bool { return a.player.film.styleView }

// SetStyleView shows the subtitle style in the side panel, opening the panel for it, or goes back to its tabs.
func (a *app) SetStyleView(on bool) {
	a.player.film.styleView = on
	if on {
		a.player.panel = true
	}
}

func (a *app) ModeMenu() *bool { return &a.player.film.modeMenu }

func (a *app) Search() *string { return &a.player.film.search }

func (a *app) SearchFocus() *bool { return &a.player.film.searchFocus }

func (a *app) Follow() *int { return &a.player.film.follow }

func (a *app) Scrub() (*float64, *bool) { return &a.player.film.scrub, &a.player.film.scrubbing }

// OSD is the message on the picture and how much longer it shows. A message set since the last frame starts showing at now.
func (a *app) OSD(now time.Time) (string, time.Duration) {
	f := &a.player.film
	if f.osd == "" {
		return "", 0
	}
	if f.osdAt.IsZero() {
		f.osdAt = now
	}
	left := 1400*time.Millisecond - now.Sub(f.osdAt)
	if left <= 0 {
		f.osd = ""
		return "", 0
	}
	return f.osd, left
}

// Pointer notes when the pointer moved over the player, which keeps its controls showing.
func (a *app) Pointer(x, y float32, now time.Time) {
	f := &a.player.film
	if f.pointer == [2]float32{x, y} && !f.moved.IsZero() {
		return
	}
	f.pointer = [2]float32{x, y}
	f.moved = now
}

func (a *app) LastMove() time.Time { return a.player.film.moved }

func (a *app) Info() film.Info { return a.player.film.info }

func (a *app) LibraryPath() string { return a.library.Path() }

func (a *app) ClipPoster(start time.Duration) ui.ImageSource { return a.player.film.clipFrames[start] }

func (a *app) UploadNote() *bool { return &a.player.film.uploadNote }

// ToggleFullScreen enters or leaves full screen. Without a window, as in tests that set none, it does nothing.
func (a *app) ToggleFullScreen() {
	if a.player.screen != nil {
		a.player.screen.ToggleFullScreen()
	}
}

// filmFillsWindow reports whether the open film takes the whole window, without the sidebar and the page's padding and under the title bar. A film opened from 全部影片 does.
func (a *app) filmFillsWindow() bool { return a.page == pageFilms && a.openKey != "" }

// filmOwnsTitleBar reports whether the open film draws its own header bar under the title bar, keeping clear of the traffic lights and dragging the window, so the content has no drag band over it (contentBand). A film that fills the window does.
func (a *app) filmOwnsTitleBar() bool { return a.filmFillsWindow() }

// filmPadding is the padding around an open film that fills the window (filmFillsWindow): none, the picture runs to the window's edges.
func (a *app) filmPadding() (x, y float32) { return 0, 0 }

// closeFilm goes back from the open film to the page it was opened from, as the film's back button does, and stops looping its line.
func (a *app) closeFilm() {
	a.openKey = ""
	a.player.film.loop = playerLoop{}
	a.stopPlayback()
}

// escapeFilm closes what is open over the film, the style view, then leaves full screen, and else closes the film. The window does not leave full screen on Esc by itself, so without this Esc in full screen would close the film and leave the library filling the screen.
func (a *app) escapeFilm() {
	if a.player.film.styleView {
		a.player.film.styleView = false
		return
	}
	if a.player.screen != nil && a.player.screen.IsFullScreen() {
		a.player.screen.SetFullScreen(false)
		return
	}
	a.closeFilm()
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
	a.playerSeek(at)
}

// toggleSubtitles is V: it hides the subtitles, or shows them again in the mode they had.
func (a *app) toggleSubtitles() {
	if a.player.subMode == film.SubOff {
		a.SetSubMode(a.player.lastMode)
	} else {
		a.SetSubMode(film.SubOff)
	}
	a.flash(locales.FilmSubtitlesMode(film.ModeName(a.player.subMode)))
}

// cycleSubtitles is S: original, translation, bilingual, and round again.
func (a *app) cycleSubtitles() {
	next := film.SubOriginal
	switch a.player.subMode {
	case film.SubOriginal:
		next = film.SubTranslation
	case film.SubTranslation:
		next = film.SubBilingual
	}
	a.SetSubMode(next)
	a.flash(locales.FilmSubtitlesMode(film.ModeName(next)))
}

// nudgeDelay is Z and X: the subtitles a tenth of a second earlier or later.
func (a *app) nudgeDelay(d time.Duration) { a.SetDelay(a.player.film.delay + d) }

// scaleSubtitles is ⌘+ and ⌘−: both lines larger or smaller, the original by three quarters of the step, within the style view's ranges.
func (a *app) scaleSubtitles(step float32) {
	look := &a.player.look
	look.Translation.Size = min(max(look.Translation.Size+step, 16), 48)
	look.Original.Size = min(max(look.Original.Size+float32(math.Round(float64(step)*0.75)), 12), 40)
	a.flash(locales.PlayerOsdSubtitleSize(int(look.Translation.Size)))
	a.playerSaveSettings()
}

// searchSubtitles is ⌘F: the side panel's subtitle list, its search field focused.
func (a *app) searchSubtitles() {
	a.player.panel = true
	a.player.film.styleView = false
	a.player.filmSide = ""
	a.player.film.searchFocus = true
}

// playerSaveSettings keeps the subtitle settings, saying so on the picture when they cannot be saved.
func (a *app) playerSaveSettings() {
	if err := a.saveSettings(); err != nil {
		a.flash(locales.PlayerSaveSubtitlesFailed())
	}
}

// playerKeys are the player's shortcuts. They work while a film is open; those that move the picture wait for it to load.
func (a *app) playerKeys() []shortcut {
	playing := func() bool { return a.openKey != "" && a.player.play != nil }
	open := func() bool { return a.openKey != "" }
	return []shortcut{
		{Group: groupPlayback, Label: locales.PlayerKeyPlayPause(), Key: ui.KeySpace, When: playing, Run: func() { a.player.play.toggle() }},
		{Group: groupPlayback, Label: locales.FilmSkipBack(5), Key: ui.KeyLeft, When: playing, Run: func() { a.Skip(-5 * time.Second) }},
		{Group: groupPlayback, Label: locales.FilmSkipForward(5), Key: ui.KeyRight, When: playing, Run: func() { a.Skip(5 * time.Second) }},
		{Group: groupPlayback, Label: locales.FilmSkipBack(30), Mods: ui.Cmd, Key: ui.KeyLeft, When: playing, Run: func() { a.Skip(-30 * time.Second) }},
		{Group: groupPlayback, Label: locales.FilmSkipForward(30), Mods: ui.Cmd, Key: ui.KeyRight, When: playing, Run: func() { a.Skip(30 * time.Second) }},
		{Group: groupPlayback, Label: locales.PlayerKeyPrevLine(), Key: ui.KeyA, When: playing, Run: a.PrevLine},
		{Group: groupPlayback, Label: locales.PlayerKeyNextLine(), Key: ui.KeyD, When: playing, Run: a.NextLine},
		{Group: groupPlayback, Label: locales.PlayerKeyReplayLine(), Key: ui.KeyR, When: playing, Run: a.ReplayLine},
		{Group: groupPlayback, Label: locales.PlayerKeyLoopLine(), Key: ui.KeyL, When: playing, Checked: a.LoopLine, Run: a.ToggleLoopLine},
		{Group: groupPlayback, Label: locales.PlayerKeyVolumeUp(), Key: ui.KeyUp, When: open, Run: func() { a.playerNudgeVolume(1) }},
		{Group: groupPlayback, Label: locales.PlayerKeyVolumeDown(), Key: ui.KeyDown, When: open, Run: func() { a.playerNudgeVolume(-1) }},
		{Group: groupPlayback, Label: locales.PlayerKeyMute(), Key: ui.KeyM, When: open, Run: a.ToggleMute},
		{Group: groupPlayback, Label: locales.PlayerKeySlower(), Mods: ui.Shift, Key: ui.KeyComma, When: open, Run: func() { a.playerStepRate(-1) }},
		{Group: groupPlayback, Label: locales.PlayerKeyFaster(), Mods: ui.Shift, Key: ui.KeyPeriod, When: open, Run: func() { a.playerStepRate(1) }},
		{Group: groupSubtitles, Label: locales.PlayerKeyToggleSubtitles(), Key: ui.KeyV, When: open, Run: a.toggleSubtitles},
		{Group: groupSubtitles, Label: locales.PlayerKeyCycleSubtitles(), Key: ui.KeyS, When: open, Run: a.cycleSubtitles},
		{Group: groupSubtitles, Label: locales.PlayerKeyDelayEarlier(), Key: ui.KeyZ, When: open, Run: func() { a.nudgeDelay(-100 * time.Millisecond) }},
		{Group: groupSubtitles, Label: locales.PlayerKeyDelayLater(), Key: ui.KeyX, When: open, Run: func() { a.nudgeDelay(100 * time.Millisecond) }},
		{Group: groupSubtitles, Label: locales.PlayerKeyLarger(), Mods: ui.Cmd, Key: ui.KeyEqual, When: open, Run: func() { a.scaleSubtitles(2) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySmaller(), Mods: ui.Cmd, Key: ui.KeyMinus, When: open, Run: func() { a.scaleSubtitles(-2) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySearch(), Mods: ui.Cmd, Key: ui.KeyF, When: open, Run: a.searchSubtitles},
		{Group: groupView, Label: locales.PlayerKeyToggleSidebar(), Mods: ui.Cmd | ui.Shift, Key: ui.KeyT, When: open, Run: a.TogglePanel},
		{Group: groupView, Label: locales.FilmFullScreen(), Key: ui.KeyF, When: open, Run: a.ToggleFullScreen},
		{Group: groupView, Label: locales.FilmBackToLibrary(), Key: ui.KeyEscape, When: open, Run: a.escapeFilm},
	}
}
