package player

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/apps/mobile/study"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// sleepMode is when the sleep timer holds the film.
type sleepMode uint8

const (
	sleepOff sleepMode = iota
	// sleepLine holds it once the line playing ends.
	sleepLine
	sleep15
	sleep30
	// sleepFilm holds it at its end, as it would anyway, but says so.
	sleepFilm
)

// sleepModes are the sleep timer's choices, in the order the settings list them.
var sleepModes = []sleepMode{sleepOff, sleepLine, sleep15, sleep30, sleepFilm}

// minutes is how long a timed sleep mode waits, 0 for the others.
func (m sleepMode) minutes() int {
	switch m {
	case sleep15:
		return 15
	case sleep30:
		return 30
	}
	return 0
}

// The subtitle delay's limits and step: the settings move the lines by a tenth of a second, at most ten seconds either way.
const (
	delayStep = 100 * time.Millisecond
	delayMax  = 10 * time.Second
)

// play is the player's state on the engine's goroutine.
type play struct {
	cx  *core.Scope
	s   *session.Session
	key string
	// watcher tells the computer where a film being made is watched, nil for any other film (session.WatchMaking).
	watcher *session.Watcher
	ref     *core.Ref
	cues    *core.Resource[[]cues.Cue]
	// lines are the film's lines as the player shows them: the cues kept, with the fixes of their translations laid over them (session.Corrections).
	lines *core.Memo[[]cues.Cue]
	// marks are the word marks of the lines' originals, index for index, as the film's computer last answered them (session.FilmMarks), nil while there are none or Settings › Subtitle Style turns them off (marks.go).
	marks   *core.Memo[[][]sync.Mark]
	playing *core.Signal[bool]
	at      *core.Signal[time.Duration]
	length  *core.Signal[time.Duration]
	aspect  *core.Signal[float32]
	failed  *core.Signal[bool]
	// loop is the line looped, -1 for none.
	loop  *core.Signal[int]
	mode  *core.Persisted[string]
	rate  *core.Persisted[float32]
	saved time.Duration
	// reported is where the video last said it was, and watched how long it played since the time watched was last kept (session.Watch).
	reported, watched time.Duration
	// current is the line showing, -1 for none; shown is the line the card shows: the one showing, else the last one heard, else the first.
	current *core.Memo[int]
	shown   *core.Memo[int]
	// delay moves the lines against the picture, later for more than 0, for this film alone (store.Film.DelayMS).
	delay *core.Signal[time.Duration]
	// follow is shadowing: the film holds after each line, held set, for the learner to say it, and plays on when they play it again.
	follow *core.Persisted[bool]
	held   *core.Signal[bool]
	// audioOnly covers the picture; onPicture shows the lines over the picture while the phone is upright too, where the card shows them otherwise.
	audioOnly *core.Persisted[bool]
	onPicture *core.Persisted[bool]
	// sleep is the sleep timer, until when a timed one runs out.
	sleep      *core.Signal[sleepMode]
	sleepUntil time.Time
	// autoNext is 自动播放下一部; card is the card that plays the next film once this one ends, and nextStop stops its countdown (next.go).
	autoNext *core.Persisted[bool]
	// repeat is 循环播放: the film plays again from its start once it ends, in place of the next.
	repeat   *core.Persisted[bool]
	card     *core.Signal[upNext]
	nextStop func() bool
	// revealed is the line whose texts the mode hides that a tap shows, -1 for none: the card blurs them again on the next line.
	revealed *core.Signal[int]
	// heard is the line last playing, -1 since a seek: the line whose end shadowing and the sleep timer wait for.
	heard int
	now   func() time.Time
	toast *ui.Toaster
	// pip is the system's picture in picture of the video, as it last reported it: whether it is offered and whether its window shows.
	pip *core.Signal[core.PictureInPictureState]
	// status is the status bar while the player shows (Layer.StatusBar).
	status func() core.StatusBarSetting
	// cut counts the dictionaries that cut Japanese and Chinese lines into words read since the player opened, so the lines' words are cut again once one is (tokens).
	cut *core.Signal[int]
	// recut is set while a cut of the lines again is posted (cutAgain), so the lines waiting for a dictionary, however many, are cut again once.
	recut atomic.Bool
}

// say shows title in a toast in place of the last one of kind, as a line saved and then taken back says only the latter: the toast's ID is its kind, so one still showing takes the new title.
func (p *play) say(kind, title string) {
	p.toast.Show(ui.ToastContent{ID: kind, Title: title})
}

