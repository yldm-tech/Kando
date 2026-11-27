package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The sidebar's own parts are tested in its package (sidebar/sidebar_test.go) against a fake App; these tests run it in the app, against the shelf, the readiness checks, the settings and the player.

// TestSidebarKeepsContentBelowTheTitleBar checks the drag band over the content by the rule contentBand follows: a page that draws its own header bar at its top (pageOwnsTitleBar) owns the room under the title bar and gets no band over it, so nothing on it moves down as far as a band would push it; any other page gets the band, which moves its top down by exactly the band's height and nothing on it further (what is centered in the room left moves half as far): the title bar's height, and on macOS no more than the sidebar card's top row, though the real window's title bar is 64 points tall with the traffic lights moved into that row. It reads only where the content's texts sit, not what the pages show, so it holds for whichever page draws a header bar: the library page beside the sidebar, and an open film that hides the sidebar. The sidebar clears the traffic lights in its own card (TestSidebarCardClearsTheTrafficLights).
func TestSidebarKeepsContentBelowTheTitleBar(t *testing.T) {
	// 64 is the title bar of the real macOS window, whose 14-point traffic lights sit at {24, 25}.
	tall := ns.Points(64)
	if runtime.GOOS == "darwin" {
		tall = sidebar.LightsRow
	}
	open := func(a *app) {
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
		a.openKey = "j"
	}
	for _, scene := range []struct {
		name  string
		setup func(*app)
	}{
		{"the library page", nil},
		{"an open film", open},
	} {
		for _, c := range []struct {
			bar  ns.Edges
			band ns.Points
		}{
			{ns.Edges{Top: 28, Left: 80}, 28},
			{ns.Edges{Top: 64, Left: 84}, tall},
		} {
			owns, top, shifts := contentShifts(t, c.bar, scene.setup)
			if !owns && abs32(shifts[top]-c.band) > 0.5 {
				t.Fatalf("%s draws no header bar of its own, yet its top text %q moved %v under a %v-high title bar, want the drag band's %v", scene.name, top, shifts[top], c.bar.Top, c.band)
			}
			for text, d := range shifts {
				if owns && d >= c.band-0.5 {
					t.Fatalf("%s draws its own header bar, yet %q moved %v under a %v-high title bar, as far as a %v-high drag band over it would push it", scene.name, text, d, c.bar.Top, c.band)
				}
				if !owns && d > c.band+0.5 {
					t.Fatalf("%s draws no header bar of its own, yet %q moved %v under a %v-high title bar, further than the drag band's %v", scene.name, text, d, c.bar.Top, c.band)
				}
			}
		}
	}
}

// contentShifts starts the window without a title bar and with bar, after setup, and returns whether the page in the content column draws its own header bar (pageOwnsTitleBar), the column's top text without a title bar, and how far down bar moved each text in that column, right of the sidebar while it shows.
func contentShifts(t *testing.T, bar ns.Edges, setup func(*app)) (owns bool, top string, shifts map[string]ns.Points) {
	t.Helper()
	plain, plainTT, _ := newTestApp(t, &fakePipeline{})
	barred, barredTT, _ := newTestApp(t, &fakePipeline{})
	useTitleBar(barredTT, bar)
	if setup != nil {
		setup(plain)
		setup(barred)
	}
	refresh(plainTT, plain)
	refresh(barredTT, barred)
	left := ns.Points(0)
	if plain.sidebarShown() {
		left = sidebar.Width
	}
	shifts = map[string]ns.Points{}
	topY := ns.Points(0)
	for _, text := range plainTT.Texts() {
		before := pageFrame(plainTT.Query(whole(text))[0])
		if before.Origin.X < left {
			continue
		}
		if top == "" || before.Origin.Y < topY {
			top, topY = text, before.Origin.Y
		}
		after := barredTT.Query(whole(text))
		if len(after) == 0 {
			t.Fatalf("%q shown without a title bar is missing under one: %q", text, barredTT.Texts())
		}
		shifts[text] = pageFrame(after[0]).Origin.Y - before.Origin.Y
	}
	if len(shifts) == 0 {
		t.Fatalf("no text right of x=%v in %q", left, plainTT.Texts())
	}
	return barred.pageOwnsTitleBar(), top, shifts
}

// sidebarCount is the count the sidebar shows on the row of source (sidebar.SourceAll and the others).
func sidebarCount(t *testing.T, tt *nsgotest.App, source string) string {
	t.Helper()
	return tt.Find("id:count-" + source).Text
}

// switcherLabel is the accessibility label of the library switcher while the library named name is open.
func switcherLabel(name string) string { return testlocale.Messages().SidebarSwitcherLabel(name) }

// chooseFromSwitcher opens the library switcher's menu and chooses item, or the library item whose name is item and whose count follows it.
func chooseFromSwitcher(t *testing.T, tt *nsgotest.App, a *app, item string) {
	t.Helper()
	refresh(tt, a)
	tt.Press("id:sidebar-switcher")
	label := ""
	for _, l := range popupLabels(tt) {
		if l == item || strings.HasPrefix(l, item+" · ") {
			label = l
		}
	}
	if label == "" {
		t.Fatalf("no %q in the switcher's menu %q", item, popupLabels(tt))
	}
	tt.ChoosePopup(label)
}

// applyUntil runs the background updates, as the engine would, until done reports true.
func applyUntil(t *testing.T, tt *nsgotest.App, a *app, updates chan func(), done func() bool) {
	t.Helper()
	waitUntil(t, tt, a, updates, done)
}

