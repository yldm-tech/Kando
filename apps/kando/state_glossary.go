package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/glossary"
	"kando/apps/kando/playlist"
	"kando/apps/kando/subscribe"
	"kando/packages/core"
	"kando/packages/media"
)

// The glossary: how the user wants names and terms translated (core.Term), for the open film alone, for the films of its series or for every film. A film's own terms are kept beside its video (media.TermsPath), the series' and the global ones in glossary.json beside settings.json (package glossary). A job translates with its film's glossary (glossaryOfJob), the terms its lines hold sent with each batch (media.WithGlossary). The player's 术语 tab lists the open film's and changes them (film.Glossary). A film is in a series when its job names one (core.Job.Series): the videos a subscription that is a series adds are (subscribe.Rules.Series).

// glossaryUI is the glossary's state. store is what glossary.json holds; storeErr says why it could not be read, which keeps it from being written over. video is the open film's video, whose own terms film holds once read (filmRead), filmGen telling the read of the film open from one of a film opened before it. source and target are the fields a new term is typed in.
type glossaryUI struct {
	store    glossary.Store
	storeErr error
	video    string
	film     []core.Term
	filmRead bool
	filmGen  int
	source   string
	target   string
	// episodes is the show each film's name says it is an episode of, by name (glossaryEpisodeOf).
	episodes map[string]glossaryEpisode
	// suggest is the terms suggested for the open film's glossary (state_glossary_suggest.go).
	suggest glossarySuggest
}

func newGlossaryUI() glossaryUI { return glossaryUI{} }

// glossaryHooks reads glossary.json at startup and follows the film open, whose own terms and the terms suggested for it it reads.
func (a *app) glossaryHooks() areaHooks {
	return areaHooks{
		restore: func(savedSettings) { a.glossaryLoad() },
		afterChange: func() bool {
			dropped := a.glossaryFollow()
			return a.glossarySuggestFollow() || dropped
		},
	}
}

// glossaryLoad reads glossary.json beside settings.json. Without a settings file, as in tests, the glossary is kept in memory alone. A file that cannot be read is left as it is, and nothing is written over it (glossarySave).
func (a *app) glossaryLoad() {
	if a.settingsPath == "" {
		return
	}
	a.gloss.store, a.gloss.storeErr = glossary.Load(filepath.Join(filepath.Dir(a.settingsPath), "glossary.json"))
}

// glossarySave writes glossary.json, and says on the picture when it could not; a file that could not be read at startup is not written over.
func (a *app) glossarySave() bool { return a.glossaryWrite(a.gloss.store) }

// glossaryWrite writes store to glossary.json, and says on the picture when it could not; a file that could not be read at startup is not written over.
func (a *app) glossaryWrite(store glossary.Store) bool {
	if a.gloss.storeErr != nil {
		a.flash(a.messages().GlossaryUnreadable())
		return false
	}
	if err := store.Save(); err != nil {
		a.flash(a.messages().GlossarySaveFailed())
		return false
	}
	return true
}

// glossaryFollow reads the own terms of the film open once another one is, off the engine's goroutine, and reports whether it dropped the ones it held.
func (a *app) glossaryFollow() bool {
	video := ""
	if a.openKey != "" {
		video = a.player.film.video
	}
	g := &a.gloss
	if video == g.video {
		return false
	}
	g.video, g.film, g.filmRead, g.source, g.target = video, nil, false, "", ""
	g.filmGen++
	if video == "" {
		return true
	}
	gen, update := g.filmGen, a.update
	go func() {
		terms, err := media.LoadTerms(media.TermsPath(video))
		update(func() {
			if a.gloss.filmGen != gen {
				return
			}
			// A file that cannot be read keeps the film's terms from being written over it (glossarySetFilm).
			a.gloss.film, a.gloss.filmRead = terms, err == nil
		})
	}()
	return true
}

// glossaryStart is job as it starts to run and the glossary it translates with: its film's own terms, read from beside its video, its series' and the global ones (core.MergeTerms). A job that has not downloaded its video yet, of a link a subscription that is a series found (subscribe.Rules.Series), joins that series first, which library.json keeps.
func (a *app) glossaryStart(job core.Job) (core.Job, []core.Term) {
	if job.Series == "" && job.VideoPath == "" {
		if sub, ok := a.glossarySubscriptionOf(job); ok {
			job.Series = glossarySeriesOf(sub)
			if a.gloss.store.Name(job.Series, sub.Name) {
				a.glossarySave()
			}
			if _, kept := a.library.Find(job.ID); kept {
				a.library.Upsert(job)
				if err := a.library.Save(); err != nil {
					a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
				}
			}
		}
	}
	var own []core.Term
	if job.VideoPath != "" {
		own, _ = media.LoadTerms(media.TermsPath(job.VideoPath))
	}
	series, global := a.gloss.store.Terms(job.Series)
	return job, core.MergeTerms(own, series, global)
}