// noteStudy tells the user, before the first lookup, explanation, follow-up question or recap this phone asks a computer for, that the lines it uses go to the AI service set up on the film's computer; it says so once, ever (session.NoteStudy).
func (p *play) noteStudy() {
	if p.s.NoteStudy() {
		p.toast.Show(ui.ToastContent{ID: "study-note", Title: locales.PlayerStudyNote(p.cx, computer(p)), Duration: 6 * time.Second})
	}
}

func newPlay(cx *core.Scope, s *session.Session, key string) *play {
	f, _ := s.Film(key)
	playable := s.VideoFile(key) != "" || (f.Streamed || f.FromSource()) && !f.Done() && s.StreamLink(key) != ""
	p := &play{cx: cx, s: s, key: key, ref: &core.Ref{},
		playing:   core.NewSignal(cx, playable),
		at:        core.NewSignal(cx, time.Duration(f.PositionMS)*time.Millisecond),
		length:    core.NewSignal(cx, time.Duration(f.Film.DurationMS)*time.Millisecond),
		aspect:    core.NewSignal(cx, float32(16.0/9)),
		failed:    core.NewSignal(cx, false),
		loop:      core.NewSignal(cx, -1),
		mode:      core.NewPersisted(cx, modeKey, kcore.SubBilingual, core.PersistOptions{}),
		rate:      core.NewPersisted(cx, speedKey, float32(1), core.PersistOptions{}),
		follow:    core.NewPersisted(cx, followKey, false, core.PersistOptions{}),
		audioOnly: core.NewPersisted(cx, audioOnlyKey, false, core.PersistOptions{}),
		onPicture: core.NewPersisted(cx, onPictureKey, false, core.PersistOptions{}),
		held:      core.NewSignal(cx, false),
		delay:     core.NewSignal(cx, time.Duration(f.DelayMS)*time.Millisecond),
		sleep:     core.NewSignal(cx, sleepOff),
		autoNext:  core.NewPersisted(cx, autoNextKey, true, core.PersistOptions{}),
		repeat:    core.NewPersisted(cx, repeatKey, false, core.PersistOptions{}),
		card:      core.NewSignal(cx, upNext{}),
		revealed:  core.NewSignal(cx, -1),
		cut:       core.NewSignal(cx, 0),
		pip:       core.NewSignal(cx, core.PictureInPictureState{}),
		saved:     time.Duration(f.PositionMS) * time.Millisecond,
		heard:     -1,
		now:       time.Now,
		toast:     ui.NewToaster(cx),
	}
	// The lines are those of the film the film takes them from (session.LinesFilm): its own, or for a film of a source, those of the computer's film of the same file, read again once it takes them. The memo keeps them from being read again at every change of the film's record, as its place is kept while it plays.
	linesFrom := core.NewMemo(cx, func() [2]string {
		lf, _ := s.LinesFilm(key)
		return [2]string{lf.Desktop, lf.Film.ID}
	})
	p.cues = core.NewResource(cx, func() ([2]string, bool) { return linesFrom.Get(), true }, func(_ context.Context, from [2]string, _ core.ResourceInfo) ([]cues.Cue, error) {
		list, _, err := s.Store().Cues(from[0], from[1])
		return cues.FromSync(list), err
	})
	p.lines = core.NewMemoFunc(cx, func() []cues.Cue {
		list, _ := p.cues.Get()
		return corrected(list, s.Corrections(key))
	}, slices.Equal[[]cues.Cue])
	p.marks = core.NewMemoFunc(cx, func() [][]sync.Mark {
		m, ok := s.FilmMarks(key)
		if !ok || s.Prefs().Subtitles.NoMarks {
			return nil
		}
		list := p.list()
		originals := make([]string, len(list))
		for i, c := range list {
			originals[i] = c.Original
		}
		return m.Of(originals)
	}, sameMarks)
	p.current = core.NewMemo(cx, func() int { return cues.Index(p.list(), p.subAt()) })
	p.shown = core.NewMemo(cx, func() int {
		if i := p.current.Get(); i >= 0 {
			return i
		}
		list := p.list()
		if len(list) == 0 {
			return -1
		}
		if i := cues.Next(list, p.subAt()); i > 0 {
			return i - 1
		} else if i < 0 {
			return len(list) - 1
		}
		return 0
	})
	// The lines kept play at once; a desktop that has changed them since, as by translating the film again, gives its own, which take their place.
	s.Follow(key, p.cues.Refetch)
	// A film being made tells its computer where it is watched while it shows, so its lines are made there first, and reads them again as they come.
	p.watcher = s.WatchMaking(key, p.at.GetUntracked(), p.cues.Refetch)
	cx.Cleanup(func() { p.watcher.Stop() })
	// A film streamed from a source lets its URL and its connection go with its player.
	cx.Cleanup(func() { s.EndStream(key) })
	// The lines' word marks, as the phone keeps them, then as the computer has them once it answers.
	s.OpenMarks(key)
	// The lines saved from the film and their notes, as the phone keeps them, then as the desktop has them once it answers.
	s.OpenStudy(key)
	// The film's glossary, once the phone knows the film's library, which the first opening asks the desktop for.
	askedGlossary := false
	cx.Effect(func() {
		if askedGlossary || !s.Studyable(key) {
			return
		}
		askedGlossary = true
		core.Untracked(cx, func() bool { s.OpenGlossary(key); return true })
	})
	// The screen stays on while the film plays, and while the card counts down to the next, so the phone does not lock as it would play.
	var release func()
	cx.Effect(func() {
		awake := p.playing.Get() || p.card.Get() != (upNext{})
		if awake && release == nil {
			release = cx.KeepAwake(locales.PlayerKeepAwake(cx), core.KeepAwakeOptions{Display: true})
		} else if !awake && release != nil {
			release()
			release = nil
		}
	})
	// The card goes once the film plays again, and once the app goes to the background, where its countdown would play the next film unseen.
	cx.Effect(func() {
		if p.playing.Get() || cx.Env().Lifecycle() == core.LifecycleBackground {
			core.Untracked(cx, func() bool { p.closeNext(); return true })
		}
	})
	// A line shown by a tap is hidden again once another plays.
	cx.Effect(func() {
		if i := p.shown.Get(); i != p.revealed.GetUntracked() {
			p.revealed.Set(-1)
		}
	})
	cx.Cleanup(p.save)
	return p
}

