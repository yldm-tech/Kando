package main

import (
	"time"

	"lingocut/apps/lingocut/film"
)

// playerUI is the state of the open film: the picture, the subtitle file and how subtitles look. The settings in it are saved with the app's settings (savedSettings).
type playerUI struct {
	play     *filmPlay
	playPath string
	playErr  string
	playGen  int
	playFrom time.Duration
	playJump bool
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
}

func newPlayerUI() playerUI {
	return playerUI{
		look:         film.DefaultLook(),
		subMode:      film.SubBilingual,
		subOrder:     film.SubTranslationFirst,
		untranslated: film.UntranslatedDim,
		yieldSubs:    true,
	}
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
		return
	}
	a.player.positions[play.path] = at
}

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

// playerHooks: the player saves how subtitles look.
func (a *app) playerHooks() areaHooks {
	return areaHooks{save: a.savePlayer, restore: a.restorePlayer}
}

// changedLook is the look saveSettings stores: nil while it is still film.DefaultLook, so the default is not saved as if the user had chosen it and a later default reaches those who never changed theirs. Edit, the face the style pane is editing, is not part of the look's appearance and does not count as a change.
func changedLook(look film.Look) *film.Look {
	def := film.DefaultLook()
	def.Edit = look.Edit
	if look == def {
		return nil
	}
	return &look
}
