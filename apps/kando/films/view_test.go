package films

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/category"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeApp is the shelf the page reads, kept as plain fields as the app keeps them, with the calls the page made.
type fakeApp struct {
	live.Rev
	dir, page, filter, query, source string
	spoken                           string
	watch                            string
	making                           bool
	name                             string
	place                            string
	pathErr                          error
	saveErr                          string
	items, queue                     []Film
	durations                        map[string]time.Duration
	resume                           map[string]time.Duration
	played                           map[string]time.Time
	furthest                         map[string]time.Duration
	remedies                         map[string]Remedy
	handoff                          Handoff
	hints                            map[string]string
	details                          map[string]string
	notice                           Notice
	expanded, linkFocus              bool
	sort                             Order
	view                             string
	language                         string
	removeOpen, removeFolder         bool
	removing                         []Film
	selection                        Selection
	posterFocus                      bool
	renameOpen                       bool
	renameName, renameOriginal       string
	calls                            []string
	retryable, failed                int
	// untranslatable is whether no AI service is set up (CanTranslate).
	untranslatable bool
}

func newFakeApp() *fakeApp {
	lib := "/films/我的影片.kando"
	return &fakeApp{
		dir: lib, page: sidebar.PageFilms, filter: sidebar.SourceAll, spoken: sidebar.SpokenAll, watch: sidebar.WatchAll, sort: Order{By: SortAdded}, view: ViewGrid, place: "/films", language: "简体中文",
		name: "新资料库",
		items: []Film{
			{Key: "done", JobID: "done", Title: "Rivers of Europe", Name: "Rivers of Europe", Source: "https://youtu.be/rivers", Stage: core.StageComplete,
				VideoPath: lib + "/Rivers/video.mp4", SubtitlePath: lib + "/Rivers/video.bilingual.srt", Language: "简体中文"},
			{Key: "algebra", JobID: "algebra", Title: "Linear Algebra", Name: "Linear Algebra", Source: "https://youtu.be/algebra", Stage: core.StageComplete,
				VideoPath: lib + "/Algebra/video.mp4", SubtitlePath: lib + "/Algebra/video.bilingual.srt", Untranslated: 3},
			{Key: "running", JobID: "running", Title: "Morning Market", Name: "Morning Market", Source: "https://youtu.be/market", Stage: core.StageDownloading},
		},
		durations: map[string]time.Duration{lib + "/Rivers/video.mp4": 125 * time.Second},
		resume:    map[string]time.Duration{},
		played:    map[string]time.Time{},
		furthest:  map[string]time.Duration{},
		remedies:  map[string]Remedy{},
		hints:     map[string]string{},
	}
}

func (f *fakeApp) call(format string, args ...string) {
	f.calls = append(f.calls, strings.Join(append([]string{format}, args...), " "))
}

func (f *fakeApp) LibraryDir() string     { return f.dir }
func (f *fakeApp) LibraryGone() bool      { return false }
func (f *fakeApp) MakingLibrary() bool    { return f.making }
func (f *fakeApp) SaveErr() string        { return f.saveErr }
func (f *fakeApp) Page() string           { return f.page }
func (f *fakeApp) Films() []Film          { return f.items }
func (f *fakeApp) Source() *string        { return &f.source }
func (f *fakeApp) PickLibrary()           { f.call("pick-library") }
func (f *fakeApp) OpenSites()             { f.call("sites") }
func (f *fakeApp) Select(page string)     { f.call("select " + page) }
func (f *fakeApp) BeginNew()              { f.call("new"); f.making = true }
func (f *fakeApp) CancelMaking()          { f.call("cancel-making"); f.making = false }
func (f *fakeApp) LibraryPlace() string   { return f.place }
func (f *fakeApp) PickLibraryParent()     { f.call("pick-parent") }
func (f *fakeApp) CreateLibrary()         { f.call("create", f.name); f.making = false }
func (f *fakeApp) NewLibraryName() string { return f.name }
func (f *fakeApp) BindName(edit func(*string) bool) error {
	edit(&f.name)
	return nil
}
func (f *fakeApp) NewLibraryPath() (string, error) {
	if f.pathErr != nil {
		return "", f.pathErr
	}
	return filepath.Join(f.place, f.name+".kando"), nil
}
func (f *fakeApp) Start()                             { f.call("start", f.source) }
func (f *fakeApp) AddList(source string)              { f.call("add-list", source) }
func (f *fakeApp) PickVideos()                        { f.call("pick-videos") }
func (f *fakeApp) PickVideosShortcut() string         { return "mod+o" }
func (f *fakeApp) DropFiles(paths []string)           { f.call("drop", paths...) }
func (f *fakeApp) Poster(string) ns.ImageSource       { return ns.ImageSource{} }
func (f *fakeApp) Thumbnail(Film) ns.ImageSource      { return ns.ImageSource{} }
func (f *fakeApp) Duration(path string) time.Duration { return f.durations[path] }
func (f *fakeApp) Quote(item Film) (string, string) {
	if item.Key == "done" {
		return "河流塑造了大地", "Rivers shape the land"
	}
	return "", ""
}
func (f *fakeApp) Open(item Film)                        { f.call("open", item.Key) }
func (f *fakeApp) OpenAt(item Film, at time.Duration)    { f.call("open-at", item.Key, at.String()) }
func (f *fakeApp) Reveal(path string)                    { f.call("reveal", path) }
func (f *fakeApp) Reprocess(item Film)                   { f.call("reprocess", item.Key) }
func (f *fakeApp) AskReprocess(items ...Film)            { f.call("ask-reprocess", filmKeys(items)) }
func (f *fakeApp) Reprocessing() (*bool, []Film, string) { return new(bool), nil, "" }
func (f *fakeApp) ReprocessFilms(items ...Film)          { f.call("reprocess-films", filmKeys(items)) }
func (f *fakeApp) CloseReprocess()                       { f.call("close-reprocess") }
func (f *fakeApp) TranslateAgain(item Film, language string) {
	f.call("translate-again", item.Key, language)
}
func (f *fakeApp) TargetLanguage() string                 { return "日本語" }
func (f *fakeApp) Retranslating() (*bool, []Film, string) { return new(bool), nil, "" }
func (f *fakeApp) TranslateFilmsAgain(items []Film, language string) {
	f.call("translate-films", filmKeys(items), language)
}
func (f *fakeApp) CloseRetranslate()              { f.call("close-retranslate") }
func (f *fakeApp) Retranslate()                   { f.call("retranslate") }
func (f *fakeApp) TranslateMissing(item Film)     { f.call("translate-missing", item.Key) }
func (f *fakeApp) CanTranslate() bool             { return !f.untranslatable }
func (f *fakeApp) Remove(id string)               { f.call("remove", id) }
func (f *fakeApp) Retry(id string)                { f.call("retry", id) }
func (f *fakeApp) Failures(string) (int, int)     { return f.retryable, f.failed }
func (f *fakeApp) FailureDetail(item Film) string { return f.details[item.JobID] }
func (f *fakeApp) ReportFailure(id string)        { f.call("report", id) }
func (f *fakeApp) RetryAll(string)                { f.call("retry-all") }
func (f *fakeApp) RemoveFailed(string)            { f.call("remove-failed") }
func (f *fakeApp) Withdraw(id string)             { f.call("withdraw", id) }
func (f *fakeApp) FailureHint(item Film) (string, string) {
	return f.hints[item.JobID], ""
}
func (f *fakeApp) TranslateFilms(items []Film) {
	keys := []string{"translate-films"}
	for _, item := range NotTranslated(items) {
		keys = append(keys, item.Key)
	}
	f.call(strings.Join(keys, " "))
}
func (f *fakeApp) TranslationHint(item Film) (string, string) {
	if item.TranslationErr == "" {
		return "", ""
	}
	return "没有 AI 服务", "ai"
}
func (f *fakeApp) FailureRemedy(item Film) (Remedy, bool) {
	r, ok := f.remedies[item.JobID]
	return r, ok
}
func (f *fakeApp) RunRemedy(item Film, action string) { f.call("remedy", item.Key, action) }
func (f *fakeApp) LinkFocus() *bool                   { return &f.linkFocus }
func (f *fakeApp) Query() *string                     { return &f.query }
func (f *fakeApp) JobFilm(id string) (Film, bool)     { return Find(f.items, id) }
func (f *fakeApp) SourceFilter() string               { return f.filter }
func (f *fakeApp) SpokenFilter() string               { return f.spoken }
func (f *fakeApp) WatchFilter() string                { return f.watch }
func (f *fakeApp) SetWatchFilter(state string)        { f.call("watch", state); f.watch = state }
func (f *fakeApp) MarkWatched(items ...Film) {
	for _, item := range items {
		f.call("mark-watched", item.Key)
	}
}
func (f *fakeApp) MarkUnwatched(items ...Film) {
	for _, item := range items {
		f.call("mark-unwatched", item.Key)
	}
}
func (f *fakeApp) Enqueue(...string) error { return nil }
func (f *fakeApp) Cancel(id string)        { f.call("cancel", id) }
func (f *fakeApp) ContinueHere(id string)  { f.call("continue-here", id) }
func (f *fakeApp) Handoff() Handoff        { return f.handoff }
func (f *fakeApp) CloseHandoff()           { f.call("close-handoff"); f.handoff = Handoff{} }
func (f *fakeApp) LeftOn(path string) string {
	if f.handoff.Film.VideoPath == path {
		return f.handoff.Device
	}
	return ""
}
func (f *fakeApp) RunningJob() string { return "running" }
func (f *fakeApp) QueueFilms() []Film {
	var out []Film
	for _, item := range f.items {
		if InQueue(item) {
			out = append(out, item)
		}
	}
	return append(out, f.queue...)
}
func (f *fakeApp) OpenSettings(section string)        { f.call("settings", section) }
func (f *fakeApp) ResumeAt(path string) time.Duration { return f.resume[path] }
func (f *fakeApp) PlayedAt(path string) time.Time     { return f.played[path] }
func (f *fakeApp) Furthest(path string) time.Duration { return f.furthest[path] }
func (f *fakeApp) Notice() *Notice                    { return &f.notice }
func (f *fakeApp) QueueExpanded() *bool               { return &f.expanded }
func (f *fakeApp) LibrarySort() Order                 { return f.sort }
func (f *fakeApp) LibraryView() string                { return f.view }
func (f *fakeApp) SetLibraryView(view string)         { f.call("view", view); f.view = view }
func (f *fakeApp) Language() *string                  { return &f.language }
func (f *fakeApp) SetLibrarySort(o Order) {
	direction := "ascending"
	if !o.Ascending() {
		direction = "descending"
	}
	f.call("sort", o.By, direction)
	f.sort = o
}
func (f *fakeApp) AskRemove(items ...Film) {
	f.call("ask-remove", filmKeys(items))
	f.removing, f.removeOpen = items, true
}
func (f *fakeApp) Removing() (*bool, []Film, bool) { return &f.removeOpen, f.removing, f.removeFolder }
func (f *fakeApp) CloseRemove()                    { f.call("close-remove"); f.removeOpen = false }
func (f *fakeApp) TrashFilms(items ...Film)        { f.call("trash", filmKeys(items)); f.removeOpen = false }
func (f *fakeApp) AskRename(item Film) {
	f.call("ask-rename", item.Key)
	f.renameOpen, f.renameName, f.renameOriginal = true, item.Name, "original"
}
func (f *fakeApp) Renaming() (*bool, *string, string) {
	return &f.renameOpen, &f.renameName, f.renameOriginal
}
func (f *fakeApp) CloseRename()                                         { f.call("close-rename"); f.renameOpen = false }
func (f *fakeApp) RenameFilm()                                          { f.call("rename", f.renameName); f.renameOpen = false }
func (f *fakeApp) MoveFilmMenu(*ns.Scope, ...Film) []nsdesktop.MenuItem { return nil }
func (f *fakeApp) CategoryShown() string                                { return category.All }
func (f *fakeApp) CategoryName(string) string                           { return "" }
func (f *fakeApp) InCategories(items []Film) []Film                     { return items }
func (f *fakeApp) SetCategoryFilter(string)                             {}
func (f *fakeApp) CategoryMenu(*ns.Scope, ...Film) []nsdesktop.MenuItem { return nil }

