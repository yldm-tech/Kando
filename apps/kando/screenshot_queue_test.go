package main

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestScreenshotsQueue renders the queue (screenshot_helpers_test.go): three links added at once while the pipeline holds the first in its download, so the queue shows one card downloading and the others waiting; and the confirmation ⌘Q opens over it while that job runs. Besides that it renders what the message bar says when the link added in the top bar is one the library already has waiting, one it has already processed, with 打开, and a channel's. It also renders the card of a download yt-dlp was too old for (Street Food Night Tour, j5) when the yt-dlp is one the system's packages installed, which Kando cannot update: without a yt-dlp Kando can download for itself, with one, and while that one downloads.
func TestScreenshotsQueue(t *testing.T) {
	f := newScreenshotFixture(t)
	// add types link into the top bar and clicks 开始处理. The fixture's tools have no speech model, which precheck asks for before a job is added, so the scene's pipeline is one precheck does not look into.
	add := func(a *app, tt *ui.Tester, link string) {
		t.Helper()
		a.pipeline = &fakePipeline{block: true}
		screenshotClick(t, tt, "视频链接")
		tt.Type(link)
		screenshotClick(t, tt, "开始处理")
		tt.Frame()
		if len(a.library.Jobs) != len(f.jobs) {
			t.Fatalf("%s was queued: %+v", link, a.library.Jobs)
		}
	}
	f.shoot(t, "queue-already-queued", func(a *app, tt *ui.Tester, _ chan func()) {
		add(a, tt, "https://youtu.be/"+protoFilmByID("j6").VideoID)
	})
	f.shoot(t, "queue-already-in-library", func(a *app, tt *ui.Tester, _ chan func()) {
		add(a, tt, "https://youtu.be/"+protoFilmByID("j1").VideoID)
	})
	f.shoot(t, "queue-channel-link", func(a *app, tt *ui.Tester, _ chan func()) {
		add(a, tt, "https://www.youtube.com/@veritasium")
	})
	f.shoot(t, "queue", func(a *app, _ *ui.Tester, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
	})
	f.shoot(t, "queue-quit", func(a *app, _ *ui.Tester, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
		a.areasBeforeQuit(&mygo.QuitEvent{})
	})
	f.shoot(t, "queue-outdated-system", func(a *app, tt *ui.Tester, _ chan func()) {
		queueShotOutdated(t, f, a, tt, false)
	})
	f.shoot(t, "queue-outdated-system-kit", func(a *app, tt *ui.Tester, _ chan func()) {
		queueShotOutdated(t, f, a, tt, true)
	})
	f.shoot(t, "queue-outdated-system-kit-downloading", func(a *app, tt *ui.Tester, _ chan func()) {
		queueShotOutdated(t, f, a, tt, true)
		if kit, ok := media.KitFor("yt-dlp"); ok {
			a.queue.ytdlp.kitRetry = true
			a.kits.running["yt-dlp"] = &kitDownload{written: kit.Size / 3, total: kit.Size, cancel: func() {}}
		}
		tt.Frame()
	})
}

// screenshotQueueDownloading adds three links at once while the pipeline holds the first in its download, and returns once that one is downloading.
func screenshotQueueDownloading(t *testing.T, a *app, updates chan func()) {
	t.Helper()
	a.pipeline = &fakePipeline{block: true}
	t.Cleanup(a.stop)
	_ = a.Enqueue("https://www.youtube.com/watch?v=aqz-KE-bpKQ", "https://www.bilibili.com/video/BV1GJ411x7h7", "https://vimeo.com/76979871")
	deadline := time.After(5 * time.Second)
	for !slices.ContainsFunc(a.library.Jobs, func(j core.Job) bool { return j.Stage == core.StageDownloading }) {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("no job started downloading: %+v", a.library.Jobs)
		}
	}
}

// queueShotOutdated makes j5's download one yt-dlp was too old for, with the yt-dlp a Linux distribution's packages put in /usr/bin, and scrolls 处理中 under the top bar. kit gives Kando a folder of its own in the fixture's home, so that it offers to download its own yt-dlp where there is one for the platform.
func queueShotOutdated(t *testing.T, f *screenshotFixture, a *app, tt *ui.Tester, kit bool) {
	t.Helper()
	if kit {
		saved := media.OwnToolsDir
		media.OwnToolsDir = filepath.Join(f.home, "Library", "Application Support", "Kando", "tools")
		t.Cleanup(func() { media.OwnToolsDir = saved })
	}
	job := f.job(t, "j5")
	job.FailKind = core.FailOutdated
	a.library.Upsert(job)
	a.queue.ytdlp.path, a.queue.ytdlp.version = "/usr/bin/yt-dlp", "2024.03.10"
	a.queue.ytdlp.install = media.YTDLPInstall{Kind: "system", Dir: "/usr/bin"}
	tt.Frame()
	for range 4 {
		r, ok := tt.Find(locales.FilmsQueueTitle())
		if !ok {
			t.Fatalf("no queue to scroll to; texts %q", tt.Texts())
		}
		if d := r.Y - 66; d*d >= 0.25 {
			tt.Move(760, 500)
			tt.Scroll(760, 500, 0, d)
			tt.Frame()
		}
	}
}

// TestScreenshotsQueueOtherLibrary renders 英语听力 opened while a link added to 我的影片 downloads: the job goes on, and 英语听力's 处理中 shows it, and the film waiting in 我的影片 after it, each saying the library it saves into, above 英语听力's own film.
func TestScreenshotsQueueOtherLibrary(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "queue-other-library", func(a *app, tt *ui.Tester, updates chan func()) {
		a.pipeline = &fakePipeline{block: true}
		t.Cleanup(a.stop)
		_ = a.Enqueue("https://www.youtube.com/watch?v=aqz-KE-bpKQ")
		screenshotWait(t, tt, updates, "the download", func() bool {
			return slices.ContainsFunc(a.library.Jobs, func(j core.Job) bool { return j.Stage == core.StageDownloading })
		})
		// The running job keeps the pipeline it started with; the library opened next reads its posters with the fake ffmpeg.
		a.pipeline = f.pipeline
		t.Cleanup(func() {
			if a.libraryWatch != nil {
				close(a.libraryWatch)
				a.libraryWatch = nil
			}
		})
		a.switchLibrary(f.libs["listening"])
		tt.Frame()
		screenshotWait(t, tt, updates, "posters", func() bool { return len(a.lib.posterBusy) == 0 })
	})
}
