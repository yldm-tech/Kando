package main

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
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

// vocabLookup is the word looked up on the picture (film.Lookup): the word as clicked, the line it was clicked in, cue, its index in the open film's lines, and the film's video, title and the language it is spoken in, so the word goes into the vocabulary with them; busy while a dictionary or the AI service is asked, then its entry and the id of the dictionary pack it came from (source, "" for the AI service), with the ids of the dictionaries asked first that did not have the word when the AI service answered (missed), or why there is none (say), with needsAI set when the AI service's settings are what helps; dictHint while the card shows, once, the hint to install an offline dictionary (pluginsDictHint).
type vocabLookup struct {
	open              bool
	word              string
	cue               int
	line, translation string
	video, film       string
	spoken            string
	start, end        time.Duration
	busy, failed      bool
	needsAI           bool
	language          string
	entry             lookup.Entry
	source            string
	missed            []string
	dictHint          bool
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

// vocabHooks: the time the open film plays adds up while it plays and goes into the study record of its library a minute at a time, once it is held or closed, before another library opens and as the app quits; the stats dialog, open as another library opens, reads the other libraries again; the word looked up closes once its film is not the one open.
func (a *app) vocabHooks() areaHooks {
	return areaHooks{
		playing:        a.vocabPlaying,
		leavingLibrary: func() { a.vocabFlushWatch() },
		libraryChanged: func() {
			if a.vocab.stats.open {
				a.vocabReadOthers()
			}
		},
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

// vocabFlushWatch puts the time played since it was last told into the open library's study record, on today's date, and reports whether there was any. A study record that could not be read is not written (study.ErrUnread), so the time is not kept, and the picture was told so once, when it was read (savedSync). The learner did nothing to cause this write, so a record that cannot be written says nothing on the picture either: the time stays in the book in memory and goes into the file with its next write.
func (a *app) vocabFlushWatch() bool {
	d := a.vocab.watch.pending
	if d <= 0 {
		return false
	}
	a.vocab.watch.pending = 0
	a.savedSync()
	if a.lockShut() || a.saved.book.Dir() == "" || a.saved.book.Unread() != nil {
		return false
	}
	a.saved.book.Watch(a.saved.now(), d)
	_ = a.saved.book.Save()
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
	out := film.Lookup{Open: true, Word: l.word, Line: l.line, Busy: l.busy, Failed: l.failed, NeedsAI: l.needsAI, Say: l.say, Term: vocabTerm(a.FilmTerms(), l.word, l.entry.Word), DictHint: l.dictHint}
	if tier, rank, ok := a.pluginsWordLevel(lookup.WordLanguage(lookup.Query{Spoken: l.spoken, Word: l.word, Line: l.line}), l.entry.Word, l.word); ok {
		label, id := tier.Label, tier.ID
		out.Level = func(cx *core.Scope) string { return cmp.Or(label.In(i18n.Language(cx)), id) }
		out.Tone, out.Rank = tier.Tone, rank
	}
	if !l.busy && !l.failed {
		out.Word = cmp.Or(l.entry.Word, l.word)
		out.Reading, out.Meaning = l.entry.Reading, l.entry.Meaning
		out.Source = a.vocabSourceText(l.source, l.missed)
		book := a.savedBook()
		out.Saved = book.WordIndex(out.Word, l.spoken) >= 0
	}
	return out
}

// vocabTerm is the term of terms, the open film's glossary, for the word looked up, as clicked or as its dictionary form gives it, "source → target", "" for none: a term whose words are that word, in any case, or one with that word among its words, as "river" of "Mekong River"; the first of these the glossary lists.
func vocabTerm(terms []kcore.Term, words ...string) string {
	var keys []string
	for _, w := range words {
		if k := strings.ToLower(strings.TrimSpace(w)); k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	pair := func(t kcore.Term) string { return t.Source + " → " + t.Target }
	for _, t := range terms {
		if slices.Contains(keys, strings.ToLower(strings.TrimSpace(t.Source))) {
			return pair(t)
		}
	}
	for _, t := range terms {
		for _, part := range lookup.Tokens(t.Source, nil) {
			if part.Word && slices.Contains(keys, strings.ToLower(part.Text)) {
				return pair(t)
			}
		}
	}
	return ""
}

// LookUpWord is a click on word in line i of the open film's lines, on the picture: the film holds, and the card over it shows what word means there (vocabResolve): from the words looked up before on this computer, or else from the dictionary packs turned on, or else as the AI service set in 「AI 服务」 says; the dictionaries and the AI service are asked off the engine's goroutine. When none answers and no AI service can, as while offline mode is on and it is not on this computer, the card says why and offers its settings.
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
	q := vocabQuery(item, a.prefs.language, word, original)
	title := cmp.Or(item.Name, item.Title)
	a.vocab.lookup = vocabLookup{open: true, word: word, cue: i, line: original, translation: translation, video: a.player.film.video,
		film: title, spoken: q.Spoken, start: cues[i].Start, end: cues[i].End, language: q.Language}
	ask := a.vocabResolve(q, title)
	a.vocab.lookup.dictHint = a.pluginsDictHint(q)
	if ask.cached {
		a.vocab.lookup.entry = ask.entry
		return
	}
	if !ask.answers() {
		a.vocab.lookup.failed, a.vocab.lookup.needsAI, a.vocab.lookup.say = true, true, ask.why
		return
	}
	a.vocab.lookup.busy = true
	a.vocab.gen++
	gen := a.vocab.gen
	// The dictionaries whose lookup failed, as one with a damaged index, are not among those that did not have the word (missed); Answer reports them on the goroutine that asks before it returns.
	var broken []string
	reportFailed := ask.ask.Failed
	ask.ask.Failed = func(d lookup.Dictionary, err error) {
		broken = append(broken, d.ID())
		if reportFailed != nil {
			reportFailed(d, err)
		}
	}
	// answered is the entry and the id of the dictionary pack it came from, "" for the AI service (vocabAsk.answer), with the dictionaries whose lookup failed.
	type answered struct {
		entry  lookup.Entry
		source string
		broken []string
	}
	answer := func(ctx context.Context) (answered, error) {
		entry, source, err := ask.answer(ctx)
		return answered{entry, source, broken}, err
	}
	now := a.saved.now
	a.vocab.cancel = aiAsk(a, lookupTimeout, answer, func() bool { return gen == a.vocab.gen && a.vocab.lookup.open }, func(got answered, err error) {
		l := &a.vocab.lookup
		a.vocab.cancel = nil
		l.busy = false
		switch {
		case errors.Is(err, lookup.ErrNoAnswer):
			l.failed, l.needsAI, l.say = true, true, ask.why
			return
		case err != nil:
			l.failed, l.needsAI = true, !errors.Is(err, lookup.ErrNoMeaning)
			msg := err.Error()
			l.say = func(cx *core.Scope) string { return locales.VocabLookupFailed(cx, msg) }
			return
		}
		l.entry, l.source = got.entry, got.source
		if got.source == "" {
			// The dictionaries asked before the AI service that did not have the word (lookup.Ask.Answer), not those whose lookup failed; the card says so.
			for _, d := range ask.ask.Dicts {
				// Only the dictionary packs are named as not having the word; a plugin's lookup source is no dictionary.
				if _, _, plugin := a.pluginsLookupNames(d.ID()); !plugin && !slices.Contains(got.broken, d.ID()) {
					l.missed = append(l.missed, d.ID())
				}
			}
			cache := a.vocabCache()
			cache.Put(q, got.entry, now())
			_ = cache.Save()
		}
	})
}

// vocabAsk is a lookup as vocabResolve found it on the engine's goroutine: the entry the cache keeps for it (cached), or what answers it off that goroutine (ask, lookup.Ask.Answer), and, when no AI service can answer, why (the dictionaries may still answer).
type vocabAsk struct {
	cached bool
	entry  lookup.Entry
	ask    lookup.Ask
	why    func(*core.Scope) string
}

// vocabResolve is how the word looked up as q, in a line of the film titled film, is answered, for the picture (LookUpWord) and paired phones (phoneStudy.LookUp) alike: from the words looked up before on this computer, else from the dictionary packs turned on, in the order Settings › 插件 keeps them, else from the lookup sources of the script plugins running, else by the AI service. It reads the cache, the dictionaries open and the AI service set, so it runs on the engine's goroutine; lookup.Ask.Answer asks them off it.
func (a *app) vocabResolve(q lookup.Query, film string) vocabAsk {
	ask := vocabAsk{ask: lookup.Ask{Query: q, Film: film}}
	if e, ok := a.vocabCache().Get(q); ok {
		ask.cached, ask.entry = true, e
		return ask
	}
	ask.ask.Dicts, ask.ask.Others = a.pluginsDictsFor(lookup.WordLanguage(q), kcore.TargetTag(q.Language))
	// The lookup sources of the script plugins running come after the dictionary packs and before the AI service.
	ask.ask.Dicts = append(ask.ask.Dicts, a.pluginsLookups(q)...)
	update := a.update
	ask.ask.Failed = func(d lookup.Dictionary, err error) { update(func() { a.pluginsDictFailed(d, err) }) }
	// With dictionaries for the word, no AI service means only that the words they lack find nothing.
	noAI := locales.VocabLookupNoAI
	if len(ask.ask.Dicts) > 0 || len(ask.ask.Others) > 0 {
		noAI = locales.VocabLookupNotInDicts
	}
	var t media.ChatTranslator
	t, ask.why = a.paletteTranslator(noAI)
	if ask.why == nil {
		ask.ask.AI = t
	}
	return ask
}

// answers reports whether anything can answer the lookup: the cache, a dictionary, or the AI service.
func (k vocabAsk) answers() bool { return k.cached || k.ask.Answers() }

// answer is the entry of the word looked up and the id of the dictionary pack it came from, "" when the AI service gave it, now or when it was looked up before (the cache keeps the AI service's answers alone); lookup.ErrNoAnswer when nothing answered and no AI service can (why says why). It runs off the engine's goroutine.
func (k vocabAsk) answer(ctx context.Context) (lookup.Entry, string, error) {
	if k.cached {
		return k.entry, "", nil
	}
	return k.ask.Answer(ctx)
}

// vocabSourceText says where an entry came from, as the lookup card shows it under the meaning: the dictionary pack of the id dict, by its name, with the credit its license asks for, a lookup source of a script plugin, by its name and its plugin's, or the AI service, saying so when the dictionaries missed, asked first, did not have the word (docs/plugin-system.md 7.5): by its name when one was asked, as "your dictionaries" when several were; in the language shown under the card's scope.
func (a *app) vocabSourceText(dict string, missed []string) func(*core.Scope) string {
	if dict == "" {
		switch len(missed) {
		case 0:
			return locales.VocabLookupSourceAI
		case 1:
			var name plugins.Text
			if i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == missed[0] }); i >= 0 {
				name = a.plugins.packs[i].Name
			}
			return func(cx *core.Scope) string {
				return locales.VocabLookupSourceAINotInDict(cx, cmp.Or(name.In(i18n.Language(cx)), missed[0]))
			}
		}
		return locales.VocabLookupSourceAINotInDicts
	}
	if plugin, source, ok := a.pluginsLookupNames(dict); ok {
		return func(cx *core.Scope) string {
			lang := i18n.Language(cx)
			return locales.VocabLookupSourcePlugin(cx, plugin.In(lang), source.In(lang))
		}
	}
	var name, credit plugins.Text
	if i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == dict }); i >= 0 {
		p := a.plugins.packs[i]
		name = p.Name
		if p.Dictionary != nil {
			credit = p.Dictionary.Attribution
		}
	}
	return func(cx *core.Scope) string {
		lang := i18n.Language(cx)
		n := cmp.Or(name.In(lang), dict)
		if c := credit.In(lang); c != "" {
			return locales.VocabLookupSourceDictCredit(cx, n, c)
		}
		return locales.VocabLookupSourceDict(cx, n)
	}
}