func TestSidebarListsTheSourcesWithCounts(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "yt", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: testVideos + "/yt-clip/video.mp4"},
		{ID: "bili", Source: "https://www.bilibili.com/video/BV1GJ411x7h7", Stage: core.StageComplete, VideoPath: testVideos + "/bili-clip/video.mp4"},
		{ID: "file", Source: "file:///videos/file-clip/video.mp4", Stage: core.StageComplete, VideoPath: testVideos + "/file-clip/video.mp4"},
	}
	a.edit(func(s *session) {
		s.Videos = []media.Video{{Name: "loose-clip.mp4", Path: testVideos + "/loose/loose-clip.mp4"}}
	})
	refresh(tt, a)
	want := []struct{ label, source, count string }{{"全部影片", sidebar.SourceAll, "4"}, {"YouTube", sidebar.SourceYouTube, "1"}, {"哔哩哔哩", sidebar.SourceBilibili, "1"}, {"本地文件", sidebar.SourceLocal, "2"}}
	last := -1
	for _, row := range want {
		if got := sidebarCount(t, tt, row.source); got != row.count {
			t.Errorf("%s shows %q, want %q", row.label, got, row.count)
		}
		at := slices.Index(tt.Texts(), row.label)
		if at < last {
			t.Errorf("%s is out of order in %q", row.label, tt.Texts())
		}
		last = at
	}
	// 其他网站 shows only while the library holds a film from another site, or while it is the source shown.
	if hasText(tt, "其他网站") {
		t.Fatalf("其他网站 shown without a film from another site: %q", tt.Texts())
	}
	a.side.filter = sidebar.SourceWeb
	refresh(tt, a)
	if got := sidebarCount(t, tt, sidebar.SourceWeb); got != "0" {
		t.Fatalf("其他网站 shows %q while chosen", got)
	}
	a.side.filter = sidebar.SourceAll
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "web", Source: "https://vimeo.com/76979871", Stage: core.StageComplete, VideoPath: testVideos + "/web-clip/video.mp4"})
	refresh(tt, a)
	if got := sidebarCount(t, tt, sidebar.SourceWeb); got != "1" {
		t.Fatalf("其他网站 shows %q", got)
	}
	texts := tt.Texts()
	if web := slices.Index(texts, "其他网站"); web < slices.Index(texts, "哔哩哔哩") || web > slices.Index(texts, "本地文件") {
		t.Fatalf("其他网站 is out of order in %q", texts)
	}
	// No settings rows: the settings open from the gear.
	for _, page := range []string{pageGeneral, pageSpeech, pageAI, pageTranslate, pageAccount} {
		if tt.Exists("id:source-" + page) {
			t.Fatalf("settings row %q still in the sidebar", page)
		}
	}
	var rows []string
	var walk func(v *nsgotest.View)
	walk = func(v *nsgotest.View) {
		if v.Text != "" {
			rows = append(rows, v.Text)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(tt.Find("id:sidebar-sources"))
	for _, label := range []string{"通用", "语音识别", "AI 服务", "翻译", "账号"} {
		if slices.Contains(rows, label) {
			t.Fatalf("settings row %q still in the sidebar's list: %q", label, rows)
		}
	}
}

// TestSidebarFiltersByTheSpokenLanguage checks that the sidebar lists the languages heard in the library's done films once there are more than one, counting only done films, that choosing one shows only the films heard in it, together with the source chosen, without the hero, that a language none of the films is heard in any longer filters nothing out, and that another library starts with every language.
func TestSidebarFiltersByTheSpokenLanguage(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "yt-ja", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: testVideos + "/yt-ja/video.mp4", SpokenLanguage: "ja"},
		{ID: "yt-en", Source: "https://youtu.be/jNQXAC9IVRw", Stage: core.StageComplete, VideoPath: testVideos + "/yt-en/video.mp4", SpokenLanguage: "en"},
		{ID: "file-ja", Source: "file:///videos/file-ja/video.mp4", Stage: core.StageComplete, VideoPath: testVideos + "/file-ja/video.mp4", SpokenLanguage: "ja"},
		{ID: "old", Source: "file:///videos/old/video.mp4", Stage: core.StageComplete, VideoPath: testVideos + "/old/video.mp4"},
		{ID: "running", Source: "https://youtu.be/running", Stage: core.StageTranscribing, SpokenLanguage: "ko"},
	}
	refresh(tt, a)
	if got := tt.Find("id:spoken-count-ja").Text; got != "2" {
		t.Fatalf("日语 shows %q, want 2", got)
	}
	if got := tt.Find("id:spoken-count-en").Text; got != "1" {
		t.Fatalf("英语 shows %q, want 1", got)
	}
	if tt.Exists("id:spoken-ko") {
		t.Fatal("a job still in 处理中 counts for its language")
	}
	shows := func(keys ...string) {
		t.Helper()
		for _, key := range []string{"yt-ja", "yt-en", "file-ja", "old"} {
			if want := slices.Contains(keys, key); tt.Exists("id:film-"+key) != want {
				t.Fatalf("film %s shown %v, want %v: %q", key, !want, want, tt.Texts())
			}
		}
	}
	shows("yt-ja", "yt-en", "file-ja", "old")
	if !tt.Exists("id:films-hero") {
		t.Fatal("no hero before a language is chosen")
	}
	tt.Press("id:spoken-ja")
	refresh(tt, a)
	if a.SpokenFilter() != "ja" || a.page != pageFilms {
		t.Fatalf("filter %q on page %q", a.SpokenFilter(), a.page)
	}
	shows("yt-ja", "file-ja")
	if tt.Exists("id:films-hero") {
		t.Fatal("the hero shows while a language is chosen")
	}
	// Both filters apply.
	tt.Press("id:source-" + sidebar.SourceYouTube)
	refresh(tt, a)
	shows("yt-ja")
	tt.Press("id:spoken-en")
	refresh(tt, a)
	shows("yt-en")
	tt.Press("id:source-" + sidebar.SourceLocal)
	refresh(tt, a)
	if !hasText(tt, testlocale.Messages().FilmsSpokenEmpty(testlocale.Messages().SettingsSpeechLanguageEn())) {
		t.Fatalf("no word that no local film is in English: %q", tt.Texts())
	}
	tt.Press("id:spoken-" + sidebar.SpokenAll)
	refresh(tt, a)
	shows("file-ja", "old")
	// Once the films are heard in one language, the list goes and the language chosen filters nothing out.
	tt.Press("id:source-" + sidebar.SourceAll)
	tt.Press("id:spoken-en")
	a.library.Jobs = slices.DeleteFunc(a.library.Jobs, func(job core.Job) bool { return job.ID == "yt-en" })
	refresh(tt, a)
	if tt.Exists("id:sidebar-languages") || a.SpokenFilter() != sidebar.SpokenAll {
		t.Fatalf("languages listed %v, filter %q, with films heard in one", tt.Exists("id:sidebar-languages"), a.SpokenFilter())
	}
	shows("yt-ja", "file-ja", "old")
	a.side.spoken = "ja"
	a.sidebarLibraryChanged()
	if a.side.spoken != sidebar.SpokenAll {
		t.Fatalf("another library kept the language %q", a.side.spoken)
	}
}

