package main

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
	"kando/apps/kando/lookup"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/core"
)

// vocabUI is the vocabulary's: the word looked up on the picture (lookup), the words looked up on this computer (cache), the time films play, which goes into the study record now and then (watch), and the dialog of the study's stats and the vocabulary's export (bind_vocab.go). The words themselves are in the open library's study record, the saved lines' book (state_saved.go).
type vocabUI struct {
	lookup vocabLookup
	// cache is the words looked up on this computer, read from beside settings.json the first time one is looked up (vocabCache).
	cache *lookup.Cache
	// gen numbers the lookups, so the answer to one the learner left is dropped; cancel stops the one asking the AI service.
	gen    int
	cancel context.CancelFunc
	watch  vocabWatch
	stats  vocabStats
}

// vocabLookup is the word looked up on the picture (film.Lookup): the word as clicked, the line it was clicked in, cue, its index in the open film's lines, and the film's video and title, so the word goes into the vocabulary with them; busy while the AI service is asked, then its entry, or why there is none (say), with needsAI set when the AI service's settings are what helps.
type vocabLookup struct {
	open              bool
	word              string
	cue               int
	line, translation string
	video, film       string
	start             time.Duration
	busy, failed      bool
	needsAI           bool
	language          string
	entry             lookup.Entry
	say               func(*core.Scope) string
}

// vocabWatch is the time the open film has played: since is when the player last said it was playing, zero while it is not, and pending how long it played since the study record was last told (vocabFlushWatch).
type vocabWatch struct {
	since   time.Time
	pending time.Duration
}

// The time films play goes into the study record a minute at a time, and when they stop; a gap between two of the player's reports longer than watchGap, as while the computer slept, does not count.
const (
	watchFlush = time.Minute
	watchGap   = 3 * time.Second
	// lookupTimeout is how long a lookup waits for the AI service.
	lookupTimeout = 45 * time.Second
)

func newVocabUI() vocabUI { return vocabUI{stats: newVocabStats()} }

// vocabHooks: the time the open film plays adds up while it plays and goes into the study record of its library a minute at a time, once it is held or closed, before another library opens and as the app quits; the word looked up closes once its film is not the one open.
func (a *app) vocabHooks() areaHooks {
	return areaHooks{
		playing:        a.vocabPlaying,
		leavingLibrary: func() { a.vocabFlushWatch() },
		afterChange: func() bool {
			held := a.vocabHeld()
			return a.vocabFollowFilm() || held
		},
		quitting: func() {
			a.vocabFlushWatch()
			a.vocabStopLookup()
		},
	}
}

// vocabKeys are the 学习 menu's Study Stats and Export Vocabulary…, which open the stats dialog at its two tabs; neither has a key.
func (a *app) vocabKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupStudy, Label: locales.VocabStatsMenuItem(cx), Run: func() { a.OpenStats(vocabTabStats) }},
		{Group: groupStudy, Label: locales.VocabExportMenuItem(cx), Run: func() { a.OpenStats(vocabTabExport) }},
	}
}

// vocabPlaying adds the time since the player's last report to the time the open film played, while it plays, and tells the study record once a minute has added up or the film stops. It changes nothing a binding reads but the open stats dialog's numbers, which it reports.
func (a *app) vocabPlaying() bool {
	now := a.saved.now()
	w := &a.vocab.watch
	if !a.vocabFilmPlaying() {
		return a.vocabHeld()
	}
	if !w.since.IsZero() {
		if d := now.Sub(w.since); d > 0 && d <= watchGap {
			w.pending += d
		}
	}
	w.since = now
	if w.pending < watchFlush {
		return false
	}
	return a.vocabFlushWatch() && a.vocab.stats.open
}

// vocabFilmPlaying reports whether a film plays now.
func (a *app) vocabFilmPlaying() bool {
	play := a.player.play
	return play != nil && play.path != "" && play.Playing()
}

// vocabHeld puts the time played into the study record once no film plays, as after a film was held or closed, and reports whether that changed what the open stats dialog shows. A film that has just started playing counts from now, before the player's first report.
func (a *app) vocabHeld() bool {
	if a.vocabFilmPlaying() {
		if a.vocab.watch.since.IsZero() {
			a.vocab.watch.since = a.saved.now()
		}
		return false
	}
	a.vocab.watch.since = time.Time{}
	return a.vocabFlushWatch() && a.vocab.stats.open
}

// vocabFlushWatch puts the time played since it was last told into the open library's study record, on today's date, and reports whether there was any.
func (a *app) vocabFlushWatch() bool {
	d := a.vocab.watch.pending
	if d <= 0 {
		return false
	}
	a.vocab.watch.pending = 0
	a.savedSync()
	if a.lockShut() || a.saved.book.Dir() == "" {
		return false
	}
	a.saved.book.Watch(a.saved.now(), d)
	a.savedWrite()
	return true
}

// vocabFollowFilm closes the word looked up once its film is not the one open, and reports whether it did.
func (a *app) vocabFollowFilm() bool {
	l := a.vocab.lookup
	if !l.open || a.player.film.video == l.video {
		return false
	}
	a.CloseLookup()
	return true
}