func (f *fakeApp) StoredMenu(*ns.Scope, ...Film) []nsdesktop.MenuItem { return nil }

func (f *fakeApp) StoredMark(locales.Messages, Film) (string, bool, bool) { return "", false, false }

func (f *fakeApp) StoredPlace(Film) string { return "" }
func (f *fakeApp) Selection() *Selection   { return &f.selection }
func (f *fakeApp) PosterFocus() *bool      { return &f.posterFocus }
func (f *fakeApp) UndoMove()               { f.call("undo-move") }
func (f *fakeApp) CancelMove()             { f.call("cancel-move") }
func (f *fakeApp) Choose(dir string)       { f.call("choose", dir) }

// testEnvironment is a Mac desktop in testlocale's language.
func testEnvironment() ns.Environment {
	return testlocale.Environment(ns.Environment{OS: ns.OSMacOS, Idiom: ns.IdiomDesktop, AppleKeyboard: true})
}

// start shows the library page of a as the main window's content column does, on a Mac, in testlocale's language. It is as high as the window, as the window's row of sidebar and page holds it, however short the window is.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		a.Attach(cx)
		return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{View{App: a}}}
	}, nsgotest.Size(1068, 782), nsgotest.Environment(testEnvironment()))
}

// change runs fn on the app's state as a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func whole(s string) string { return `text:"` + s + `"` }

func wantCalls(t *testing.T, a *fakeApp, want ...string) {
	t.Helper()
	if !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
	a.calls = nil
}

func TestShelfShowsTheHeroTheQueueAndTheGrid(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, text := range []string{msgs.FilmsQueueTitle(), msgs.SourceAll(), "Rivers of Europe", "Linear Algebra", "Morning Market",
		msgs.FilmsHeroKickerBilingual(msgs.SettingsLanguageSimplifiedChinese()), msgs.FilmsGridCount(2)} {
		if !app.Exists(whole(text)) {
			t.Errorf("no %q in %q", text, app.Texts())
		}
	}
	// While 处理中 has cards the hero is compact: its kicker, title and buttons, under a lower card.
	if got := app.Find("id:films-hero").Frame.Size.Height; got != heroCompactHeight || app.Exists(whole("“河流塑造了大地”")) || app.Exists("id:films-hero >> "+whole("YouTube · 2:05")) {
		t.Errorf("the hero over 处理中 is %v high: %q", got, app.Texts())
	}
	// With 处理中 empty it is whole again, with its line and its first bilingual line; the line says how long the film is, which under a poster is left to the chip on it.
	running := a.items[2]
	change(app, a, func() { a.items = a.items[:2] })
	if got := app.Find("id:films-hero").Frame.Size.Height; got != heroHeight || !app.Exists(whole("“河流塑造了大地”")) || !app.Exists(whole("Rivers shape the land")) {
		t.Errorf("the hero with nothing processing is %v high: %q", got, app.Texts())
	}
	if !app.Exists("id:films-hero >> "+whole("YouTube · 2:05")) || !app.Exists("id:film-done >> "+whole("2:05")) || !app.Exists("id:films-grid >> "+whole("YouTube")) || app.Exists("id:films-grid >> "+whole("YouTube · 2:05")) {
		t.Errorf("the length is not on the hero's line and the poster alone: %q", app.Texts())
	}
	change(app, a, func() { a.items = append(a.items, running) })
	// A done film with untranslated lines offers to translate them under its line.
	app.Press("id:film-translate-missing")
	app.Press("id:film-done")
	wantCalls(t, a, "translate-missing algebra", "open done")
	// The hero plays the film, or shows it in the file manager when there is nowhere to resume from.
	app.Press("id:hero-play")
	app.Press("id:hero-alt")
	wantCalls(t, a, "open-at done 0s", "reveal "+a.items[0].VideoPath)
	change(app, a, func() { a.resume[a.items[0].VideoPath] = 70 * time.Second })
	if !app.Exists(whole(msgs.FilmsHeroResume("1:10"))) || !app.Exists(whole(msgs.FilmsHeroFromStart())) {
		t.Fatalf("the hero does not offer to resume: %q", app.Texts())
	}
	app.Press("id:hero-alt")
	wantCalls(t, a, "open-at done 0s")
}

