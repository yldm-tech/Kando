package sidebar

import (
	"path/filepath"
	"slices"
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// fakeApp is the shelf and navigation the sidebar reads, kept as plain fields as the app keeps them, with the calls the sidebar made.
type fakeApp struct {
	live.Rev
	dir       string
	libraries []string
	counts    map[string]int
	page      string
	filter    string
	films     int
	bySource  map[string]int
	query     string
	focus     bool
	ready     Readiness
	saveErr   string
	calls     []string
	// paletteButton is PaletteButton: whether the status card has the palette's button.
	paletteButton bool
}

func newFakeApp() *fakeApp {
	return &fakeApp{
		dir:       "/films/我的影片.kando",
		libraries: []string{"/films/我的影片.kando", "/films/课程.kando"},
		counts:    map[string]int{"/films/课程.kando": 3},
		filter:    SourceAll,
		films:     4,
		bySource:  map[string]int{SourceAll: 4, SourceYouTube: 1, SourceBilibili: 1, SourceLocal: 2},
		ready:     Readiness{State: Ready, Message: locales.ReadinessReady},
	}
}

func (f *fakeApp) LibraryDir() string       { return f.dir }
func (f *fakeApp) KnownLibraries() []string { return f.libraries }
func (f *fakeApp) SaveErr() string          { return f.saveErr }
func (f *fakeApp) Page() string             { return f.page }
func (f *fakeApp) Select(page string)       { f.calls = append(f.calls, "select "+page); f.page = page }
func (f *fakeApp) Choose(dir string)        { f.calls = append(f.calls, "choose "+dir); f.dir = dir }
func (f *fakeApp) BeginNew()                { f.calls = append(f.calls, "new") }
func (f *fakeApp) PickLibrary()             { f.calls = append(f.calls, "pick") }
func (f *fakeApp) Reveal(path string)       { f.calls = append(f.calls, "reveal "+path) }
func (f *fakeApp) Forget(dir string)        { f.calls = append(f.calls, "forget "+dir) }
func (f *fakeApp) FilmCount() int           { return f.films }
func (f *fakeApp) SourceFilter() string     { return f.filter }
func (f *fakeApp) SourceCount(s string) int { return f.bySource[s] }
func (f *fakeApp) Query() *string           { return &f.query }
func (f *fakeApp) SetQuery(text string)     { f.query = text }
func (f *fakeApp) QueryFocus() *bool        { return &f.focus }
func (f *fakeApp) SubmitQuery()             { f.calls = append(f.calls, "submit "+f.query) }
func (f *fakeApp) Readiness() Readiness     { return f.ready }
func (f *fakeApp) PaletteButton() bool      { return f.paletteButton }
func (f *fakeApp) PaletteKey() string       { return "mod+k" }
func (f *fakeApp) TogglePalette()           { f.calls = append(f.calls, "palette") }
func (f *fakeApp) OpenSettings(section string) {
	f.calls = append(f.calls, "settings "+section)
}
func (f *fakeApp) SetSourceFilter(source string) {
	f.calls = append(f.calls, "filter "+source)
	f.page, f.filter = PageFilms, source
}
func (f *fakeApp) LibraryCount(dir string) (int, bool) {
	n, ok := f.counts[dir]
	return n, ok
}

// start shows the sidebar of a as the main window does, on a Mac, in testlocale's language.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: []core.Node{View{App: a}}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
}

// change runs fn on the app's state as a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func count(t *testing.T, app *nsgotest.App, source string) string {
	t.Helper()
	return app.Find("id:count-" + source).Text
}