// TestSidebarFiltersByHowFarFilmsWereWatched checks that the sidebar counts the done films in each watch state among those of the source, the language and the search chosen, so a count is how many films choosing it shows; that choosing one shows only those films, without the hero; that marking a film from its menu moves it at once; that Return in the search field opens only a film the filter shows; and that another library starts with every state.
func TestSidebarFiltersByHowFarFilmsWereWatched(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	videos := map[string]string{}
	for _, id := range []string{"yt-done", "yt-new", "file-half", "file-new"} {
		videos[id] = testVideos + "/" + id + "/video.mp4"
	}
	a.library.Jobs = []core.Job{
		{ID: "yt-done", Title: "Harbor Tour", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: videos["yt-done"]},
		{ID: "yt-new", Title: "Mountain Trail", Source: "https://youtu.be/jNQXAC9IVRw", Stage: core.StageComplete, VideoPath: videos["yt-new"]},
		{ID: "file-half", Title: "Harbor Lights", Source: core.FileURL(videos["file-half"]), Stage: core.StageComplete, VideoPath: videos["file-half"]},
		{ID: "file-new", Title: "Quiet Garden", Source: core.FileURL(videos["file-new"]), Stage: core.StageComplete, VideoPath: videos["file-new"]},
		{ID: "running", Source: "https://youtu.be/running", Stage: core.StageTranscribing},
	}
	a.lib.durations = map[string]time.Duration{}
	for _, video := range videos {
		a.lib.durations[video] = 100 * time.Second
	}
	a.palette.furthest = map[string]time.Duration{videos["yt-done"]: 100 * time.Second, videos["file-half"]: 40 * time.Second}
	a.player.positions = map[string]time.Duration{videos["file-half"]: 40 * time.Second}
	refresh(tt, a)
	counts := func(unwatched, inProgress, watched int) {
		t.Helper()
		for state, want := range map[string]int{sidebar.WatchUnwatched: unwatched, sidebar.WatchInProgress: inProgress, sidebar.WatchWatched: watched} {
			if got := tt.Find("id:watch-count-" + state).Text; got != strconv.Itoa(want) {
				t.Fatalf("%s counts %s, want %d", state, got, want)
			}
		}
	}
	shows := func(keys ...string) {
		t.Helper()
		for id := range videos {
			if want := slices.Contains(keys, id); tt.Exists("id:film-"+id) != want {
				t.Fatalf("film %s shown %v, want %v: %q", id, !want, want, tt.Texts())
			}
		}
	}
	counts(2, 1, 1)
	// The counts follow the other filters.
	tt.Press("id:source-" + sidebar.SourceYouTube)
	refresh(tt, a)
	counts(1, 0, 1)
	tt.Press("id:source-" + sidebar.SourceAll)
	apply(tt, a, func() { a.SetQuery("harbor") })
	counts(0, 1, 1)
	apply(tt, a, func() { a.SetQuery("") })

	tt.Press("id:watch-" + sidebar.WatchUnwatched)
	refresh(tt, a)
	if a.WatchFilter() != sidebar.WatchUnwatched || a.page != pageFilms {
		t.Fatalf("filter %q on page %q", a.WatchFilter(), a.page)
	}
	shows("yt-new", "file-new")
	if tt.Exists("id:films-hero") {
		t.Fatal("the hero shows while a watch state is chosen")
	}
	// Both filters apply.
	tt.Press("id:source-" + sidebar.SourceLocal)
	refresh(tt, a)
	shows("file-new")
	tt.Press("id:source-" + sidebar.SourceAll)
	refresh(tt, a)
	// Marked watched from its menu, a film leaves 未看 at once, and 已看完 counts it.
	tt.RightClick("id:film-yt-new")
	tt.ChoosePopup(testlocale.Messages().FilmsMarkWatched())
	refresh(tt, a)
	shows("file-new")
	counts(1, 1, 2)
	// Return opens the one film the search finds among those the filter shows, not one it hides.
	apply(tt, a, func() { a.SetQuery("harbor") })
	apply(tt, a, a.SubmitQuery)
	if a.openKey != "" {
		t.Fatalf("Return opened %q, which 未看 hides", a.openKey)
	}
	tt.Press("id:watch-" + sidebar.WatchInProgress)
	apply(tt, a, a.SubmitQuery)
	if a.openKey != "file-half" {
		t.Fatalf("Return opened %q, want the one film in progress named Harbor", a.openKey)
	}
	a.closeFilm()
	a.side.watch = sidebar.WatchWatched
	a.sidebarLibraryChanged()
	if a.side.watch != sidebar.WatchAll {
		t.Fatalf("another library kept the watch state %q", a.side.watch)
	}
}