// vocabQuery is what word, clicked in line of the film item, is looked up with: the language the film is spoken in, as the user named it or else as the speech recognition heard it, and the language its meaning is written in (paletteAnswerLanguage, fallback the settings' target language).
func vocabQuery(item films.Film, fallback, word, line string) lookup.Query {
	return lookup.Query{Word: word, Spoken: cmp.Or(item.SpeechLanguage, item.Spoken), Language: paletteAnswerLanguage(item, fallback), Line: line}
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

// ToggleLookupWord is the card's Add to Vocabulary: the word looked up goes into the open library's vocabulary with what it means and the line it was clicked in, whose sound is cut to be kept with it (savedKeepSound), or, once it is there, out of it with that sound; the picture says which.
func (a *app) ToggleLookupWord() {
	l := a.vocab.lookup
	if !l.open || l.busy || l.failed {
		return
	}
	a.savedSync()
	book := &a.saved.book
	text := cmp.Or(l.entry.Word, l.word)
	if book.RemoveWord(text, l.spoken) {
		a.flash(a.messages().VocabOsdRemoved(text))
		a.savedWrite()
		a.savedDropSound(book.RemovedWords[len(book.RemovedWords)-1].ID)
		return
	}
	book.AddWord(saved.Word{Text: text, Spoken: l.spoken, Reading: l.entry.Reading, Meaning: l.entry.Meaning, Language: l.language,
		Context: l.line, ContextTranslation: l.translation, Video: book.Key(l.video), Start: l.start, End: l.end, Film: l.film, SavedAt: a.saved.now()})
	a.flash(a.messages().VocabOsdAdded(text))
	a.savedWrite()
	a.savedKeepSound(saved.WordLine(book.Words[0]))
}

// ExplainLookupLine is the card's Explain Line: the card closes and the side panel explains the line the word was clicked in (ExplainLine).
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
