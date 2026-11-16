package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sidebar"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsSidebar renders the sidebar in the prototype's library states, at the size of its window (spec/library/<state>.window.png), with the status line ready as the prototype's is (screenshot_helpers_test.go):
//   - library-prototype-size: 我的影片 with 全部影片 chosen (library_default)
//   - sidebar-switcher-open: the library switcher pressed, its menu open (library_switcher_open); the menu is the system's, which the picture does not show
//   - sidebar-empty: a new empty library, every count 0 (library_empty)
//   - sidebar-focused: 全部影片 chosen with a click, so the sidebar holds the focus and rings the chosen row
//   - sidebar-languages: the films heard in three languages, so the sidebar lists them under the sources, and 英语 chosen, the grid showing the films heard in English
func TestScreenshotsSidebar(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "sidebar-languages", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		heard := map[string]string{"j1": "en", "j3": "en", "j8": "en", "j9": "en", "j10": "en", "j4": "zh", "j11": "zh", "j15": "ja"}
		for i, job := range a.library.Jobs {
			a.library.Jobs[i].SpokenLanguage = heard[job.ID]
		}
		refresh(tt, a)
		tt.Press("id:spoken-en")
		tt.HoverOut()
	})
	f.shoot(t, "library-prototype-size", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
	})
	f.shoot(t, "sidebar-switcher-open", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		refresh(tt, a)
		if got := tt.Find("id:sidebar-switcher").Props.Label; got != testlocale.Messages().SidebarSwitcherLabel("我的影片") {
			t.Fatalf("switcher labelled %q", got)
		}
		tt.Press("id:sidebar-switcher")
		if len(tt.PopupMenu) == 0 {
			t.Fatal("the switcher's menu did not open")
		}
	})
	f.shoot(t, "sidebar-focused", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		refresh(tt, a)
		tt.Press("id:source-" + sidebar.SourceAll)
		tt.HoverOut()
	})
	f.shoot(t, "sidebar-empty", func(a *app, tt *nsgotest.App, updates chan func()) {
		screenshotReady(t, f, a)
		dir := filepath.Join(f.home, "Movies", "未命名资料库.kando")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		a.Choose(dir)
		screenshotWait(t, tt, a, updates, "the switcher's counts", func() bool {
			_, ok := a.LibraryCount(f.lib)
			return ok
		})
	})
}

// screenshotReady gives the fake tools what the status line asks for, a speech model on disk and an EveryAPI key, so it says 已就绪 as the prototype's does.
func screenshotReady(t *testing.T, f *screenshotFixture, a *app) {
	t.Helper()
	tools := a.pipeline.(media.Tools)
	tools.WhisperModel = filepath.Join(f.home, "models", "ggml-small.bin")
	if err := os.MkdirAll(filepath.Dir(tools.WhisperModel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tools.WhisperModel, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	tools.Translator.APIKey = "sk-screenshot"
	a.pipeline = tools
	a.checkReadiness(true)
	msgs := testlocale.Messages()
	if r := a.readiness(); r.State != sidebar.Ready {
		t.Fatalf("status line %v, want %q", r.State, msgs.ReadinessReady())
	}
}