// hasVideo reports whether the film has a video to play: on the phone (session.VideoFile), or for a film kept to stream (streamed) in its computer's storage. A film kept by its lines alone is studied line by line, holding still, until it has. Read it in a binding.
func (p *play) hasVideo() bool {
	if p.streamed() {
		return p.s.StreamLink(p.key) != ""
	}
	return p.s.VideoFile(p.key) != ""
}

// streamed reports whether the film is kept to stream rather than downloaded (store.Film.Streamed): from its computer on its network, or from its storage away from it (session.StreamLink). A film of a source streams from its file there until its download is done, even while it downloads, unless the video of the computer's film matched to it is on the phone (session.VideoFile). Read it in a binding.
func (p *play) streamed() bool {
	f, ok := p.s.Film(p.key)
	return ok && (f.Streamed || f.FromSource()) && !f.Done() && p.s.VideoFile(p.key) == ""
}

// untranslated reports whether the film's lines were not translated by its computer (sync.Film.Translation is empty), as a film heard in the language its subtitles are for: its lines are its own alone. Read it in a binding.
func (p *play) untranslated() bool {
	f, _ := p.s.LinesFilm(p.key)
	return f.Film.Translation == ""
}

// subAt is where the playhead is in the lines' time: the picture's, less the delay.
func (p *play) subAt() time.Duration { return p.at.Get() - p.delay.Get() }

// shownMode is the mode the picture shows the lines in: the one chosen, but for a film the desktop did not translate, whose lines show as Original shows them rather than dimmed as lines not translated yet.
func (p *play) shownMode() string {
	if mode := p.mode.Get(); !p.untranslated() || (mode != kcore.SubTranslation && mode != kcore.SubBilingual) {
		return mode
	}
	return kcore.SubOriginal
}

// hides reports whether the mode hides the original, or the translation, of the line the card shows until a tap shows it.
func (p *play) hides(original bool) bool {
	if p.revealed.Get() == p.shown.Get() {
		return false
	}
	switch p.shownMode() {
	case kcore.SubOff:
		return true
	case kcore.SubOriginal:
		return !original
	case kcore.SubTranslation:
		return original
	}
	return false
}