// TestHeroIsTheFilmBeingWatched: the hero shows the film played last that can be continued, saying so, rather than the film finished last; with none to continue, the newest finished film never played; and with every film played to the end, the newest finished one.
func TestHeroIsTheFilmBeingWatched(t *testing.T) {
	a := newFakeApp()
	rivers, algebra := a.items[0], a.items[1]
	app := start(t, a)
	msgs := testlocale.Messages()
	hero := func() string { return app.Find("id:films-hero").Props.Label }
	if hero() != rivers.Name {
		t.Fatalf("the hero of a library never played is %q", hero())
	}
	// Linear Algebra stopped at 1:10 yesterday and Rivers of Europe at 0:30 the week before: Linear Algebra is the one to continue.
	now := time.Date(2026, 11, 16, 20, 0, 0, 0, time.Local)
	change(app, a, func() {
		a.resume[algebra.VideoPath], a.played[algebra.VideoPath] = 70*time.Second, now.AddDate(0, 0, -1)
		a.resume[rivers.VideoPath], a.played[rivers.VideoPath] = 30*time.Second, now.AddDate(0, 0, -7)
	})
	if hero() != algebra.Name || !app.Exists(whole(msgs.FilmsHeroKickerContinue())) || !app.Exists(whole(msgs.FilmsHeroResume("1:10"))) {
		t.Fatalf("the hero is %q, not Linear Algebra to continue: %q", hero(), app.Texts())
	}
	// A film stopped before Kando kept when it played comes after one that has a time.
	change(app, a, func() { delete(a.played, rivers.VideoPath) })
	if hero() != algebra.Name {
		t.Fatalf("the hero is %q", hero())
	}
	// Linear Algebra was watched to the end, so Rivers of Europe is the one to continue.
	change(app, a, func() {
		delete(a.resume, algebra.VideoPath)
		a.furthest[algebra.VideoPath] = 5 * time.Minute
	})
	if hero() != rivers.Name || !app.Exists(whole(msgs.FilmsHeroKickerContinueBilingual(msgs.SettingsLanguageSimplifiedChinese()))) {
		t.Fatalf("the hero is %q: %q", hero(), app.Texts())
	}
	// The hero is named by its film and gives no hint of its own that could say 最近完成 over a kicker that says 继续观看.
	if d := app.Find("id:films-hero").Props.Hint; d != "" {
		t.Fatalf("the hero's hint is %q", d)
	}
	// With nothing to continue, the newest finished film never played is the hero, ahead of one watched to the end.
	change(app, a, func() {
		delete(a.resume, rivers.VideoPath)
		a.played[rivers.VideoPath] = now
		delete(a.furthest, algebra.VideoPath)
		delete(a.played, algebra.VideoPath)
	})
	if hero() != algebra.Name || !app.Exists(whole(msgs.FilmsHeroKicker())) {
		t.Fatalf("the hero is %q, not the film never played: %q", hero(), app.Texts())
	}
	// Every film was watched: the newest finished one.
	change(app, a, func() { a.played[algebra.VideoPath] = now })
	if hero() != rivers.Name {
		t.Fatalf("the hero of a library watched through is %q", hero())
	}
}

// TestPostersShowHowMuchWasWatched: a poster draws an accent line along its bottom, inside its hairline, as far as the film has played, and a check once it played to its end; a film never played has neither. The grid's heading is its count alone, the top bar names the source and not the library, and the poster's tooltip names the film in full.
func TestPostersShowHowMuchWasWatched(t *testing.T) {
	a := newFakeApp()
	rivers := a.items[0]
	app := start(t, a)
	msgs := testlocale.Messages()
	poster := "id:film-done"
	if app.Exists(poster+" >> id:film-progress") || app.Exists(poster+" >> id:film-watched") {
		t.Fatal("a film never played shows how much was watched")
	}
	change(app, a, func() { a.furthest[rivers.VideoPath] = 50 * time.Second })
	bar, track := app.Find(poster+" >> id:film-progress"), app.Find(poster)
	if bar.Frame.Size.Width < track.Frame.Size.Width-2 || bar.Frame.Size.Height != 3 {
		t.Fatalf("the track is %v in a poster %v", bar.Frame, track.Frame)
	}
	if fill := bar.Children[0].Frame.Size.Width; fill < bar.Frame.Size.Width*0.39 || fill > bar.Frame.Size.Width*0.41 {
		t.Fatalf("0:50 of 2:05 fills %v of %v", fill, bar.Frame.Size.Width)
	}
	if app.Exists(poster + " >> id:film-watched") {
		t.Fatal("a film played partway is marked watched")
	}
	change(app, a, func() { a.furthest[rivers.VideoPath] = 124 * time.Second })
	if !app.Exists(poster+" >> id:film-watched") || app.Exists(poster+" >> id:film-progress") {
		t.Fatal("a film played to its end is not marked watched")
	}
	if got := app.Find(poster + " >> id:film-watched").Props.Label; got != msgs.FilmsWatched() {
		t.Fatalf("the check reads %q", got)
	}
	// The grid's heading is the count, with no title repeating the top bar's; the top bar does not repeat the library's name the sidebar shows.
	if !app.Exists(whole(msgs.FilmsGridCount(2))) || app.Exists("id:films-top-bar >> "+whole(LibraryName(a.dir))) {
		t.Fatalf("texts %q", app.Texts())
	}
	if got := app.Find("id:films-grid >> " + whole("Linear Algebra")).MaxLines; got != 2 {
		t.Fatalf("a poster's title keeps %d lines", got)
	}
	before := len(app.Query(whole("Linear Algebra")))
	app.Hover("id:film-algebra")
	app.Advance(time.Second)
	if got := len(app.Query(whole("Linear Algebra"))); got != before+1 {
		t.Fatalf("resting on the poster shows %d texts naming the film, want the %d there were and its tooltip", got, before)
	}
}

// TestPosterNamesTheSpokenLanguage: the tag over a poster names the language heard in the film, and the translation's language too once that is not the one set for new films.
func TestPosterNamesTheSpokenLanguage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	tag := "id:film-done >> " + whole(msgs.FilmsPosterBilingual(msgs.FilmsLanguageTagSimplifiedChinese()))
	if !app.Exists(tag) {
		t.Fatalf("a film whose spoken language is not known: %q", app.Texts())
	}
	change(app, a, func() { a.items[0].Spoken = "ja" })
	if !app.Exists("id:film-done >> " + whole("JA")) {
		t.Fatalf("a Japanese film translated into the language set: %q", app.Texts())
	}
	change(app, a, func() { a.language = "English" })
	if !app.Exists("id:film-done >> " + whole(msgs.FilmsPosterLanguages("JA", msgs.FilmsLanguageTagSimplifiedChinese()))) {
		t.Fatalf("a Japanese film translated into another language than the one set: %q", app.Texts())
	}
}

func TestCardsStayWhileTheAppChanges(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	poster, card := app.Find("id:film-done").ID, app.Find("id:queue-running").ID
	// A change that leaves the shelf as it was, as a keystroke elsewhere, builds nothing again.
	change(app, a, func() {})
	if app.Find("id:film-done").ID != poster || app.Find("id:queue-running").ID != card {
		t.Fatal("a change that left the shelf alone built its cards again")
	}
	// A job's progress moves its card's line and bar, and keeps the card.
	change(app, a, func() { a.items[2].Progress = core.Fraction(0.42) })
	if app.Find("id:queue-running").ID != card {
		t.Fatal("a progress report built the card again")
	}
	if !app.Exists("text:42%") {
		t.Fatalf("the card does not say how far it is: %q", app.Texts())
	}
	steps := app.Find("id:queue-steps")
	if v := steps.Props.Value; v.Max != 4 || v.Now < 0.41 || v.Now > 0.43 {
		t.Fatalf("the steps bar reads %+v", v)
	}
}