// TestSidebarCountFollowsTheSelection checks that a source's count stays in the secondary text's color on its chosen row, as on the others, with or without the sidebar's focus: NSGo's sidebar marks the chosen row with a faint fill of the text's color, which the count reads on as the label does. MyGo's macOS sidebar filled the row with the accent while it had the focus and turned the count near white; NSGo draws one look everywhere, which this checks instead.
func TestSidebarCountFollowsTheSelection(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "yt", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: testVideos + "/yt-clip/video.mp4"},
		{ID: "file", Source: "file:///videos/file-clip/video.mp4", Stage: core.StageComplete, VideoPath: testVideos + "/file-clip/video.mp4"},
	}
	refresh(tt, a)
	if got := sidebarCount(t, tt, sidebar.SourceAll); got != "2" {
		t.Fatalf("全部影片 shows %q, want 2", got)
	}
	ink := func(source string) ns.Color { return tt.Find("id:count-" + source).TextStyle.Color }
	secondary := ink(sidebar.SourceYouTube)
	if chosen := ink(sidebar.SourceAll); chosen != secondary {
		t.Fatalf("the chosen row's count is %v, the others' %v", chosen, secondary)
	}
	tt.Press("id:source-" + sidebar.SourceAll)
	tt.HoverOut()
	if got := ink(sidebar.SourceAll); got != secondary {
		t.Fatalf("the chosen row's count with the focus is %v, want %v as without it", got, secondary)
	}
	if !tt.Find("id:source-" + sidebar.SourceAll).Props.Selected {
		t.Fatal("全部影片 is not the chosen row")
	}
}

// TestSidebarCardClearsTheTrafficLights checks that the card keeps an empty row for the traffic lights under a hidden title bar, and none without one.
func TestSidebarCardClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(t, &fakePipeline{})
	useTitleBar(tt, ns.Edges{Top: 28, Left: 80})
	// The card is 8 points in from the window's top and its row for the traffic lights is 48 high.
	if y := pageFrame(tt.Find(whole("资料库"))).Origin.Y; y < 56 {
		t.Fatalf("switcher at y=%v, inside the traffic lights' row", y)
	}
	_, plain, _ := newTestApp(t, &fakePipeline{})
	if y := pageFrame(plain.Find(whole("资料库"))).Origin.Y; y >= 28 {
		t.Fatalf("switcher at y=%v without a title bar", y)
	}
}

func TestSwitcherMenuListsTheLibrariesWithCounts(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "甲.kando")
	second := filepath.Join(parent, "乙.kando")
	for _, path := range []string{filepath.Join(first, "a", "video.mp4"), filepath.Join(second, "b", "video.mp4"), filepath.Join(second, "talk.mp4")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var revealed string
	prev := revealInFolder
	revealInFolder = func(_ *ns.Scope, path string, done func(error)) {
		revealed = path
		done(nil)
	}
	t.Cleanup(func() { revealInFolder = prev })

	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	a.refreshLibrary()
	a.countLibraries()
	applyUntil(t, tt, a, updates, func() bool { _, ok := a.LibraryCount(second); return ok })
	if !hasText(tt, "甲") || tt.Find("id:sidebar-switcher").Props.Label != switcherLabel("甲") {
		t.Fatalf("switcher does not name the open library: %q", tt.Texts())
	}
	tt.Press("id:sidebar-switcher")
	msgs := testlocale.Messages()
	want := []string{"甲 · 1", "乙 · 2", "-", "新建资料库…", "打开已有资料库…", msgs.SidebarRevealLibrary("甲", platformName()), "-", msgs.LockMenuSet()}
	if got := popupLabels(tt); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	tt.ChoosePopup(msgs.SidebarRevealLibrary("甲", platformName()))
	if revealed != first {
		t.Fatalf("revealed %q, want %q", revealed, first)
	}

	// Switching shows all of the new library's films, and the library left gets its count.
	a.SetSourceFilter(sidebar.SourceLocal)
	chooseFromSwitcher(t, tt, a, "乙")
	if a.snap().LibraryDir != second || a.side.filter != sidebar.SourceAll {
		t.Fatalf("library %q source %q after switching", a.snap().LibraryDir, a.side.filter)
	}
	applyUntil(t, tt, a, updates, func() bool { _, ok := a.LibraryCount(first); return ok })
	tt.Press("id:sidebar-switcher")
	want = []string{"甲 · 1", "乙 · 2", "-", "新建资料库…", "打开已有资料库…", msgs.SidebarRevealLibrary("乙", platformName()), "-", msgs.LockMenuSet()}
	if got := popupLabels(tt); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	tt.ClosePopup()
}

// TestLibraryCountsKeepTheLatestRun checks that counts read before a library switch, arriving after the ones read after it, do not replace them and leave the library just left without a count.
func TestLibraryCountsKeepTheLatestRun(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "甲.kando")
	second := filepath.Join(parent, "乙.kando")
	for _, path := range []string{filepath.Join(first, "a.mp4"), filepath.Join(second, "b.mp4"), filepath.Join(second, "c.mp4")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	// The startup count, of 乙, is still running when the user switches to 乙 and 甲 is counted.
	a.countLibraries()
	var startup func()
	select {
	case startup = <-updates:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the startup count")
	}
	a.edit(func(s *session) { s.LibraryDir = second })
	a.countLibraries()
	waitUpdate(t, tt, a, updates)
	apply(tt, a, startup)
	if n, ok := a.LibraryCount(first); !ok || n != 1 {
		t.Fatalf("count of the library just left = %d, %v; want 1", n, ok)
	}
}

// sidebarStatusLine is what the sidebar's status line says.
func sidebarStatusLine(tt *nsgotest.App) string { return tt.Find("id:sidebar-status").Text }

