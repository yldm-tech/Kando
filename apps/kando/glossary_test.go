package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kando/apps/kando/glossary"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/subscribe"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// glossaryOpen is "talk" open with its 术语 tab showing, its job in the series "geo" named 地理课 with another film, the glossary kept in glossary.json beside the settings, and the path of the film's own terms file.
func glossaryOpen(t *testing.T) (*app, *nsgotest.App, string) {
	t.Helper()
	a, tt, updates := playerOpen(t)
	a.glossaryLoad()
	a.gloss.store.Name("geo", "地理课")
	a.library.Jobs[0].Series = "geo"
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "other", Source: "https://www.youtube.com/watch?v=ooooooooooo", Stage: core.StageComplete, Series: "geo", LibraryDir: a.library.Jobs[0].LibraryDir})
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead })
	tt.Press("id:film-tab-terms")
	return a, tt, media.TermsPath(a.player.film.video)
}

// A term added in the 术语 tab is the film's own, kept beside its video; its pill moves it to the series and to every film, kept in glossary.json, and back to the film; locking it is kept too, and the film's job translates with it.
func TestTermsTabKeepsEachScopeWhereItBelongs(t *testing.T) {
	a, tt, own := glossaryOpen(t)
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.GlossarySeriesLine("地理课", 2))
	tt.Type("id:film-term-source", "Hello")
	tt.Type("id:film-term-target", "哈喽")
	tt.Press("id:film-term-add")
	want := []core.Term{{Source: "Hello", Target: "哈喽", Scope: core.ScopeFilm, Origin: core.OriginUser}}
	if terms, err := media.LoadTerms(own); err != nil || !reflect.DeepEqual(terms, want) {
		t.Fatalf("the film's terms file holds %+v, %v", terms, err)
	}
	wantText(t, tt, "Hello → 哈喽")

	tt.Press("id:film-term-scope")
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("a series term stayed in the film's file: %v", err)
	}
	saved, err := glossary.Load(a.gloss.store.Path())
	if series, _ := saved.Terms("geo"); err != nil || len(series) != 1 || series[0].Target != "哈喽" {
		t.Fatalf("glossary.json holds %+v, %v", saved, err)
	}
	wantText(t, tt, msgs.GlossaryScopeSeries())

	tt.Press("id:film-term-scope")
	saved, _ = glossary.Load(a.gloss.store.Path())
	if series, global := saved.Terms("geo"); len(series) != 0 || len(global) != 1 {
		t.Fatalf("moved to every film, glossary.json holds %+v", saved)
	}

	tt.Press("id:film-term-scope")
	tt.Press("id:film-term-lock")
	want[0].Locked = true
	if terms, _ := media.LoadTerms(own); !reflect.DeepEqual(terms, want) {
		t.Fatalf("back on the film, its file holds %+v", terms)
	}
	saved, _ = glossary.Load(a.gloss.store.Path())
	if series, global := saved.Terms("geo"); len(series) != 0 || len(global) != 0 {
		t.Fatalf("back on the film, glossary.json holds %+v", saved)
	}
	wantText(t, tt, msgs.GlossaryMetaLocked(msgs.GlossaryScopeFilm()))

	// A term for the same words typed again changes its translation where it is, keeping its lock.
	tt.Type("id:film-term-source", "hello")
	tt.Type("id:film-term-target", "你好")
	tt.Press("id:film-term-add")
	if terms := a.FilmTerms(); len(terms) != 1 || terms[0].Target != "你好" || !terms[0].Locked || terms[0].Scope != core.ScopeFilm {
		t.Fatalf("terms %+v", terms)
	}

	a.gloss.store.Set("geo", core.Term{Source: "Bye", Target: "再见", Scope: core.ScopeSeries})
	a.gloss.store.Set("geo", core.Term{Source: "river", Target: "河", Scope: core.ScopeGlobal})
	_, terms := a.glossaryStart(a.library.Jobs[0])
	if len(terms) != 3 || terms[0].Source != "hello" || terms[1].Source != "Bye" || terms[2].Source != "river" {
		t.Fatalf("the film's job translates with %+v", terms)
	}

	tt.Press("id:film-term-remove")
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("the removed term's file stayed: %v", err)
	}
}