// A view that clips its children clips its own outer shadow on Apple's targets, so the hero and the posters cast their shadows from a view that does not clip, and clip their picture in a view inside it.
func TestShadowsAreNotClipped(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	for _, id := range []string{"id:films-hero", "id:film-done"} {
		card := app.Find(id)
		if card.Style.Effects == nil || len(card.Style.Effects.Shadows) == 0 {
			t.Fatalf("%s casts no shadow: %+v", id, card.Style)
		}
		if card.Style.Clip {
			t.Fatalf("%s clips, so on a Mac it loses its shadow", id)
		}
		if len(card.Children) == 0 || !card.Children[0].Style.Clip || card.Children[0].Style.CornerRadius != card.Style.CornerRadius {
			t.Fatalf("%s does not clip its picture to its corners inside it", id)
		}
	}
}

func TestShowAllStaysWhileAJobRuns(t *testing.T) {
	a := newFakeApp()
	for i := range QueueShown {
		key := "waiting-" + string(rune('a'+i))
		a.queue = append(a.queue, Film{Key: key, JobID: key, Title: key, Name: key, Source: "https://youtu.be/" + key, Stage: core.StageQueued})
	}
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.FilmsQueueShowAll(QueueShown + 1))) {
		t.Fatalf("no 显示全部 for %d cards: %q", QueueShown+1, app.Texts())
	}
	button := app.Find("id:queue-more").ID
	// Progress and changes elsewhere leave the button, and the keyboard focus on it, alone.
	change(app, a, func() { a.items[2].Progress = core.Fraction(0.42) })
	change(app, a, func() {})
	if app.Find("id:queue-more").ID != button {
		t.Fatal("a progress report built 显示全部 again")
	}
	app.Press("id:queue-more")
	if !app.Exists(whole(msgs.FilmsQueueCollapse())) || !app.Exists("id:queue-waiting-f") {
		t.Fatalf("显示全部 did not show every card: %q", app.Texts())
	}
}

func TestFilmMenuOffersWhatCanBeDone(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:film-more >> nth=0")
	var labels []string
	for _, it := range app.PopupMenu {
		labels = append(labels, it.Label)
	}
	// The menu is the first done card's: Linear Algebra, which has untranslated lines, comes after Rivers of Europe in the order added.
	want := []string{msgs.FilmsPlay(), msgs.FilmsMarkWatched(), msgs.FilmsShowInFinder("macOS"), msgs.FilmsRename(), msgs.FilmsReprocess(), msgs.FilmsTranslateAgain(), "", msgs.FilmsRemoveFromLibrary()}
	if !slices.Equal(labels, want) {
		t.Fatalf("menu %q, want %q", labels, want)
	}
	app.ChoosePopup(msgs.FilmsRename())
	wantCalls(t, a, "ask-rename done")
	// A film stopped partway can also start over, as the poster and 播放 open it where it stopped.
	a.resume[a.items[0].VideoPath] = 70 * time.Second
	app.RightClick("id:film-done")
	if len(app.PopupMenu) < 2 || app.PopupMenu[1].Label != msgs.FilmsPlayFromBeginning() {
		t.Fatalf("the menu of a film stopped partway: %+v", app.PopupMenu)
	}
	app.ChoosePopup(msgs.FilmsPlayFromBeginning())
	wantCalls(t, a, "open-at done 0s")
	delete(a.resume, a.items[0].VideoPath)
	// The rename sheet holds the title, and Enter renames.
	if !app.Exists("id:rename-name") {
		t.Fatalf("no rename field\n%s", app.Dump())
	}
	app.Type("id:rename-name", " (2026)")
	app.Submit("id:rename-name")
	wantCalls(t, a, "rename Rivers of Europe (2026)")
	// The sheet goes once its motion has ended.
	app.Advance(time.Second)
	if app.Exists("id:rename-name") {
		t.Fatal("the rename sheet stayed open")
	}
	// 从资料库移除 asks first; Enter keeps the film, 移除 moves it to the Trash.
	app.RightClick("id:film-done")
	app.ChoosePopup(msgs.FilmsRemoveFromLibrary())
	wantCalls(t, a, "ask-remove done")
	if !app.Exists(whole(msgs.FilmsRemoveConfirm("Rivers of Europe"))) || !app.Exists(whole(msgs.FilmsRemoveFiles())) {
		t.Fatalf("no confirmation: %q", app.Texts())
	}
	app.Key("enter")
	wantCalls(t, a, "close-remove")
	change(app, a, func() { a.removeOpen, a.removeFolder = true, true })
	if !app.Exists(whole(msgs.FilmsRemoveFolder())) {
		t.Fatalf("the confirmation does not say the folder goes: %q", app.Texts())
	}
	app.Press("id:remove-confirm-trash")
	wantCalls(t, a, "trash done")
}

// TestFilmMenuMarksHowFarItWasWatched checks that a film's menu offers 标记为已看完 or 标记为未看, whichever applies, both for a film in progress, 标记为已看完 again for a watched film stopped partway, as while it is watched again, and 标记为已看完 dimmed while its length is not known; and that choosing one marks the film.
func TestFilmMenuMarksHowFarItWasWatched(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	video := a.items[0].VideoPath
	marks := func() (watched, unwatched, dimmed bool) {
		t.Helper()
		app.RightClick("id:film-done")
		for _, it := range app.PopupMenu {
			switch it.Label {
			case msgs.FilmsMarkWatched():
				watched, dimmed = true, it.Disabled
			case msgs.FilmsMarkUnwatched():
				unwatched = true
			}
		}
		return
	}
	cases := []struct {
		name                       string
		furthest, resume, length   time.Duration
		watched, unwatched, dimmed bool
	}{
		{name: "never played", length: 125 * time.Second, watched: true},
		{name: "opened and closed at its start", furthest: 2 * time.Second, length: 125 * time.Second, watched: true},
		{name: "in progress", furthest: 40 * time.Second, resume: 40 * time.Second, length: 125 * time.Second, watched: true, unwatched: true},
		{name: "watched", furthest: 124 * time.Second, length: 125 * time.Second, unwatched: true},
		{name: "watched, playing again", furthest: 125 * time.Second, resume: 30 * time.Second, length: 125 * time.Second, watched: true, unwatched: true},
		{name: "length not known", furthest: 40 * time.Second, resume: 40 * time.Second, watched: true, unwatched: true, dimmed: true},
	}
	for _, c := range cases {
		change(app, a, func() {
			a.furthest[video], a.resume[video], a.durations[video] = c.furthest, c.resume, c.length
		})
		if watched, unwatched, dimmed := marks(); watched != c.watched || unwatched != c.unwatched || dimmed != c.dimmed {
			t.Fatalf("%s: 标记为已看完 %v (dimmed %v), 标记为未看 %v, want %v (%v), %v", c.name, watched, dimmed, unwatched, c.watched, c.dimmed, c.unwatched)
		}
	}
	change(app, a, func() { a.durations[video] = 125 * time.Second })
	app.RightClick("id:film-done")
	app.ChoosePopup(msgs.FilmsMarkWatched())
	app.RightClick("id:film-done")
	app.ChoosePopup(msgs.FilmsMarkUnwatched())
	wantCalls(t, a, "mark-watched done", "mark-unwatched done")
}

// A done film that translated none of its lines says so under its title rather than calling itself bilingual, says why, and offers 翻译, which translates its missing lines, and the settings that fix it.
func TestAFilmWithoutATranslationOffersToTranslateIt(t *testing.T) {
	a := newFakeApp()
	a.items[0].TranslationErr, a.items[0].Untranslated = "KANDO_LLM_MODEL is not set", 12
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists("text:"+msgs.FilmsNoTranslation()) || !app.Exists(whole("没有 AI 服务")) {
		t.Fatalf("the card does not say it has no translation: %q", app.Texts())
	}
	if app.Exists(whole(msgs.FilmsPosterBilingual(LanguageShort(msgs, "简体中文")))) {
		t.Fatal("a film without a translation is tagged bilingual")
	}
	if app.Exists("text:" + msgs.FilmsUntranslated(12)) {
		t.Fatal("a film without a translation counts its untranslated lines")
	}
	app.Press("id:film-translate")
	app.Press("id:film-translation-settings")
	// Linear Algebra, which translated all but three lines, still offers 补译未译的句子.
	app.Press("id:film-translate-missing")
	wantCalls(t, a, "translate-missing done", "settings ai", "translate-missing algebra")
}