// glossarySubscriptionOf is the subscription that is a series whose checks found the link of job, in job's library.
func (a *app) glossarySubscriptionOf(job core.Job) (subscribe.Sub, bool) {
	key := queueSourceKey(job.Source)
	for _, sub := range a.subs.store.Subs {
		if !sub.Series || !films.InLibrary(sub.Library, job) {
			continue
		}
		if slices.ContainsFunc(sub.Seen, func(link string) bool { return queueSourceKey(link) == key }) {
			return sub, true
		}
	}
	return subscribe.Sub{}, false
}

// glossaryJob is the job of the film open, found by its video.
func (a *app) glossaryJob() (core.Job, bool) {
	video := a.gloss.video
	if video == "" {
		return core.Job{}, false
	}
	for _, job := range a.library.Jobs {
		if job.VideoPath != "" && filepath.Clean(job.VideoPath) == filepath.Clean(video) {
			return job, true
		}
	}
	return core.Job{}, false
}

// FilmTerms is the open film's glossary as its translation uses it: its own terms, its series' and the global ones, a narrower scope's taking the place of a wider one's for the same words.
func (a *app) FilmTerms() []core.Term {
	job, _ := a.glossaryJob()
	series, global := a.gloss.store.Terms(job.Series)
	return core.MergeTerms(a.gloss.film, series, global)
}

// TermsWritable reports whether the open film's own terms can be changed: it has a video to keep them beside, and they were read.
func (a *app) TermsWritable() bool { return a.gloss.video != "" && a.gloss.filmRead }

// FilmSeries is the name of the open film's series and how many films of the library are in it; "" and 0 for a film of no series.
func (a *app) FilmSeries() (string, int) {
	job, ok := a.glossaryJob()
	if !ok || job.Series == "" {
		return "", 0
	}
	n := 0
	for _, other := range a.library.Jobs {
		if other.Series == job.Series {
			n++
		}
	}
	name := ""
	if series, ok := a.gloss.store.Find(job.Series); ok {
		name = series.Name
	}
	return name, n
}

// LeaveSeries takes the open film out of its series: its job names none from then on, and it is translated with its own terms and the global ones. The series' terms stay with the series.
func (a *app) LeaveSeries() {
	job, ok := a.glossaryJob()
	if !ok || job.Series == "" {
		return
	}
	// A series the app guessed from the films' names is not offered again for this film's show once the film leaves it.
	if strings.HasPrefix(job.Series, "name-") {
		if e := a.glossaryEpisodeOf(job); e.ok {
			store := a.gloss.store.Clone()
			if store.SetNotSeries(glossary.NotSeriesKey(a.snap().LibraryDir, e.key)) && a.glossaryWrite(store) {
				a.gloss.store = store
			}
		}
	}
	job.Series = ""
	a.library.Upsert(job)
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
}

// NewTermSource and NewTermTarget are the fields a new term is typed in; AddTerm adds what they hold as one of the open film's own terms, or changes the term for the same words to that translation in the scope it has, and empties them.
func (a *app) NewTermSource() *string { return &a.gloss.source }
func (a *app) NewTermTarget() *string { return &a.gloss.target }

func (a *app) AddTerm() {
	t := core.Term{Source: strings.TrimSpace(a.gloss.source), Target: strings.TrimSpace(a.gloss.target), Scope: core.ScopeFilm, Origin: core.OriginUser}
	if !t.Valid() || !a.TermsWritable() {
		return
	}
	if i := slices.IndexFunc(a.FilmTerms(), func(o core.Term) bool { return core.SameTerm(o.Source, t.Source) }); i >= 0 {
		old := a.FilmTerms()[i]
		t.Scope, t.Locked = old.Scope, old.Locked
	}
	if a.glossaryChange(correctKindTermAdd, t) {
		a.gloss.source, a.gloss.target = "", ""
	}
}