// glossaryBlock keeps glossary.json from being written, a directory standing where its temporary file goes, until the returned func takes it away.
func glossaryBlock(t *testing.T, a *app) func() {
	t.Helper()
	tmp := a.gloss.store.Path() + ".tmp"
	if err := os.MkdirAll(filepath.Join(tmp, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.RemoveAll(tmp); err != nil {
			t.Fatal(err)
		}
	}
}

// A term moved out of the film's own while glossary.json cannot be written stays in the film's file, and a series term moved on from there stays in the series, in memory as on disk.
func TestAMovedTermIsKeptWhenTheGlossaryCannotBeSaved(t *testing.T) {
	a, tt, own := glossaryOpen(t)
	msgs := testlocale.Messages()
	tt.Type("id:film-term-source", "Hello")
	tt.Type("id:film-term-target", "哈喽")
	tt.Press("id:film-term-add")
	want := []core.Term{{Source: "Hello", Target: "哈喽", Scope: core.ScopeFilm, Origin: core.OriginUser}}

	unblock := glossaryBlock(t, a)
	tt.Press("id:film-term-scope")
	if terms, err := media.LoadTerms(own); err != nil || !reflect.DeepEqual(terms, want) {
		t.Fatalf("after a failed move to the series the film's file holds %+v, %v", terms, err)
	}
	if !reflect.DeepEqual(a.gloss.film, want) {
		t.Fatalf("after a failed move to the series the film holds %+v", a.gloss.film)
	}
	if series, global := a.gloss.store.Terms("geo"); len(series) != 0 || len(global) != 0 {
		t.Fatalf("after a failed move the store holds series %+v, global %+v", series, global)
	}
	if a.player.film.osd != msgs.GlossarySaveFailed() {
		t.Fatalf("the picture says %q", a.player.film.osd)
	}

	unblock()
	tt.Press("id:film-term-scope")
	if series, _ := a.gloss.store.Terms("geo"); len(series) != 1 {
		t.Fatalf("the move to the series was not kept: %+v", series)
	}
	glossaryBlock(t, a)
	tt.Press("id:film-term-scope")
	if series, global := a.gloss.store.Terms("geo"); len(series) != 1 || len(global) != 0 {
		t.Fatalf("after a failed move to every film the store holds series %+v, global %+v", series, global)
	}
	if terms := a.FilmTerms(); len(terms) != 1 || terms[0].Scope != core.ScopeSeries {
		t.Fatalf("after a failed move to every film the film lists %+v", terms)
	}
	saved, err := glossary.Load(a.gloss.store.Path())
	if series, global := saved.Terms("geo"); err != nil || len(series) != 1 || len(global) != 0 {
		t.Fatalf("glossary.json holds %+v, %v", saved, err)
	}
}

// While glossary.json cannot be read, a term of the film's own is not moved out of its file, to the series or to every film, since glossary.json is not written over.
func TestAFilmTermStaysWhileTheGlossaryCannotBeRead(t *testing.T) {
	a, tt, own := glossaryOpen(t)
	msgs := testlocale.Messages()
	tt.Type("id:film-term-source", "Hello")
	tt.Type("id:film-term-target", "哈喽")
	tt.Press("id:film-term-add")
	want := []core.Term{{Source: "Hello", Target: "哈喽", Scope: core.ScopeFilm, Origin: core.OriginUser}}
	path := a.gloss.store.Path()
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, a.glossaryLoad)
	if a.gloss.storeErr == nil {
		t.Fatal("a glossary.json that is not JSON read")
	}

	// The job still names the series glossary.json held, which the store no longer has.
	tt.Press("id:film-term-scope")
	if terms, _ := media.LoadTerms(own); !reflect.DeepEqual(terms, want) {
		t.Fatalf("moved to the series, the film's file holds %+v", terms)
	}
	if a.player.film.osd != msgs.GlossaryUnreadable() {
		t.Fatalf("the picture says %q", a.player.film.osd)
	}
	tt.Press("id:film-terms-leave-series")
	tt.Press("id:film-term-scope")
	if terms, _ := media.LoadTerms(own); !reflect.DeepEqual(terms, want) {
		t.Fatalf("moved to every film, the film's file holds %+v", terms)
	}
	if data, _ := os.ReadFile(path); string(data) != "{" {
		t.Fatalf("glossary.json was written over: %s", data)
	}
	if terms := a.FilmTerms(); !reflect.DeepEqual(terms, want) {
		t.Fatalf("the film lists %+v", terms)
	}
}

