package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSidebar renders the sidebar in the prototype's library states, at the size of its window (spec/library/<state>.window.png), with the status line ready as the prototype's is (screenshot_helpers_test.go):
//   - library-prototype-size: 我的影片 with 全部影片 chosen (library_default)
//   - sidebar-switcher-open: the library switcher pressed, its menu open (library_switcher_open); the menu is the system's, which the picture does not show
//   - sidebar-empty: a new empty library, every count 0 (library_empty)
func TestScreenshotsSidebar(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "library-prototype-size", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
	})
	f.shoot(t, "sidebar-switcher-open", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
		tt.Frame()
		pill, ok := tt.Find("切换资料库，当前：我的影片")
		if !ok {
			t.Fatalf("no library switcher in %q", tt.Texts())
		}
		tt.Press(pill.X+pill.W/2, pill.Y+pill.H/2)
		tt.Frame()
		if len(tt.Menu()) == 0 {
			t.Fatal("the switcher's menu did not open")
		}
	})
	f.shoot(t, "sidebar-empty", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotReady(t, f, a)
		dir := filepath.Join(f.home, "Movies", "未命名资料库.lingocut")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		a.Choose(dir)
		screenshotWait(t, tt, updates, "the switcher's counts", func() bool {
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
	if r := a.readiness(); r.Message != readyMessage {
		t.Fatalf("status line %q, want %q", r.Message, readyMessage)
	}
}
