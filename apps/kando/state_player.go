package main

import (
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/nowplaying"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
)

// playerUI is the state of the open film: the picture, the subtitle file and how subtitles look. The settings in it are saved with the app's settings (savedSettings).
type playerUI struct {
	// next is the card that plays the next film once the open one ends, and nextOff is set while 自动播放下一部 is off (player_next.go).
	next    playerNext
	nextOff bool
	// play is the video open in the system's player (playback.go), from when startPlayback opens it until it stops; it plays only once the player has loaded it (filmPlay.loaded). playPath is its video, the film's own, "" without one.
	play     *filmPlay
	playPath string
	// playErr is playerErrUnplayable when the video would not play, playerErrPlugins when GStreamer lacks the plug-ins for it, one of film's stream errors for a film that keeps no video and does not stream (playerStream), or "".
	playErr string
	// streams reports whether a film that keeps no video, from source, streams on this computer; nil is media.Streams. Tests set it, as which ones stream depends on the system.
	streams func(source string) bool
	// playGen counts the videos opened and the seeks made, so a report meant for one before is dropped.
	playGen  int
	playFrom time.Duration
	// playJump has the video open playing at playFrom (继续, 从头看, a jump to a line) rather than held on its first frame.
	playJump bool
	// playSame is set when the playback opens the open film's video again, as a jump to a line of the open film does, so the film keeps its state; any other playback is a film opening, which starts it afresh.
	playSame bool
	// opening is where the video opens at, which the transport shows until the system's player has loaded it, and where a film without a picture stands.
	opening time.Duration
	// ref is the system player's ref (film.View's core.Video), through which a seek reaches it.
	ref core.Ref
	// remuxDir is where the copies of videos the system's player cannot open go (playerRemux), "" for the user's cache; tests set their own.
	remuxDir string
	// remuxStop stops the copy being made for the open video, nil while none is.
	remuxStop func()
	// length is the open video's length once known, kept while a seek opens it again.
	length   time.Duration
	subs     subtitleView
	filmSide string
	look     film.Look
	// subMode is one of film.SubBilingual, film.SubTranslation, film.SubOriginal or film.SubOff. It is the one owner of which lines show; film.Look does not hold it.
	subMode string
	// subOrder is film.SubTranslationFirst or film.SubOriginalFirst. It is the one owner of which line goes on top.
	subOrder string
	// untranslated is film.UntranslatedDim or film.UntranslatedBlank.
	untranslated string
	// yieldSubs moves the subtitles up while the player's controls show.
	yieldSubs bool
	// positions is where each video was when its playback stopped, by video path, so it can resume there.
	positions map[string]time.Duration
	// played is when each video last played, by video path, kept as its resume position is, so the library can offer the film watched last first.
	played map[string]time.Time
	// delays is how far the subtitles of each video were moved against its picture, by video path, so a film opens with them where they were left. A video whose subtitles are in place has none.
	delays map[string]time.Duration
	// lifts is where the subtitles of each video were moved to over its picture (film.App.SubLift), by video path, so a film opens with them off its own burned-in text where they were left. A video whose subtitles are in their place has none.
	lifts map[string]float32
	// subRoom is where the lines on the picture last showed and how far a drag could take them, as the player laid them out (film.App.SetSubRoom), which Move Subtitles Up and Down keep to. It stays from film to film, the zero room until lines first showed.
	subRoom film.SubRoom
	// subTipSeen is kept in the settings once the tip that the subtitles can be dragged has shown (playerSubTip), so it shows only once; tipTimer takes the tip away, nil until it first shows with the app running.
	subTipSeen bool
	tipTimer   *film.Timer
	// saves is when the player last had the areas keep its resume positions and subtitle delays and places on disk (playerKeep).
	saves playerSaves
	// osdTimer takes the message flash put on the picture away, one timer under the app's scope that each message sets again, nil until the first message with the app running (flash).
	osdTimer *film.Timer
	// loopTimer takes a playing film back to the start of the looped line when it reaches its end between two reports of the system's player (playerLoopSchedule), nil until a loop is first timed.
	loopTimer *film.Timer
	// lastMode is the mode V turns the subtitles back on to.
	lastMode string
	// panel shows the side panel over the picture. It stays as it is from film to film.
	panel bool
	// referenceHidden hides the reference layer of the films that have one (film.Referencer), as the subtitle menu chose; it stays as it is from film to film while the app runs.
	referenceHidden bool
	// volume is how loud films play, from 0 to 1, and muted silences them without losing it. Both stay as they are from film to film.
	volume float64
	muted  bool
	// volumeGen counts volume changes, so only the last of a quick run of them is written to the settings (playerVolumeChanged); volumeUnsaved is set until the change is kept in the settings.
	volumeGen     int
	volumeUnsaved bool
	// screen is the window the player enters and leaves full screen in: the main window once it exists (playerReady), and nil before, as in tests that set none, when full screen does nothing.
	screen fullScreen
	// windowHidden is set while the main window is put away by its close button (state_window.go), and windowAway while none of the app shows, the window minimized, hidden or covered (playerAwakeReady): either lets the display sleep while only the film's sound is heard.
	windowHidden bool
	windowAway   bool
	// keepAwake is desktop.KeepAwake once the window is ready (playerReady), and nil before and in tests, which must not hold the machine awake; wake gives back the hold it took while the open film plays, nil while there is none (player_awake.go).
	keepAwake func(reason string, display bool) (release func())
	wake      func()
	// nowPlaying is what the system shows of the open film, where the media keys and its controls command it (playback_nowplaying.go).
	nowPlaying playerNowPlaying
	// rate is how fast films play, one of film.Rates, 1 as recorded. It stays as it is from film to film.
	rate float64
	// rateGen counts speed changes, so only the last of a quick run of them reaches the settings (playerRateChanged); rateSettling is set until it has.
	rateGen      int
	rateSettling bool
	// pauseLines pauses the film at the end of each subtitle line (P), and loopGap is what a looped line waits after each pass, "" for nothing or playerLoopGapLine for as long as it played. Both stay as they are from film to film.
	pauseLines bool
	loopGap    string
	film       filmUI
}