// A video found in the library's folder without subtitles offers 生成字幕 under its line and in its menu in place of 重新处理, which start it without asking; one with subtitles found beside it offers 重新处理, which asks first.
func TestAFolderVideoOffersToMakeItsSubtitles(t *testing.T) {
	a := newFakeApp()
	bare := a.dir + "/Lecture 2.mp4"
	a.items = []Film{
		{Key: bare, Title: "Lecture 2", Name: "Lecture 2", Detail: bare, VideoPath: bare},
		{Key: "copied", Title: "Copied", Name: "Copied", VideoPath: a.dir + "/Copied/video.mp4", SubtitlePath: a.dir + "/Copied/video.bilingual.srt"},
	}
	app := start(t, a)
	msgs := testlocale.Messages()
	if got := len(app.Query("id:film-make-subtitles")); got != 1 {
		t.Fatalf("%d 生成字幕 buttons for one video without subtitles: %q", got, app.Texts())
	}
	app.Press("id:film-make-subtitles")
	wantCalls(t, a, "reprocess "+bare)
	app.Press("id:film-more >> nth=0")
	var labels []string
	for _, it := range app.PopupMenu {
		labels = append(labels, it.Label)
	}
	want := []string{msgs.FilmsPlay(), msgs.FilmsMarkWatched(), msgs.FilmsShowInFinder("macOS"), msgs.FilmsRename(), msgs.FilmsMakeSubtitles(), "", msgs.FilmsRemoveFromLibrary()}
	if !slices.Equal(labels, want) {
		t.Fatalf("menu %q, want %q", labels, want)
	}
	app.ChoosePopup(msgs.FilmsMakeSubtitles())
	wantCalls(t, a, "reprocess "+bare)
	app.Press("id:film-more >> nth=1")
	app.ChoosePopup(msgs.FilmsReprocess())
	wantCalls(t, a, "ask-reprocess copied")
}

func TestNoticeOffersItsButtons(t *testing.T) {
	a := newFakeApp()
	a.notice = Notice{Kind: "warning", Text: "这个视频已在资料库中。", Section: "translate", Film: "done", Open: true}
	app := start(t, a)
	if !app.Exists(whole(a.notice.Text)) {
		t.Fatalf("no notice: %q", app.Texts())
	}
	if app.Find("id:films-notice").Props.Role != ns.RoleStatus {
		t.Fatal("the notice is not a status to assistive technology")
	}
	app.Press("id:notice-settings")
	app.Press("id:notice-film")
	wantCalls(t, a, "settings translate", "open done")
	if app.Exists("id:films-notice") {
		t.Fatal("打开 left the notice open")
	}
	// A notice naming a film no longer in the library has no 打开.
	change(app, a, func() { a.notice = Notice{Kind: "success", Text: "已加入。", Film: "gone", Open: true} })
	if app.Exists("id:notice-film") || app.Exists("id:notice-settings") {
		t.Fatal("the notice offers buttons it has no use for")
	}
	app.Press("id:notice-close")
	if a.notice.Open || app.Exists("id:films-notice") {
		t.Fatal("× did not close the notice")
	}
}

// TestNoticeOffersMoveButtons checks the bar after a film was moved to another library, 打开 <library> and Undo, and the bar of a copy going on, 取消.
func TestNoticeOffersMoveButtons(t *testing.T) {
	a := newFakeApp()
	msgs := testlocale.Messages()
	a.notice = Notice{Kind: "success", Text: "已移动。", Library: "/Movies/英语听力.kando", Undo: true, Open: true}
	app := start(t, a)
	if !app.Exists(whole(msgs.FilmsNoticeOpenLibrary("英语听力"))) {
		t.Fatalf("no 打开 for the library: %q", app.Texts())
	}
	app.Press("id:notice-undo")
	change(app, a, func() { a.notice.Open = true })
	app.Press("id:notice-library")
	wantCalls(t, a, "undo-move", "choose /Movies/英语听力.kando")
	if a.notice.Open {
		t.Fatal("打开 left the notice open")
	}
	change(app, a, func() { a.notice = Notice{Kind: "success", Text: "正在移动…", Stop: true, Open: true} })
	if app.Exists("id:notice-undo") || app.Exists("id:notice-library") {
		t.Fatal("the bar of a copy going on offers Undo or 打开")
	}
	app.Press("id:notice-stop")
	wantCalls(t, a, "cancel-move")
}

func TestTopBarAddsTheLink(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Type(msgs.FilmsLinkLabel(), "https://www.bilibili.com/video/BV1GJ411x7h7")
	if !app.Exists(whole(msgs.SourceBilibili())) {
		t.Fatalf("no chip for the link: %q", app.Texts())
	}
	app.Submit("id:films-link")
	app.Press(whole(msgs.FilmsStart()))
	app.Press("id:films-local")
	app.Press(whole(msgs.FilmsBrowseSites()))
	wantCalls(t, a, "start https://www.bilibili.com/video/BV1GJ411x7h7", "start https://www.bilibili.com/video/BV1GJ411x7h7", "pick-videos", "select "+sidebar.PageBrowse)
	change(app, a, func() { a.linkFocus = true })
	if !app.Find("id:films-link").Focused || a.linkFocus {
		t.Fatal("换个链接's request did not focus the link field")
	}
}

func TestDroppedFilesGoToTheApp(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.DropFiles("id:films-page", "file:///Users/me/Movies/talk.mp4", "https://example.com/not-a-file")
	wantCalls(t, a, "drop "+filepath.FromSlash("/Users/me/Movies/talk.mp4"))
	if app.Exists("id:films-drop-zone") {
		t.Fatal("the drop zone stayed after the drop")
	}
}

func TestSortMenuChecksTheOrderShown(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:films-sort")
	// The orders, the one shown checked, then the two directions, recently added's own (newest first) checked, all radio items.
	menu := app.PopupMenu
	for _, it := range menu {
		if !it.Separator && !it.Radio {
			t.Fatalf("the sort menu's %q is not a radio item", it.Label)
		}
	}
	if len(menu) != len(Sorts)+3 || !menu[0].Checked || menu[1].Checked || !menu[len(Sorts)].Separator ||
		menu[len(Sorts)+1].Label != msgs.FilmsSortAscending() || menu[len(Sorts)+1].Checked || !menu[len(Sorts)+2].Checked {
		t.Fatalf("sort menu %+v", menu)
	}
	app.ChoosePopup(msgs.FilmsSortTitle())
	wantCalls(t, a, "sort title ascending")
	if !app.Exists(whole(msgs.FilmsSortButton(msgs.FilmsSortTitle()))) {
		t.Fatalf("the button does not name the order: %q", app.Texts())
	}
	// By title, Linear Algebra comes before Rivers of Europe.
	if algebra, rivers := app.Find("id:films-grid >> "+whole("Linear Algebra")), app.Find("id:films-grid >> "+whole("Rivers of Europe")); pageX(algebra) >= pageX(rivers) {
		t.Fatalf("the grid is not by title: Linear Algebra at %v, Rivers of Europe at %v", pageX(algebra), pageX(rivers))
	}
}

