package main

import (
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"

	"github.com/egoist/mygo/ui"
)

// playerUI is the state of the open film: the picture, the subtitle file and how subtitles look. The settings in it are saved with the app's settings (savedSettings).
type playerUI struct {
	play     *filmPlay
	playPath string
	playErr  string
	playGen  int
	playFrom time.Duration
	playJump bool
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
	// lastMode is the mode V turns the subtitles back on to.
	lastMode string
	// panel shows the side panel over the picture. It stays as it is from film to film.
	panel bool
	film  filmUI
}

// filmUI is the player's state that belongs to the one film open, which opening another starts afresh (resetFilm).
type filmUI struct {
	styleView   bool
	modeMenu    bool
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
	a.areasPositionsChanged()
}

// resumePositions and setResumePositions are the player's resume positions by video path, for the area that keeps them on disk with the shelf. The map is the player's own; setResumePositions takes positions over as they are.
func (a *app) resumePositions() map[string]time.Duration { return a.player.positions }

func (a *app) setResumePositions(positions map[string]time.Duration) { a.player.positions = positions }

// ResumeAt is where the video at path stopped last time, or 0 when it was not watched or was watched to the end.
func (a *app) ResumeAt(path string) time.Duration { return a.player.positions[path] }

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
}

// savedPlayer is the player's part of settings.json: how subtitles look and which lines show.
type savedPlayer struct {
	Look         *film.Look `json:"look,omitempty"`
	SubMode      string     `json:"subMode,omitempty"`
	SubOrder     string     `json:"subOrder,omitempty"`
	Untranslated string     `json:"untranslated,omitempty"`
	YieldSubs    *bool      `json:"yieldSubs,omitempty"`
}

// clean leaves the player's settings as they were read; restorePlayer replaces a value it does not know with the default.
func (s *savedPlayer) clean() {}

func (a *app) savePlayer(saved *savedSettings) {
	yieldSubs := a.player.yieldSubs
	saved.savedPlayer = savedPlayer{
		Look:         changedLook(a.player.look),
		SubMode:      a.player.subMode,
		SubOrder:     a.player.subOrder,
		Untranslated: a.player.untranslated,
		YieldSubs:    &yieldSubs,
	}
}

// playerHooks: the player saves how subtitles look, closes the open film once it has left the shelf, and stops the open video when the app quits, so its sound does not play on and where it stopped is kept for next time.
func (a *app) playerHooks() areaHooks {
	return areaHooks{save: a.savePlayer, restore: a.restorePlayer, shelfChanged: a.closeFilmThatLeft, afterFrame: a.loadClipFrames, quitting: a.stopPlayback}
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