func TestStatusLineSaysWhatIsMissing(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli"}
	withModel := found
	withModel.WhisperModel = model
	noCLI := withModel
	noCLI.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
	keyed := withModel
	keyed.Translator = media.ChatTranslator{BaseURL: "https://api.example.com/v1", Model: "gpt", APIKey: "k"}
	gone := found
	gone.WhisperModel = filepath.Join(t.TempDir(), "gone.bin")
	msgs := testlocale.Messages()
	cases := []struct {
		name     string
		pipeline media.Pipeline
		want     string
		section  string
	}{
		{"no tools", media.Tools{FFmpeg: "/bin/ffmpeg"}, "缺少组件：yt-dlp、whisper-cli", settings.SectionGeneral},
		{"no model", found, msgs.PrecheckNoSpeechModel(), settings.SectionSpeech},
		{"model gone", gone, msgs.PrecheckNoSpeechModel(), settings.SectionSpeech},
		{"no translation model", withModel, msgs.PrecheckNoLLMModel(), settings.SectionAI},
		{"everyapi without a sign-in", noCLI, msgs.PrecheckEveryAPISignIn(), settings.SectionAI},
		{"ready", keyed, msgs.ReadinessReady(), settings.SectionGeneral},
		{"another pipeline", &fakePipeline{}, msgs.ReadinessReady(), settings.SectionGeneral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, tt, _ := newTestApp(t, c.pipeline)
			if got := sidebarStatusLine(tt); got != msgs.ReadinessCheckingTools() {
				t.Fatalf("before the first check: %q", got)
			}
			a.checkReadiness(true)
			refresh(tt, a)
			if got := sidebarStatusLine(tt); got != c.want {
				t.Fatalf("status %q, want %q", got, c.want)
			}
			tt.Press("id:sidebar-settings")
			wantSettings(t, a, c.section)
		})
	}

	t.Run("no library", func(t *testing.T) {
		a, tt, _ := newTestApp(t, keyed)
		a.edit(func(s *session) { s.LibraryDir = "" })
		a.checkReadiness(true)
		refresh(tt, a)
		if got := sidebarStatusLine(tt); got != msgs.PrecheckNoLibrary() {
			t.Fatalf("status %q, want %q", got, msgs.PrecheckNoLibrary())
		}
	})
}

// TestStatusLineWaitsForTheEveryAPISignIn checks that the sign-in is asked off the engine's goroutine, that its answer is kept until 重新检测, and that an answer arriving after the AI service changed does not hide the new state.
func TestStatusLineWaitsForTheEveryAPISignIn(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := make(chan error)
	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Credential: func(context.Context, bool) (media.Endpoint, error) {
		return media.Endpoint{}, <-answers
	}}
	msgs := testlocale.Messages()
	a, tt, updates := newTestApp(t, tools)
	check := func(signIn bool) {
		a.checkReadiness(signIn)
		refresh(tt, a)
	}
	want := func(status, why string) {
		t.Helper()
		if got := sidebarStatusLine(tt); got != status {
			t.Fatalf("%s: status %q, want %q", why, got, status)
		}
	}
	check(true)
	want(msgs.ReadinessCheckingSignIn(), "asking")
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, tt, a, updates)
	want(msgs.PrecheckEveryAPISignedOut(), "answered")
	// Choosing the speech model again keeps the answer instead of asking the CLI again.
	check(false)
	want(msgs.PrecheckEveryAPISignedOut(), "kept")
	if len(updates) != 0 {
		t.Fatalf("asked again: %d updates", len(updates))
	}
	// 重新检测 asks again.
	check(true)
	want(msgs.ReadinessCheckingSignIn(), "asking again")
	// While it waits, an API key replaces the sign-in; the late answer leaves the key's state.
	keyed := tools
	keyed.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, APIKey: "k"}
	a.pipeline = keyed
	check(false)
	want(msgs.ReadinessReady(), "keyed")
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, tt, a, updates)
	want(msgs.ReadinessReady(), "late sign-in answer")

	// Of two sign-in checks running at once, the later one's answer stands, whichever comes last.
	later := make(chan error)
	again := tools
	again.Translator.Credential = func(context.Context, bool) (media.Endpoint, error) { return media.Endpoint{}, <-later }
	a.pipeline = tools
	check(true)
	a.pipeline = again
	check(true)
	later <- nil
	waitUpdate(t, tt, a, updates)
	want(msgs.ReadinessReady(), "the later check")
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, tt, a, updates)
	want(msgs.ReadinessReady(), "the earlier check's answer after the later one's")
}

// TestStatusLineAsksForTheSignInOnce checks that the changes that do not ask again, as each keystroke in 「AI 服务」, wait for the sign-in check already running instead of starting another everyapi process each.
func TestStatusLineAsksForTheSignInOnce(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := make(chan error)
	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Credential: func(context.Context, bool) (media.Endpoint, error) {
		return media.Endpoint{}, <-answers
	}}
	msgs := testlocale.Messages()
	a, tt, updates := newTestApp(t, tools)
	a.checkReadiness(true)
	asked := a.side.ready.gen
	for range 3 {
		a.checkReadiness(false)
		refresh(tt, a)
		if got := sidebarStatusLine(tt); got != msgs.ReadinessCheckingSignIn() {
			t.Fatalf("status %q, want %q", got, msgs.ReadinessCheckingSignIn())
		}
	}
	if a.side.ready.gen != asked {
		t.Fatalf("started %d more sign-in checks while one was running", a.side.ready.gen-asked)
	}
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, tt, a, updates)
	if got := sidebarStatusLine(tt); got != msgs.PrecheckEveryAPISignedOut() {
		t.Fatalf("the running check's answer was dropped: %q", got)
	}
}

