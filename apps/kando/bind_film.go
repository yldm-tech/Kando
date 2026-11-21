package main

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The open film's reads and clicks (film.App). The upload row is in cloud_app.go. The page reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

// filmPage is the open film, item, as the library column shows it over the library page (films.View): the player, which fills the window or the content column.
func (a *app) filmPage(item func() films.Film) ns.Node {
	return film.View{App: a, Item: func() film.Item { return filmItem(item()) }}
}

// filmItem is the library's film as the player shows it.
func filmItem(f films.Film) film.Item {
	return film.Item{Key: f.Key, Title: f.Title, Name: f.Name, Source: f.Source, Detail: f.Detail, Stage: f.Stage,
		JobID: f.JobID, VideoPath: f.VideoPath, SubtitlePath: f.SubtitlePath, Language: f.Language, Spoken: f.Spoken, SpeechLanguage: f.SpeechLanguage, Untranslated: f.Untranslated, TranslationErr: f.TranslationErr, SameLanguage: f.SameLanguage,
		PlatformSubtitles: f.PlatformSubtitles, LocalLines: f.LocalLines, SubtitlesOnly: f.SubtitlesOnly, VideoComing: f.VideoComing}
}

// openFilm is the film the player shows: the open key's, on the shelf or among the jobs.
func (a *app) openFilm() (films.Film, bool) {
	if a.openKey == "" {
		return films.Film{}, false
	}
	if item, ok := films.Find(a.films(), a.openKey); ok {
		return item, true
	}
	return a.JobFilm(a.openKey)
}

// ProcessFilm is the player's button for the open film, a video found in the folder that no job has processed: 生成字幕 makes the subtitles of one without any at once, and 重新处理整部影片 asks first for one with subtitles found beside it, which it would replace (AskReprocess). Either goes into the queue, its picture playing on (startFilm).
func (a *app) ProcessFilm() {
	item, ok := a.openFilm()
	switch {
	case !ok:
	case item.SubtitlePath == "":
		a.Reprocess(item)
	default:
		a.AskReprocess(item)
	}
}

func (a *app) TranslateFilmAgain(language string) {
	if item, ok := a.openFilm(); ok {
		a.TranslateAgain(item, language)
	}
}

func (a *app) TranslateFilmMissing() {
	if item, ok := a.openFilm(); ok {
		a.TranslateMissing(item)
	}
}

// JobStatus is the line the open film's card says while its job waits or runs (films.QueueMeta).
func (a *app) JobStatus(cx *ns.Scope) string {
	item, ok := a.openFilm()
	if !ok || item.JobID == "" || item.Stage == "" || item.Stage.Done() {
		return ""
	}
	return films.QueueMeta(locales.For(cx), item)
}

// RedoFailure is why processing or translating the open film again failed, as the message bar says it, and the settings section that fixes it (jobQueue.redoEnded); both "" unless the open film is done and its last redo since it was queued failed rather than being stopped.
func (a *app) RedoFailure(cx *ns.Scope) (text, section string) {
	item, ok := a.openFilm()
	if !ok || item.JobID == "" || item.Stage != core.StageComplete {
		return "", ""
	}
	ended, ok := a.queue.redoEnded[item.JobID]
	if !ok || ended.Err == nil || errors.Is(ended.Err, context.Canceled) || errors.Is(ended.Err, core.ErrLibrarySwitched) {
		return "", ""
	}
	m := locales.For(cx)
	reason, section := jobfailure.Hint(m, statusText(m, ended), jobfailure.Of(ended))
	if ended.Redo == core.RedoAll {
		return m.FilmReprocessFailed(reason), section
	}
	return m.FilmRetranslateFailed(reason), section
}

// StageLabel names a job's stage as the library does.
func (a *app) StageLabel(cx *ns.Scope, stage core.Stage) string {
	return films.StageLabel(locales.For(cx), stage)
}

// FilmUpload is the upload of the open film, as the cloud area reports it (UploadView).
func (a *app) FilmUpload() film.Upload {
	item, ok := a.openFilm()
	if !ok {
		return film.Upload{}
	}
	return a.UploadView(item)
}

func (a *app) BeginFilmUpload() {
	if item, ok := a.openFilm(); ok {
		a.BeginUpload(item)
	}
}

// FillsWindow reports whether the open film takes the whole window (filmFillsWindow), its title bar under the traffic lights.
func (a *app) FillsWindow() bool { return a.filmFillsWindow() }