// A film whose terms file a newer Kando wrote keeps its glossary as it was: a term moved from every film to the film's own stays with every film.
func TestATermStaysWhenTheFilmsFileIsNewer(t *testing.T) {
	a, tt, own := glossaryOpen(t)
	msgs := testlocale.Messages()
	apply(tt, a, func() {
		a.gloss.store.Set("geo", core.Term{Source: "delta", Target: "三角洲", Scope: core.ScopeGlobal})
	})
	newer := []byte(`{"version": 2, "terms": [{"source": "a", "target": "b", "scope": "film"}]}`)
	if err := os.WriteFile(own, newer, 0o600); err != nil {
		t.Fatal(err)
	}
	tt.Press("id:film-term-scope")
	if data, _ := os.ReadFile(own); string(data) != string(newer) {
		t.Fatalf("the newer file was written over: %s", data)
	}
	if _, global := a.gloss.store.Terms("geo"); len(global) != 1 || global[0].Source != "delta" {
		t.Fatalf("the global term was taken out: %+v", global)
	}
	if a.player.film.osd != msgs.GlossaryNewer() {
		t.Fatalf("the picture says %q", a.player.film.osd)
	}
}

// 不是系列 takes the open film out of its series, which library.json keeps: the series' terms no longer apply to it, and they stay with the series.
func TestLeavingASeries(t *testing.T) {
	a, tt, _ := glossaryOpen(t)
	a.gloss.store.Set("geo", core.Term{Source: "Bye", Target: "再见", Scope: core.ScopeSeries})
	refresh(tt, a)
	wantText(t, tt, "Bye → 再见")
	tt.Press("id:film-terms-leave-series")
	if job, _ := a.library.Find("talk"); job.Series != "" {
		t.Fatalf("job %+v", job)
	}
	if tt.Exists("id:film-terms-series") || hasText(tt, "Bye → 再见") {
		t.Fatalf("the series shows: %q", tt.Texts())
	}
	if series, _ := a.gloss.store.Terms("geo"); len(series) != 1 {
		t.Fatalf("the series lost its terms: %+v", series)
	}
}

// A subscription that is a series puts the videos it queues in its series, named after it, as their jobs start; no longer a series, it takes the films it added out of it, and back again.
func TestSubscriptionSeries(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.glossaryLoad()
	sub := subscribeTo(t, tt, a, p, updates)
	tt.Press("id:subscription-series")
	if !sub.Series {
		t.Fatal("the switch did not make the subscription a series")
	}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubLong, testSubOld}}
	checkNow(t, tt, a, p, updates)
	id := glossarySeriesOf(*sub)
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Series != id || a.queue.active != a.library.Jobs[0].ID {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	if series, ok := a.gloss.store.Find(id); !ok || series.Name != "Earth Science" {
		t.Fatalf("series %+v %v", series, ok)
	}
	if saved, _ := glossary.Load(a.gloss.store.Path()); len(saved.Series) != 1 {
		t.Fatalf("glossary.json holds %+v", saved)
	}
	// What the running job reports does not take its series away.
	apply(tt, a, func() {
		job := a.library.Jobs[0]
		job.Series, job.Stage = "", core.StageDownloading
		a.show(job)
	})
	if a.library.Jobs[0].Series != id {
		t.Fatalf("a report dropped the series: %+v", a.library.Jobs[0])
	}
	tt.Press("id:subscription-series")
	if a.library.Jobs[0].Series != "" {
		t.Fatalf("no longer a series, the job is in %q", a.library.Jobs[0].Series)
	}
	tt.Press("id:subscription-series")
	if a.library.Jobs[0].Series != id {
		t.Fatalf("a series again, the job is in %q", a.library.Jobs[0].Series)
	}
	if subscribe.DefaultRules().Series {
		t.Fatal("a new subscription is a series")
	}
}

