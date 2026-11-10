package main

import (
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// playerUI is the state of the open film: the picture, the subtitle file and how subtitles look. The settings in it are saved with the app's settings (savedSettings).
type playerUI struct {
	play     *filmPlay
	playPath string
	// playErr is playerErrUnplayable when the video would not open, or "".
	playErr  string
	playGen  int
	playFrom time.Duration
	// playJump has the video open playing at playFrom (a seek, 继续, 从头看, a jump to a line, a film coming back) rather than held on its first frame.
	playJump bool
	// playSame is set when the playback opens the open film's video again, as a seek, a film coming back and a jump to a line of the open film do, so the film keeps its state; any other playback is a film opening, which starts it afresh.
	playSame bool
	// opening is where the video being opened will start, which the transport shows until its picture comes.
	opening time.Duration
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
	// delays is how far the subtitles of each video were moved against its picture, by video path, so a film opens with them where they were left. A video whose subtitles are in place has none.
	delays map[string]time.Duration
	// saves is when the player last had the areas keep its resume positions and subtitle delays on disk (playerKeep).
	saves playerSaves
	// lastMode is the mode V turns the subtitles back on to.
	lastMode string
	// panel shows the side panel over the picture. It stays as it is from film to film.
	panel bool
	// volume is how loud films play, from 0 to 1, and muted silences them without losing it. Both stay as they are from film to film.
	volume float64
	muted  bool
	// volumeGen counts volume changes, so only the last of a quick run of them is written to the settings (playerVolumeChanged); volumeUnsaved is set until the change is kept in the settings.
	volumeGen     int
	volumeUnsaved bool
	// screen is the window the player enters and leaves full screen in: the main window once it exists (playerReady), and nil before, as in tests that set none, when full screen does nothing.
	screen fullScreen
	// inactive is set while the app is not the active one, and windowHidden while the main window is put away (state_window.go): either keeps the picture out of sight while the sound plays on (playerSight).
	inactive     bool
	windowHidden bool
	// keepAwake is mygo.Power.KeepAwake once the window is ready (playerReady), and nil before and in tests, which must not hold the machine awake; wake gives back the hold it took while the open film plays, nil while there is none (player_awake.go).
	keepAwake func(reason string, display bool) (release func())
	wake      func()
	film      filmUI
}

// fullScreen is the part of a window the player uses to enter and leave full screen, as *mygo.Window has it.
type fullScreen interface {
	IsFullScreen() bool
	SetFullScreen(v bool)
	ToggleFullScreen()
}

// filmUI is the player's state that belongs to the one film open, which opening another starts afresh (resetFilm).
type filmUI struct {
	styleView bool
	modeMenu  bool
	// video is the path of the film's video, which delay is kept by in playerUI.delays.
	video       string
	delay       time.Duration
	search      string
	searchFocus bool
	follow      int
	scrub       float64
	scrubbing   bool
	uploadNote  bool
	// info is the film's folder as readFilmInfo read it for the video at infoFor.
	info       film.Info
	infoFor    string
	clipFrames map[time.Duration]ui.ImageSource
	clipBusy   map[time.Duration]bool
	// osd is the message on the picture; osdAt is when it started showing, zero until it first shows.
	osd   string
	osdAt time.Time
	// moved is when the pointer last moved over the player, at pointer.
	moved   time.Time
	pointer [2]float32
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
		film:         filmUI{follow: -1},
	}
}

// resetFilm starts the state of one film afresh, as another film opens.
func (a *app) resetFilm() {
	a.player.film = filmUI{follow: -1}
	a.player.length = 0
}

// flash shows text on the picture for a moment.
func (a *app) flash(text string) {
	a.player.film.osd = text
	a.player.film.osdAt = time.Time{}
}

// rememberPosition keeps where the open video stopped, so ResumeAt can offer to continue from there. A video watched to the end starts over.
func (a *app) rememberPosition(play *filmPlay) {
	if play == nil || play.path == "" {
		return
	}
	at := play.playhead()
	if a.player.positions == nil {
		a.player.positions = map[string]time.Duration{}
	}
	if play.finished || at <= 0 {
		delete(a.player.positions, play.path)
	} else {
		a.player.positions[play.path] = at
	}
	a.playerPositionsChanged()
}

// resumePositions and setResumePositions are the player's resume positions by video path, for the area that keeps them on disk with the shelf. The map is the player's own; setResumePositions takes positions over as they are.
func (a *app) resumePositions() map[string]time.Duration { return a.player.positions }

func (a *app) setResumePositions(positions map[string]time.Duration) { a.player.positions = positions }

// ResumeAt is where the video at path stopped last time, or 0 when it was not watched or was watched to the end.
func (a *app) ResumeAt(path string) time.Duration { return a.player.positions[path] }

// subtitleDelays and setSubtitleDelays are how far the subtitles of each video were moved, by video path, for the area that keeps them on disk with the shelf beside the resume positions. The map is the player's own; setSubtitleDelays takes delays over as they are.
func (a *app) subtitleDelays() map[string]time.Duration { return a.player.delays }

func (a *app) setSubtitleDelays(delays map[string]time.Duration) { a.player.delays = delays }