func (a *app) RetryOpen(id string) {
	if _, ok := a.library.Find(id); ok {
		a.Retry(id)
		if a.queue.job != nil {
			a.openKey = a.queue.job.ID
			a.loadSubtitles(a.queue.job.ID, a.queue.job.VideoPath, a.queue.job.SubtitlePath)
		}
	}
}

// SetFilmSpeechLanguage is the info tab's spoken language: its job, a done film's, keeps code as the language spoken in the open film (core.Job.SpeechLanguage), saved in library.json, so Process Again transcribes it in that language rather than the one the settings name for every video. code is one of the languages the settings offer (media.SpeechLanguages) or the one heard in the film; any other, a film without a job and one still being processed change nothing.
func (a *app) SetFilmSpeechLanguage(code string) {
	item, ok := a.openFilm()
	if !ok || item.JobID == "" {
		return
	}
	job, ok := a.library.Find(item.JobID)
	if !ok || job.Stage != core.StageComplete || job.SpeechLanguage == code || (!media.KnownSpeechLanguage(code) && code != job.SpokenLanguage) {
		return
	}
	job.SpeechLanguage = code
	a.library.Upsert(job)
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
}

// Play is the open video once the system's player has loaded it, nil before and without one.
func (a *app) Play() film.Player {
	if !a.playerLoaded() {
		return nil
	}
	return a.player.play
}

func (a *app) PlaySound() bool { return a.playerLoaded() && a.player.play.sound }

// playerLoaded reports whether a video is open and the system's player has loaded it, so it plays, holds and seeks.
func (a *app) playerLoaded() bool { return a.player.play != nil && a.player.play.loaded }

func (a *app) PlayPath() string { return a.player.playPath }

func (a *app) PlayErr() string { return a.player.playErr }

func (a *app) ErrText(cx *ns.Scope, id string) string { return playerErrTextIn(locales.For(cx), id) }

func (a *app) OpeningText(cx *ns.Scope) string { return a.playerRemuxText(locales.For(cx)) }

// PosterFile is the file of the poster the library keeps for the video at path, which the player shows while the picture opens; the image is read off the engine's goroutine when it is shown, and a video without one shows the black stage.
func (a *app) PosterFile(path string) string { return posterDest(a.lib.posterDir, path) }

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

func (a *app) Video() film.Video { return a.playerVideo() }

func (a *app) VideoRef() *ns.Ref { return &a.player.ref }

func (a *app) VideoLoaded(info ns.VideoInfo) { a.playerLoad(info) }

func (a *app) VideoProgress(position, duration time.Duration) bool {
	return a.playerProgress(position, duration)
}

func (a *app) VideoEnded() { a.playerEnded() }

func (a *app) VideoFailed(err error) { a.playerFailed(err) }

func (a *app) Subs() film.Subs {
	subs := film.Subs{Key: a.player.subs.key, Ready: a.player.subs.ready, Err: a.player.subs.err, Cues: a.player.subs.cues}
	if a.player.subs.showDrafts {
		subs.Drafts = a.player.subs.drafts
	}
	subs.Gaps, subs.Making = a.player.subs.gaps, a.player.subs.making
	return subs
}

// playerDraftsFollow tells whether the open film's lines are marked drafts (subtitleView.showDrafts) and reports whether that changed. Lines are marked drafts while the film is made, until the final proofread makes them final. Without the final proofread none becomes final, so none is marked; and a film done with lines the final proofread did not get to, as when the service went, is left as one done, rather than marked for good with nothing to make it final. A film made again (core.Job.Redo) shows the lines it had, done, until the redo is.
func (a *app) playerDraftsFollow() bool {
	show := false
	if a.prefs.review && a.openKey != "" && a.player.subs.key != "" {
		item, ok := a.openFilm()
		show = ok && item.Key == a.player.subs.key && item.Stage != "" && !item.Stage.Done() && !a.filmRedoing(item.JobID)
	}
	if show == a.player.subs.showDrafts {
		return false
	}
	a.player.subs.showDrafts = show
	return true
}

// filmRedoing reports whether the job of the film open, id, is making a done film again, which shows the lines it had until it is done.
func (a *app) filmRedoing(id string) bool {
	job, ok := a.library.Find(id)
	return ok && job.Redo != ""
}

func (a *app) FilmSide() string { return a.player.filmSide }

func (a *app) SetFilmSide(id string) { a.player.filmSide = id }

// Seek plays from at, where the scrubber or a line of the side panel was clicked; a looped line moves with it (playerSeek).
func (a *app) Seek(at time.Duration) { a.playerSeek(at) }

func (a *app) Jumps() int { return a.player.film.jumps }