// fullScreen is the part of a window the player uses to enter and leave full screen (windowScreen, player_awake.go).
type fullScreen interface {
	IsFullScreen() bool
	SetFullScreen(v bool)
	ToggleFullScreen()
}

// playerNowPlaying is what the system shows of the open film.
type playerNowPlaying struct {
	// center is nil until the window is ready (playerHooks' ready), and in tests that connect none, so nothing is shown before then.
	center nowplaying.Center
	// path is the video the system shows, "" while it shows none; title is its name there.
	path  string
	title string
}

// filmUI is the player's state that belongs to the one film open, which opening another starts afresh (resetFilm).
type filmUI struct {
	styleView bool
	modeMenu  bool
	speedMenu bool
	// video is the path of the film's video, which delay is kept by in playerUI.delays.
	video string
	delay time.Duration
	// subTip shows the tip that the subtitles can be dragged over them (playerSubTip).
	subTip      bool
	search      string
	searchFocus bool
	// searchFocused is set while the subtitle search field holds the keyboard focus (SearchFocused).
	searchFocused bool
	uploadNote    bool
	// info is the film's folder as readFilmInfo read it for the video at infoFor, and reference the captions its site made itself, which its job kept beside it (media.ReadReference).
	info      film.Info
	infoFor   string
	reference []kcore.Segment
	// clipFrames are the JPEG frames of the 片段 tab by the start of each (film_clips.go), and clipBusy those being read.
	clipFrames map[time.Duration][]byte
	clipBusy   map[time.Duration]bool
	// osd is the message on the picture, which flash takes away after osdFor.
	osd string
	// loop is the subtitle line playing over and over (ToggleLoopLine). It belongs to this film only and is not saved.
	loop playerLoop
	// jumps counts the user's moves of the picture (playerSeek), after which the subtitle list follows the playing line again (film.App.Jumps).
	jumps int
	// stop is where the player stopped at a line's end on this pass through it, so it does not stop there again as it plays on (playerStopped). lineEnd is the end of the line, in the subtitles' time, Pause After Each Line pauses the film at next, 0 for none (playerTimerWait).
	stop    playerStop
	lineEnd time.Duration
	// peek is 1 more than the index of the line the picture shows both lines of, whatever the subtitle mode (PeekLine), and 0 for none, until the user moves the picture (playerSeek).
	peek int
}