func TestSourcesListTheirCountsInOrder(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	want := []struct{ label, source, count string }{
		{msgs.SidebarSourceAll(), SourceAll, "4"}, {"YouTube", SourceYouTube, "1"}, {msgs.SidebarSourceBilibili(), SourceBilibili, "1"}, {msgs.SidebarSourceLocal(), SourceLocal, "2"},
	}
	texts := app.Texts()
	last := -1
	for _, row := range want {
		if got := count(t, app, row.source); got != row.count {
			t.Errorf("%s shows %q, want %q", row.label, got, row.count)
		}
		at := slices.Index(texts, row.label)
		if at < last {
			t.Errorf("%s is out of order in %q", row.label, texts)
		}
		last = at
	}
	if !app.Exists(`text:"` + msgs.SidebarBrowse() + `"`) {
		t.Errorf("no %s row in %q", msgs.SidebarBrowse(), texts)
	}
	// 其他网站 shows only while the library holds a film from another site, or while it is the source shown.
	web := `text:"` + msgs.SidebarSourceWeb() + `"`
	if app.Exists(web) {
		t.Fatalf("%s shown without a film from another site: %q", msgs.SidebarSourceWeb(), app.Texts())
	}
	change(app, a, func() { a.filter = SourceWeb })
	if got := count(t, app, SourceWeb); got != "0" {
		t.Fatalf("%s shows %q while chosen", msgs.SidebarSourceWeb(), got)
	}
	change(app, a, func() { a.filter, a.bySource[SourceWeb], a.bySource[SourceAll] = SourceAll, 1, 5 })
	if got, all := count(t, app, SourceWeb), count(t, app, SourceAll); got != "1" || all != "5" {
		t.Fatalf("%s shows %q and %s %q, want 1 and 5", msgs.SidebarSourceWeb(), got, msgs.SidebarSourceAll(), all)
	}
}

func TestChoosingASourceFiltersTheLibrary(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Press("id:source-" + SourceYouTube)
	app.Press("id:source-" + SourceYouTube)
	app.Press("id:source-" + PageBrowse)
	if want := []string{"filter " + SourceYouTube, "select " + PageBrowse}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q: the source already chosen does nothing", a.calls, want)
	}
	// Off the library and the browser, as on a settings page, no row is chosen; the arrow keys then start from the first.
	change(app, a, func() { a.page = PageGeneral })
	for _, id := range []string{SourceAll, PageBrowse, SourceYouTube} {
		if v := app.Find("id:source-" + id); v.Props.Selected {
			t.Errorf("%s is chosen on a settings page", id)
		}
	}
	change(app, a, func() { a.page, a.filter = PageFilms, SourceLocal })
	if !app.Find("id:source-" + SourceLocal).Props.Selected {
		t.Errorf("%s is not chosen while the library shows it", SourceLocal)
	}
}

func TestSwitcherMenuListsTheLibrariesWithCounts(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if got := app.Find("id:sidebar-switcher").Props.Label; got != msgs.SidebarSwitcherLabel("我的影片") {
		t.Fatalf("switcher labelled %q", got)
	}
	app.Press("id:sidebar-switcher")
	var labels []string
	for _, it := range app.PopupMenu {
		labels = append(labels, it.Label)
	}
	want := []string{msgs.SidebarLibraryCount("我的影片", 4), msgs.SidebarLibraryCount("课程", 3), "", msgs.SidebarNewLibrary(), msgs.SidebarOpenLibrary(), msgs.SidebarRevealLibrary("我的影片", "macOS")}
	if !slices.Equal(labels, want) {
		t.Fatalf("switcher menu %q, want %q", labels, want)
	}
	if !app.PopupMenu[0].Checked || app.PopupMenu[1].Checked {
		t.Errorf("the open library is not the one checked: %+v", app.PopupMenu[:2])
	}
	app.ChoosePopup(msgs.SidebarLibraryCount("课程", 3))
	if !slices.Equal(a.calls, []string{"choose /films/课程.kando"}) {
		t.Fatalf("calls %q after choosing 课程", a.calls)
	}
	if got := app.Find("id:sidebar-switcher").Props.Label; got != msgs.SidebarSwitcherLabel("课程") {
		t.Fatalf("switcher labelled %q after the switch", got)
	}
}

func TestSearchShowsWithFilmsAndTakesTheFocus(t *testing.T) {
	a := newFakeApp()
	a.films = 0
	a.focus = true
	app := start(t, a)
	if app.Exists("id:sidebar-search") {
		t.Fatal("search field shown for an empty library")
	}
	if a.focus {
		t.Fatal("the focus request outlived the hidden field")
	}
	change(app, a, func() { a.films = 4 })
	if app.Find("id:sidebar-search").Focused {
		t.Fatal("the field took a focus asked for while it was hidden")
	}
	change(app, a, func() { a.focus = true })
	if !app.Find("id:sidebar-search").Focused || a.focus {
		t.Fatalf("⌘F's request did not focus the field: focused %v, request %v", app.Find("id:sidebar-search").Focused, a.focus)
	}
	app.Type("id:sidebar-search", "talk")
	if a.query != "talk" {
		t.Fatalf("query %q after typing", a.query)
	}
	app.Press("id:sidebar-search-clear")
	if a.query != "" || app.Find("id:sidebar-search").Text != "" {
		t.Fatalf("query %q, field %q after the clear button", a.query, app.Find("id:sidebar-search").Text)
	}
	app.Type("id:sidebar-search", "again")
	app.Key("escape")
	if a.query != "" {
		t.Fatalf("query %q after Escape", a.query)
	}
	// Return in the field reports the search, which the app plays when it leaves a single film.
	a.calls = nil
	app.Type("id:sidebar-search", "rivers\n")
	if !slices.Equal(a.calls, []string{"submit rivers"}) {
		t.Fatalf("Return in the field called %q", a.calls)
	}
}