// TestLLMTestAnswersTheSignIn checks that 测试 in 「AI 服务」 succeeding through the EveryAPI CLI's key replaces a kept "not signed in", which only 重新检测 on another page would otherwise ask again.
func TestLLMTestAnswersTheSignIn(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer srv.Close()
	signedIn := false
	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Client: srv.Client(), Credential: func(context.Context, bool) (media.Endpoint, error) {
		if !signedIn {
			return media.Endpoint{}, media.ErrEveryAPISignedOut
		}
		return media.Endpoint{BaseURL: srv.URL + "/v1", APIKey: "relay"}, nil
	}}
	msgs := testlocale.Messages()
	a, tt, updates := newTestApp(t, tools)
	a.checkReadiness(true)
	waitUpdate(t, tt, a, updates)
	if got := sidebarStatusLine(tt); got != msgs.PrecheckEveryAPISignedOut() {
		t.Fatalf("status %q, want %q", got, msgs.PrecheckEveryAPISignedOut())
	}

	// The user signs in to the CLI and presses 测试.
	signedIn = true
	a.testLLM()
	waitUpdate(t, tt, a, updates)
	if a.LLMResult() != "连接成功" || sidebarStatusLine(tt) != msgs.ReadinessReady() {
		t.Fatalf("test result %q, status %q", a.LLMResult(), sidebarStatusLine(tt))
	}
}

// TestStatusDotIsGreenWhenReady checks the dot's fill in the model of the views: green once a job can start.
func TestStatusDotIsGreenWhenReady(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.checkReadiness(true)
	refresh(tt, a)
	if got := sidebarStatusLine(tt); got != testlocale.Messages().ReadinessReady() {
		t.Fatalf("status %q", got)
	}
	if got, want := tt.Find("id:sidebar-status-dot").Style.Background, ns.RGB(0x30D158); got != want {
		t.Fatalf("dot = %v, want %v", got, want)
	}
}

// TestSidebarInEnglish checks the sidebar in English: its sources, the switcher and its menu, and a status line that follows the language as it changes, though it was found before the change.
func TestSidebarInEnglish(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.checkReadiness(true)
	refresh(tt, a)
	wantText(t, tt, "已就绪，可以开始处理")

	a.cx.SetLocale("en")
	refresh(tt, a)
	for _, want := range []string{"All Films", "YouTube", "Bilibili", "Local Files", "Library", "Ready to process"} {
		wantText(t, tt, want)
	}
	// The library page beside the sidebar is another area's, so only the sidebar's own words are checked.
	for _, chinese := range []string{"本地文件", "已就绪，可以开始处理"} {
		wantNoText(t, tt, chinese)
	}
	name := sidebar.Title(a.snap().LibraryDir)
	if got := tt.Find("id:sidebar-switcher").Props.Label; got != "Switch library, current: "+name {
		t.Fatalf("switcher labelled %q", got)
	}
	tt.Press("id:sidebar-switcher")
	menu := popupLabels(tt)
	for _, want := range []string{"New Library…", "Open Existing Library…", locales.In("en").SidebarRevealLibrary(name, platformName())} {
		if !slices.Contains(menu, want) {
			t.Fatalf("menu %q lacks %q", menu, want)
		}
	}
	tt.ClosePopup()

	testlocale.Use(t, "en")
	b, btt, _ := newTestApp(t, media.Tools{FFmpeg: "/bin/ffmpeg"})
	b.checkReadiness(true)
	refresh(btt, b)
	wantText(t, btt, "Missing components: yt-dlp, whisper-cli")
}

// TestSwitcherNamesTheSystemsFileManager checks that the switcher's reveal names the file manager of the system it runs on, and that the settings page's reveal does too.
func TestSwitcherNamesTheSystemsFileManager(t *testing.T) {
	for _, tc := range []struct {
		name             string
		os               ns.OS
		switcher, button string
	}{
		{"macOS", ns.OSMacOS, "在访达中显示「甲」", "在访达中显示"},
		{"Windows", ns.OSWindows, "在文件资源管理器中显示「甲」", "在文件资源管理器中显示"},
		{"Linux", ns.OSLinux, "在文件管理器中显示「甲」", "在文件管理器中显示"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usePlatform(t, tc.os)
			lib := filepath.Join(t.TempDir(), "甲.kando")
			if err := os.MkdirAll(lib, 0o755); err != nil {
				t.Fatal(err)
			}
			a, tt, _ := newTestApp(t, &fakePipeline{})
			a.edit(func(s *session) {
				s.LibraryDir = lib
				s.Libraries = []string{lib}
			})
			refresh(tt, a)
			tt.Press("id:sidebar-switcher")
			if menu := popupLabels(tt); !slices.Contains(menu, tc.switcher) {
				t.Fatalf("menu %q lacks %q", menu, tc.switcher)
			}
			tt.ClosePopup()
			st := settingsOpenAt(a, tt, settings.SectionGeneral)
			wantText(t, st, tc.button)
		})
	}
}

// TestFindFocusesTheSidebarSearch checks that ⌘F on the library page puts the keyboard in the sidebar's search field, and that Escape there empties it.
func TestFindFocusesTheSidebarSearch(t *testing.T) {
	a, tt := librarySearchWith(t)
	tt.Key("mod+f")
	if !tt.Find("搜索影片").Focused || a.side.queryFocus {
		t.Fatalf("⌘F: focused %v, request left %v", tt.Find("搜索影片").Focused, a.side.queryFocus)
	}
	typeFocused(t, tt, "algebra")
	if a.side.query != "algebra" || hasText(tt, "Rivers of Europe") {
		t.Fatalf("query %q: %q", a.side.query, tt.Texts())
	}
	tt.Key("escape")
	if a.side.query != "" || !hasText(tt, "Rivers of Europe") {
		t.Fatalf("Escape left query %q: %q", a.side.query, tt.Texts())
	}
}