// playerLoop is the line the player loops, in the subtitles' time, so the film's subtitle delay (filmUI.delay) moves it with its line: playback goes back to start once it reaches end. An end of 0 is the end of the video, for the last line when it has no end of its own.
type playerLoop struct {
	on bool
	// key is the film the loop was made for (app.openKey); a loop of another film is no loop (playerLooping).
	key        string
	start, end time.Duration
}

// playerStop is the player stopping at the end of a line: Pause After Each Line pausing there, or a looped line holding for its gap before it plays again (Pause Between Repeats).
type playerStop struct {
	on bool
	// seeks is the playback's count of seeks (filmPlay.seeks) at the stop: a seek, the loop's return included, starts another pass through the line.
	seeks int
	// end is the end of the line it stopped at, in the subtitles' time.
	end time.Duration
	// resume is when a loop's gap ends and its line plays again, zero for a pause the user ends with Space.
	resume time.Time
}

func newPlayerUI() playerUI {
	return playerUI{
		look:         film.DefaultLook(),
		subMode:      film.SubBilingual,
		subOrder:     film.SubTranslationFirst,
		untranslated: film.UntranslatedDim,
		yieldSubs:    true,
		lastMode:     film.SubBilingual,
		panel:        true,
		volume:       1,
		rate:         1,
	}
}

// resetFilm starts the state of one film afresh, as another film opens.
func (a *app) resetFilm() {
	a.player.film = filmUI{}
	a.player.length = 0
}

// osdFor is how long a message shows on the picture (prototype script 1972).
const osdFor = 1400 * time.Millisecond

// flash shows text on the picture for osdFor, from the last message on when one takes over another. The timer runs under the app's scope, so the message goes even when what said it, as a button of a panel since closed, has gone; without the app's scope, as in tests that start no app, it stays.
func (a *app) flash(text string) {
	a.player.film.osd = text
	if a.cx == nil {
		return
	}
	if a.player.osdTimer == nil {
		a.player.osdTimer = film.NewTimer(a.cx)
	}
	a.player.osdTimer.Set(osdFor, live.Do(a, func() { a.player.film.osd = "" }))
}

// rememberPosition keeps where the open video stopped, so ResumeAt can offer to continue from there, and that it played now (PlayedAt). A video watched to the end starts over.
func (a *app) rememberPosition(play *filmPlay) {
	if play == nil || play.path == "" {
		return
	}
	at := play.playhead()
	if a.player.positions == nil {
		a.player.positions = map[string]time.Duration{}
	}
	if a.player.played == nil {
		a.player.played = map[string]time.Time{}
	}
	a.player.played[play.path] = a.playerNow()
	if play.finished || at <= 0 {
		delete(a.player.positions, play.path)
	} else {
		a.player.positions[play.path] = at
	}
	a.playerPositionsChanged()
}

// playerForget forgets where the video at path stopped, so it opens at its start and the library offers no 继续 for it, and with played when it last played too, as for a film never played. The caller has the change saved (playerPositionsChanged).
func (a *app) playerForget(path string, played bool) {
	delete(a.player.positions, path)
	if played {
		delete(a.player.played, path)
	}
}

// resumePositions and setResumePositions are the player's resume positions by video path, for the area that keeps them on disk with the shelf. The map is the player's own; setResumePositions takes positions over as they are.
func (a *app) resumePositions() map[string]time.Duration { return a.player.positions }