// CycleTermScope moves the open film's term for source to the next scope: the film's own, its series' when it is in one, every film's, and back to the film's.
func (a *app) CycleTermScope(source string) {
	t, ok := a.glossaryTerm(source)
	if !ok {
		return
	}
	job, _ := a.glossaryJob()
	switch {
	case t.Scope == core.ScopeFilm && job.Series != "":
		t.Scope = core.ScopeSeries
	case t.Scope == core.ScopeGlobal:
		t.Scope = core.ScopeFilm
	default:
		t.Scope = core.ScopeGlobal
	}
	a.glossaryChange(correctKindTermScope, t)
}

// ToggleTermLock locks the open film's term for source, so the translation must write it as it is and no machine pass changes it, or unlocks it.
func (a *app) ToggleTermLock(source string) {
	if t, ok := a.glossaryTerm(source); ok {
		t.Locked = !t.Locked
		kind := correctKindTermUnlock
		if t.Locked {
			kind = correctKindTermLock
		}
		a.glossaryChange(kind, t)
	}
}

// RemoveTerm takes the open film's term for source out of the glossary it is in.
func (a *app) RemoveTerm(source string) {
	t, ok := a.glossaryTerm(source)
	if !ok {
		return
	}
	t.Target = ""
	a.glossaryChange(correctKindTermRemove, t)
}

// glossaryTerm is the open film's term for source, as FilmTerms lists it.
func (a *app) glossaryTerm(source string) (core.Term, bool) {
	terms := a.FilmTerms()
	i := slices.IndexFunc(terms, func(t core.Term) bool { return core.SameTerm(t.Source, source) })
	if i < 0 {
		return core.Term{}, false
	}
	return terms[i], true
}

// glossaryPut puts t in the glossary of its scope and takes the same words out of the others the open film has, so the film has one entry for them; a t without a target is taken out of all of them. It saves what changed and reports whether it could; when glossary.json cannot take the change, the film's terms file is written back as it was, so a term moved out of it is not lost.
func (a *app) glossaryPut(t core.Term) bool {
	job, _ := a.glossaryJob()
	film := core.RemoveTerm(a.gloss.film, t.Source)
	if t.Valid() && t.Scope == core.ScopeFilm {
		film = core.SetTerm(film, t)
	}
	// The change is made on a copy, which takes the place of the store once glossary.json holds it: a series' terms are shared with the store otherwise (glossary.Store.Clone).
	store := a.gloss.store.Clone()
	var changed bool
	if t.Valid() {
		changed = store.Set(job.Series, t)
	} else {
		changed = store.Remove(job.Series, t.Source)
	}
	return a.glossaryCommit(film, store, changed)
}

// glossaryHeld is every entry the open film's glossary holds for one source: the film's own, its series' and every film's, each nil when there is none. Undo and Redo put a change of the glossary back by it, as a change takes the source out of every scope (glossaryPut) and the term the film shows is only the narrowest of them.
type glossaryHeld struct {
	film, series, global *core.Term
}

// glossaryHeldOf is what the open film's glossary holds for source now.
func (a *app) glossaryHeldOf(source string) glossaryHeld {
	job, _ := a.glossaryJob()
	series, global := a.gloss.store.Terms(job.Series)
	find := func(terms []core.Term) *core.Term {
		if i := slices.IndexFunc(terms, func(t core.Term) bool { return core.SameTerm(t.Source, source) }); i >= 0 {
			t := terms[i]
			return &t
		}
		return nil
	}
	return glossaryHeld{film: find(a.gloss.film), series: find(series), global: find(global)}
}

// glossaryRestore makes the open film's glossary hold for source what held does, in each scope, and saves it as glossaryPut does, reporting whether it could.
func (a *app) glossaryRestore(source string, held glossaryHeld) bool {
	job, _ := a.glossaryJob()
	film := core.RemoveTerm(a.gloss.film, source)
	if held.film != nil {
		film = core.SetTerm(film, *held.film)
	}
	store := a.gloss.store.Clone()
	changed := store.Remove(job.Series, source)
	if held.global != nil {
		store.Global = core.SetTerm(store.Global, *held.global)
		changed = true
	}
	if held.series != nil {
		if _, ok := store.Find(job.Series); !ok && job.Series != "" {
			store.Series = append(store.Series, glossary.Series{ID: job.Series})
		}
		if series, ok := store.Find(job.Series); ok {
			series.Terms = core.SetTerm(series.Terms, *held.series)
			changed = true
		}
	}
	return a.glossaryCommit(film, store, changed)
}