// TestSearchFromTheBrowser checks that the sidebar's search, which narrows the library page, goes there from the browser's page as the user types, or presses ⌘F, so what it finds shows; an empty field leaves the browser where it is.
func TestSearchFromTheBrowser(t *testing.T) {
	a, tt := librarySearchWith(t)
	a.Select(pageBrowse)
	refresh(tt, a)
	tt.Focus("搜索影片")
	if a.page != pageBrowse {
		t.Fatalf("focusing the field went to %q", a.page)
	}
	typeFocused(t, tt, "rivers")
	if a.page != pageFilms || !shows(tt, "Rivers of Europe") || shows(tt, "Linear Algebra") {
		t.Fatalf("typing on the browser's page: page %q, %q", a.page, tt.Texts())
	}
	a.side.query = ""
	a.Select(pageBrowse)
	refresh(tt, a)
	tt.Key("mod+f")
	if a.page != pageFilms || !tt.Find("搜索影片").Focused {
		t.Fatalf("⌘F on the browser's page: page %q, focused %v", a.page, tt.Find("搜索影片").Focused)
	}
}

// TestSearchIsForItsLibrary checks that the search is not saved, that opening another library empties it, and that a library with no film shows no search field and leaves ⌘F to nothing.
func TestSearchIsForItsLibrary(t *testing.T) {
	a, tt := librarySearchWith(t)
	t.Cleanup(func() {
		if a.libraryWatch != nil {
			close(a.libraryWatch)
			a.libraryWatch = nil
		}
	})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.side.query = "rivers"
	refresh(tt, a)
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "rivers") {
		t.Fatalf("settings.json keeps the search: %s", data)
	}
	other := filepath.Join(t.TempDir(), "其他.kando")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	a.switchLibrary(other)
	refresh(tt, a)
	if a.side.query != "" {
		t.Fatalf("another library kept the search %q", a.side.query)
	}
	if tt.Exists("搜索影片") || a.sidebarSearchable() {
		t.Fatalf("an empty library shows the search field: %q", tt.Texts())
	}
	// A request for the focus while there is no field is dropped, so the field does not take the focus once it shows.
	a.side.queryFocus = true
	refresh(tt, a)
	if a.side.queryFocus {
		t.Fatal("the request for the search field's focus outlived a change without the field")
	}
}

// TestFindKeepsSearchingSubtitlesInAFilm checks that ⌘F over an open film is still the player's 搜索字幕, not the library's search, and that the menu bar gives ⌘F to the one item it runs: 编辑 › 搜索影片 on the library page and 字幕 › 搜索字幕 over a film.
func TestFindKeepsSearchingSubtitlesInAFilm(t *testing.T) {
	a, tt := librarySearchWith(t)
	const find = "mod+f"
	bar := a.menuBar(a.cx)
	library, subtitles := findMenuItem(t, bar, "编辑", "搜索影片"), findMenuItem(t, bar, "字幕", "搜索字幕")
	if library.Disabled || library.Shortcut != find || !subtitles.Disabled || subtitles.Shortcut != "" {
		t.Fatalf("on the library page: 搜索影片 %+v, 搜索字幕 %+v", library, subtitles)
	}

	a.openKey = "donea"
	a.player.filmSide = "info"
	refresh(tt, a)
	tt.Key(find)
	if a.side.queryFocus || a.player.filmSide != "" || !a.player.panel {
		t.Fatalf("⌘F over a film: library focus %v, side %q, panel %v", a.side.queryFocus, a.player.filmSide, a.player.panel)
	}
	bar = a.menuBar(a.cx)
	library, subtitles = findMenuItem(t, bar, "编辑", "搜索影片"), findMenuItem(t, bar, "字幕", "搜索字幕")
	if !library.Disabled || library.Shortcut != "" || subtitles.Disabled || subtitles.Shortcut != find {
		t.Fatalf("over a film: 搜索影片 %+v, 搜索字幕 %+v", library, subtitles)
	}
}

// TestSidebarOpensThePaletteWithoutAMenuBar checks the status card's button that opens the command palette where the app has no menu bar of its own, as on Windows and Linux: none while the title bar with the palette's button shows (package chrome); in full screen, which hides that bar, the card has it, its tooltip naming the key as the platform writes it, and it opens the palette, which lists the shortcut cheat sheet first; out of full screen it gives way to the bar's again. Where the menu bar carries the palette and the cheat sheet, as on macOS, the card has none.
func TestSidebarOpensThePaletteWithoutAMenuBar(t *testing.T) {
	keep := appMenuBar
	t.Cleanup(func() { appMenuBar = keep })
	appMenuBar = false
	msgs := testlocale.Messages()
	for _, os := range []ns.OS{ns.OSWindows, ns.OSLinux} {
		usePlatform(t, os)
		a, tt, _ := newTestApp(t, &fakePipeline{})
		if tt.Exists("id:sidebar-palette") {
			t.Fatalf("%v: the status card has the palette's button beside the title bar's", os)
		}
		env := testEnvironment()
		env.FullScreen = true
		tt.SetEnvironment(env)
		refresh(tt, a)
		if tt.Exists("id:chrome-bar") {
			t.Fatalf("%v: the title bar shows in full screen", os)
		}
		if got := tt.Find("id:sidebar-palette").Props.Label; got != msgs.SidebarCommands() {
			t.Fatalf("%v: in full screen the status card's palette button is named %q", os, got)
		}
		tt.Hover("id:sidebar-palette")
		tt.Advance(time.Second)
		refresh(tt, a)
		wantText(t, tt, msgs.SidebarCommandsTooltip("Ctrl+K"))
		tt.Press("id:sidebar-palette")
		refresh(tt, a)
		if !a.palette.open {
			t.Fatalf("%v: the button did not open the palette", os)
		}
		if v := a.PaletteView(a.cx); len(v.Commands) == 0 || v.Commands[0].Label != msgs.CheatsheetMenuItem() {
			t.Fatalf("%v: the palette opens with %+v", os, v.Commands)
		}
		tt.Press(msgs.CheatsheetMenuItem())
		refresh(tt, a)
		if !a.overlay.cheatsheet || a.palette.open {
			t.Fatalf("%v: cheat sheet %v, palette %v", os, a.overlay.cheatsheet, a.palette.open)
		}
		env.FullScreen = false
		tt.SetEnvironment(env)
		refresh(tt, a)
		if tt.Exists("id:sidebar-palette") || !tt.Exists("id:chrome-search") {
			t.Fatalf("%v: out of full screen the card's button stays or the bar's does not come back", os)
		}
	}

	usePlatform(t, ns.OSMacOS)
	appMenuBar = true
	_, mac, _ := newTestApp(t, &fakePipeline{})
	if mac.Exists("id:sidebar-palette") {
		t.Fatal("the status card has the palette's button beside a menu bar")
	}
}