func (a *app) setResumePositions(positions map[string]time.Duration) { a.player.positions = positions }

// ResumeAt is where the video at path stopped last time, or 0 when it was not watched or was watched to the end.
func (a *app) ResumeAt(path string) time.Duration { return a.player.positions[path] }

// playedTimes and setPlayedTimes are when each video last played, by video path, for the area that keeps them on disk with the shelf beside the resume positions. The map is the player's own; setPlayedTimes takes played over as it is.
func (a *app) playedTimes() map[string]time.Time { return a.player.played }

func (a *app) setPlayedTimes(played map[string]time.Time) { a.player.played = played }

// PlayedAt is when the video at path last played, the zero time when it never did.
func (a *app) PlayedAt(path string) time.Time { return a.player.played[path] }

// subtitleDelays and setSubtitleDelays are how far the subtitles of each video were moved, by video path, for the area that keeps them on disk with the shelf beside the resume positions. The map is the player's own; setSubtitleDelays takes delays over as they are.
func (a *app) subtitleDelays() map[string]time.Duration { return a.player.delays }

func (a *app) setSubtitleDelays(delays map[string]time.Duration) { a.player.delays = delays }

// subtitleLifts and setSubtitleLifts are where the subtitles of each video were moved to over its picture, by video path, for the area that keeps them on disk with the shelf beside the subtitle delays. The map is the player's own; setSubtitleLifts takes lifts over as they are.
func (a *app) subtitleLifts() map[string]float32 { return a.player.lifts }

func (a *app) setSubtitleLifts(lifts map[string]float32) { a.player.lifts = lifts }

// How often a playing film is kept on disk (playerKeep): at most every playerKeepEvery, once it has moved more than playerKeepMoved since, and a subtitle delay or place once it has rested playerDelaySettle, so a run of Z or X presses, or of ⌥↑ and ⌥↓, is saved once.
const (
	playerKeepEvery   = 20 * time.Second
	playerKeepMoved   = 2 * time.Second
	playerDelaySettle = time.Second
)

// playerSaves is what playerKeep needs to save the open film now and then rather than at every report of where it is.
type playerSaves struct {
	// play is the playback the position was last looked at for, and seek the seek it was at then; at is when its position was last kept and head where it was then; playing is whether it played when last looked at.
	play    *filmPlay
	seek    int
	at      time.Time
	head    time.Duration
	playing bool
	// delayDirty is set while a subtitle delay or place changed since the last save; delayAt is when it was first looked at since its last change, zero until then; delayGen counts the changes, so only the timer of the last of a run of them saves it.
	delayDirty bool
	delayAt    time.Time
	delayGen   int
}

// playerAfterChange keeps where the open film is on disk when it paused, ended or was moved (playerKeep), keeps the display awake while a film plays (player_awake.go), times the return of a looped line to its start (playerLoopSchedule), shows the card that plays the next film once the film ends (playerNextFollow) and shows the tip that the subtitles can be dragged the first time a film plays with them (playerSubTip). The positions it saves are what the library's 继续 reads, and the tip shows over the picture, so it reports when it saved one or showed the tip.
func (a *app) playerAfterChange() bool {
	changed := a.playerKeep(a.playerNow())
	a.playerKeepAwake()
	a.playerLoopSchedule()
	tip := a.playerSubTip()
	stream := a.playerStreamFollow()
	coming := a.playerComingFollow()
	drafts := a.playerDraftsFollow()
	gaps := a.playerGapsFollow()
	next := a.playerNextFollow()
	redo := a.subtitlesFollowRedo()
	return changed || tip || stream || coming || drafts || gaps || next || redo
}

// playerTipFor is how long the tip that the subtitles can be dragged shows.
const playerTipFor = 6 * time.Second

