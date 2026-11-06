package main

import (
	"time"

	"lingocut/apps/lingocut/film"
)

// Subtitle modes: which lines of a bilingual cue the player shows. V steps through them in this order.
const (
	subBilingual   = "bilingual"
	subTranslation = "translation"
	subOriginal    = "original"
	subOff         = "off"
)

// Subtitle orders: which line of a bilingual cue goes on top.
const (
	subTranslationFirst = "translation-first"
	subOriginalFirst    = "original-first"
)

// What the player shows for a cue that has no translation yet.
const (
	untranslatedDim   = "dim"
	untranslatedBlank = "blank"
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
	// subMode is one of subBilingual, subTranslation, subOriginal or subOff.
	subMode string
	// subOrder is subTranslationFirst or subOriginalFirst.
	subOrder string
	// untranslated is untranslatedDim or untranslatedBlank.
	untranslated string
	// yieldSubs moves the subtitles up while the player's controls show.
	yieldSubs bool
	// positions is where each video was when its playback stopped, by video path, so it can resume there.
	positions map[string]time.Duration
}

func newPlayerUI() playerUI {
	return playerUI{
		look:         film.DefaultLook(),
		subMode:      subBilingual,
		subOrder:     subTranslationFirst,
		untranslated: untranslatedDim,
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

// restorePlayer takes the subtitle settings saved last time. A value this version does not know keeps the default.
func (a *app) restorePlayer(saved savedSettings) {
	if saved.Look != nil {
		a.player.look = *saved.Look
	}
	switch saved.SubMode {
	case subBilingual, subTranslation, subOriginal, subOff:
		a.player.subMode = saved.SubMode
	}
	switch saved.SubOrder {
	case subTranslationFirst, subOriginalFirst:
		a.player.subOrder = saved.SubOrder
	}
	switch saved.Untranslated {
	case untranslatedDim, untranslatedBlank:
		a.player.untranslated = saved.Untranslated
	}
	if saved.YieldSubs != nil {
		a.player.yieldSubs = *saved.YieldSubs
	}
}