// glossaryCommit saves film as the open film's own terms and store as glossary.json when changed, and keeps them; when glossary.json cannot take the change, the film's terms file is written back as it was, so a term moved out of it is not lost. It reports whether it could.
func (a *app) glossaryCommit(film []core.Term, store glossary.Store, changed bool) bool {
	old := a.gloss.film
	// A glossary.json that cannot be read is not written over, so the film's file is not changed either: a term moved out of it would be in neither.
	if changed && a.gloss.storeErr != nil {
		a.flash(a.messages().GlossaryUnreadable())
		return false
	}
	filmChanged := !slices.Equal(film, old)
	if filmChanged && !a.glossarySetFilm(film) {
		return false
	}
	if changed && !a.glossaryWrite(store) {
		// The film's file goes back to what it held; it says on the picture only if that could not be written either.
		if filmChanged {
			a.glossarySetFilm(old)
		}
		return false
	}
	a.gloss.store = store
	return true
}

// glossarySetFilm writes terms as the open film's own and keeps them, and says on the picture when they could not be written.
func (a *app) glossarySetFilm(terms []core.Term) bool {
	if !a.TermsWritable() {
		return false
	}
	if err := media.SaveTerms(media.TermsPath(a.gloss.video), terms); err != nil {
		if errors.Is(err, media.ErrTermsNewer) {
			a.flash(a.messages().GlossaryNewer())
		} else {
			a.flash(a.messages().GlossarySaveFailed())
		}
		return false
	}
	a.gloss.film = terms
	return true
}

// glossarySeriesOf is the ID of the series a subscription's videos are in while it is one (subscribe.Rules.Series).
func glossarySeriesOf(sub subscribe.Sub) string { return "sub-" + sub.ID }

// glossarySubscriptionSeries follows sub's rules after they changed: becoming a series puts the films it added in its series, and no longer being one takes them out of it.
func (a *app) glossarySubscriptionSeries(sub subscribe.Sub) {
	links := make([]string, len(sub.Added))
	for i, v := range sub.Added {
		links[i] = v.URL
	}
	a.glossarySeries(sub, links, sub.Series)
}

// glossarySeries puts the films of sub's library whose jobs came from links in sub's series, or with join false takes those in it out, and saves what changed.
func (a *app) glossarySeries(sub subscribe.Sub, links []string, join bool) {
	id := glossarySeriesOf(sub)
	keys := make([]string, len(links))
	for i, link := range links {
		keys[i] = queueSourceKey(link)
	}
	changed := false
	for _, job := range a.library.Jobs {
		if !films.InLibrary(sub.Library, job) || !slices.Contains(keys, queueSourceKey(job.Source)) {
			continue
		}
		switch {
		case join && job.Series == "":
			job.Series = id
		case !join && job.Series == id:
			job.Series = ""
		default:
			continue
		}
		a.library.Upsert(job)
		changed = true
	}
	if join && a.gloss.store.Name(id, sub.Name) {
		a.glossarySave()
	}
	if changed {
		if err := a.library.Save(); err != nil {
			a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
		}
	}
}

// Series guessed from names: films of a library whose names say they are episodes of one show (core.EpisodeOf), none of them in a series yet, are offered as one in the open film's 术语 tab, which asks, as a name can mislead (film.Glossary's SeriesGuess). Confirmed, they join a series named after the show; turned down, the show is kept as not a series (glossary.Store.NotSeries) and not offered again.

// glossaryEpisode is what core.EpisodeOf read from a name, which glossaryUI.episodes keeps by name so the guess reads each name once.
type glossaryEpisode struct {
	show, key string
	ok        bool
}

// glossaryEpisodeOf is the show the film of job is an episode of, as its name says (core.EpisodeOf): its site's title, or its video's file name.
func (a *app) glossaryEpisodeOf(job core.Job) glossaryEpisode {
	name := strings.TrimSpace(job.Title)
	if name == "" && job.VideoPath != "" {
		name = strings.TrimSuffix(filepath.Base(job.VideoPath), filepath.Ext(job.VideoPath))
	}
	if name == "" {
		return glossaryEpisode{}
	}
	if e, ok := a.gloss.episodes[name]; ok {
		return e
	}
	var e glossaryEpisode
	e.show, e.key, e.ok = core.EpisodeOf(name)
	if a.gloss.episodes == nil {
		a.gloss.episodes = map[string]glossaryEpisode{}
	}
	a.gloss.episodes[name] = e
	return e
}

