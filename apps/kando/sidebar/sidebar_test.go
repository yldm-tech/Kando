package sidebar

import (
	"path/filepath"
	"slices"
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"

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
	jobs      Jobs
	counts    map[string]int
	page      string
	filter    string
	films     int
	bySource  map[string]int
	spoken    string
	bySpoken  []SpokenCount
	watch     string
	byWatch   WatchCounts
	query     string
	focus     bool
	ready     Readiness
	saveErr   string
	calls     []string
	// paletteButton is PaletteButton: whether the status card has the palette's button.
	paletteButton bool
	// passwords are the libraries with a password, and locked those of them that are locked.
	passwords, locked map[string]bool
	// mediaSources is MediaSourcesShown: whether a media source was added.
	mediaSources bool
}

func newFakeApp() *fakeApp {
	return &fakeApp{
		dir:       "/films/我的影片.kando",
		libraries: []string{"/films/我的影片.kando", "/films/课程.kando"},
		counts:    map[string]int{"/films/课程.kando": 3},
		filter:    SourceAll,
		spoken:    SpokenAll,
		watch:     WatchAll,
		films:     4,
		bySource:  map[string]int{SourceAll: 4, SourceYouTube: 1, SourceBilibili: 1, SourceLocal: 2},
		ready:     Readiness{State: Ready, Message: locales.ReadinessReady},
	}
}

func (f *fakeApp) LibraryDir() string          { return f.dir }
func (f *fakeApp) KnownLibraries() []string    { return f.libraries }
func (f *fakeApp) SaveErr() string             { return f.saveErr }
func (f *fakeApp) Page() string                { return f.page }
func (f *fakeApp) Select(page string)          { f.calls = append(f.calls, "select "+page); f.page = page }
func (f *fakeApp) Choose(dir string)           { f.calls = append(f.calls, "choose "+dir); f.dir = dir }
func (f *fakeApp) BeginNew()                   { f.calls = append(f.calls, "new") }
func (f *fakeApp) PickLibrary()                { f.calls = append(f.calls, "pick") }
func (f *fakeApp) Reveal(path string)          { f.calls = append(f.calls, "reveal "+path) }
func (f *fakeApp) Forget(dir string)           { f.calls = append(f.calls, "forget "+dir) }
func (f *fakeApp) FilmCount() int              { return f.films }
func (f *fakeApp) SourceFilter() string        { return f.filter }
func (f *fakeApp) SourceCount(s string) int    { return f.bySource[s] }
func (f *fakeApp) Query() *string              { return &f.query }
func (f *fakeApp) SetQuery(text string)        { f.query = text }
func (f *fakeApp) QueryFocus() *bool           { return &f.focus }
func (f *fakeApp) SubmitQuery()                { f.calls = append(f.calls, "submit "+f.query) }
func (f *fakeApp) Readiness() Readiness        { return f.ready }
func (f *fakeApp) PaletteButton() bool         { return f.paletteButton }
func (f *fakeApp) PaletteKey() string          { return "mod+k" }
func (f *fakeApp) TogglePalette()              { f.calls = append(f.calls, "palette") }
func (f *fakeApp) Jobs() Jobs                  { return f.jobs }
func (f *fakeApp) ShowJobs()                   { f.calls = append(f.calls, "jobs") }
func (f *fakeApp) MediaSourcesShown() bool     { return f.mediaSources }
func (f *fakeApp) SpokenCounts() []SpokenCount { return f.bySpoken }
func (f *fakeApp) SpokenFilter() string        { return f.spoken }
func (f *fakeApp) OpenSettings(section string) {
	f.calls = append(f.calls, "settings "+section)
}
func (f *fakeApp) SetSourceFilter(source string) {
	f.calls = append(f.calls, "filter "+source)
	f.page, f.filter = PageFilms, source
}
func (f *fakeApp) WatchCounts() WatchCounts { return f.byWatch }
func (f *fakeApp) WatchFilter() string      { return f.watch }
func (f *fakeApp) SetWatchFilter(state string) {
	f.calls = append(f.calls, "watch "+state)
	f.page, f.watch = PageFilms, state
}
func (f *fakeApp) SetSpokenFilter(code string) {
	f.calls = append(f.calls, "spoken "+code)
	f.page, f.spoken = PageFilms, code
}
func (f *fakeApp) LibraryCount(dir string) (int, bool) {
	n, ok := f.counts[dir]
	return n, ok
}

