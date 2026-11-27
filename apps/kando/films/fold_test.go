package films

import (
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sidebar"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// withShow is a's library with a show's three episodes added first, in a folder of the show's, done.
func withShow(a *fakeApp) []Film {
	var eps []Film
	for _, n := range []string{"01", "02", "03"} {
		path := "/Videos/River Walk/River Walk EP" + n + ".mp4"
		eps = append(eps, Film{Key: path, Title: "River Walk EP" + n, Name: "River Walk EP" + n, Source: core.FileURL(path), Stage: core.StageComplete, VideoPath: path, SubtitlePath: path + ".srt"})
	}
	a.items = append(slices.Clone(eps), a.items...)
	return eps
}

// workTestID is the test ID of the card of the work a film keyed key is in.
func workTestID(a *fakeApp, key string) string {
	w, _ := a.Works().Of(key)
	return "id:work-" + w.Key
}

// A show's episodes fold into one card that says how many it holds and how many are not watched, while the heading still counts films; a click opens the work's page, which lists them in their order and goes on with the first, and Escape goes back.
func TestAShowsEpisodesFoldIntoOneCard(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	app := start(t, a)
	msgs := testlocale.Messages()
	card := workTestID(a, eps[0].Key)
	if !app.Exists(card) || app.Exists("id:film-"+eps[0].Key) {
		t.Fatalf("the episodes are not one card: %q", app.Texts())
	}
	if !app.Exists(whole("River Walk")) || !app.Exists(whole(msgs.SourceLocal()+" · "+msgs.FilmsWorkEpisodes(3))) || !app.Exists(card+" >> "+whole("3")) {
		t.Fatalf("the work's card: %q", app.Texts())
	}
	if !app.Exists(whole(msgs.FilmsGridCount(5))) {
		t.Fatalf("the heading does not count the films: %q", app.Texts())
	}
	app.Press(card)
	w, _ := a.Works().Of(eps[0].Key)
	wantCalls(t, a, "show-work "+w.Key)
	app.Settle()
	if !app.Exists("id:work-name") || !app.Exists(whole(msgs.FilmsWorkWatched(3, 0))) || !app.Exists(whole(msgs.FilmsWorkStart())) {
		t.Fatalf("the work's page: %q", app.Texts())
	}
	for _, ep := range eps {
		if !app.Exists("id:film-" + ep.Key) {
			t.Fatalf("the work's page has no %s: %q", ep.Key, app.Texts())
		}
	}
	if app.Exists("id:film-done") || app.Exists("id:films-hero") {
		t.Fatalf("the work's page shows the library: %q", app.Texts())
	}
	app.Press("id:work-continue")
	wantCalls(t, a, "open "+eps[0].Key)
	app.Key("escape")
	wantCalls(t, a, "show-work ")
	app.Settle()
	if !app.Exists(card) {
		t.Fatalf("Escape did not go back: %q", app.Texts())
	}
}

// A work's card holds the episodes the filters leave, its mark counting those not watched; one left is a film's card. A work split apart shows a card an episode.
func TestAWorksCardHoldsTheEpisodesShown(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	a.durations[eps[0].VideoPath], a.furthest[eps[0].VideoPath] = time.Minute, time.Minute
	a.played[eps[0].VideoPath] = time.Now()
	app := start(t, a)
	card := workTestID(a, eps[0].Key)
	if !app.Exists(card + " >> " + whole("2")) {
		t.Fatalf("the card does not count 2 not watched: %q", app.Texts())
	}
	change(app, a, func() { a.watch = sidebar.WatchUnwatched })
	if !app.Exists(card) || app.Exists("id:film-"+eps[0].Key) {
		t.Fatalf("under 未看 the work is not a card of its 2 not watched: %q", app.Texts())
	}
	change(app, a, func() { a.query = "EP03" })
	if app.Exists(card) || !app.Exists("id:film-"+eps[2].Key) {
		t.Fatalf("a search that leaves one episode does not show it alone: %q", app.Texts())
	}
	// A search for the show's name finds its episodes.
	change(app, a, func() { a.watch, a.query = sidebar.WatchAll, "river walk" })
	if !app.Exists(card) {
		t.Fatalf("a search for the show does not find it: %q", app.Texts())
	}
	change(app, a, func() {
		a.query = ""
		w, _ := a.Works().Of(eps[0].Key)
		a.SetWorkApart(w, true)
	})
	if app.Exists(card) || !app.Exists("id:film-"+eps[0].Key) || !app.Exists("id:film-"+eps[2].Key) {
		t.Fatalf("a work split apart is still one card: %q", app.Texts())
	}
}

// A ⌘-click on a work's card selects its episodes, a second leaves them out, and ⌘A selects every film the page shows, a work's among them.
func TestAWorksCardSelectsItsEpisodes(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	app := start(t, a)
	card := workTestID(a, eps[0].Key)
	app.PressWith(card, ns.Modifiers{Platform: true})
	if !slices.Equal(a.selection.Keys, Keys(eps)) {
		t.Fatalf("the ⌘-click selected %q", a.selection.Keys)
	}
	app.PressWith(card, ns.Modifiers{Platform: true})
	if a.selection.Len() != 0 {
		t.Fatalf("a second ⌘-click left %q", a.selection.Keys)
	}
	app.Key("mod+a")
	if a.selection.Len() != 5 {
		t.Fatalf("⌘A selected %q", a.selection.Keys)
	}
}

// A work goes on with the episode watched last when it was stopped partway, else the next one, else its first; once every one was watched, from its first again.
func TestContinue(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	w, _ := a.Works().Of(eps[0].Key)
	for _, ep := range eps {
		a.durations[ep.VideoPath] = time.Minute
	}
	if f, how := Continue(a, w, a.items); f.Key != eps[0].Key || how != GoOnStart {
		t.Fatalf("a work never played goes on with %s, %d", f.Key, how)
	}
	a.played[eps[1].VideoPath], a.furthest[eps[1].VideoPath] = time.Now(), 20*time.Second
	if f, how := Continue(a, w, a.items); f.Key != eps[1].Key || how != GoOnResume {
		t.Fatalf("a work stopped partway goes on with %s, %d", f.Key, how)
	}
	a.furthest[eps[1].VideoPath] = time.Minute
	if f, how := Continue(a, w, a.items); f.Key != eps[2].Key || how != GoOnNext {
		t.Fatalf("a work whose second episode was watched goes on with %s, %d", f.Key, how)
	}
	for _, ep := range eps {
		a.furthest[ep.VideoPath] = time.Minute
	}
	if f, how := Continue(a, w, a.items); f.Key != eps[0].Key || how != GoOnAgain {
		t.Fatalf("a work watched whole goes on with %s, %d", f.Key, how)
	}
}

// Episodes marked watched keep no time they played (MarkWatched), so a work none of whose episodes played goes on with the first not watched, and starts again once every one was marked.
func TestContinueAfterMarkedWatched(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	w, _ := a.Works().Of(eps[0].Key)
	for _, ep := range eps {
		a.durations[ep.VideoPath] = time.Minute
	}
	a.furthest[eps[0].VideoPath], a.furthest[eps[1].VideoPath] = time.Minute, time.Minute
	if f, how := Continue(a, w, a.items); f.Key != eps[2].Key || how != GoOnNext {
		t.Fatalf("a work whose first two episodes were marked watched goes on with %s, %d", f.Key, how)
	}
	a.furthest[eps[2].VideoPath] = time.Minute
	if f, how := Continue(a, w, a.items); f.Key != eps[0].Key || how != GoOnAgain {
		t.Fatalf("a work marked watched whole goes on with %s, %d", f.Key, how)
	}
}

// In the list a work is a row of its name and how many episodes it holds, selected while they all are, which Return opens the page of.
func TestAWorkIsARowOfTheList(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	a.view = ViewList
	app := start(t, a)
	msgs := testlocale.Messages()
	row := "id:films-list >> " + whole(msgs.FilmsWorkLabel("River Walk", 3))
	if !app.Exists(row) || app.Exists("id:films-list >> "+whole(eps[0].Name)) {
		t.Fatalf("the list has no row for the work: %q", app.Texts())
	}
	app.PressWith(row, ns.Modifiers{Platform: true})
	if !slices.Equal(a.selection.Keys, Keys(eps)) {
		t.Fatalf("selecting the work's row selected %q", a.selection.Keys)
	}
	a.calls = nil
	app.Key("enter")
	w, _ := a.Works().Of(eps[0].Key)
	wantCalls(t, a, "show-work "+w.Key)
}

// Once the episode played last was watched to its end, the hero offers the next one of its work, as the Apple TV app does; the newest film never played comes after that.
func TestHeroOffersAWorksNextEpisode(t *testing.T) {
	a := newFakeApp()
	eps := withShow(a)
	for _, ep := range eps {
		a.durations[ep.VideoPath] = time.Minute
	}
	a.played[eps[0].VideoPath], a.furthest[eps[0].VideoPath] = time.Now(), time.Minute
	if hero, ok := HeroFilm(a, a.items); !ok || hero.Key != eps[1].Key {
		t.Fatalf("the hero is %q, not the next episode", hero.Key)
	}
	// Not one stopped partway, which is continued itself.
	a.furthest[eps[0].VideoPath], a.resume[eps[0].VideoPath] = 20*time.Second, 20*time.Second
	if hero, _ := HeroFilm(a, a.items); hero.Key != eps[0].Key {
		t.Fatalf("the hero is %q, not the episode stopped partway", hero.Key)
	}
}