func TestStatusLineSaysWhatIsMissing(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if got := app.Find("id:sidebar-status").Text; got != msgs.ReadinessReady() {
		t.Fatalf("status %q, want %q", got, msgs.ReadinessReady())
	}
	if got := app.Find("id:sidebar-status-dot").Style.Background; got != readyDot {
		t.Errorf("dot %v while ready, want green %v", got, readyDot)
	}
	change(app, a, func() {
		a.ready = Readiness{State: Missing, Message: locales.PrecheckNoSpeechModel, Section: "speech"}
	})
	if got := app.Find("id:sidebar-status").Text; got != msgs.PrecheckNoSpeechModel() {
		t.Fatalf("status %q, want %q", got, msgs.PrecheckNoSpeechModel())
	}
	if got := app.Find("id:sidebar-status-dot").Style.Background; got != missingDot {
		t.Errorf("dot %v while missing, want orange %v", got, missingDot)
	}
	app.Press("id:sidebar-settings")
	if !slices.Equal(a.calls, []string{"settings speech"}) {
		t.Fatalf("the gear called %q", a.calls)
	}
}

func TestSaveErrorShowsUnderTheSources(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	change(app, a, func() { a.saveErr = "disk full" })
	if !app.Exists(`text:"disk full"`) {
		t.Fatalf("save error not shown: %q", app.Texts())
	}
	change(app, a, func() { a.saveErr = "" })
	if app.Exists(`text:"disk full"`) {
		t.Fatal("save error still shown once cleared")
	}
}

// The sidebar says itself in English for an English app, and follows the language as the app changes it, keeping what the search field holds.
func TestSidebarFollowsTheLanguage(t *testing.T) {
	testlocale.Use(t, "en")
	a := newFakeApp()
	app := start(t, a)
	for _, want := range []string{"All Films", "Browse"} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("no %q in %q", want, app.Texts())
		}
	}
	app.Type("id:sidebar-search", "talk")
	field := app.Find("id:sidebar-search").ID
	ja := locales.In("ja")
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true, Locale: "ja"})
	if !app.Exists(`text:"` + ja.SidebarSourceAll() + `"`) {
		t.Fatalf("no %q after the switch to Japanese: %q", ja.SidebarSourceAll(), app.Texts())
	}
	if v := app.Find("id:sidebar-search"); v.ID != field || v.Text != "talk" {
		t.Fatalf("the search field was built again: view %d holding %q", v.ID, v.Text)
	}
}

func TestTitleIsTheFolderWithoutItsExtension(t *testing.T) {
	for dir, want := range map[string]string{
		filepath.Join("films", "我的影片.kando"): "我的影片",
		filepath.Join("films", "Plain"):      "Plain",
		string(filepath.Separator):           string(filepath.Separator),
	} {
		if got := Title(dir); got != want {
			t.Errorf("Title(%q) = %q, want %q", dir, got, want)
		}
	}
}

// TestStatusCardOpensThePalette checks that the status card carries the palette's button only while PaletteButton holds, named for what it opens, and that it opens the palette.
func TestStatusCardOpensThePalette(t *testing.T) {
	a := newFakeApp()
	if app := start(t, a); app.Exists("id:sidebar-palette") {
		t.Fatal("the palette's button shows without PaletteButton")
	}
	a.paletteButton = true
	app := start(t, a)
	if got := app.Find("id:sidebar-palette").Props.Label; got != testlocale.Messages().SidebarCommands() {
		t.Fatalf("the button is named %q", got)
	}
	app.Press("id:sidebar-palette")
	if !slices.Equal(a.calls, []string{"palette"}) {
		t.Fatalf("the button called %q", a.calls)
	}
}