func TestFailedCardsOfferTheirRemedy(t *testing.T) {
	a := newFakeApp()
	a.queue = []Film{
		{Key: "failed", JobID: "failed", Title: "Region", Name: "Region", Source: "https://youtu.be/region", Stage: core.StageFailed},
		{Key: "plain", JobID: "plain", Title: "Plain", Name: "Plain", Source: "https://youtu.be/plain", Stage: core.StageFailed},
	}
	a.remedies["failed"] = Remedy{Headline: "你所在的地区不能看", Why: "换个链接试试。", Actions: []RemedyAction{{ID: "swap", Label: "换个链接", Primary: true}, {ID: "update", Label: "更新中", Disabled: true}}}
	a.hints["plain"] = "网络断开"
	a.details = map[string]string{"plain": "transcribe: whisper-cli: exit status 3: failed to load model\nsecond line"}
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, text := range []string{"你所在的地区不能看", "换个链接试试。", "网络断开"} {
		if !app.Exists(whole(text)) {
			t.Fatalf("no %q in %q", text, app.Texts())
		}
	}
	app.Press("id:queue-remedy-swap")
	if app.Enabled("id:queue-remedy-update") {
		t.Fatal("a remedy at work can be pressed")
	}
	app.Press(whole(msgs.FilmsRetry()))
	app.Press("id:queue-cancel")
	// A failure without a known fix shows its whole error over its reason and can be reported.
	if !app.Exists("id:queue-reason-tip") {
		t.Fatal("the reason has no tooltip with the whole error")
	}
	app.Press("id:queue-report")
	wantCalls(t, a, "remedy failed swap", "retry plain", "withdraw running", "report plain")
}

// A job waiting or running can be watched before it is done: its card's thumbnail opens the film, one still downloading too, as opening it puts it first and it plays once its sound or its video is in; a failed one does not.
func TestAProcessingFilmCanBeWatched(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Press("id:queue-watch")
	change(app, a, func() {
		a.items[2].Stage, a.items[2].VideoPath, a.items[2].SubtitlePath = core.StageTranslating, a.dir+"/Market/video.mp4", a.dir+"/Market/video.srt"
	})
	app.Press("id:queue-watch")
	wantCalls(t, a, "open running", "open running")
	change(app, a, func() { a.items[2].Stage = core.StageFailed })
	if app.Exists("id:queue-watch") {
		t.Fatal("a failed film offers to be watched")
	}
}

// Past one failure, the heading of 处理中 offers to retry those that can be and to remove them all.
func TestTheQueueOffersToRetryOrRemoveEveryFailure(t *testing.T) {
	a := newFakeApp()
	a.retryable, a.failed = 1, 1
	app := start(t, a)
	if app.Exists("id:queue-retry-all") || app.Exists("id:queue-remove-failed") {
		t.Fatal("one failure offers bulk actions")
	}
	change(app, a, func() { a.retryable, a.failed = 2, 3 })
	app.Press("id:queue-retry-all")
	app.Press("id:queue-remove-failed")
	wantCalls(t, a, "retry-all", "remove-failed")
	change(app, a, func() { a.retryable = 1 })
	if app.Exists("id:queue-retry-all") || !app.Exists("id:queue-remove-failed") {
		t.Fatal("Retry All stayed for a single retryable failure")
	}
}

// 重新翻译为 ▸ offers the language set now first and checks the film's own; a card waiting or running says the language it translates into.
func TestTranslateAgainNamesItsLanguage(t *testing.T) {
	a := newFakeApp()
	a.items[2].Target = "日本語"
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.FilmsQueueInto(msgs.SettingsLanguageJapanese()))) {
		t.Fatalf("the running card does not say its language: %q", app.Texts())
	}
	app.Press("id:film-more >> nth=0")
	var into []string
	for _, it := range app.PopupMenu {
		if it.Label == msgs.FilmsTranslateAgain() {
			for _, sub := range it.Items {
				into = append(into, fmt.Sprint(sub.Label, sub.Checked))
			}
		}
	}
	if len(into) != len(subtitle.TargetLanguages()) || into[0] != msgs.SettingsLanguageJapanese()+"false" || into[1] != msgs.SettingsLanguageSimplifiedChinese()+"true" {
		t.Fatalf("submenu %q", into)
	}
	app.ChoosePopup(msgs.FilmsTranslateAgain(), msgs.SettingsLanguageJapanese())
	wantCalls(t, a, "translate-again done 日本語")
}

// A film that was not translated at all has no language of its own: 重新翻译为 ▸ checks none, rather than the language it was meant to be translated into.
func TestTranslateAgainChecksNoLanguageForAFilmWithoutATranslation(t *testing.T) {
	a := newFakeApp()
	a.items[0].TranslationErr = "no translator"
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:film-more >> nth=0")
	var checked []string
	for _, it := range app.PopupMenu {
		if it.Label == msgs.FilmsTranslateAgain() {
			for _, sub := range it.Items {
				if sub.Checked {
					checked = append(checked, sub.Label)
				}
			}
		}
	}
	if len(checked) != 0 {
		t.Fatalf("the submenu checks %q", checked)
	}
}

// Past one done film shown without a translation, the grid's heading offers to translate them all at once, while an AI service is set up to; a film whose job runs again is not counted.
func TestTheGridOffersToTranslateTheFilmsWithoutATranslation(t *testing.T) {
	a := newFakeApp()
	a.items[0].TranslationErr = "no translator"
	a.items[2].TranslationErr = "no translator"
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:films-translate-all") {
		t.Fatal("one film without a translation offers to translate them all")
	}
	change(app, a, func() { a.items[1].TranslationErr = "no translator" })
	if !app.Exists(whole(msgs.FilmsTranslateAll(2))) {
		t.Fatalf("no Translate 2 Films: %q", app.Texts())
	}
	app.Press("id:films-translate-all")
	wantCalls(t, a, "translate-films done algebra")
	change(app, a, func() { a.untranslatable = true })
	if app.Exists("id:films-translate-all") {
		t.Fatal("Translate All shows without an AI service")
	}
}

// The message bar's Translate translates every film of the library that was not translated at all, and closes the bar.
func TestTheMessageBarTranslatesTheFilmsWithoutATranslation(t *testing.T) {
	a := newFakeApp()
	a.items[0].TranslationErr = "no translator"
	a.items[1].TranslationErr = "no translator"
	a.notice = Notice{Kind: "warning", Text: "2 films", Translate: true, Open: true}
	app := start(t, a)
	app.Press("id:notice-translate")
	wantCalls(t, a, "translate-films done algebra")
	if a.notice.Open {
		t.Fatal("the bar stayed open")
	}
}

func TestEmptyPagesSayWhatToDo(t *testing.T) {
	a := newFakeApp()
	a.dir, a.items = "", nil
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.FilmsNoLibraryTitle())) {
		t.Fatalf("no page for no library: %q", app.Texts())
	}
	app.Press("id:empty-new")
	wantCalls(t, a, "new")
	// The new-library sheet names the path the library will have, and 创建 waits for a name it can use.
	app.Settle()
	a.Changed()
	app.Settle()
	if !app.Exists("id:newlib-name") {
		t.Fatalf("no new-library sheet\n%s", app.Dump())
	}
	app.Clear("id:newlib-name")
	app.Type("id:newlib-name", "课程")
	if a.name != "课程" || !app.Exists("text:课程.kando") {
		t.Fatalf("name %q, texts %q", a.name, app.Texts())
	}
	change(app, a, func() { a.pathErr = errors.New("名称不能为空") })
	if app.Enabled("id:newlib-create") || !app.Exists(whole("名称不能为空")) {
		t.Fatal("创建 is enabled for a name it cannot use")
	}
	change(app, a, func() { a.pathErr = nil })
	app.Press("id:newlib-create")
	wantCalls(t, a, "create 课程")
	change(app, a, func() { a.dir = "/films/课程.kando" })
	if !app.Exists(whole(msgs.FilmsEmptyTitle())) {
		t.Fatalf("no page for an empty library: %q", app.Texts())
	}
	app.Press("id:empty-sites")
	wantCalls(t, a, "sites")
	app.Press("id:empty-browse")
	wantCalls(t, a, "select "+sidebar.PageBrowse)
	// A source no film comes from says so; a search no film matches offers to clear it.
	change(app, a, func() { a.items, a.filter = newFakeApp().items, sidebar.SourceLocal })
	if !app.Exists(whole(msgs.FilmsSourceEmpty())) {
		t.Fatalf("no page for an empty source: %q", app.Texts())
	}
	change(app, a, func() { a.filter, a.query = sidebar.SourceAll, "ocean" })
	if !app.Exists(whole(msgs.FilmsSearchNoMatch("ocean"))) || app.Exists("id:films-hero") {
		t.Fatalf("a search with no match: %q", app.Texts())
	}
	app.Press("id:films-search-clear")
	if a.query != "" || !app.Exists("id:films-hero") {
		t.Fatal("清除搜索 left the search")
	}
}

