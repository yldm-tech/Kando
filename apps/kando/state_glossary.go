package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/glossary"
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
}

func newGlossaryUI() glossaryUI { return glossaryUI{} }

// glossaryHooks reads glossary.json at startup and follows the film open, whose own terms it reads.
func (a *app) glossaryHooks() areaHooks {
	return areaHooks{
		restore:     func(savedSettings) { a.glossaryLoad() },
		afterChange: a.glossaryFollow,
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
func (a *app) glossarySave() bool {
	if a.gloss.storeErr != nil {
		a.flash(a.messages().GlossaryUnreadable())
		return false
	}
	if err := a.gloss.store.Save(); err != nil {
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
	if a.glossaryPut(t) {
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
	a.glossaryPut(t)
}

// ToggleTermLock locks the open film's term for source, so the translation must write it as it is and no machine pass changes it, or unlocks it.
func (a *app) ToggleTermLock(source string) {
	if t, ok := a.glossaryTerm(source); ok {
		t.Locked = !t.Locked
		a.glossaryPut(t)
	}
}

// RemoveTerm takes the open film's term for source out of the glossary it is in.
func (a *app) RemoveTerm(source string) {
	t, ok := a.glossaryTerm(source)
	if !ok {
		return
	}
	t.Target = ""
	a.glossaryPut(t)
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

// glossaryPut puts t in the glossary of its scope and takes the same words out of the others the open film has, so the film has one entry for them; a t without a target is taken out of all of them. It saves what changed and reports whether it could.
func (a *app) glossaryPut(t core.Term) bool {
	job, _ := a.glossaryJob()
	film := core.RemoveTerm(a.gloss.film, t.Source)
	if t.Valid() && t.Scope == core.ScopeFilm {
		film = core.SetTerm(film, t)
	}
	if !slices.Equal(film, a.gloss.film) && !a.glossarySetFilm(film) {
		return false
	}
	store := a.gloss.store
	var changed bool
	if t.Valid() {
		changed = a.gloss.store.Set(job.Series, t)
	} else {
		changed = a.gloss.store.Remove(job.Series, t.Source)
	}
	if changed && !a.glossarySave() {
		a.gloss.store = store
		return false
	}
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