// playerSubTip shows the tip that the subtitles can be dragged and double-clicked back (film.App.SubTip) the first time a film plays with subtitles that show on the picture, for playerTipFor, and keeps in the settings that it has, so it never shows again. Without the app's scope, as in tests that start no app, it stays. It reports whether it showed the tip.
func (a *app) playerSubTip() bool {
	p, subs := a.player.play, &a.player.subs
	if a.player.subTipSeen || a.openKey == "" || p == nil || !p.Playing() || subs.key != a.openKey || !subs.ready || len(subs.cues) == 0 || a.player.subMode == film.SubOff {
		return false
	}
	a.player.subTipSeen = true
	a.player.film.subTip = true
	a.playerSaveSettings()
	if a.cx != nil {
		if a.player.tipTimer == nil {
			a.player.tipTimer = film.NewTimer(a.cx)
		}
		a.player.tipTimer.Set(playerTipFor, live.Do(a, func() { a.player.film.subTip = false }))
	}
	return true
}

// playerKeep saves where the open film is while it plays, so a crash, a force quit or a power cut loses little of it rather than all since it opened: at once when a playback opened playing at a point picked for it (继续, 从头看) or a seek the user made lands, and when it pauses or ends, and in between every playerKeepEvery once it has moved on. A film held on the frame it opened on is not kept until it plays, as stopPlayback does not keep it either, and a looped line going back to its start does not count as a seek, nor its hold in the gap before it does as a pause. A subtitle delay is saved once it has rested playerDelaySettle. now is the time it looks. It reports whether it saved anything.
func (a *app) playerKeep(now time.Time) bool {
	s := &a.player.saves
	saved := false
	if s.delayDirty {
		if s.delayAt.IsZero() {
			s.delayAt = now
		} else if now.Sub(s.delayAt) >= playerDelaySettle {
			a.playerPositionsChanged()
			saved = true
		}
	}
	play := a.player.play
	if play == nil || play.path == "" || !play.loaded {
		s.play, s.playing = nil, false
		return saved
	}
	playing, head := play.Playing(), play.playhead()
	keep := func() {
		a.rememberPosition(play)
		s.at, s.head = now, head
		saved = true
	}
	switch {
	case s.play != play:
		s.play, s.seek, s.at, s.head = play, play.seeks, now, head
		if play.played {
			keep()
		}
	case !play.played:
	case s.seek != play.seeks:
		s.seek = play.seeks
		if play.keepSeek {
			keep()
		}
	case s.playing && !playing && !a.playerInGap(play):
		keep()
	case now.Sub(s.at) >= playerKeepEvery && (head-s.head).Abs() > playerKeepMoved:
		keep()
	}
	s.playing = playing
	return saved
}

// playerNoteDelay keeps d as how far the subtitles of the open film's video are moved, to be saved once it rests (playerKeep), or as the film closes (playerSaveDelay). Moving them back in place forgets it.
func (a *app) playerNoteDelay(d time.Duration) {
	video := a.player.film.video
	if video == "" {
		return
	}
	if was, ok := a.player.delays[video]; ok && was == d || !ok && d == 0 {
		return
	}
	if d == 0 {
		delete(a.player.delays, video)
	} else {
		if a.player.delays == nil {
			a.player.delays = map[string]time.Duration{}
		}
		a.player.delays[video] = d
	}
	a.playerSaveSettled()
}

// playerNoteLift keeps lift as where the subtitles of the open film's video are moved to (film.App.SubLift), to be saved once it rests (playerKeep), or as the film closes (playerSaveDelay), as a delay is. Putting them back in their place forgets it.
func (a *app) playerNoteLift(lift float32) {
	video := a.player.film.video
	if video == "" {
		return
	}
	if was, ok := a.player.lifts[video]; ok && was == lift || !ok && lift == 0 {
		return
	}
	if lift == 0 {
		delete(a.player.lifts, video)
	} else {
		if a.player.lifts == nil {
			a.player.lifts = map[string]float32{}
		}
		a.player.lifts[video] = lift
	}
	a.playerSaveSettled()
}