// TestPageFollowsTheLanguage switches the interface's language with the library page showing and the removal confirmation open: the page's words, the view switch's, the link field's and the confirmation's names, and its buttons, say the new language, and none of those controls is built again for it.
func TestPageFollowsTheLanguage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	if !app.Exists(whole(testlocale.Messages().FilmsStart())) {
		t.Fatalf("no %q", testlocale.Messages().FilmsStart())
	}
	change(app, a, func() { a.removing, a.removeOpen = a.items[:1], true })
	ids := []string{"films-view", "films-link", "films-start", "remove-confirm", "remove-cancel"}
	before := map[string]*nsgotest.View{}
	for _, id := range ids {
		before[id] = app.Find("id:" + id)
	}
	testlocale.Use(t, "en")
	app.SetEnvironment(testEnvironment())
	en := testlocale.Messages()
	for _, text := range []string{en.FilmsStart(), en.FilmsQueueTitle(), en.SourceAll(), en.FilmsLocalVideo(), en.FilmsCancel(), en.FilmsRemove(), en.FilmsRemoveFiles()} {
		if !app.Exists(whole(text)) {
			t.Errorf("no %q in English: %q", text, app.Texts())
		}
	}
	for id, want := range map[string]string{
		"films-view":      en.FilmsView(),
		"films-view-grid": en.FilmsViewGrid(),
		"films-link":      en.FilmsLinkLabel(),
		"remove-confirm":  en.FilmsRemoveConfirm("Rivers of Europe"),
	} {
		if got := app.Find("id:" + id).Props.Label; got != want {
			t.Errorf("%s is named %q in English, want %q", id, got, want)
		}
	}
	for _, id := range ids {
		if app.Find("id:"+id).ID != before[id].ID {
			t.Errorf("%s was built again for the language", id)
		}
	}
}

// pageX is where v starts across the window.
func pageX(v *nsgotest.View) ns.Points {
	x := v.Frame.Origin.X
	for p := v.Parent; p != nil; p = p.Parent {
		x += p.Frame.Origin.X - p.Offset.X
	}
	return x
}

// TestTopBarCentersItsControls checks the prototype's toolbar: 65 points high with 10 above its controls and 18 below them, so they are centered 28.5 points down.
func TestTopBarCentersItsControls(t *testing.T) {
	app := start(t, newFakeApp())
	bar, start := app.Find("id:films-top-bar"), app.Find("id:films-start")
	if bar.Frame.Size.Height != 65 {
		t.Fatalf("the bar is %v high", bar.Frame.Size.Height)
	}
	y := start.Frame.Origin.Y
	for v := start.Parent; v != nil; v = v.Parent {
		y += v.Frame.Origin.Y - v.Offset.Y
	}
	if center := y + start.Frame.Size.Height/2; center < 28 || center > 29 {
		t.Fatalf("开始处理 is centered %v points down, want 28.5", center)
	}
}

// TestKeyboardFocusShowsOnEveryButton checks that each of the page's own buttons, which draw their own look, shows a ring while Tab has the keyboard on it, as ui's buttons do, and none before.
func TestKeyboardFocusShowsOnEveryButton(t *testing.T) {
	a := newFakeApp()
	a.notice = Notice{Kind: "success", Text: "已加入。", Open: true}
	app := start(t, a)
	for _, id := range []string{"id:films-local", "id:notice-close", "id:films-hero", "id:hero-play", "id:hero-alt", "id:queue-cancel", "id:films-sort", "id:film-done", "id:film-more >> nth=0"} {
		if w := app.Find(id).Style.Outline.Width; w != 0 {
			t.Fatalf("%s shows a ring of %v without the focus", id, w)
		}
		app.Focus(id)
		if v := app.Find(id); !v.Focused || v.Style.Outline.Width == 0 {
			t.Fatalf("%s shows no ring with the keyboard on it (focused %v)", id, v.Focused)
		}
	}
}

// TestMoreActionsNamesItsFilm checks that each poster's ⋯ is named for the film it acts on, so a screen reader moving between them tells them apart.
func TestMoreActionsNamesItsFilm(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	for i, name := range []string{"Rivers of Europe", "Linear Algebra"} {
		if got := app.Find(fmt.Sprintf("id:film-more >> nth=%d", i)).Props.Label; got != msgs.FilmsMoreActions(name) {
			t.Fatalf("⋯ %d is named %q, want %q", i, got, msgs.FilmsMoreActions(name))
		}
	}
}

// startThemed is start in the window's theme, as the main window's view applies it, with k, Kando's roles at its root, which follow the environment.
func startThemed(t *testing.T, a *fakeApp, env ns.Environment) (*nsgotest.App, *theme.Colors) {
	t.Helper()
	k := new(theme.Colors)
	app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		a.Attach(cx)
		theme.Apply(cx)
		*k = theme.Of(cx)
		return ns.View{Style: ns.Style{FlexGrow: 1}, Children: []ns.Node{View{App: a}}}
	}, nsgotest.Size(1068, 782), nsgotest.Environment(env))
	return app, k
}

// TestEmptyLibraryArtFollowsTheAppearance checks that the play tile over an empty library is the theme's tile in both appearances, a step off the dark page rather than a near-white square on it, with its glyph in the accent's glyph color for the dark (theme.Of's Glyph).
func TestEmptyLibraryArtFollowsTheAppearance(t *testing.T) {
	a := newFakeApp()
	a.items = nil
	env := testEnvironment()
	app, k := startThemed(t, a, env)
	tile := func() *nsgotest.View { return app.Find("id:films-start-art") }
	if got := tile().Style.Background; got != k.Tile() {
		t.Fatalf("the tile is %#x on the light page, want the theme's tile %#x", uint32(got), uint32(k.Tile()))
	}
	env.Dark = true
	app.SetEnvironment(env)
	if got := tile().Style.Background; got != k.Tile() {
		t.Fatalf("the tile is %#x on the dark page, want the theme's tile %#x", uint32(got), uint32(k.Tile()))
	}
	if r, g, b, _ := tile().Style.Background.Components(); r > 0.3 || g > 0.3 || b > 0.3 {
		t.Fatalf("the tile is %#x on the dark page, want a dark face", uint32(tile().Style.Background))
	}
	inked := 0
	for _, op := range app.Find("id:films-start-art >> id:films-start-glyph").Ops {
		if op.Color == 0 {
			continue
		}
		if inked++; ns.Color(op.Color) != k.Glyph() {
			t.Fatalf("the glyph is drawn in %#x, want the accent's glyph color %#x", uint32(op.Color), uint32(k.Glyph()))
		}
	}
	if inked == 0 {
		t.Fatal("the tile draws no glyph")
	}
}

// TestWorkingRemedyFollowsTheAppearance checks that what a failed card says while its remedy is at work (as while yt-dlp updates) is in the theme's accent ink, which reads on the dark card where the accent's own blue does not, and that the red of a remedy not at work is the theme's danger ink.
func TestWorkingRemedyFollowsTheAppearance(t *testing.T) {
	a := newFakeApp()
	a.queue = []Film{{Key: "failed", JobID: "failed", Title: "Region", Name: "Region", Source: "https://youtu.be/region", Stage: core.StageFailed}}
	a.remedies["failed"] = Remedy{Headline: "正在更新 yt-dlp…", Working: true}
	env := testEnvironment()
	env.Dark = true
	app, k := startThemed(t, a, env)
	ink := func() ns.Color { return app.Find(whole(a.remedies["failed"].Headline)).TextStyle.Color }
	if got := ink(); got != k.AccentInk() || got == theme.Accent {
		t.Fatalf("the remedy at work says it in %#x on the dark card, want the dark accent ink %#x", uint32(got), uint32(k.AccentInk()))
	}
	change(app, a, func() { a.remedies["failed"] = Remedy{Headline: "你所在的地区不能看"} })
	if got := ink(); got != k.DangerInk() {
		t.Fatalf("the remedy says it in %#x, want the danger ink %#x", uint32(got), uint32(k.DangerInk()))
	}
}

