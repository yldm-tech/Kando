package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// screenshotDropFolder writes ~/Downloads/动漫, a folder of the user's of 2 seasons of 13 episodes each, a booklet and a program named as an episode, and returns it. The episodes are files of 1.1 GB that take no room (sparse), so the sheet says what such a folder takes.
func screenshotDropFolder(t *testing.T, f *screenshotFixture) string {
	t.Helper()
	show := filepath.Join(f.home, "Downloads", "动漫")
	for season := 1; season <= 2; season++ {
		dir := filepath.Join(show, fmt.Sprintf("第%d季", season))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for ep := 1; ep <= 13; ep++ {
			path := filepath.Join(dir, fmt.Sprintf("第%02d话.mp4", ep))
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(path, 1100<<20); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, body := range map[string]string{"第1季/booklet.pdf": "pdf", "第2季/第05话.mp4.exe": "MZ"} {
		if err := os.WriteFile(filepath.Join(show, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { os.RemoveAll(show) })
	return show
}

// TestScreenshotsDrop renders what dropping a large folder does (package dropsheet):
//   - drop-sheet: ~/Downloads/动漫 dropped, 26 episodes in two season folders: how many and how much room, the booklet skipped, the two ways to add them with 只加入 chosen, the category named after the folder, and the program named as an episode
//   - drop-sheet-generate: the same, 全部生成字幕 chosen
//   - drop-added: the library once they were only added, the episodes among its films without subtitles, and the message bar saying so and naming the program it left out
func TestScreenshotsDrop(t *testing.T) {
	f := newScreenshotFixture(t)
	open := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotReady(t, f, a)
		show := screenshotDropFolder(t, f)
		apply(tt, a, func() { a.dropFiles([]string{show}) })
		dropRead(t, tt, a, updates)
		if !tt.Exists("id:drop-sheet") {
			t.Fatal("the sheet did not open")
		}
	}
	f.shoot(t, "drop-sheet", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates)
	})
	f.shoot(t, "drop-sheet-generate", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates)
		tt.Press("id:drop-generate")
		tt.HoverOut()
		refresh(tt, a)
	})
	f.shoot(t, "drop-added", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates)
		t.Cleanup(func() { os.Remove(filepath.Join(f.lib, "categories.json")) })
		tt.Press("id:drop-add")
		refresh(tt, a)
		if len(addedFilms(a)) != 26 {
			t.Fatalf("added %d", len(addedFilms(a)))
		}
	})
}