// vocabCache is the words looked up on this computer, read the first time it is needed.
func (a *app) vocabCache() *lookup.Cache {
	if a.vocab.cache == nil {
		path := ""
		if a.settingsPath != "" {
			path = filepath.Join(filepath.Dir(a.settingsPath), lookup.FileName)
		}
		a.vocab.cache = lookup.Load(path)
	}
	return a.vocab.cache
}

// Lookup is the word looked up on the picture, as its card shows it.
func (a *app) Lookup() film.Lookup {
	l := a.vocab.lookup
	if !l.open {
		return film.Lookup{}
	}
	out := film.Lookup{Open: true, Word: l.word, Line: l.line, Busy: l.busy, Failed: l.failed, NeedsAI: l.needsAI, Say: l.say}
	if !l.busy && !l.failed {
		out.Word = cmp.Or(l.entry.Word, l.word)
		out.Reading, out.Meaning = l.entry.Reading, l.entry.Meaning
		book := a.savedBook()
		out.Saved = book.WordIndex(out.Word) >= 0
	}
	return out
}

// LookUpWord is a click on word in line i of the open film's lines, on the picture: the film holds, and the card over it shows what word means there, from the words looked up before on this computer, or else as the AI service set in 「AI 服务」 says, which it is asked off the engine's goroutine. Without an AI service that can answer, as while offline mode is on and it is not on this computer, the card says why and offers its settings.
func (a *app) LookUpWord(i int, word string) {
	cues := a.playerCues()
	word = strings.TrimSpace(word)
	if i < 0 || i >= len(cues) || word == "" || a.player.film.video == "" {
		return
	}
	if p := a.Play(); p != nil && p.Playing() {
		p.Toggle()
	}
	a.vocabStopLookup()
	translation, original := film.SplitCue(cues[i].Text)
	item, _ := a.openFilm()
	language := paletteAnswerLanguage(item, a.prefs.language)
	a.vocab.lookup = vocabLookup{open: true, word: word, cue: i, line: original, translation: translation, video: a.player.film.video,
		film: cmp.Or(item.Name, item.Title), start: cues[i].Start, language: language}
	if e, ok := a.vocabCache().Get(word, language); ok {
		a.vocab.lookup.entry = e
		return
	}
	t, why := a.paletteTranslator(locales.VocabLookupNoAI)
	if why != nil {
		a.vocab.lookup.failed, a.vocab.lookup.needsAI, a.vocab.lookup.say = true, true, why
		return
	}
	a.vocab.lookup.busy = true
	a.vocab.gen++
	gen := a.vocab.gen
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	a.vocab.cancel = cancel
	update, now := a.update, a.saved.now
	system, user := lookup.Prompt(language), lookup.Request(word, original, a.vocab.lookup.film)
	go func() {
		defer cancel()
		reply, err := t.Chat(ctx, system, user)
		var entry lookup.Entry
		if err == nil {
			entry, err = lookup.Parse(word, reply)
		}
		update(func() {
			l := &a.vocab.lookup
			if gen != a.vocab.gen || !l.open {
				return
			}
			a.vocab.cancel = nil
			l.busy = false
			if err != nil {
				l.failed, l.needsAI = true, !errors.Is(err, lookup.ErrNoMeaning)
				msg := err.Error()
				l.say = func(cx *core.Scope) string { return locales.VocabLookupFailed(cx, msg) }
				return
			}
			l.entry = entry
			cache := a.vocabCache()
			cache.Put(word, language, entry, now())
			_ = cache.Save()
		})
	}()
}

// vocabStopLookup stops the lookup asking the AI service, whose answer is then dropped.
func (a *app) vocabStopLookup() {
	a.vocab.gen++
	if a.vocab.cancel != nil {
		a.vocab.cancel()
		a.vocab.cancel = nil
	}
}

// CloseLookup closes the card of the word looked up, as its × and Escape do.
func (a *app) CloseLookup() {
	a.vocabStopLookup()
	a.vocab.lookup = vocabLookup{}
}

// ToggleLookupWord is the card's Add to Vocabulary: the word looked up goes into the open library's vocabulary with what it means and the line it was clicked in, or, once it is there, out of it; the picture says which.
func (a *app) ToggleLookupWord() {
	l := a.vocab.lookup
	if !l.open || l.busy || l.failed {
		return
	}
	a.savedSync()
	book := &a.saved.book
	text := cmp.Or(l.entry.Word, l.word)
	if book.RemoveWord(text) {
		a.flash(a.messages().VocabOsdRemoved(text))
		a.savedWrite()
		return
	}
	book.AddWord(saved.Word{Text: text, Reading: l.entry.Reading, Meaning: l.entry.Meaning, Language: l.language,
		Context: l.line, ContextTranslation: l.translation, Video: book.Key(l.video), Start: l.start, Film: l.film, SavedAt: a.saved.now()})
	a.flash(a.messages().VocabOsdAdded(text))
	a.savedWrite()
}

// ExplainLookupLine is the card's Explain Line: the card closes and the command palette explains the line the word was clicked in (ExplainLine).
func (a *app) ExplainLookupLine() {
	i := a.vocab.lookup.cue
	a.CloseLookup()
	a.ExplainLine(i)
}

// LookupSettings is the card's Open Settings, when no AI service can answer: the card closes and the settings open at 「AI 服务」.
func (a *app) LookupSettings() {
	a.CloseLookup()
	a.OpenSettings(settings.SectionAI)
}