// list is the film's lines as the player shows them, the learner's fixes laid over them, none until they are read.
func (p *play) list() []cues.Cue { return p.lines.Get() }

// line is line i of the film, the zero Cue for none.
func (p *play) line(i int) cues.Cue {
	if l := p.list(); i >= 0 && i < len(l) {
		return l[i]
	}
	return cues.Cue{}
}

// speed is the speed playing.
func (p *play) speed() float32 { return p.rate.Get() }

// seek plays from at.
func (p *play) seek(at time.Duration) {
	p.closeNext()
	at = max(at, 0)
	p.ref.Seek(at)
	p.at.Set(at)
	p.heard = -1
	p.watcher.Move(at, true)
}

// progress hears the video's position as it plays: it moves the playhead, takes a looped line back to its start, holds the film after a line for shadowing or the sleep timer, and keeps the position now and then.
func (p *play) progress(at, length time.Duration) {
	if length > 0 {
		p.length.Set(length)
	}
	p.at.Set(at)
	p.watcher.Move(at, false)
	// The time watched counts what played between two of the video's reports, at its speed, not a seek.
	if d := at - p.reported; p.playing.GetUntracked() && d > 0 && d < time.Second {
		p.watched += time.Duration(float64(d) / float64(max(p.rate.Get(), 0.25)))
	}
	p.reported = at
	list, delay, total := p.list(), p.delay.GetUntracked(), p.length.GetUntracked()
	sub := at - delay
	if i := p.loop.GetUntracked(); i >= 0 && i < len(list) && cues.LoopBack(list, i, sub, total) {
		start, _ := cues.Span(list, i, total)
		p.seek(start + delay)
		return
	}
	if h := p.heard; h >= 0 && h < len(list) && p.playing.GetUntracked() {
		if start, end := cues.Span(list, h, total); end > start && sub >= end {
			p.heard = -1
			p.lineEnded()
		}
	}
	if i := cues.Index(list, sub); i >= 0 && p.playing.GetUntracked() {
		p.heard = i
	}
	if m := p.sleep.GetUntracked(); m.minutes() > 0 && !p.now().Before(p.sleepUntil) && p.playing.GetUntracked() {
		p.sleep.Set(sleepOff)
		p.hold()
		p.say("sleep", locales.PlayerSleepPaused(p.cx))
	}
	if d := at - p.saved; d >= saveEvery || d <= -saveEvery {
		p.keep(false)
	}
}

// lineEnded holds the film once a line has played to its end, for shadowing or the sleep timer set to the line's end.
func (p *play) lineEnded() {
	switch {
	case p.sleep.GetUntracked() == sleepLine:
		p.sleep.Set(sleepOff)
		p.hold()
		p.say("sleep", locales.PlayerSleepLineDone(p.cx))
	case p.follow.Get():
		p.hold()
		p.held.Set(true)
	}
}

// hold holds the film and keeps where it is.
func (p *play) hold() {
	p.playing.Set(false)
	p.save()
}

// ended is the film played to its end: a line looped plays again, the sleep timer's End of Film holds it there, repeat plays it again from its start, and otherwise the next film of its course or playlist plays after it (upNextAtEnd), as l, the app's player, goes on to it.
// Played again, the film is not held, so its end is not kept as where it was left nor sent at once: the position kept goes back to its start with the next report (progress), and the time watched counts on as it plays.
func (p *play) ended(l *Layer) {
	if i := p.loop.GetUntracked(); i >= 0 {
		p.step(i)
		p.playing.Set(true)
		return
	}
	if p.sleep.GetUntracked() == sleepFilm {
		p.hold()
		p.sleep.Set(sleepOff)
		return
	}
	if p.repeat.Get() && p.hasVideo() {
		p.held.Set(false)
		p.seek(0)
		p.playing.Set(true)
		return
	}
	p.hold()
	p.upNextAtEnd(l)
}

// save keeps the position playing with the film, as the film is held or left, and sends it to the film's computer.
func (p *play) save() { p.keep(true) }

// keep keeps the position playing with the film; send sends it to the film's computer at once, else it goes with the next sync, as while the film plays on.
func (p *play) keep(send bool) {
	p.saved = p.at.GetUntracked()
	if send {
		p.s.SavePosition(p.key, p.saved)
	} else {
		p.s.KeepPosition(p.key, p.saved)
	}
	p.s.Watch(p.key, p.watched)
	p.watched = 0
}