func (a *app) Skip(d time.Duration) {
	if !a.playerLoaded() {
		return
	}
	a.skip(d)
	m := a.messages()
	switch d {
	case -5 * time.Second:
		a.flash(m.FilmSeconds("−5"))
	case 5 * time.Second:
		a.flash(m.FilmSeconds("+5"))
	case -30 * time.Second:
		a.flash(m.FilmSeconds("−30"))
	case 30 * time.Second:
		a.flash(m.FilmSeconds("+30"))
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
	a.flash(a.messages().PlayerOsdDelay(film.DelayTextIn(a.messages(), d)))
}

// SubLift is where the open film's subtitles were moved to over its picture, 0 while they are in their place (film.App.SubLift).
func (a *app) SubLift() float32 { return a.player.lifts[a.player.film.video] }

// SetSubLift moves the open film's subtitles to lift and keeps it for the film's video, so they are where they were left when it opens again; other films keep theirs. Moving them is what the tip says, so the tip goes.
func (a *app) SetSubLift(lift float32) {
	a.player.film.subTip = false
	a.playerNoteLift(lift)
}

// SetSubRoom keeps where the lines on the picture show and may go as the player laid them out, for Move Subtitles Up and Down (playerNudgeLift). Nothing on screen reads it, so it changes nothing there.
func (a *app) SetSubRoom(room film.SubRoom) { a.player.subRoom = room }

func (a *app) SubTip() bool { return a.player.film.subTip }

// playerNudgeLift is ⌥↑ and ⌥↓: the open film's subtitles a step up (dir 1) or down (dir -1) over the picture, from where they show and within the room a drag has (film.NudgeLift).
func (a *app) playerNudgeLift(dir int) {
	a.SetSubLift(film.NudgeLift(a.SubLift(), dir, a.player.subRoom))
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

func (a *app) SearchFocused() *bool { return &a.player.film.searchFocused }

// OSD is the message on the picture, which flash takes away after a moment.
func (a *app) OSD() string { return a.player.film.osd }

// Flash shows text on the picture for a moment (flash).
func (a *app) Flash(text string) { a.flash(text) }

func (a *app) Info() film.Info { return a.player.film.info }

func (a *app) LibraryPath() string { return a.library.Path() }

func (a *app) ClipPoster(start time.Duration) ([]byte, bool) {
	data, ok := a.player.film.clipFrames[start]
	return data, ok
}

// SetFilmChapterUse cuts the open film, whose site marks chapters, at them or by time, and keeps that beside its video with its chapters (media.SetChapterUse), so it opens the same way again; its 片段 follow at once. A folder that cannot be written to keeps the choice until the film is closed.
func (a *app) SetFilmChapterUse(use core.ChapterUse) {
	info := &a.player.film.info
	path := a.player.film.infoFor
	if path == "" || len(info.Chapters) < 2 || !use.Known() || info.ChapterUse == use {
		return
	}
	_ = media.SetChapterUse(path, use)
	info.ChapterUse = use
	a.loadClipFrames()
}

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
func (a *app) filmPadding() (x, y ns.Points) { return 0, 0 }

// closeFilm goes back from the open film to the page it was opened from, as the film's back button does, and stops looping its line.
func (a *app) closeFilm() {
	a.openKey = ""
	a.player.film.loop = playerLoop{}
	a.stopPlayback()
}

// escapeFilm closes what is open over the film, the transport's menus, then the style view, then leaves full screen, and else closes the film. The window does not leave full screen on Esc by itself, so without this Esc in full screen would close the film and leave the library filling the screen.
func (a *app) escapeFilm() {
	if f := &a.player.film; f.modeMenu || f.speedMenu {
		f.modeMenu, f.speedMenu = false, false
		return
	}
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
	a.flash(a.messages().FilmSubtitlesMode(film.ModeNameIn(a.messages(), a.player.subMode)))
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
	a.flash(a.messages().FilmSubtitlesMode(film.ModeNameIn(a.messages(), next)))
}

// nudgeDelay is Z and X: the subtitles a tenth of a second earlier or later.
func (a *app) nudgeDelay(d time.Duration) { a.SetDelay(a.player.film.delay + d) }

// scaleSubtitles is ⌘+ and ⌘−: both lines larger or smaller, the original by three quarters of the step, within the style view's ranges.
func (a *app) scaleSubtitles(step float32) {
	look := &a.player.look
	look.Translation.Size = min(max(look.Translation.Size+step, 16), 48)
	look.Original.Size = min(max(look.Original.Size+float32(math.Round(float64(step)*0.75)), 12), 40)
	a.flash(a.messages().PlayerOsdSubtitleSize(int(look.Translation.Size)))
	a.playerSaveSettings()
}

// searchSubtitles is ⌘F: the side panel's subtitle list, its search field focused.
func (a *app) searchSubtitles() {
	a.player.panel = true
	a.player.film.styleView = false
	a.player.filmSide = ""
	a.player.film.searchFocus = true
}

// copyCurrentLine is ⇧⌘C: the subtitle line at the playhead on the clipboard, as the side panel's Copy Line puts it (film.LineText), saying so on the picture. Between two lines, or before the film's subtitles are read, there is nothing to copy and it does nothing. It goes by PlayHead, so while the picture is still opening it copies the line where the film opens, and it copies with the subtitles turned off (V) as well: hiding the lines from the picture is no reason to keep them off the clipboard. ⌘C stays Edit › Copy's, for the text fields. Without the app's scope, as in tests that start no app, there is no clipboard to copy to.
func (a *app) copyCurrentLine() {
	subs := &a.player.subs
	if subs.key != a.openKey || !subs.ready || a.cx == nil {
		return
	}
	cue, ok := film.CueAt(subs.cues, a.playerLineAt())
	if !ok {
		return
	}
	a.cx.SetClipboard(film.LineText(cue.Text))
	a.flash(a.messages().PlayerOsdCopied())
}

// playerSaveSettings keeps the subtitle settings, saying so on the picture when they cannot be saved.
func (a *app) playerSaveSettings() {
	if err := a.saveSettings(); err != nil {
		a.flash(a.messages().PlayerSaveSubtitlesFailed())
	}
}

// playerKeys are the player's shortcuts. They work while a film is open; those that move the picture wait for the system's player to have loaded it. The rows that step a value, seeking, the volume, the speed, the subtitle delay and place and the line before or after, go on while their key is held.
func (a *app) playerKeys(cx *ns.Scope) []shortcut {
	playing := func() bool { return a.openKey != "" && a.playerLoaded() }
	open := func() bool { return a.openKey != "" }
	// Space and the arrows are left to the speed or mode menu while one is open, for its rows, wherever the keyboard focus is, and to the other translations of a line while they show (correctKeys).
	menus := func() bool { return a.player.film.modeMenu || a.player.film.speedMenu || a.correctPicking() }
	transport := func() bool { return playing() && !menus() }
	volume := func() bool { return open() && !menus() }
	// L also works while the picture opens, so turning the loop off is not missed then.
	opening := func() bool { return a.openKey != "" && a.player.play != nil }
	rows := []shortcut{
		{Group: groupPlayback, Label: locales.PlayerKeyPlayPause(cx), Key: "space", When: transport, Run: func() { a.player.play.toggle() }},
		{Group: groupPlayback, Label: locales.FilmSkipBack(cx, 5), Key: "left", Repeat: true, When: transport, Run: func() { a.Skip(-5 * time.Second) }},
		{Group: groupPlayback, Label: locales.FilmSkipForward(cx, 5), Key: "right", Repeat: true, When: transport, Run: func() { a.Skip(5 * time.Second) }},
		{Group: groupPlayback, Label: locales.FilmSkipBack(cx, 30), Key: "mod+left", Repeat: true, When: playing, Run: func() { a.Skip(-30 * time.Second) }},
		{Group: groupPlayback, Label: locales.FilmSkipForward(cx, 30), Key: "mod+right", Repeat: true, When: playing, Run: func() { a.Skip(30 * time.Second) }},
		{Group: groupPlayback, Label: locales.PlayerKeyPrevLine(cx), Key: "a", Repeat: true, When: playing, Run: a.PrevLine},
		{Group: groupPlayback, Label: locales.PlayerKeyNextLine(cx), Key: "d", Repeat: true, When: playing, Run: a.NextLine},
		{Group: groupPlayback, Label: locales.PlayerKeyReplayLine(cx), Key: "r", When: playing, Run: a.ReplayLine},
		{Group: groupPlayback, Label: locales.PlayerKeyLoopLine(cx), Key: "l", When: opening, Checked: a.LoopLine, Run: a.ToggleLoopLine},
		{Group: groupPlayback, Label: locales.PlayerKeyExtendLoop(cx), Key: "shift+l", When: opening, Run: a.ExtendLoop},
		{Group: groupPlayback, Label: locales.PlayerKeyPauseLines(cx), Key: "p", When: open, Checked: a.PauseLines, Run: a.TogglePauseLines},
		{Group: groupPlayback, Label: locales.PlayerKeyLoopGap(cx), When: open, Checked: a.playerLoopGap, Run: a.playerToggleLoopGap},
		{Group: groupPlayback, Label: locales.PlayerKeyNextFilm(cx), Key: "shift+n", When: func() bool { return open() && a.HasNext() }, Run: a.PlayNext},
		{Group: groupPlayback, Label: locales.PlayerKeyPreviousFilm(cx), Key: "shift+p", When: open, Run: a.PlayPrevious},
		{Group: groupPlayback, Label: locales.PlayerKeyAutoNext(cx), When: open, Checked: a.AutoNext, Run: a.ToggleAutoNext},
		{Group: groupPlayback, Label: locales.PlayerKeyVolumeUp(cx), Key: "up", Repeat: true, When: volume, Run: func() { a.playerNudgeVolume(1) }},
		{Group: groupPlayback, Label: locales.PlayerKeyVolumeDown(cx), Key: "down", Repeat: true, When: volume, Run: func() { a.playerNudgeVolume(-1) }},
		{Group: groupPlayback, Label: locales.PlayerKeyMute(cx), Key: "m", When: open, Run: a.ToggleMute},
		{Group: groupSubtitles, Label: locales.PlayerKeyToggleSubtitles(cx), Key: "v", When: open, Run: a.toggleSubtitles},
		{Group: groupSubtitles, Label: locales.PlayerKeyCycleSubtitles(cx), Key: "s", When: open, Run: a.cycleSubtitles},
		{Group: groupSubtitles, Label: locales.PlayerKeyPeekLine(cx), Key: "t", When: playing, Run: a.PeekLine},
		{Group: groupSubtitles, Label: locales.PlayerKeyDelayEarlier(cx), Key: "z", Repeat: true, When: open, Run: func() { a.nudgeDelay(-100 * time.Millisecond) }},
		{Group: groupSubtitles, Label: locales.PlayerKeyDelayLater(cx), Key: "x", Repeat: true, When: open, Run: func() { a.nudgeDelay(100 * time.Millisecond) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySubtitlesUp(cx), Key: "alt+up", Repeat: true, When: open, Run: func() { a.playerNudgeLift(1) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySubtitlesDown(cx), Key: "alt+down", Repeat: true, When: open, Run: func() { a.playerNudgeLift(-1) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySubtitlesReset(cx), Key: "alt+0", When: open, Run: func() { a.SetSubLift(0) }},
		{Group: groupSubtitles, Label: locales.PlayerKeyLarger(cx), Key: "mod+=", Repeat: true, When: open, Run: func() { a.scaleSubtitles(2) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySmaller(cx), Key: "mod+-", Repeat: true, When: open, Run: func() { a.scaleSubtitles(-2) }},
		{Group: groupSubtitles, Label: locales.PlayerKeySearch(cx), Key: "mod+f", When: open, Run: a.searchSubtitles},
		{Group: groupSubtitles, Label: locales.PlayerKeyCopyLine(cx), Key: "mod+shift+c", InText: true, When: open, Run: a.copyCurrentLine},
		{Group: groupView, Label: locales.PlayerKeyToggleSidebar(cx), Key: "mod+shift+t", When: open, Run: a.TogglePanel},
		{Group: groupView, Label: locales.FilmFullScreen(cx), Key: "f", When: open, Run: a.ToggleFullScreen},
		// While a line is selected for correcting, its other translations show or a bar asks about spreading a fix, Escape puts those away first (correctKeys).
		{Group: groupView, Label: locales.FilmBackToLibrary(cx), Key: "escape", When: func() bool {
			return open() && !a.correctSelecting() && !a.correctPicking() && !a.correctSpreading()
		}, Run: a.escapeFilm},
	}
	// The speed keys are only where the system's player plays at the speed picked, not on Linux (film.RateWorks), so neither the cheat sheet, Settings › 快捷键 nor the menu bar offers keys that do nothing there.
	if !film.RateWorks(cx) {
		return rows
	}
	mute := slices.IndexFunc(rows, func(s shortcut) bool { return s.Key == "m" })
	return slices.Insert(rows, mute+1,
		shortcut{Group: groupPlayback, Label: locales.PlayerKeySlower(cx), Key: playerSlowerKey, Repeat: true, When: open, Run: func() { a.playerStepRate(-1) }},
		shortcut{Group: groupPlayback, Label: locales.PlayerKeyFaster(cx), Key: playerFasterKey, Repeat: true, When: open, Run: func() { a.playerStepRate(1) }},
	)
}
