package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsQueue renders the queue (screenshot_helpers_test.go): three links added at once while the pipeline holds the first in its download, so the queue shows one card downloading and the others waiting; and the confirmation ⌘Q opens over it while that job runs. Besides that it renders what the message bar says when the link added in the top bar is one the library already has waiting, one it has already processed, with 打开, and a channel's. It also renders the card of a download yt-dlp was too old for (Street Food Night Tour, j5) when the yt-dlp is one the system's packages installed, which Kando cannot update: without a yt-dlp Kando can download for itself, with one, and while that one downloads. And it renders j5 as a post on X whose video X did not show: downloaded signed out, and through Firefox's sign-in.
func TestScreenshotsQueue(t *testing.T) {
	f := newScreenshotFixture(t)
	// add types link into the top bar and clicks 开始处理. The fixture's tools have no speech model, which precheck asks for before a job is added, so the scene's pipeline is one precheck does not look into.
	add := func(t *testing.T, a *app, tt *nsgotest.App, link string) {
		t.Helper()
		a.pipeline = &fakePipeline{block: true}
		tt.Focus("视频链接")
		typeFocused(t, tt, link)
		screenshotClick(t, tt, "开始处理")
		refresh(tt, a)
		if len(a.library.Jobs) != len(f.jobs) {
			t.Fatalf("%s was queued: %+v", link, a.library.Jobs)
		}
	}
	f.shoot(t, "queue-already-queued", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		add(t, a, tt, "https://youtu.be/"+protoFilmByID("j6").VideoID)
	})
	f.shoot(t, "queue-already-in-library", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		add(t, a, tt, "https://youtu.be/"+protoFilmByID("j1").VideoID)
	})
	f.shoot(t, "queue-channel-link", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		add(t, a, tt, "https://www.youtube.com/@veritasium")
	})
	f.shoot(t, "queue", func(t *testing.T, a *app, _ *nsgotest.App, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
	})
	// The downloading job and the next were sent by a paired phone, whose name their cards give.
	f.shoot(t, "queue-from-phone", func(t *testing.T, a *app, _ *nsgotest.App, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
		for _, job := range a.library.Jobs {
			if job.Source == "https://www.youtube.com/watch?v=aqz-KE-bpKQ" || strings.Contains(job.Source, "bilibili.com") {
				job.Device = "iPhone 17"
				a.library.Upsert(job)
			}
		}
	})
	f.shoot(t, "queue-quit", func(t *testing.T, a *app, _ *nsgotest.App, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
		a.areasBeforeQuit(&quitEvent{})
	})
	f.shoot(t, "queue-outdated-system", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		queueShotOutdated(t, f, a, tt, false)
	})
	f.shoot(t, "queue-outdated-system-kit", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		queueShotOutdated(t, f, a, tt, true)
	})
	f.shoot(t, "queue-outdated-system-kit-downloading", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		queueShotOutdated(t, f, a, tt, true)
		if kit, ok := media.KitFor("yt-dlp"); ok {
			a.queue.ytdlp.kitRetry = true
			a.kits.running["yt-dlp"] = &kitDownload{written: kit.Size / 3, total: kit.Size, cancel: func() {}}
		}
		refresh(tt, a)
	})
	f.shoot(t, "queue-x-hidden-signed-out", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		queueShotHiddenX(t, f, a, tt, "")
	})
	f.shoot(t, "queue-x-hidden-signed-in", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		queueShotHiddenX(t, f, a, tt, "firefox")
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
			onEngine(a, fn)
		case <-deadline:
			t.Fatalf("no job started downloading: %+v", a.library.Jobs)
		}
	}
}

// queueShotOutdated makes j5's download one yt-dlp was too old for, with the yt-dlp a Linux distribution's packages put in /usr/bin, and scrolls 处理中 under the top bar. kit gives Kando a folder of its own in the fixture's home, so that it offers to download its own yt-dlp where there is one for the platform.
func queueShotOutdated(t *testing.T, f *screenshotFixture, a *app, tt *nsgotest.App, kit bool) {
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
	queueShotScroll(t, a, tt)
}

// queueShotHiddenX makes j5 a post on X that yt-dlp found no video in (core.FailHiddenVideo) when it borrowed the sign-in of cookies, "" for none, on a computer with Chrome and Firefox, and scrolls 处理中 under the top bar.
func queueShotHiddenX(t *testing.T, f *screenshotFixture, a *app, tt *nsgotest.App, cookies string) {
	t.Helper()
	job := f.job(t, "j5")
	job.Source, job.FailKind, job.Cookies = "https://x.com/streetfood/status/2122000096271503767", core.FailHiddenVideo, cookies
	job.Err = errors.New("download: yt-dlp: exit status 1: ERROR: [twitter] 2122000096271503767: No video could be found in this tweet")
	a.library.Upsert(job)
	a.queue.ytdlp.browsers = []media.Browser{media.Browsers[0], media.Browsers[1]}
	queueShotScroll(t, a, tt)
}

// queueShotScroll scrolls 处理中 under the top bar.
func queueShotScroll(t *testing.T, a *app, tt *nsgotest.App) {
	t.Helper()
	refresh(tt, a)
	title := whole(testlocale.Messages().FilmsQueueTitle())
	if !tt.Exists(title) {
		t.Fatalf("no queue to scroll to; texts %q", tt.Texts())
	}
	if d := pageFrame(tt.Find(title)).Origin.Y - 66; d*d >= 0.25 {
		tt.Scroll(title, d)
		refresh(tt, a)
	}
}

// TestScreenshotsQueueOtherLibrary renders 英语听力 opened while a link added to 我的影片 downloads: the job goes on, and 英语听力's 处理中 shows it, and the film waiting in 我的影片 after it, each saying the library it saves into, above 英语听力's own film.
func TestScreenshotsQueueOtherLibrary(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "queue-other-library", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.pipeline = &fakePipeline{block: true}
		t.Cleanup(a.stop)
		_ = a.Enqueue("https://www.youtube.com/watch?v=aqz-KE-bpKQ")
		screenshotWait(t, tt, a, updates, "the download", func() bool {
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
		refresh(tt, a)
		screenshotWait(t, tt, a, updates, "posters", func() bool { return len(a.lib.posterBusy) == 0 })
	})
}