// playerSaveSettled has a subtitle delay or place just changed saved once it rests playerDelaySettle, so a run of changes is saved once.
func (a *app) playerSaveSettled() {
	a.player.saves.delayDirty = true
	a.player.saves.delayAt = time.Time{}
	// A delay that rests is saved without waiting for the film to report where it is, as a held film reports nothing.
	a.player.saves.delayGen++
	gen := a.player.saves.delayGen
	if update := a.update; update != nil {
		time.AfterFunc(playerDelaySettle, func() {
			update(func() {
				if s := &a.player.saves; s.delayGen == gen && s.delayDirty {
					a.playerPositionsChanged()
				}
			})
		})
	}
}

// playerSaveDelay saves a subtitle delay or place that changed since the last save, as its film closes or another opens.
func (a *app) playerSaveDelay() {
	if a.player.saves.delayDirty {
		a.playerPositionsChanged()
	}
}

// playerPositionsChanged has the areas that keep the player's resume positions and subtitle delays and places on disk save them, which saves a delay or place waiting to be saved too.
func (a *app) playerPositionsChanged() {
	a.player.saves.delayDirty = false
	a.player.saves.delayAt = time.Time{}
	a.areasPositionsChanged()
}

// restorePlayer takes the subtitle settings saved last time. A value this version does not know keeps the default; loadSavedSettings reads a saved look over film.DefaultLook, so a field it lacks keeps the default too.
func (a *app) restorePlayer(saved savedSettings) {
	if saved.Look != nil {
		a.player.look = *saved.Look
	}
	switch saved.SubMode {
	case film.SubBilingual, film.SubTranslation, film.SubOriginal, film.SubOff:
		a.player.subMode = saved.SubMode
		if saved.SubMode != film.SubOff {
			a.player.lastMode = saved.SubMode
		}
	}
	switch saved.SubOrder {
	case film.SubTranslationFirst, film.SubOriginalFirst:
		a.player.subOrder = saved.SubOrder
	}
	switch saved.Untranslated {
	case film.UntranslatedDim, film.UntranslatedBlank:
		a.player.untranslated = saved.Untranslated
	}
	if saved.YieldSubs != nil {
		a.player.yieldSubs = *saved.YieldSubs
	}
	if saved.Volume != nil {
		a.player.volume = min(max(*saved.Volume, 0), 1)
	}
	a.player.muted = saved.Muted
	if saved.Rate != nil {
		a.player.rate = playerNearestRate(*saved.Rate)
	}
	a.player.pauseLines = saved.PauseLines
	a.player.loopGap = saved.LoopGap
	a.player.subTipSeen = saved.SubtitleTipSeen
	a.player.nextOff = saved.PlayNext != nil && !*saved.PlayNext
}

// savedPlayer is the player's part of settings.json: how subtitles look, which lines show, how loud and how fast films play, whether they stop after each line, whether the next film plays by itself, and whether the tip that the subtitles can be dragged has shown. Where each film's subtitles were moved to is the film's, kept in library.json (shelf.Library.Lifts); a look saved by a version that kept one place for every film (its Lift) reads without it, so every film's subtitles start at the look's place.
type savedPlayer struct {
	Look         *film.Look `json:"look,omitempty"`
	SubMode      string     `json:"subMode,omitempty"`
	SubOrder     string     `json:"subOrder,omitempty"`
	Untranslated string     `json:"untranslated,omitempty"`
	YieldSubs    *bool      `json:"yieldSubs,omitempty"`
	// Volume is how loud films play, from 0 to 1; Muted silences them.
	Volume *float64 `json:"volume,omitempty"`
	Muted  bool     `json:"muted,omitempty"`
	// Rate is how fast films play, one of film.Rates.
	Rate *float64 `json:"rate,omitempty"`
	// PauseLines pauses films at the end of each line; LoopGap is what a looped line waits after each pass, "" or playerLoopGapLine.
	PauseLines bool   `json:"pauseAfterLine,omitempty"`
	LoopGap    string `json:"loopGap,omitempty"`
	// SubtitleTipSeen is set once the tip that the subtitles can be dragged has shown.
	SubtitleTipSeen bool `json:"subtitleTipSeen,omitempty"`
	// PlayNext is whether the next film plays by itself once one ends (自动播放下一部); nil plays it.
	PlayNext *bool `json:"playNext,omitempty"`
}