// toggle starts or holds the film; one without its video on the phone holds still.
func (p *play) toggle() {
	if !p.hasVideo() {
		return
	}
	p.held.Set(false)
	p.playing.Update(func(on bool) bool { return !on })
	p.watcher.Move(p.at.GetUntracked(), true)
	if !p.playing.GetUntracked() {
		p.save()
	}
}

// step plays from the start of line i, which the loop moves to while it is on.
func (p *play) step(i int) {
	if i < 0 || i >= len(p.list()) {
		return
	}
	if p.loop.GetUntracked() >= 0 {
		p.loop.Set(i)
	}
	p.held.Set(false)
	p.seek(p.list()[i].Start + p.delay.GetUntracked())
	p.playing.Set(p.hasVideo())
}

// prev plays from the line before the one heard, or from the start of the one playing when it has played for more than a moment.
func (p *play) prev() {
	sub := p.at.GetUntracked() - p.delay.GetUntracked()
	if i := cues.Index(p.list(), sub); i >= 0 && sub-p.list()[i].Start > restartWithin {
		p.step(i)
		return
	}
	p.step(cues.Prev(p.list(), sub))
}

// restartWithin is how far into a line Previous Line plays it again rather than the one before.
const restartWithin = 1500 * time.Millisecond

// next plays from the line after the one heard.
func (p *play) next() {
	p.step(cues.Next(p.list(), p.at.GetUntracked()-p.delay.GetUntracked()))
}

// replay plays the line the card shows again from its start.
func (p *play) replay() { p.step(p.shown.Get()) }

// toggleLoop loops the line the card shows, or stops looping.
func (p *play) toggleLoop() {
	if p.loop.GetUntracked() >= 0 {
		p.loop.Set(-1)
		return
	}
	i := p.current.Get()
	if i < 0 {
		i = cues.Next(p.list(), p.at.GetUntracked()-p.delay.GetUntracked())
	}
	if i >= 0 {
		p.loop.Set(i)
		if p.current.Get() != i {
			p.seek(p.list()[i].Start + p.delay.GetUntracked())
		}
	}
}

// setDelay moves the lines by d against the picture, for this film alone, within delayMax either way.
func (p *play) setDelay(d time.Duration) {
	d = min(max(d, -delayMax), delayMax)
	p.delay.Set(d)
	p.s.SetDelay(p.key, d)
}

// setSleep sets the sleep timer, a timed one running from now.
func (p *play) setSleep(m sleepMode) {
	p.sleepUntil = p.now().Add(time.Duration(m.minutes()) * time.Minute)
	p.sleep.Set(m)
}

// studyCue is line i as the learning state saves it.
func (p *play) studyCue(i int) study.Cue {
	c := p.line(i)
	return study.Cue{Start: c.Start, End: c.End, Original: c.Original, Translation: c.Translation}
}

// isSaved reports whether line i is saved. Read it in a binding.
func (p *play) isSaved(i int) bool {
	if i < 0 || i >= len(p.list()) {
		return false
	}
	_, ok := p.s.SavedAt(p.key, p.list()[i].Start)
	return ok
}

// toggleSave saves the line the card shows, or takes it back, and says which.
func (p *play) toggleSave() {
	i := p.shown.Get()
	if i < 0 {
		return
	}
	if !p.s.Studyable(p.key) {
		p.say("save", locales.PlayerCannotSave(p.cx))
		return
	}
	if p.s.ToggleSave(p.key, p.studyCue(i)) {
		p.say("save", locales.PlayerSavedToast(p.cx))
		return
	}
	p.say("save", locales.PlayerUnsavedToast(p.cx))
}

// around is line i with the lines around it, as an explanation's context, and i's index among them.
func (p *play) around(i int) ([]sync.Cue, int) {
	list := p.list()
	from, to := max(i-explainBefore, 0), min(i+explainAfter+1, len(list))
	out := make([]sync.Cue, 0, to-from)
	for _, c := range list[from:to] {
		out = append(out, sync.Cue{StartMS: c.Start.Milliseconds(), EndMS: c.End.Milliseconds(), Original: c.Original, Translation: c.Translation})
	}
	return out, i - from
}

// explainBefore and explainAfter are how many lines before the line explained, and after it, go with it as its context, as the desktop's Explain Line sends them.
const (
	explainBefore = 2
	explainAfter  = 1
)
