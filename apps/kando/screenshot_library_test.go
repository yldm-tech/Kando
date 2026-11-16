package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsLibrary renders the library page from the prototype's library (screenshot_helpers_test.go) to PNG files in the directory named by KANDO_SCREENSHOTS, in the states the prototype's library screenshots show: the first screen with the message bar the prototype shows after its onboarding and 浏览网站 beside the link field, scrolled to 处理中 and to the grid as far as the prototype scrolls, ⋯ on the first poster, a YouTube link typed into the field, and a new empty library, whose page offers 浏览网站 too. It also renders the grid with a film whose translation left a line untranslated, which offers to translate it, and with two films made without a translation once an AI service is set up, which the message bar and the grid's heading offer to translate. Besides those it renders the grid ordered by title, a query in the sidebar's search field and one that matches no film, the whole page, the page before there is a library, the new-library sheet, the 从资料库移除 confirmation for a film alone in its folder and for one whose folder holds another folder, the 重命名… dialog and the poster it renamed, a video dropped over an open film, and a library copied without its job records whose videos show the subtitles exported beside them. The prototype's drag-over state is rendered by TestScreenshotDropZone in the films package, because the tester cannot hold files over the window without dropping them. It is skipped when the variable is unset.
func TestScreenshotsLibrary(t *testing.T) {
	f := newScreenshotFixture(t)
	// onboarding is the message bar the prototype shows once its onboarding is skipped.
	onboarding := func(a *app) {
		*a.Notice() = films.Notice{Kind: "warning", Text: "随时可以在「帮助 › 重新查看入门引导」回到这里。", Open: true}
	}
	// scrollTo scrolls the shelf until the top of the text label is at y in the window, as the prototype's scrolled states have their section headers.
	scrollTo := func(t *testing.T, a *app, tt *nsgotest.App, label string, y ns.Points) {
		t.Helper()
		sel := whole(label)
		if !tt.Exists(sel) {
			t.Fatalf("no %q to scroll to; texts %q", label, tt.Texts())
		}
		if d := pageFrame(tt.Find(sel)).Origin.Y - y; d*d >= 0.25 {
			tt.Scroll(sel, d)
			refresh(tt, a)
		}
	}

	f.shoot(t, "library", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
	})
	// The prototype's scrollTop 442 puts 处理中 at 64, under the top bar; 520 puts 全部影片 (the grid's) at 307 + 2, its text's top.
	f.shoot(t, "library-scrolled-queue", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
		refresh(tt, a)
		scrollTo(t, a, tt, "处理中", 66)
	})
	f.shoot(t, "library-scrolled-grid", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
		refresh(tt, a)
		scrollTo(t, a, tt, "8 部", 315)
	})
	f.shoot(t, "library-card-menu", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
		refresh(tt, a)
		scrollTo(t, a, tt, "8 部", 315)
		// ⋯ opens the system's menu, which the tester lists (tt.PopupMenu) rather than draws; the picture shows the button pressed open.
		tt.Press("id:film-more >> nth=0")
		if len(tt.PopupMenu) == 0 {
			t.Fatal("⋯ opened no menu")
		}
	})
	// Linear Algebra, Lecture 1 as its translation left a line untranslated: the note under its title and the button that translates that line.
	f.shoot(t, "library-translate-missing", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
		job := f.job(t, "j3")
		job.Untranslated = 1
		a.library.Upsert(job)
		refresh(tt, a)
		scrollTo(t, a, tt, "8 部", 315)
	})
	// Linear Algebra, Lecture 1 and the film after it made before an AI service was set up, their subtitles the original lines alone: once a service is set up, the message bar at the top of the page offers to translate them (library-translate-offer), and so does the grid's heading, Translate 2 Films (library-translate-all).
	untranslated := func(t *testing.T, a *app, tt *nsgotest.App) {
		t.Helper()
		for _, id := range []string{"j3", "j4"} {
			job := f.job(t, id)
			job.TranslationErr, job.Untranslated = "no translator", 18
			a.library.Upsert(job)
		}
		tools := a.pipeline.(media.Tools)
		tools.Translator.Model = ""
		a.pipeline = tools
		a.libraryOfferTranslate()
		tools.Translator.Model, tools.Translator.APIKey = media.EveryAPIModel, "sk-demo"
		a.pipeline = tools
		a.libraryOfferTranslate()
		refresh(tt, a)
		if !tt.Exists("id:notice-translate") || !tt.Exists("id:films-translate-all") {
			t.Fatalf("no offer to translate the films: %q", tt.Texts())
		}
	}
	f.shoot(t, "library-translate-offer", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) { untranslated(t, a, tt) })
	f.shoot(t, "library-translate-all", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		untranslated(t, a, tt)
		scrollTo(t, a, tt, "8 部", 315)
	})
	// The grid ordered by title, chosen from the sort button beside its heading (whose menu is the system's, which the tester lists rather than draws): Latin names first, then Chinese ones by pinyin.
	f.shoot(t, "library-sorted-title", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
		a.SetLibrarySort(films.SortTitle)
		refresh(tt, a)
		scrollTo(t, a, tt, "8 部", 315)
	})
	f.shoot(t, "library-url-typed", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
		refresh(tt, a)
		tt.Focus("视频链接")
		typeFocused(t, tt, "https://www.youtube.com/watch?v=k9Yt2LmQ0pX")
	})
	// A query in the sidebar's search field: the hero steps aside, 处理中 stays whole, and the grid keeps the films whose names hold the query; the page is tall enough to show the grid under 处理中.
	f.shootSize(t, "library-search", 1304, 1100, func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		tt.Focus("搜索影片")
		typeFocused(t, tt, "rivers")
	})
	// A query no film's name holds: the grid says so and offers 清除搜索.
	f.shootSize(t, "library-search-none", 1304, 1100, func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		tt.Focus("搜索影片")
		typeFocused(t, tt, "quantum")
	})
	f.shoot(t, "library-empty", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		empty := filepath.Join(f.home, "Movies", "未命名资料库.kando")
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		a.edit(func(s *session) {
			s.LibraryDir = empty
			s.Libraries = append(s.Libraries, empty)
		})
		// The fixture's queued jobs belong to other libraries, and with no job running the real app would already be running one of them; the scene leaves them out so it shows the empty library alone (queue-other-library shows those cards).
		a.library.Jobs = slices.DeleteFunc(a.library.Jobs, func(job core.Job) bool { return job.Stage == core.StageQueued })
		a.refreshLibrary()
	})
	f.shootSize(t, "library-full", 1304, 1400, func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		onboarding(a)
	})
	f.shoot(t, "library-none", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		a.edit(func(s *session) {
			s.LibraryDir = ""
			s.Libraries = nil
		})
		a.refreshLibrary()
	})
	f.shoot(t, "library-new", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		// The sidebar's library menu and the empty page both open it; the scene does not depend on the sidebar's labels.
		a.BeginNew()
	})
	removeAlgebra := func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		tt.RightClick("id:film-" + f.job(t, "j3").ID)
		tt.ChoosePopup(testlocale.Messages().FilmsRemoveFromLibrary())
	}
	f.shootSize(t, "library-remove", 1304, 1100, removeAlgebra)
	// A folder inside the film's folder keeps that folder out of the Trash, and the confirmation says only the video and its subtitles go.
	if err := os.MkdirAll(filepath.Join(filepath.Dir(f.job(t, "j3").VideoPath), "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.shootSize(t, "library-remove-files", 1304, 1100, removeAlgebra)
	// 重命名… on the same poster, with a new title typed in place of the one it shows, and then the poster under that title once Enter keeps it.
	renameAlgebra := func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		tt.RightClick("id:film-" + f.job(t, "j3").ID)
		tt.ChoosePopup(testlocale.Messages().FilmsRename())
		refresh(tt, a)
		// The field takes the keyboard as the dialog opens; the title typed replaces the one it shows.
		tt.Clear("id:rename-name")
		typeFocused(t, tt, "线性代数 · 第一讲")
	}
	f.shootSize(t, "library-rename", 1304, 1100, renameAlgebra)
	f.shootSize(t, "library-renamed", 1304, 1100, func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		renameAlgebra(t, a, tt, updates)
		tt.Submit("id:rename-name")
		// The dialog goes once its motion has ended.
		tt.Advance(time.Second)
		if !finds(tt, "线性代数 · 第一讲") {
			t.Fatalf("the renamed poster is not on the shelf: %q", tt.Texts())
		}
	})
	// A video dropped over an open film: the window goes back to the library, whose message bar says what became of it.
	dropped := filepath.Join(f.home, "Downloads", "Ocean Currents.mp4")
	if err := os.MkdirAll(filepath.Dir(dropped), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropped, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.shoot(t, "drop-on-film", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.DropFiles("id:main-column", fileURLs(dropped)...)
	})
	// A library copied from another Mac without its job records (sidecarLibrary): the video that has its exported subtitles beside it shows them, and only the one without says it has none.
	f.shoot(t, "library-sidecar-subtitles", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		dir, _ := f.sidecarLibrary(t)
		f.showLibrary(t, a, tt, updates, dir)
	})
	// Off macOS the library names the system's file manager, here the Linux one: the hero's 在文件管理器中显示, the page before there is a library, and the new-library sheet, which promises no Kando folder icon there.
	usePlatform(t, ns.OSLinux)
	f.shoot(t, "library-linux", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {})
	f.shoot(t, "library-none-linux", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		a.edit(func(s *session) {
			s.LibraryDir = ""
			s.Libraries = nil
		})
		a.refreshLibrary()
	})
	f.shoot(t, "library-new-linux", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) { a.BeginNew() })
	// The hero film with the picture its site gives it, which takes the place of the frame grabbed for it, as a library from an earlier Kando shows once the site is asked. The site's picture is the fixture's generic scene at full size. It runs last: it leaves the site's picture in the film's cover.jpg.
	f.shoot(t, "library-site-cover", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		picture, err := screenshotScenes.ReadFile(scenePath("generic", 1920))
		if err != nil {
			t.Fatal(err)
		}
		prev := fetchSiteThumbnail
		fetchSiteThumbnail = func(_ context.Context, _, dest string) error {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			return os.WriteFile(dest, picture, 0o644)
		}
		t.Cleanup(func() { fetchSiteThumbnail = prev })
		// The site's picture is kept in the posters folder of the app's caches, as main sets it.
		a.lib.posterDir = filepath.Join(f.home, "Library", "Caches", "Kando", "posters")
		job := f.job(t, "j1")
		job.Thumbnail = "https://i.ytimg.com/vi/" + "a1B2c3D4e5F" + "/maxresdefault.jpg"
		a.library.Upsert(job)
		delete(a.lib.posters, job.VideoPath)
		a.loadExtras()
		screenshotWait(t, tt, a, updates, "the site's cover", func() bool { return len(a.lib.posterBusy) == 0 })
		refresh(tt, a)
	})
}