func (f *fakeApp) LibraryHasPassword(dir string) bool { return f.passwords[dir] }
func (f *fakeApp) LibraryLocked(dir string) bool      { return f.locked[dir] }
func (f *fakeApp) SetPassword(dir string)             { f.calls = append(f.calls, "set password "+dir) }
func (f *fakeApp) ChangePassword(dir string)          { f.calls = append(f.calls, "change password "+dir) }
func (f *fakeApp) RemovePassword(dir string)          { f.calls = append(f.calls, "remove password "+dir) }
func (f *fakeApp) LockNow()                           { f.calls = append(f.calls, "lock now") }

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

// windowFrame is v's frame in the window's points, as its ancestors and their scroll offsets place it.
func windowFrame(v *nsgotest.View) core.Bounds {
	b := v.Frame
	for p := v.Parent; p != nil; p = p.Parent {
		b.Origin.X += p.Frame.Origin.X - p.Offset.X
		b.Origin.Y += p.Frame.Origin.Y - p.Offset.Y
	}
	return b
}

// TestLanguagesShowWithMoreThanOne checks that the sidebar lists the languages heard in the library's films, each with its count, under 全部语言, right under the sources, only while there are films in more than one, and that choosing one filters the library by it, 全部语言 by none, with the row chosen following the filter on the library page only.
func TestLanguagesShowWithMoreThanOne(t *testing.T) {
	a := newFakeApp()
	a.bySpoken = []SpokenCount{{Code: "ja", Count: 3}}
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:sidebar-languages") {
		t.Fatalf("languages listed with films heard in one: %q", app.Texts())
	}
	change(app, a, func() { a.bySpoken = []SpokenCount{{Code: "ja", Count: 3}, {Code: "en", Count: 1}} })
	if !app.Exists(`text:"` + msgs.SidebarLanguages() + `"`) {
		t.Fatalf("no %s heading in %q", msgs.SidebarLanguages(), app.Texts())
	}
	texts := app.Texts()
	all, ja, en := slices.Index(texts, msgs.SidebarLanguagesAll()), slices.Index(texts, msgs.SettingsSpeechLanguageJa()), slices.Index(texts, msgs.SettingsSpeechLanguageEn())
	if all < 0 || ja < all || en < ja {
		t.Fatalf("languages out of order in %q", texts)
	}
	if got, want := app.Find("id:spoken-count-ja").Text, "3"; got != want {
		t.Fatalf("日语 shows %q, want %q", got, want)
	}
	if got, want := app.Find("id:spoken-count-en").Text, "1"; got != want {
		t.Fatalf("英语 shows %q, want %q", got, want)
	}
	if app.Exists("id:spoken-count-" + SpokenAll) {
		t.Fatal("全部语言 shows a count")
	}
	// The languages follow the last source, with no room between them.
	local, heading := windowFrame(app.Find("id:source-"+SourceLocal)), windowFrame(app.Find(`text:"`+msgs.SidebarLanguages()+`"`))
	if gap := heading.Origin.Y - (local.Origin.Y + local.Size.Height); gap < 0 || gap > 40 {
		t.Fatalf("the languages start %v below the last source", gap)
	}
	if !app.Find("id:spoken-" + SpokenAll).Props.Selected {
		t.Fatal("全部语言 is not chosen while no language is")
	}
	app.Press("id:spoken-ja")
	app.Press("id:spoken-ja")
	app.Press("id:spoken-" + SpokenAll)
	if want := []string{"spoken ja", "spoken " + SpokenAll}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q: the language already chosen does nothing", a.calls, want)
	}
	if a.filter != SourceAll {
		t.Fatalf("choosing a language changed the source to %q", a.filter)
	}
	change(app, a, func() { a.spoken = "en" })
	if !app.Find("id:spoken-en").Props.Selected || app.Find("id:spoken-"+SpokenAll).Props.Selected {
		t.Fatal("英语 is not the row chosen while the library shows it")
	}
	change(app, a, func() { a.page = PageGeneral })
	if app.Find("id:spoken-en").Props.Selected {
		t.Fatal("a language is chosen on a settings page")
	}
	change(app, a, func() { a.bySpoken = a.bySpoken[:1] })
	if app.Exists("id:sidebar-languages") {
		t.Fatal("languages still listed once the films are heard in one")
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
	want := []string{msgs.SidebarLibraryCount("我的影片", 4), msgs.SidebarLibraryCount("课程", 3), "", msgs.SidebarNewLibrary(), msgs.SidebarOpenLibrary(), msgs.SidebarRevealLibrary("我的影片", "macOS"), "", msgs.LockMenuSet()}
	if !slices.Equal(labels, want) {
		t.Fatalf("switcher menu %q, want %q", labels, want)
	}
	if !app.PopupMenu[0].Checked || app.PopupMenu[1].Checked || !app.PopupMenu[0].Radio || !app.PopupMenu[1].Radio {
		t.Errorf("the libraries are not radio items with the open one chosen: %+v", app.PopupMenu[:2])
	}
	app.ChoosePopup(msgs.SidebarLibraryCount("课程", 3))
	if !slices.Equal(a.calls, []string{"choose /films/课程.kando"}) {
		t.Fatalf("calls %q after choosing 课程", a.calls)
	}
	if got := app.Find("id:sidebar-switcher").Props.Label; got != msgs.SidebarSwitcherLabel("课程") {
		t.Fatalf("switcher labelled %q after the switch", got)
	}
}