// TestSidebarSearchReturnPlaysTheOnlyFilm checks that Return in the sidebar's search field plays the film when the search leaves one, and puts the keyboard on the first poster when it leaves several.
func TestSidebarSearchReturnPlaysTheOnlyFilm(t *testing.T) {
	a, tt, _ := paletteLibrary(t, &fakePipeline{})
	tt.Type("id:sidebar-search", "e\n")
	refresh(tt, a)
	if a.openKey != "" {
		t.Fatalf("Return with two films found opened %q", a.openKey)
	}
	if first := tt.Find("id:films-grid >> role:button >> nth=0"); !first.Focused || tt.Find("id:sidebar-search").Focused {
		t.Fatalf("Return with two films found left the keyboard on the search field %v, not on the first poster %q", tt.Find("id:sidebar-search").Focused, first.Props.Label)
	}
	tt.Clear("id:sidebar-search")
	tt.Type("id:sidebar-search", "riv\n")
	if a.openKey != "rivers" || a.player.play == nil {
		t.Fatalf("Return with one film found: open %q", a.openKey)
	}
}

// The sidebar counts the jobs of 处理中 that wait or run and those that failed, but not one that was cancelled, and its count takes a player or another page back to the library.
func TestSidebarCountsTheJobs(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "wait", Source: "https://example.com/a", Stage: core.StageQueued, LibraryDir: testVideos},
		{ID: "bad", Source: "https://example.com/b", Stage: core.StageFailed, Err: media.ReadStageError("download: boom"), LibraryDir: testVideos},
		{ID: "stop", Source: "https://example.com/c", Stage: core.StageFailed, Err: context.Canceled, LibraryDir: testVideos},
	}
	a.page = sidebar.PageBrowse
	refresh(tt, a)
	if got := a.Jobs(); got != (sidebar.Jobs{Active: 1, Failed: 1}) {
		t.Fatalf("jobs %+v", got)
	}
	tt.Press("sidebar-jobs")
	if a.page != pageFilms || a.side.filter != sidebar.SourceAll {
		t.Fatalf("the count went to page %q, source %q", a.page, a.side.filter)
	}
}

// TestSidebarSearchReturnKeepsToTheLanguageChosen checks that Return in the sidebar's search field acts on the films the grid shows, those heard in the language chosen: it plays the one film shown when another of the same name is hidden, and opens none that the language hides.
func TestSidebarSearchReturnKeepsToTheLanguageChosen(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "ja", Title: "Rivers in Japanese", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: testVideos + "/ja/video.mp4", SpokenLanguage: "ja"},
		{ID: "en", Title: "Rivers in English", Source: "https://youtu.be/jNQXAC9IVRw", Stage: core.StageComplete, VideoPath: testVideos + "/en/video.mp4", SpokenLanguage: "en"},
	}
	refresh(tt, a)
	tt.Press("id:spoken-ja")
	refresh(tt, a)
	tt.Type("id:sidebar-search", "english\n")
	if a.openKey != "" {
		t.Fatalf("Return opened %q, which 日语 hides", a.openKey)
	}
	tt.Clear("id:sidebar-search")
	tt.Type("id:sidebar-search", "rivers\n")
	if a.openKey != "ja" {
		t.Fatalf("Return with one film shown of two found: open %q, want ja", a.openKey)
	}
}

// TestSidebarDropsALanguageNoLongerListed checks that a language chosen in the sidebar is let go once the list no longer offers it, as after its last film was removed, so a film heard in it later does not narrow 全部影片 by itself; a language still listed stays chosen.
func TestSidebarDropsALanguageNoLongerListed(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "ja", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: testVideos + "/ja/video.mp4", SpokenLanguage: "ja"},
		{ID: "en", Source: "https://youtu.be/jNQXAC9IVRw", Stage: core.StageComplete, VideoPath: testVideos + "/en/video.mp4", SpokenLanguage: "en"},
		{ID: "ko", Source: "https://youtu.be/9bZkp7q19f0", Stage: core.StageComplete, VideoPath: testVideos + "/ko/video.mp4", SpokenLanguage: "ko"},
	}
	refresh(tt, a)
	tt.Press("id:spoken-en")
	a.library.Jobs = slices.DeleteFunc(a.library.Jobs, func(job core.Job) bool { return job.ID == "ko" })
	refresh(tt, a)
	if a.side.spoken != "en" {
		t.Fatalf("a language still listed was let go: %q", a.side.spoken)
	}
	a.library.Jobs = slices.DeleteFunc(a.library.Jobs, func(job core.Job) bool { return job.ID == "en" })
	refresh(tt, a)
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "en2", Source: "https://youtu.be/kJQP7kiw5Fk", Stage: core.StageComplete, VideoPath: testVideos + "/en2/video.mp4", SpokenLanguage: "en"})
	refresh(tt, a)
	if a.SpokenFilter() != sidebar.SpokenAll || !tt.Exists("id:film-ja") {
		t.Fatalf("a film heard in English brought back the filter %q: %q", a.SpokenFilter(), tt.Texts())
	}
}