// glossaryGuess is the show the open film looks like an episode of and the jobs of its library that look like episodes of it too, the open film's first, none of them in a series; none when the open film is in one, its name marks no episode, no other film shares the show, or the user said the show is not a series.
func (a *app) glossaryGuess() (glossaryEpisode, []core.Job) {
	job, ok := a.glossaryJob()
	if !ok || job.Series != "" {
		return glossaryEpisode{}, nil
	}
	root := a.snap().LibraryDir
	e := a.glossaryEpisodeOf(job)
	if !e.ok || a.gloss.store.IsNotSeries(glossary.NotSeriesKey(root, e.key)) {
		return glossaryEpisode{}, nil
	}
	jobs := []core.Job{job}
	for _, other := range a.library.Jobs {
		if other.ID == job.ID || other.Series != "" || other.Stage == core.StageFailed || !films.InLibrary(root, other) {
			continue
		}
		if o := a.glossaryEpisodeOf(other); o.ok && o.key == e.key {
			jobs = append(jobs, other)
		}
	}
	if len(jobs) < 2 {
		return glossaryEpisode{}, nil
	}
	return e, jobs
}

// SeriesGuess is the films the open film looks like it is a series with, their names in the library's order, the open film's first, and the show they look like episodes of; none when there is nothing to ask (glossaryGuess).
func (a *app) SeriesGuess() (show string, names []string) {
	e, jobs := a.glossaryGuess()
	for _, job := range jobs {
		name := strings.TrimSpace(job.Title)
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(job.VideoPath), filepath.Ext(job.VideoPath))
		}
		names = append(names, name)
	}
	return e.show, names
}

// ConfirmSeries puts the films SeriesGuess offers in a series named after their show, which their translations share terms through from then on, and says so on the picture.
func (a *app) ConfirmSeries() {
	e, jobs := a.glossaryGuess()
	if len(jobs) == 0 {
		return
	}
	sum := sha256.Sum256([]byte(glossary.NotSeriesKey(a.snap().LibraryDir, e.key)))
	id := "name-" + hex.EncodeToString(sum[:6])
	store := a.gloss.store.Clone()
	if store.Name(id, e.show) && !a.glossaryWrite(store) {
		return
	}
	a.gloss.store = store
	for _, job := range jobs {
		job.Series = id
		a.library.Upsert(job)
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
	a.flash(a.messages().GlossarySeriesConfirmed(len(jobs)))
}

// DismissSeries keeps that the show SeriesGuess offers is not a series, so it is not offered again.
func (a *app) DismissSeries() {
	e, jobs := a.glossaryGuess()
	if len(jobs) == 0 {
		return
	}
	store := a.gloss.store.Clone()
	if store.SetNotSeries(glossary.NotSeriesKey(a.snap().LibraryDir, e.key)) && a.glossaryWrite(store) {
		a.gloss.store = store
	}
}

// glossaryPlaylistSeries puts the films of the open library whose links are videos of one of lists, a playlist with a title and more than one video, in a series named after the playlist, as its site groups them; films in a series already stay in theirs. Channels are not series.
func (a *app) glossaryPlaylistSeries(lists []playlist.List) {
	root := a.snap().LibraryDir
	changed := false
	store := a.gloss.store.Clone()
	named := false
	for _, list := range lists {
		if list.Channel || strings.TrimSpace(list.Title) == "" || len(list.Videos) < 2 {
			continue
		}
		sum := sha256.Sum256([]byte(queueSourceKey(list.Source)))
		id := "list-" + hex.EncodeToString(sum[:6])
		keys := map[string]bool{}
		for _, v := range list.Videos {
			keys[queueSourceKey(v.URL)] = true
		}
		joined := 0
		for _, job := range a.library.Jobs {
			if job.Series != "" || !films.InLibrary(root, job) || !keys[queueSourceKey(job.Source)] {
				continue
			}
			job.Series = id
			a.library.Upsert(job)
			changed = true
			joined++
		}
		if joined > 0 && store.Name(id, list.Title) {
			named = true
		}
	}
	if named && a.glossaryWrite(store) {
		a.gloss.store = store
	}
	if changed {
		if err := a.library.Save(); err != nil {
			a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
		}
	}
}