// The films of a playlist added from the dialog are a series named after it, those it queued and the one the library had; a channel's are not.
func TestAPlaylistsFilmsAreASeries(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing, testChannel: {Title: "Veritasium", Videos: []media.ListedVideo{
		{URL: "https://www.youtube.com/watch?v=ddddddddddd"}, {URL: "https://www.youtube.com/watch?v=eeeeeeeeeee"},
	}}})
	a, tt, updates := newTestApp(t, p)
	a.library.Jobs = []core.Job{{ID: "have", Source: "https://youtu.be/bbbbbbbbbbb", Stage: core.StageComplete, VideoPath: "/videos/Episode 2/video.mp4", LibraryDir: "/videos"}}
	refresh(tt, a)
	startJob(t, tt, testPlaylist)
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
	tt.Press("id:playlist-add")
	if len(a.library.Jobs) != 3 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	id := a.library.Jobs[0].Series
	for _, job := range a.library.Jobs {
		if job.Series == "" || job.Series != id {
			t.Fatalf("%s is in series %q, want %q", job.Source, job.Series, id)
		}
	}
	if series, ok := a.gloss.store.Find(id); !ok || series.Name != "French in Action" {
		t.Fatalf("the series is %+v", series)
	}

	startJob(t, tt, testChannel)
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
	tt.Press("id:playlist-add")
	for _, job := range a.library.Jobs {
		if strings.Contains(job.Source, "ddddddddddd") && job.Series != "" {
			t.Fatalf("a channel's film is in series %q", job.Series)
		}
	}
}

// glossaryEpisodes is "talk" named as the first episode of 地理课, with the second episode and another film in its library, open with its 术语 tab showing.
func glossaryEpisodes(t *testing.T) (*app, *nsgotest.App) {
	t.Helper()
	a, tt, updates := playerOpen(t)
	a.glossaryLoad()
	dir := a.library.Jobs[0].LibraryDir
	a.library.Jobs[0].Title = "地理课 第1集"
	a.library.Jobs = append(a.library.Jobs,
		core.Job{ID: "ep2", Title: "地理课 第2集", Source: "https://www.youtube.com/watch?v=ooooooooooo", Stage: core.StageComplete, LibraryDir: dir},
		core.Job{ID: "else", Title: "Tokyo Story", Source: "https://www.youtube.com/watch?v=ppppppppppp", Stage: core.StageComplete, LibraryDir: dir})
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead })
	tt.Press("id:film-tab-terms")
	return a, tt
}

// Films whose names say they are episodes of one show are offered as a series, which Same Series makes them, named after the show and kept in library.json.
func TestFilmsNamedAsEpisodesBecomeASeriesOnceConfirmed(t *testing.T) {
	a, tt := glossaryEpisodes(t)
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.GlossaryGuessTitle())
	wantText(t, tt, "地理课 第1集 · 地理课 第2集")
	tt.Press("id:film-terms-guess-yes")
	talk, _ := a.library.Find("talk")
	ep2, _ := a.library.Find("ep2")
	other, _ := a.library.Find("else")
	if talk.Series == "" || ep2.Series != talk.Series || other.Series != "" {
		t.Fatalf("series: talk %q, ep2 %q, else %q", talk.Series, ep2.Series, other.Series)
	}
	if name, n := a.FilmSeries(); name != "地理课" || n != 2 {
		t.Fatalf("the film's series is %q of %d", name, n)
	}
	if tt.Exists("id:film-terms-guess") {
		t.Fatal("the question stayed once answered")
	}
	wantText(t, tt, msgs.GlossarySeriesLine("地理课", 2))

	// Leaving it does not bring the question back.
	tt.Press("id:film-terms-leave-series")
	if show, names := a.SeriesGuess(); show != "" || len(names) != 0 {
		t.Fatalf("the show was offered again after the film left it: %q %q", show, names)
	}
}

// Not a Series keeps that the show is not one, in glossary.json, and it is not asked again.
func TestFilmsNamedAsEpisodesAreNotAskedAgainOnceTurnedDown(t *testing.T) {
	a, tt := glossaryEpisodes(t)
	tt.Press("id:film-terms-guess-no")
	if tt.Exists("id:film-terms-guess") {
		t.Fatal("the question stayed once turned down")
	}
	saved, err := glossary.Load(a.gloss.store.Path())
	if err != nil || len(saved.NotSeries) != 1 {
		t.Fatalf("glossary.json holds %+v, %v", saved, err)
	}
	for _, job := range a.library.Jobs {
		if job.Series != "" {
			t.Fatalf("%s joined series %q", job.ID, job.Series)
		}
	}
}