// How often a playing film is kept on disk (playerKeep): at most every playerKeepEvery, once it has moved more than playerKeepMoved since, and a subtitle delay once it has rested playerDelaySettle, so a run of Z or X presses is saved once.
const (
	playerKeepEvery   = 20 * time.Second
	playerKeepMoved   = 2 * time.Second
	playerDelaySettle = time.Second
)

// playerSaves is what playerKeep needs to save the open film now and then rather than on every frame.
type playerSaves struct {
	// play is the playback the position was last looked at for; at is when its position was last kept and head where it was then; playing is whether it played at the last frame.
	play    *filmPlay
	at      time.Time
	head    time.Duration
	playing bool
	// delayDirty is set while a subtitle delay changed since the last save; delayAt is the first frame since its last change, zero until then.
	delayDirty bool
	delayAt    time.Time
}

// playerAfterFrame loads the frames of the 片段 tab, keeps where the open film is, and how its subtitles were moved, on disk now and then, and keeps the display awake while a film plays (player_awake.go).
func (a *app) playerAfterFrame() {
	a.loadClipFrames()
	a.playerKeep(time.Now())
	a.playerKeepAwake()
}

// playerKeep saves where the open film is while it plays, so a crash, a force quit or a power cut loses little of it rather than all since it opened: at once when a playback opened playing at a point picked for it (a seek, 继续, 从头看) and when it pauses or ends, and in between every playerKeepEvery once it has moved on. A film held on the frame it opened on is not kept until it plays, as stopPlayback does not keep it either. A subtitle delay is saved once it has rested playerDelaySettle. now is the time of the frame.
func (a *app) playerKeep(now time.Time) {
	s := &a.player.saves
	if s.delayDirty {
		if s.delayAt.IsZero() {
			s.delayAt = now
		} else if now.Sub(s.delayAt) >= playerDelaySettle {
			a.playerPositionsChanged()
		}
	}
	play := a.player.play
	if play == nil || play.path == "" {
		s.play, s.playing = nil, false
		return
	}
	playing, head := play.Playing(), play.playhead()
	keep := func() {
		a.rememberPosition(play)
		s.at, s.head = now, head
	}
	switch {
	case s.play != play:
		s.play, s.at, s.head = play, now, head
		if play.played {
			keep()
		}
	case !play.played:
	case s.playing && !playing:
		keep()
	case now.Sub(s.at) >= playerKeepEvery && (head-s.head).Abs() > playerKeepMoved:
		keep()
	}
	s.playing = playing
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
	a.player.saves.delayDirty = true
	a.player.saves.delayAt = time.Time{}
}

// playerSaveDelay saves a subtitle delay that changed since the last save, as its film closes or another opens.
func (a *app) playerSaveDelay() {
	if a.player.saves.delayDirty {
		a.playerPositionsChanged()
	}
}

// playerPositionsChanged has the areas that keep the player's resume positions and subtitle delays on disk save them, which saves a delay waiting to be saved too.
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
}

// savedPlayer is the player's part of settings.json: how subtitles look, which lines show and how loud films play.
type savedPlayer struct {
	Look         *film.Look `json:"look,omitempty"`
	SubMode      string     `json:"subMode,omitempty"`
	SubOrder     string     `json:"subOrder,omitempty"`
	Untranslated string     `json:"untranslated,omitempty"`
	YieldSubs    *bool      `json:"yieldSubs,omitempty"`
	// Volume is how loud films play, from 0 to 1; Muted silences them.
	Volume *float64 `json:"volume,omitempty"`
	Muted  bool     `json:"muted,omitempty"`
}

// clean leaves the player's settings as they were read; restorePlayer replaces a value it does not know with the default.
func (s *savedPlayer) clean() {}

func (a *app) savePlayer(saved *savedSettings) {
	yieldSubs := a.player.yieldSubs
	volume := a.player.volume
	saved.savedPlayer = savedPlayer{
		Look:         changedLook(a.player.look),
		SubMode:      a.player.subMode,
		SubOrder:     a.player.subOrder,
		Untranslated: a.player.untranslated,
		YieldSubs:    &yieldSubs,
		Volume:       &volume,
		Muted:        a.player.muted,
	}
}

// playerHooks: the player saves how subtitles look, takes the main window for full screen once it exists, closes the open film once it has left the shelf, keeps where a playing film is and how its subtitles were moved on disk now and then after each frame, keeps the display awake while a film plays once the window is ready, follows the open film's video when it moves on disk so its subtitle delay is kept by the new path, follows the sound while the main window is put away (playerWindowHidden), and stops the open video when the app quits, so its sound does not play on and where it stopped is kept for next time, keeping a volume changed just before.
func (a *app) playerHooks() areaHooks {
	return areaHooks{
		save: a.savePlayer, restore: a.restorePlayer, ready: a.playerReady, shelfChanged: a.closeFilmThatLeft, afterFrame: a.playerAfterFrame, windowShown: func(shown bool) { a.playerWindowHidden(!shown) }, quitting: a.playerQuitting,
		videosMoved: func(move func(string) (string, bool)) {
			if next, ok := move(a.player.film.video); ok {
				a.player.film.video = next
			}
		},
	}
}

// playerReady points the player's full screen at the main window, which exists from now on, and gives the player mygo.Power.KeepAwake (player_awake.go).
func (a *app) playerReady() {
	a.player.screen = a.win
	a.player.keepAwake = mygo.Power.KeepAwake
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
