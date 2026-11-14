package films

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sidebar"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeApp is the shelf the page reads, kept as plain fields as the app keeps them, with the calls the page made.
type fakeApp struct {
	live.Rev
	dir, page, filter, query, source string
	making                           bool
	name                             string
	place                            string
	pathErr                          error
	saveErr                          string
	items, queue                     []Film
	durations                        map[string]time.Duration
	resume                           map[string]time.Duration
	remedies                         map[string]Remedy
	hints                            map[string]string
	notice                           Notice
	expanded, linkFocus              bool
	sort                             string
	removeOpen, removeFolder         bool
	removing                         Film
	renameOpen                       bool
	renameName, renameOriginal       string
	calls                            []string
}

func newFakeApp() *fakeApp {
	lib := "/films/我的影片.kando"
	return &fakeApp{
		dir: lib, page: sidebar.PageFilms, filter: sidebar.SourceAll, sort: SortAdded, place: "/films",
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
		remedies:  map[string]Remedy{},
		hints:     map[string]string{},
	}
}

func (f *fakeApp) call(format string, args ...string) {
	f.calls = append(f.calls, strings.Join(append([]string{format}, args...), " "))
}

func (f *fakeApp) LibraryDir() string     { return f.dir }
func (f *fakeApp) MakingLibrary() bool    { return f.making }
func (f *fakeApp) SaveErr() string        { return f.saveErr }
func (f *fakeApp) Page() string           { return f.page }
func (f *fakeApp) Films() []Film          { return f.items }
func (f *fakeApp) Source() *string        { return &f.source }
func (f *fakeApp) PickLibrary()           { f.call("pick-library") }
func (f *fakeApp) OpenSites()             { f.call("sites") }
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
func (f *fakeApp) Open(item Film)                     { f.call("open", item.Key) }
func (f *fakeApp) OpenAt(item Film, at time.Duration) { f.call("open-at", item.Key, at.String()) }
func (f *fakeApp) Reveal(path string)                 { f.call("reveal", path) }
func (f *fakeApp) Reprocess(item Film)                { f.call("reprocess", item.Key) }
func (f *fakeApp) TranslateAgain(item Film)           { f.call("translate-again", item.Key) }
func (f *fakeApp) TranslateMissing(item Film)         { f.call("translate-missing", item.Key) }
func (f *fakeApp) Remove(id string)                   { f.call("remove", id) }
func (f *fakeApp) Retry(id string)                    { f.call("retry", id) }
func (f *fakeApp) FailureHint(item Film) (string, string) {
	return f.hints[item.JobID], ""
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
func (f *fakeApp) Enqueue(...string) error            { return nil }
func (f *fakeApp) Cancel(id string)                   { f.call("cancel", id) }
func (f *fakeApp) RunningJob() string                 { return "running" }
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
func (f *fakeApp) Notice() *Notice                    { return &f.notice }
func (f *fakeApp) QueueExpanded() *bool               { return &f.expanded }
func (f *fakeApp) LibrarySort() string                { return f.sort }
func (f *fakeApp) SetLibrarySort(order string)        { f.call("sort", order); f.sort = order }
func (f *fakeApp) AskRemove(item Film) {
	f.call("ask-remove", item.Key)
	f.removing, f.removeOpen = item, true
}
func (f *fakeApp) Removing() (*bool, Film, bool) { return &f.removeOpen, f.removing, f.removeFolder }
func (f *fakeApp) CloseRemove()                  { f.call("close-remove"); f.removeOpen = false }
func (f *fakeApp) TrashFilm(item Film)           { f.call("trash", item.Key); f.removeOpen = false }
func (f *fakeApp) AskRename(item Film) {
	f.call("ask-rename", item.Key)
	f.renameOpen, f.renameName, f.renameOriginal = true, item.Name, "original"
}
func (f *fakeApp) Renaming() (*bool, *string, string) {
	return &f.renameOpen, &f.renameName, f.renameOriginal
}
func (f *fakeApp) CloseRename() { f.call("close-rename"); f.renameOpen = false }
func (f *fakeApp) RenameFilm()  { f.call("rename", f.renameName); f.renameOpen = false }

// testEnvironment is a Mac desktop in testlocale's language.
func testEnvironment() ns.Environment {
	return testlocale.Environment(ns.Environment{OS: ns.OSMacOS, Idiom: ns.IdiomDesktop, AppleKeyboard: true})
}

// start shows the library page of a as the main window's content column does, on a Mac, in testlocale's language.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		a.Attach(cx)
		return ns.View{Style: ns.Style{FlexGrow: 1}, Children: []ns.Node{View{App: a}}}
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
	for _, text := range []string{msgs.FilmsQueueTitle(), msgs.SourceAll(), "Rivers of Europe", "Linear Algebra", "Morning Market", "“河流塑造了大地”", "Rivers shape the land",
		msgs.FilmsHeroKickerBilingual(msgs.SettingsLanguageSimplifiedChinese()), msgs.FilmsGridCount(2)} {
		if !app.Exists(whole(text)) {
			t.Errorf("no %q in %q", text, app.Texts())
		}
	}
	if !app.Exists(whole("YouTube · 2:05")) {
		t.Errorf("the grid's meta lacks the length: %q", app.Texts())
	}
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
	want := []string{msgs.FilmsPlay(), msgs.FilmsShowInFinder("macOS"), msgs.FilmsRename(), msgs.FilmsReprocess(), msgs.FilmsTranslateAgain(), "", msgs.FilmsRemoveFromLibrary()}
	if !slices.Equal(labels, want) {
		t.Fatalf("menu %q, want %q", labels, want)
	}
	app.ChoosePopup(msgs.FilmsRename())
	wantCalls(t, a, "ask-rename done")
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

func TestNoticeOffersItsButtons(t *testing.T) {
	a := newFakeApp()
	a.notice = Notice{Kind: "warning", Text: "这个视频已在资料库中。", Section: "translate", Film: "done", Open: true}
	app := start(t, a)
	if !app.Exists(whole(a.notice.Text)) {
		t.Fatalf("no notice: %q", app.Texts())
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
	wantCalls(t, a, "start https://www.bilibili.com/video/BV1GJ411x7h7", "start https://www.bilibili.com/video/BV1GJ411x7h7", "pick-videos")
	change(app, a, func() { a.linkFocus = true })
	if !app.Find("id:films-link").Focused || a.linkFocus {
		t.Fatal("换个链接's request did not focus the link field")
	}
}

func TestDroppedFilesGoToTheApp(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.DropFiles("id:films-page", "file:///Users/me/Movies/talk.mp4", "https://example.com/not-a-file")
	wantCalls(t, a, "drop /Users/me/Movies/talk.mp4")
	if app.Exists("id:films-drop-zone") {
		t.Fatal("the drop zone stayed after the drop")
	}
}

func TestSortMenuChecksTheOrderShown(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:films-sort")
	if len(app.PopupMenu) != len(Sorts) || !app.PopupMenu[0].Checked || app.PopupMenu[1].Checked {
		t.Fatalf("sort menu %+v", app.PopupMenu)
	}
	app.ChoosePopup(msgs.FilmsSortTitle())
	wantCalls(t, a, "sort title")
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
	wantCalls(t, a, "remedy failed swap", "retry plain", "cancel running")
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

func TestPageFollowsTheLanguage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	if !app.Exists(whole(testlocale.Messages().FilmsStart())) {
		t.Fatalf("no %q", testlocale.Messages().FilmsStart())
	}
	testlocale.Use(t, "en")
	app.SetEnvironment(testEnvironment())
	en := testlocale.Messages()
	for _, text := range []string{en.FilmsStart(), en.FilmsQueueTitle(), en.SourceAll(), en.FilmsLocalVideo()} {
		if !app.Exists(whole(text)) {
			t.Errorf("no %q in English: %q", text, app.Texts())
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
