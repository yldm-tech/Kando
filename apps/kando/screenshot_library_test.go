package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/films"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsLibrary renders the library page from the prototype's library (screenshot_helpers_test.go) to PNG files in the directory named by KANDO_SCREENSHOTS, in the states the prototype's library screenshots show: the first screen with the message bar the prototype shows after its onboarding, scrolled to 处理中 and to the grid as far as the prototype scrolls, ⋯ on the first poster, a YouTube link typed into the field, and a new empty library. Besides those it renders the whole page, the page before there is a library, the new-library sheet, the 从资料库移除 confirmation for a film alone in its folder and for one whose folder holds another folder, and a video dropped over an open film. The prototype's drag-over state is rendered by TestScreenshotDropZone in the films package, because the tester cannot hold files over the window without dropping them. It is skipped when the variable is unset.
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
	f.shoot(t, "library-url-typed", func(a *app, tt *ui.Tester, _ chan func()) {
		onboarding(a)
		screenshotClick(t, tt, "视频链接")
		tt.Type("https://www.youtube.com/watch?v=k9Yt2LmQ0pX")
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
}
