package sites

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sites/catalog"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// fakeApp is the sheet's state as the app keeps it: the search, the logos it read, and whether the sheet is open.
type fakeApp struct {
	live.Rev
	query  string
	logos  map[string][]byte
	open   bool
	closed int
	opened []string
}

func (f *fakeApp) SitesQuery() *string { return &f.query }

func (f *fakeApp) OpenSite(s catalog.Site) { f.opened = append(f.opened, s.Domain) }

func (f *fakeApp) SiteLogo(s catalog.Site) []byte {
	if data, ok := f.logos[s.Domain]; ok {
		return data
	}
	data := catalog.Logo(s)
	if f.logos == nil {
		f.logos = map[string][]byte{}
	}
	f.logos[s.Domain] = data
	return data
}

// show opens the sheet over a window as the app does, on a Mac, in testlocale's language.
func show(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	a.open = true
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return core.Show{When: live.Func(a, func() bool { return a.open }), Then: func(*core.Scope) core.Node {
			return Sheet{App: a, OnClose: live.Do(a, func() { a.open = false; a.closed++ })}
		}}
	}, nsgotest.Size(1100, 760), nsgotest.Environment(env))
}

func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func whole(s string) string { return `text:"` + s + `"` }

// The sheet opens with the keyboard in the search and lists the featured sites first; 完成 closes it.
func TestSheetListsTheSites(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	for _, want := range []string{msgs.SitesTitle(), "YouTube", "youtube.com", msgs.SitesSource(catalog.Load().YTDLP)} {
		if !app.Exists(whole(want)) {
			t.Fatalf("no %q in %q", want, app.Texts())
		}
	}
	if !app.Find(msgs.SitesSearchLabel()).Focused {
		t.Fatal("the search does not have the keyboard")
	}
	app.Press("id:sites-done")
	if a.open || a.closed != 1 || app.Exists("id:sites") {
		t.Fatalf("完成 left the sheet open %v, closed %d", a.open, a.closed)
	}
}

// Typing narrows the grid; a link shows the one site it is on with how many were found, and a site off the list says so.
func TestSheetSearches(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	app.Type("id:sites-search", "niconico")
	if !app.Exists(whole("nicovideo.jp")) || app.Exists(whole("youtube.com")) {
		t.Fatalf("texts %q", app.Texts())
	}
	change(app, a, func() { a.query = "https://www.bilibili.com/video/BV1xx411c7mD" })
	if want := msgs.SitesFound(1, msgs.SitesSource(catalog.Load().YTDLP)); !app.Exists(whole(want)) || !app.Exists(whole("bilibili.com")) {
		t.Fatalf("no %q in %q", want, app.Texts())
	}
	change(app, a, func() { a.query = "https://missav.ai/dvaj-762" })
	if want := msgs.SitesNotFound("https://missav.ai/dvaj-762"); !app.Exists(whole(want)) || app.Exists("id:sites-grid") {
		t.Fatalf("no %q in %q", want, app.Texts())
	}
}

// The first Escape empties the search, as a search field's does; the next closes the sheet. The clear button empties it too.
func TestSheetEscape(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	app.Type("id:sites-search", "nico")
	app.Press(testlocale.Messages().SitesSearchClear())
	if a.query != "" {
		t.Fatalf("the clear button left %q", a.query)
	}
	app.Type("id:sites-search", "nico")
	app.Key("escape")
	if !a.open || a.query != "" {
		t.Fatalf("Escape in the search: open %v, query %q", a.open, a.query)
	}
	app.Key("escape")
	if a.open || a.closed != 1 {
		t.Fatalf("Escape left the sheet open %v, closed %d", a.open, a.closed)
	}
}

// A site with a logo shows it on a white plate, read once; one without shows its initial.
func TestSheetShowsLogosAndInitials(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	images := 0
	for _, v := range app.Query("role:image") {
		if len(v.Image.Source.Data) > 0 {
			images++
		}
	}
	if images == 0 {
		t.Fatalf("no logo shows\n%s", app.Dump())
	}
	for _, c := range []struct{ name, want string }{{"YouTube", "Y"}, {"9gag", "9"}, {"«Arte»", "A"}, {"", "?"}, {"…", "…"}} {
		if got := Initial(c.name); got != c.want {
			t.Errorf("Initial(%q) = %q, want %q", c.name, got, c.want)
		}
	}
	if plate("example.com") != plate("example.com") {
		t.Fatal("a plate's color changes")
	}
}

// What the sheet reads follows the app's whole revision; a change that leaves the search the same builds nothing again, so the search keeps the keyboard and what is typed in it.
func TestSheetStaysWhileTheAppChanges(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	app.Type("id:sites-search", "nico")
	field, sheet := app.Find("id:sites-search").ID, app.Find("id:sites").ID
	change(app, a, func() {})
	if v := app.Find("id:sites-search"); v.ID != field || app.Find("id:sites").ID != sheet || !v.Focused || v.Text != "nico" {
		t.Fatalf("the sheet was built again: field %d (was %d) focused %v holding %q", v.ID, field, v.Focused, v.Text)
	}
}

// A tile is a button that opens its site in the browser, the site the tile shows as the search narrows the grid.
func TestTileOpensItsSite(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	app.Press("label:" + msgs.SitesTileLabel("YouTube", "youtube.com"))
	change(app, a, func() { a.query = "ted" })
	app.Press("label:" + msgs.SitesTileLabel("TED", "ted.com"))
	if len(a.opened) != 2 || a.opened[0] != "youtube.com" || a.opened[1] != "ted.com" {
		t.Fatalf("opened %q", a.opened)
	}
}