// A library with a password offers to change or remove it, and to lock it while it is open and unlocked; a locked library shows its name with a lock and no count, in the menu and on the switcher.
func TestSwitcherMenuShowsPasswordsAndLockedLibraries(t *testing.T) {
	a := newFakeApp()
	a.passwords = map[string]bool{"/films/我的影片.kando": true, "/films/课程.kando": true}
	a.locked = map[string]bool{"/films/课程.kando": true}
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:sidebar-switcher")
	var labels []string
	for _, it := range app.PopupMenu {
		labels = append(labels, it.Label)
	}
	want := []string{msgs.SidebarLibraryCount("我的影片", 4), msgs.LockLibraryLocked("课程"), "", msgs.SidebarNewLibrary(), msgs.SidebarOpenLibrary(), msgs.SidebarRevealLibrary("我的影片", "macOS"), "", msgs.LockMenuChange(), msgs.LockMenuRemove(), msgs.LockNow()}
	if !slices.Equal(labels, want) {
		t.Fatalf("switcher menu %q, want %q", labels, want)
	}
	app.ChoosePopup(msgs.LockNow())
	app.Press("id:sidebar-switcher")
	app.ChoosePopup(msgs.LockMenuChange())
	app.Press("id:sidebar-switcher")
	app.ChoosePopup(msgs.LockMenuRemove())
	if want := []string{"lock now", "change password /films/我的影片.kando", "remove password /films/我的影片.kando"}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
	change(app, a, func() { a.locked["/films/我的影片.kando"] = true })
	if got := app.Find("id:sidebar-switcher").Props.Label; got != msgs.SidebarSwitcherLabel(msgs.LockLibraryLocked("我的影片")) {
		t.Fatalf("a locked library's switcher labelled %q", got)
	}
	app.Press("id:sidebar-switcher")
	labels = labels[:0]
	for _, it := range app.PopupMenu {
		labels = append(labels, it.Label)
	}
	if slices.Contains(labels, msgs.LockNow()) || labels[0] != msgs.LockLibraryLocked("我的影片") {
		t.Fatalf("the menu of a locked open library: %q", labels)
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
	if app.Find("id:sidebar-search").Props.Role != core.RoleSearchField {
		t.Fatal("the library search is not a search field to assistive technology")
	}
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

// The status line counts the queue's jobs while it holds any, red once one failed, so a job that ends while the player, the settings or the browser is shown is seen; the count opens 处理中.
func TestStatusLineCountsTheJobs(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:sidebar-jobs") {
		t.Fatal("the status line counts jobs while there are none")
	}
	change(app, a, func() { a.jobs = Jobs{Active: 2} })
	if !app.Exists(`text:"` + msgs.SidebarJobsActive(2) + `"`) {
		t.Fatalf("no count of the jobs: %q", app.Texts())
	}
	change(app, a, func() { a.jobs = Jobs{Active: 1, Failed: 1} })
	if !app.Exists(`text:"` + msgs.SidebarJobsActive(1) + " · " + msgs.SidebarJobsFailed(1) + `"`) {
		t.Fatalf("no count of the failed job: %q", app.Texts())
	}
	app.Press("id:sidebar-jobs")
	if !slices.Equal(a.calls, []string{"jobs"}) {
		t.Fatalf("the count called %q", a.calls)
	}
	change(app, a, func() { a.jobs = Jobs{} })
	if app.Exists("id:sidebar-jobs") {
		t.Fatal("the count stayed once the queue was empty")
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

// TestLanguageTagsFollowTheAppearance checks that a language's code (JA) is drawn in the theme's accent ink in both appearances, which reads at its 9 points on the dark sidebar where the accent's own blue does not.
func TestLanguageTagsFollowTheAppearance(t *testing.T) {
	a := newFakeApp()
	a.bySpoken = []SpokenCount{{Code: "ja", Count: 3}, {Code: "en", Count: 1}}
	var k theme.Colors
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		theme.Apply(cx)
		k = theme.Of(cx)
		return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: []core.Node{View{App: a}}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
	tag := func() core.Color { return app.Find("id:spoken-ja >> " + `text:"JA"`).TextStyle.Color }
	if got := tag(); got != k.AccentInk() {
		t.Fatalf("JA is %#x on the light sidebar, want the accent ink %#x", uint32(got), uint32(k.AccentInk()))
	}
	env.Dark = true
	app.SetEnvironment(env)
	if got := tag(); got != k.AccentInk() || got == theme.Accent {
		t.Fatalf("JA is %#x on the dark sidebar, want the dark accent ink %#x", uint32(got), uint32(k.AccentInk()))
	}
}

// TestSidebarStopsShowTheKeyboardAndThePointer checks that the library switcher, the window's first Tab stop, shows the focus ring as the other stops of the sidebar do, that the jobs line is shaded under the pointer as the switcher is, and that the switcher's and the search field's fills line up with the source rows', with the rows' corners.
func TestSidebarStopsShowTheKeyboardAndThePointer(t *testing.T) {
	a := newFakeApp()
	a.jobs = Jobs{Active: 1}
	app := start(t, a)
	app.Key("tab")
	switcher := app.Find("id:sidebar-switcher")
	if !switcher.Focused {
		t.Fatal("Tab did not put the keyboard on the library switcher")
	}
	if switcher.Style.Outline.Width == 0 {
		t.Fatal("the library switcher shows no focus ring with the keyboard on it")
	}

	if app.Find("id:sidebar-jobs").Style.Background != 0 {
		t.Fatal("the jobs line is shaded before the pointer is over it")
	}
	app.Hover("id:sidebar-jobs")
	if app.Find("id:sidebar-jobs").Style.Background == 0 {
		t.Fatal("the jobs line is not shaded under the pointer")
	}
	app.HoverOut()

	frame := func(selector string) core.Bounds {
		t.Helper()
		v := app.Find(selector)
		b := v.Frame
		for p := v.Parent; p != nil; p = p.Parent {
			b.Origin.X += p.Frame.Origin.X - p.Offset.X
			b.Origin.Y += p.Frame.Origin.Y - p.Offset.Y
		}
		return b
	}
	row := frame("id:source-" + SourceAll)
	for _, selector := range []string{"id:sidebar-switcher", "id:sidebar-search"} {
		if b := frame(selector); b.Origin.X != row.Origin.X || b.Size.Width != row.Size.Width {
			t.Fatalf("%s spans x %v to %v, the source rows %v to %v", selector, b.Origin.X, b.Origin.X+b.Size.Width, row.Origin.X, row.Origin.X+row.Size.Width)
		}
	}
	if r, want := switcher.Style.CornerRadius, app.Find("id:source-"+SourceAll).Style.CornerRadius; r != want {
		t.Fatalf("the switcher's corners are %v, the source rows' %v", r, want)
	}
}

// TestRowsSayTheirCounts checks that a screen reader hears the count a source or a language row draws at its end with its name, and only the name of a row without a count.
func TestRowsSayTheirCounts(t *testing.T) {
	a := newFakeApp()
	a.bySpoken = []SpokenCount{{Code: "ja", Count: 3}, {Code: "en", Count: 1}}
	app := start(t, a)
	msgs := testlocale.Messages()
	for selector, want := range map[string]string{
		"id:source-" + SourceAll:     msgs.SidebarRowCount(msgs.SidebarSourceAll(), 4),
		"id:source-" + SourceYouTube: msgs.SidebarRowCount("YouTube", 1),
		"id:source-" + PageBrowse:    msgs.SidebarBrowse(),
		"id:spoken-ja":               msgs.SidebarRowCount(msgs.SettingsSpeechLanguageJa(), 3),
		"id:spoken-" + SpokenAll:     msgs.SidebarLanguagesAll(),
	} {
		if got := app.Find(selector).Props.Label; got != want {
			t.Errorf("%s is named %q, want %q", selector, got, want)
		}
	}
	change(app, a, func() { a.bySource[SourceYouTube] = 2 })
	if got, want := app.Find("id:source-"+SourceYouTube).Props.Label, msgs.SidebarRowCount("YouTube", 2); got != want {
		t.Fatalf("YouTube is named %q after its count changed, want %q", got, want)
	}
}

// TestWatchStatesFilterTheLibrary checks that the sidebar lists the watch states under the languages while the library has done films, each but 全部 with its count, even at 0, and that choosing one filters the library by it, 全部 by none, with the row chosen following the filter on the library page only.
func TestWatchStatesFilterTheLibrary(t *testing.T) {
	a := newFakeApp()
	a.bySpoken = []SpokenCount{{Code: "ja", Count: 3}, {Code: "en", Count: 1}}
	a.byWatch = WatchCounts{Unwatched: 2, Watched: 2}
	app := start(t, a)
	msgs := testlocale.Messages()
	texts := app.Texts()
	heading, all, unwatched, inProgress, watched := slices.Index(texts, msgs.SidebarWatched()), slices.Index(texts, msgs.SidebarWatchAll()),
		slices.Index(texts, msgs.SidebarWatchUnwatched()), slices.Index(texts, msgs.SidebarWatchInProgress()), slices.Index(texts, msgs.SidebarWatchWatched())
	if heading < slices.Index(texts, msgs.SidebarLanguagesAll()) || all < heading || unwatched < all || inProgress < unwatched || watched < inProgress {
		t.Fatalf("watch states out of order, or not under the languages, in %q", texts)
	}
	for state, want := range map[string]string{WatchUnwatched: "2", WatchInProgress: "0", WatchWatched: "2"} {
		if got := app.Find("id:watch-count-" + state).Text; got != want {
			t.Fatalf("%s shows %q, want %q", state, got, want)
		}
	}
	if app.Exists("id:watch-count-" + WatchAll) {
		t.Fatal("全部 shows a count")
	}
	if !app.Find("id:watch-" + WatchAll).Props.Selected {
		t.Fatal("全部 is not chosen while no watch state is")
	}
	app.Press("id:watch-" + WatchInProgress)
	app.Press("id:watch-" + WatchInProgress)
	app.Press("id:watch-" + WatchAll)
	if want := []string{"watch " + WatchInProgress, "watch " + WatchAll}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q: the state already chosen does nothing", a.calls, want)
	}
	if a.filter != SourceAll || a.spoken != SpokenAll {
		t.Fatalf("choosing a watch state changed the source to %q, the language to %q", a.filter, a.spoken)
	}
	change(app, a, func() { a.watch = WatchWatched })
	if !app.Find("id:watch-"+WatchWatched).Props.Selected || app.Find("id:watch-"+WatchAll).Props.Selected {
		t.Fatal("已看完 is not the row chosen while the library shows it")
	}
	change(app, a, func() { a.page = PageGeneral })
	if app.Find("id:watch-" + WatchWatched).Props.Selected {
		t.Fatal("a watch state is chosen on a settings page")
	}
	// A library with no done film has nothing to filter.
	change(app, a, func() { a.bySource[SourceAll] = 0 })
	if app.Exists("id:sidebar-watched") {
		t.Fatal("watch states listed in a library with no done film")
	}
}

// TestSwitcherAndStatusAreCards checks the design's two cards on the sidebar's: the library switcher on a face of the card's color at 60%, darker while its menu is open, and the status line at the foot on one at 55% with 11-point corners, each in both appearances.
func TestSwitcherAndStatusAreCards(t *testing.T) {
	a := newFakeApp()
	var k theme.Colors
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		theme.Apply(cx)
		k = theme.Of(cx)
		return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: []core.Node{View{App: a}}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
	for _, dark := range []bool{false, true} {
		env.Dark = dark
		app.SetEnvironment(env)
		if got, want := app.Find("id:sidebar-switcher").Style.Background, k.Card().Faded(0.6); got != want {
			t.Errorf("dark %v: switcher face %#x, want the card's at 60%% %#x", dark, uint32(got), uint32(want))
		}
		status := app.Find("id:sidebar-status-card")
		if got, want := status.Style.Background, k.Card().Faded(0.55); got != want || status.Style.CornerRadius != 11 {
			t.Errorf("dark %v: status card face %#x corners %v, want %#x and 11", dark, uint32(got), status.Style.CornerRadius, uint32(want))
		}
	}
}

// 媒体来源 shows at the end of the list only while a media source was added, without a count, and shows its page.
func TestMediaSourcesRowShowsTheirPage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	if app.Exists("id:source-" + PageMediaSources) {
		t.Fatal("媒体来源 shows without a source")
	}
	change(app, a, func() { a.mediaSources = true })
	if !app.Exists("id:source-"+PageMediaSources) || app.Exists("id:count-"+PageMediaSources) {
		t.Fatalf("the row is missing or counted: %q", app.Texts())
	}
	app.Press("id:source-" + PageMediaSources)
	if want := []string{"select " + PageMediaSources}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
	change(app, a, func() { a.page = PageMediaSources })
	if !app.Find("id:source-" + PageMediaSources).Props.Selected {
		t.Fatal("the row is not chosen while its page shows")
	}
}

// The search field's name follows the interface's language as it changes, and the field is not built again for it: it keeps the keyboard focus and what it holds.
func TestSearchFollowsTheLanguage(t *testing.T) {
	a := newFakeApp()
	a.films = 4
	a.focus = true
	app := start(t, a)
	app.Type("id:sidebar-search", "talk")
	field := app.Find("id:sidebar-search")
	testlocale.Use(t, "en")
	app.SetEnvironment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true}))
	again := app.Find("id:sidebar-search")
	if want := testlocale.Messages().SidebarSearchLabel(); again.Props.Label != want {
		t.Errorf("the field is named %q in English, want %q", again.Props.Label, want)
	}
	if again.ID != field.ID || !again.Focused || again.Text != "talk" {
		t.Fatalf("the field after the switch: built again %v, focused %v, holding %q", again.ID != field.ID, again.Focused, again.Text)
	}
}