// TestListViewShowsTheDoneFilmsAsATable switches the done films to the list: the hero steps aside, each film is a row of its details, a click selects a row and a second click opens its film, as does Return, and a right click offers the film's menu.
func TestListViewShowsTheDoneFilmsAsATable(t *testing.T) {
	a := newFakeApp()
	a.furthest[a.items[0].VideoPath] = 125 * time.Second
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists("id:films-hero") || app.Exists("id:films-list") {
		t.Fatalf("the posters' page has no hero or shows the list: %q", app.Texts())
	}
	app.Press("id:films-view-list")
	wantCalls(t, a, "view list")
	if app.Exists("id:films-grid") || app.Exists("id:films-hero") || !app.Exists("id:films-list") {
		t.Fatalf("the list did not replace the posters and the hero: %q", app.Texts())
	}
	// 处理中 stays above the list.
	if !app.Exists(whole("Morning Market")) {
		t.Fatalf("处理中 left with the posters: %q", app.Texts())
	}
	for _, text := range []string{msgs.FilmsSortTitle(), msgs.FilmsSortDuration(), msgs.FilmsListProgress(), msgs.FilmsListSubtitles(),
		"Rivers of Europe", "2:05", msgs.FilmsWatched(), "简体中文", "Linear Algebra", msgs.FilmsUntranslated(3)} {
		if !app.Exists("id:films-list >> " + whole(text)) {
			t.Errorf("the list does not show %q: %q", text, app.Texts())
		}
	}
	row := "id:films-list >> " + whole("Linear Algebra")
	app.Press(row)
	wantCalls(t, a)
	app.Press(row)
	wantCalls(t, a, "open algebra")
	app.Key("enter")
	wantCalls(t, a, "open algebra")
	app.RightClick("id:films-list >> " + whole("Rivers of Europe"))
	if len(app.PopupMenu) == 0 || app.PopupMenu[0].Label != msgs.FilmsPlay() {
		t.Fatalf("right click menu %+v", app.PopupMenu)
	}
}

// TestListHeadingsSortTheFilms: a column's heading sorts by it in its own direction, the same heading again turns it around, and the order's heading shows which way it goes; recently added, which no column shows, marks none.
func TestListHeadingsSortTheFilms(t *testing.T) {
	a := newFakeApp()
	a.view = ViewList
	app := start(t, a)
	app.Press("id:films-list >> id:sort:" + colName)
	wantCalls(t, a, "sort title ascending")
	app.Settle()
	// The table asks the next step of its own cycle; the list turns the order around instead.
	app.Press("id:films-list >> id:sort:" + colName)
	wantCalls(t, a, "sort title descending")
	app.Press("id:films-list >> id:sort:" + colDuration)
	wantCalls(t, a, "sort duration descending")
	app.Press("id:films-list >> id:sort:" + colDuration)
	wantCalls(t, a, "sort duration ascending")
	app.Press("id:films-list >> id:sort:" + colDuration)
	wantCalls(t, a, "sort duration descending")
}

// TestListFitsTheColumnsToTheWidth: a narrow list keeps the name, how long and how far, and leaves out where a film came from first.
func TestListFitsTheColumnsToTheWidth(t *testing.T) {
	if got := listFits(2000); !slices.Equal(got, []string{colName, colDuration, colProgress, colSubtitles, colSpoken, colSource, colWatched}) {
		t.Fatalf("wide %q", got)
	}
	if got := listFits(700); !slices.Equal(got, []string{colName, colDuration, colProgress, colSubtitles}) {
		t.Fatalf("700 points %q", got)
	}
	if got := listFits(300); !slices.Equal(got, []string{colName}) {
		t.Fatalf("300 points %q", got)
	}
}

// TestListDateNamesTodayAndYesterday writes the time a film last played as Finder's list does.
func TestListDateNamesTodayAndYesterday(t *testing.T) {
	msgs := testlocale.Messages()
	now := time.Date(2026, 11, 18, 21, 0, 0, 0, time.UTC)
	if got := listDate(msgs, time.Time{}, now); got != "" {
		t.Fatalf("never played %q", got)
	}
	today, yesterday, before := listDate(msgs, now.Add(-time.Hour), now), listDate(msgs, now.Add(-22*time.Hour), now), listDate(msgs, now.AddDate(0, 0, -3), now)
	if !strings.HasPrefix(today, "今天") || !strings.Contains(today, "20:00") || !strings.HasPrefix(yesterday, "昨天") || !strings.Contains(before, "15") {
		t.Fatalf("today %q, yesterday %q, three days before %q", today, yesterday, before)
	}
}

// TestListDateReadsInNowsTimeZone: library.json keeps the offset a film played at, so one played in Tokyo the morning of the 18th, the evening of the 17th in UTC, reads as yesterday evening, not as a morning.
func TestListDateReadsInNowsTimeZone(t *testing.T) {
	msgs := testlocale.Messages()
	now := time.Date(2026, 11, 18, 21, 0, 0, 0, time.UTC)
	tokyo := time.Date(2026, 11, 18, 8, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	if got := listDate(msgs, tokyo, now); !strings.HasPrefix(got, "昨天") || !strings.Contains(got, "23:30") {
		t.Fatalf("played 08:30 JST, now 21:00 UTC: %q, want yesterday at 23:30", got)
	}
}

// filmKeys is the keys of items joined by commas, as the fake's calls record them.
func filmKeys(items []Film) string { return strings.Join(Keys(items), ",") }

// TestHeroIsWholeWhereThePageHasRoom: the hero is the design's whole card, with its line and its first bilingual line, whenever the page is high enough to show it, 处理中 under it and the first row of posters under that; and the compact card otherwise, with 处理中 or without, so a short window still shows a poster.
func TestHeroIsWholeWhereThePageHasRoom(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	isWhole := func() bool {
		t.Helper()
		got := app.Find("id:films-hero").Frame.Size.Height
		full := got == heroHeight
		if full != app.Exists(whole("“河流塑造了大地”")) || full != app.Exists("id:films-hero >> "+whole("YouTube · 2:05")) {
			t.Fatalf("the hero %v high shows its quote %v and its line %v", got, app.Exists(whole("“河流塑造了大地”")), app.Exists("id:films-hero >> "+whole("YouTube · 2:05")))
		}
		if !full && got != heroCompactHeight {
			t.Fatalf("the hero is %v high", got)
		}
		return full
	}
	// A tall window has room for the whole card over 处理中; a short one has not.
	app.Resize(1068, 1100)
	if !isWhole() {
		t.Fatal("the hero in a tall window over 处理中 is compact")
	}
	app.Resize(1068, 560)
	if isWhole() {
		t.Fatal("the hero in a short window over 处理中 is whole")
	}
	// Whole or compact, the first poster's picture is on the page, and the whole card shows exactly while it would leave it there.
	sweep := func(queued bool) {
		t.Helper()
		for height := ns.Points(560); height <= 1100; height += 10 {
			app.Resize(1068, height)
			poster := app.Find("id:film-done >> nth=0").Bounds()
			bottom := poster.Origin.Y + poster.Size.Height
			full := isWhole()
			if full && bottom > height {
				t.Fatalf("queued %v, %v high: the whole hero pushes the first poster to %v", queued, height, bottom)
			}
			if !full && bottom+heroHeight-heroCompactHeight <= height {
				t.Fatalf("queued %v, %v high: the hero is compact with room for the whole card (poster at %v)", queued, height, bottom)
			}
		}
	}
	sweep(true)
	change(app, a, func() { a.items = a.items[:2] })
	sweep(false)
	// Without 处理中 a short window still keeps the compact card.
	app.Resize(1068, 560)
	if isWhole() {
		t.Fatal("the hero in a short window is whole")
	}
}
