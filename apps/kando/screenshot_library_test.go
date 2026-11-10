package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kando/apps/kando/films"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// TestScreenshotsLibrary renders the library page from the prototype's library (screenshot_helpers_test.go) to PNG files in the directory named by KANDO_SCREENSHOTS, in the states the prototype's library screenshots show: the first screen with the message bar the prototype shows after its onboarding, scrolled to 处理中 and to the grid as far as the prototype scrolls, ⋯ on the first poster, a YouTube link typed into the field, and a new empty library. It also renders the grid with a film whose translation left a line untranslated, which offers to translate it. Besides those it renders a query in the sidebar's search field and one that matches no film, the whole page, the page before there is a library, the new-library sheet, the 从资料库移除 confirmation for a film alone in its folder and for one whose folder holds another folder, a video dropped over an open film, and a library copied without its job records whose videos show the subtitles exported beside them. The prototype's drag-over state is rendered by TestScreenshotDropZone in the films package, because the tester cannot hold files over the window without dropping them. It is skipped when the variable is unset.
func TestScreenshotsLibrary(t *testing.T) {
	f := newScreenshotFixture(t)
	// onboarding is the message bar the prototype shows once its onboarding is skipped.
	onboarding := func(a *app) {
		*a.Notice() = films.Notice{Kind: "warning", Text: "随时可以在「帮助 › 重新查看入门引导」回到这里。", Open: true}
	}
	// scrollTo scrolls the shelf until the top of the text label is at y in the window, as the prototype's scrolled states have their section headers.
	scrollTo := func(tt *ui.Tester, label string, y float32) {
		t.Helper()
		for range 4 {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("no %q to scroll to; texts %q", label, tt.Texts())
			}
			if d := r.Y - y; d*d >= 0.25 {
				tt.Move(760, 500)
				tt.Scroll(760, 500, 0, d)
				tt.Frame()
			}
		}
	}

	f.shoot(t, "library", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
	})
	// The prototype's scrollTop 442 puts 处理中 at 64, under the top bar; 520 puts 全部影片 (the grid's) at 307 + 2, its text's top.
	f.shoot(t, "library-scrolled-queue", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
		tt.Frame()
		scrollTo(tt, "处理中", 66)
	})
	f.shoot(t, "library-scrolled-grid", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
		tt.Frame()
		scrollTo(tt, "8 部", 315)
	})
	f.shoot(t, "library-card-menu", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
		tt.Frame()
		scrollTo(tt, "8 部", 315)
		// ⋯ opens the system's menu, which the tester lists (tt.Menu) rather than draws; the picture shows the button pressed open.
		screenshotClick(t, tt, "更多操作")
	})
	// Linear Algebra, Lecture 1 as its translation left a line untranslated: the note under its title and the button that translates that line.
	f.shoot(t, "library-translate-missing", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
		job := f.job(t, "j3")
		job.Untranslated = 1
		a.library.Upsert(job)
		tt.Frame()
		scrollTo(tt, "8 部", 315)
	})
	f.shoot(t, "library-url-typed", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
		screenshotClick(t, tt, "视频链接")
		tt.Type("https://www.youtube.com/watch?v=k9Yt2LmQ0pX")
	})
	// A query in the sidebar's search field: the hero steps aside, 处理中 stays whole, and the grid keeps the films whose names hold the query; the page is tall enough to show the grid under 处理中.
	f.shootSize(t, "library-search", 1304, 1100, func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotClick(t, tt, "搜索影片")
		tt.Type("rivers")
	})
	// A query no film's name holds: the grid says so and offers 清除搜索.
	f.shootSize(t, "library-search-none", 1304, 1100, func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotClick(t, tt, "搜索影片")
		tt.Type("quantum")
	})
	f.shoot(t, "library-empty", func(a *app, tt *ui.Tester, _ chan func()) {
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
	f.shootSize(t, "library-full", 1304, 1400, func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
	})
	f.shoot(t, "library-none", func(a *app, tt *ui.Tester, _ chan func()) {
		a.edit(func(s *session) {
			s.LibraryDir = ""
			s.Libraries = nil
		})
		a.refreshLibrary()
	})
	f.shoot(t, "library-new", func(a *app, tt *ui.Tester, _ chan func()) {
		// The sidebar's library menu and the empty page both open it; the scene does not depend on the sidebar's labels.
		a.BeginNew()
	})
	removeAlgebra := func(a *app, tt *ui.Tester, _ chan func()) {
		if err := tt.RightClick("Linear Algebra, Lecture 1"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("从资料库移除…"); err != nil {
			t.Fatalf("%v; menu %q", err, tt.Menu())
		}
	}
	f.shootSize(t, "library-remove", 1304, 1100, removeAlgebra)
	// A folder inside the film's folder keeps that folder out of the Trash, and the confirmation says only the video and its subtitles go.
	if err := os.MkdirAll(filepath.Join(filepath.Dir(f.job(t, "j3").VideoPath), "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.shootSize(t, "library-remove-files", 1304, 1100, removeAlgebra)
	// A video dropped over an open film: the window goes back to the library, whose message bar says what became of it.
	dropped := filepath.Join(f.home, "Downloads", "Ocean Currents.mp4")
	if err := os.MkdirAll(filepath.Dir(dropped), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropped, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.shoot(t, "drop-on-film", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.DropFiles(sidebarWidth+(1304-sidebarWidth)/2, 782/2, dropped)
	})
	// A library copied from another Mac without its job records (sidecarLibrary): the video that has its exported subtitles beside it shows them, and only the one without says it has none.
	f.shoot(t, "library-sidecar-subtitles", func(a *app, tt *ui.Tester, updates chan func()) {
		dir, _ := f.sidecarLibrary(t)
		f.showLibrary(t, a, tt, updates, dir)
	})
	// Off macOS the library names the system's file manager, here the Linux one: the hero's 在文件管理器中显示, the page before there is a library, and the new-library sheet, which promises no Kando folder icon there.
	usePlatform(t, breeze.Linux)
	f.shoot(t, "library-linux", func(a *app, tt *ui.Tester, _ chan func()) {})
	f.shoot(t, "library-none-linux", func(a *app, tt *ui.Tester, _ chan func()) {
		a.edit(func(s *session) {
			s.LibraryDir = ""
			s.Libraries = nil
		})
		a.refreshLibrary()
	})
	f.shoot(t, "library-new-linux", func(a *app, tt *ui.Tester, _ chan func()) { a.BeginNew() })
	breeze.SetPlatform(breeze.Auto)
	// The hero film with the picture its site gives it, which takes the place of the frame grabbed for it, as a library from an earlier Kando shows once the site is asked. The site's picture is the fixture's generic scene at full size. It runs last: it leaves the site's picture in the film's cover.jpg.
	f.shoot(t, "library-site-cover", func(a *app, tt *ui.Tester, updates chan func()) {
		picture, err := os.ReadFile(filepath.Join("testdata", "scenes", "generic-1920.png"))
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
		screenshotWait(t, tt, updates, "the site's cover", func() bool { return len(a.lib.posterBusy) == 0 })
		tt.Frame()
	})
}
