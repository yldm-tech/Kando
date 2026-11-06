package main

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsLibrary renders the library page at the prototype's window size (1304×782) to PNG files in the directory named by LINGOCUT_SCREENSHOTS (screenshot_helpers_test.go), with the prototype's films: the hero, 处理中 with waiting, running and failed jobs, the grid, the empty pages, the new-library sheet, the 从资料库移除 confirmation for a film alone in its folder and for one whose folder holds another folder, and a video dropped over an open film. The shared fixture's two films are too few for the hero, the queue and the grid at once, so the scenes use a library of their own, in the fixture's fake home, whose videos are placeholders the fake ffmpeg draws the posters from. It is skipped when the variable is unset.
func TestScreenshotsLibrary(t *testing.T) {
	f := newScreenshotFixture(t)
	out := f.out
	lib := filepath.Join(f.home, "Movies", "我的影片.lingocut")
	// clip makes the folder of a film in the library with a placeholder video of the given length, whose picture the fake ffmpeg draws from top to bottom (writeFakeClip).
	clip := func(name string, seconds int, top, bottom string) string {
		t.Helper()
		video := filepath.Join(lib, name, "video.mp4")
		writeFakeClip(t, video, seconds, top, bottom)
		return video
	}
	rivers := clip("How Rivers Shape Landscapes", 180, "8DB9D2", "3E6A5B")
	srt := filepath.Join(filepath.Dir(rivers), "video.bilingual.srt")
	cues := "1\n00:00:01,000 --> 00:00:04,000\n每条河都始于高山上的一股细流。\nEvery river begins as a trickle high in the mountains.\n\n"
	if err := os.WriteFile(srt, []byte(cues), 0o644); err != nil {
		t.Fatal(err)
	}
	algebra := clip("Linear Algebra, Lecture 1", 180, "3B3F6B", "1C1E33")
	meeting := clip("会议录屏 2026-10-28", 180, "D9D4CC", "8A8279")
	tides := clip("The Tide Pools of Monterey", 180, "E3B27A", "6E4A3A")
	market := clip("Morning Market in Kyoto", 120, "C9645A", "5A2E2A")
	finished := func(id, source, video string) core.Job {
		return core.Job{ID: id, Source: source, Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: filepath.Join(filepath.Dir(video), "video.bilingual.srt"), LibraryDir: lib}
	}
	jobs := []core.Job{
		{ID: "market", Source: "https://www.youtube.com/watch?v=Kyoto0market", Language: "简体中文", Stage: core.StageTranslating, VideoPath: market, LibraryDir: lib},
		{ID: "ocean", Source: "https://www.youtube.com/watch?v=Ocean0curre", Language: "简体中文", Stage: core.StageQueued, LibraryDir: lib},
		{ID: "street", Source: "https://www.youtube.com/watch?v=Street0food", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("download: yt-dlp: ERROR: Unable to extract nsig function"), LibraryDir: lib},
		{ID: "lesson", Source: "https://www.bilibili.com/video/BV1GJ411x7h7", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("check: LINGOCUT_WHISPER_MODEL is not set"), LibraryDir: lib},
		finished("rivers", "https://www.youtube.com/watch?v=TRWgBxVAb1M", rivers),
		finished("algebra", "https://www.youtube.com/watch?v=Linear0alge", algebra),
		finished("meeting", "file://"+meeting, meeting),
		finished("tides", "https://www.youtube.com/watch?v=Tide0pools0", tides),
	}

	shoot := func(name string, width, height int, setup func(a *app), steps func(a *app, tt *ui.Tester)) {
		t.Helper()
		updates := make(chan func(), 64)
		a := newApp(f.pipeline, func(fn func()) { updates <- fn })
		a.edit(func(s *session) {
			s.LibraryDir = lib
			s.Libraries = []string{lib}
			s.LibraryParent = filepath.Join(f.home, "Movies")
		})
		a.setResumePositions(map[string]time.Duration{rivers: 100 * time.Second})
		setup(a)
		a.refreshLibrary()
		tt := ui.NewTester(a.view, width, height)
		tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
		tt.Frame()
		// The posters are grabbed in the background, from the shelf or, before this page was split, from the first frame.
		deadline := time.After(30 * time.Second)
		for len(a.lib.posterBusy) > 0 {
			select {
			case fn := <-updates:
				fn()
			case <-deadline:
				t.Fatalf("posters did not load: %v", a.lib.posterBusy)
			}
		}
		tt.Frame()
		steps(a, tt)
		tt.Frame()
		file, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	withJobs := func(a *app) { a.library.Jobs = append([]core.Job(nil), jobs...) }
	none := func(*app, *ui.Tester) {}
	// try clicks label when the page has it; the page before this change lacks some of them.
	try := func(tt *ui.Tester, label string) {
		if tt.Click(label) == nil {
			tt.Frame()
		}
	}

	shoot("library", 1304, 782, withJobs, func(_ *app, tt *ui.Tester) { try(tt, "开始处理") })
	shoot("library-full", 1304, 1500, withJobs, none)
	shoot("library-empty", 1304, 782, func(a *app) {
		empty := filepath.Join(f.home, "Movies", "新资料库.lingocut")
		if err := os.Mkdir(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		a.edit(func(s *session) { s.LibraryDir = empty })
	}, none)
	shoot("library-none", 1304, 782, func(a *app) {
		a.edit(func(s *session) {
			s.LibraryDir = ""
			s.Libraries = nil
		})
	}, none)
	shoot("library-new", 1304, 782, withJobs, func(_ *app, tt *ui.Tester) {
		try(tt, "资料库菜单")
		try(tt, "新建资料库...")
	})
	removeAlgebra := func(_ *app, tt *ui.Tester) {
		if tt.RightClick("Linear Algebra, Lecture 1") == nil {
			if tt.ChooseMenuItem("从资料库移除…") == nil {
				tt.Frame()
			}
		}
	}
	shoot("library-remove", 1304, 1100, withJobs, removeAlgebra)
	// A folder inside the film's folder keeps that folder out of the Trash, and the confirmation says only the video and its subtitles go.
	shoot("library-remove-files", 1304, 1100, func(a *app) {
		withJobs(a)
		if err := os.MkdirAll(filepath.Join(filepath.Dir(algebra), "extras"), 0o755); err != nil {
			t.Fatal(err)
		}
	}, removeAlgebra)
	// A video dropped over an open film: the window goes back to the library, whose message bar says what became of it.
	dropped := filepath.Join(f.home, "Downloads", "Ocean Currents.mp4")
	if err := os.MkdirAll(filepath.Dir(dropped), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropped, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	shoot("drop-on-film", 1304, 782, func(a *app) {
		withJobs(a)
		a.openKey = "rivers"
	}, func(_ *app, tt *ui.Tester) {
		tt.DropFiles(sidebarWidth+(1304-sidebarWidth)/2, 782/2, dropped)
	})
}