// clean brings a saved speed to the nearest of film.Rates and drops a loop gap this version does not know, and leaves the player's other settings as they were read; restorePlayer replaces a value it does not know with the default.
func (s *savedPlayer) clean() {
	if s.Rate != nil {
		rate := playerNearestRate(*s.Rate)
		s.Rate = &rate
	}
	if s.LoopGap != playerLoopGapLine {
		s.LoopGap = ""
	}
}

func (a *app) savePlayer(saved *savedSettings) {
	yieldSubs := a.player.yieldSubs
	volume := a.player.volume
	rate := a.player.rate
	playNext := !a.player.nextOff
	saved.savedPlayer = savedPlayer{
		Look:            changedLook(a.player.look),
		SubMode:         a.player.subMode,
		SubOrder:        a.player.subOrder,
		Untranslated:    a.player.untranslated,
		YieldSubs:       &yieldSubs,
		Volume:          &volume,
		Muted:           a.player.muted,
		Rate:            &rate,
		PauseLines:      a.player.pauseLines,
		LoopGap:         a.player.loopGap,
		SubtitleTipSeen: a.player.subTipSeen,
		PlayNext:        &playNext,
	}
}

// playerHooks: the player saves how subtitles look, takes the main window for full screen once it exists, shows the open film where the system shows what plays once the window is ready, so the media keys command it, closes the open film once it has left the shelf, keeps where a playing film is on disk when it pauses or ends (playerAfterChange) and now and then as it plays (VideoProgress), keeps the display awake while a film plays in sight once the window is ready, follows the open film's video when it moves on disk so its subtitle delay and place are kept by the new path, lets the display sleep while the main window is put away (playerWindowHidden), and stops the open video when the app quits, so where it stopped is kept for next time, keeping a volume changed just before.
func (a *app) playerHooks() areaHooks {
	return areaHooks{
		save: a.savePlayer, restore: a.restorePlayer, ready: a.playerReady, shelfChanged: a.closeFilmThatLeft, afterChange: a.playerAfterChange, windowShown: func(shown bool) { a.playerWindowHidden(!shown) }, quitting: a.playerQuitting,
		videosMoved: func(move func(string) (string, bool)) {
			if next, ok := move(a.player.film.video); ok {
				a.player.film.video = next
			}
			// The open video's place is kept by its new path from now on, as where it plays is remembered by it (rememberPosition).
			if next, ok := move(a.player.playPath); ok {
				a.player.playPath = next
			}
			if p := a.player.play; p != nil {
				if next, ok := move(p.path); ok {
					p.path = next
				}
			}
		},
	}
}

// playerReady points the player's full screen at the main window, which exists from now on, has the player keep the display awake while a film plays in sight (playerAwakeReady), and shows the open film where the system shows what plays (playerNowPlayingConnect).
func (a *app) playerReady() {
	a.player.screen = windowScreen{a}
	a.playerAwakeReady()
	a.playerNowPlayingConnect(nowplaying.System())
}

// closeFilmThatLeft closes the open film once neither the shelf's films nor its jobs have it any more, as when its folder was deleted, so its playback stops and its position is kept here rather than while the library page is being drawn.
func (a *app) closeFilmThatLeft() {
	if a.openKey == "" {
		return
	}
	if _, ok := films.Find(a.films(), a.openKey); ok {
		return
	}
	if _, ok := a.library.Find(a.openKey); ok {
		return
	}
	a.closeFilm()
}

// changedLook is the look saveSettings stores: nil while it is still film.DefaultLook, so the default is not saved as if the user had chosen it and a later default reaches those who never changed theirs.
func changedLook(look film.Look) *film.Look {
	if look == film.DefaultLook() {
		return nil
	}
	return &look
}
