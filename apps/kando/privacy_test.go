package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// cloudTools are tools whose AI service is in the cloud, as EveryAPI with a key.
func cloudTools() media.Tools {
	return media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, APIKey: "k", Model: "m"}}
}

func TestOfflineAddsLocalVideosButNoLinks(t *testing.T) {
	g := newGatePipeline()
	a, _, updates := newTestApp(t, g)
	// A job left in Download goes on, so a failed test ends rather than waiting on it.
	t.Cleanup(func() { close(g.release) })
	a.SetOffline(true)
	err := a.Enqueue("https://www.youtube.com/watch?v=abc")
	if !errors.Is(err, media.ErrOffline) || err.Error() != testlocale.Messages().PrivacyNoDownload() || len(a.library.Jobs) != 0 {
		t.Fatalf("a link offline: %v, jobs %+v", err, a.library.Jobs)
	}
	// The link field's check says so before anything is added, with the section that turns it off.
	if msg, section := a.precheck([]string{"https://www.youtube.com/watch?v=abc"}); msg != testlocale.Messages().PrivacyNoDownload() || section != settings.SectionPrivacy {
		t.Fatalf("precheck %q in %q", msg, section)
	}
	video := filepath.Join(t.TempDir(), "talk.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Enqueue(core.FileURL(video)); err != nil {
		t.Fatalf("a local video offline: %v", err)
	}
	if got := nextStarted(t, g, updates); got != core.FileURL(video) {
		t.Fatalf("started %s", got)
	}
}

// TestOfflineHoldsDownloadsUntilItIsOff turns offline mode on while one link downloads and another waits: the download stops, the one waiting stays queued rather than failing, and it starts once offline mode is off.
func TestOfflineHoldsDownloadsUntilItIsOff(t *testing.T) {
	g := newGatePipeline()
	a, _, updates := newTestApp(t, g)
	// A job left in Download goes on, so a failed test ends rather than waiting on it.
	t.Cleanup(func() { close(g.release) })
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("first started %s", got)
	}
	ids := jobIDs(a)
	a.SetOffline(true)
	for a.queue.active == ids["https://example.com/a"] {
		(<-updates)()
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("the download under way after offline mode came on: %+v", job)
	}
	noneStarted(t, g, updates)
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageQueued {
		t.Fatalf("the download waiting while offline: %+v", job)
	}
	a.SetOffline(false)
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("after offline mode went off, started %s", got)
	}
}

func TestOfflineGoesWithThePipelineAndTheSettings(t *testing.T) {
	a, _, _ := newTestApp(t, cloudTools())
	a.SetOffline(true)
	// What changes the tools afterwards starts from the pipeline's, so it keeps offline mode.
	a.prefs.llmBase, a.prefs.llmModel = "http://127.0.0.1:11434/v1", "llama"
	a.applyLLM()
	tools := a.pipeline.(media.Tools)
	if !tools.Offline || !tools.Translator.Offline || !a.privacyLocalAI() {
		t.Fatalf("tools after the AI service changed: offline %v, translator offline %v, local %v", tools.Offline, tools.Translator.Offline, a.privacyLocalAI())
	}
	saved := a.settingsFile()
	if !saved.Offline {
		t.Fatal("offline mode is not saved")
	}
	b := newApp(cloudTools(), func(fn func()) { fn() })
	b.restoreSettings(saved)
	if !b.Offline() || !b.pipeline.(media.Tools).Offline {
		t.Fatal("offline mode is not restored into the pipeline")
	}
}

func TestOfflineStatusLineSaysWhatStillRuns(t *testing.T) {
	m := testlocale.Messages()
	a, _, _ := newTestApp(t, cloudTools())
	ready := sidebar.Readiness{State: sidebar.Ready, Message: readyMessage}
	if got := a.privacyReadiness(ready); got.State != sidebar.Ready {
		t.Fatalf("online %+v", got)
	}
	a.SetOffline(true)
	got := a.privacyReadiness(ready)
	if got.State != sidebar.Missing || got.Section != settings.SectionAI || got.Text(a.cx) != m.PrivacyNoCloudAI() {
		t.Fatalf("offline with a cloud AI service: %+v", got)
	}
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "http://localhost:1234/v1", Model: "m"}}
	got = a.privacyReadiness(ready)
	if got.State != sidebar.Ready || got.Section != settings.SectionPrivacy || got.Text(a.cx) != m.ReadinessOffline() {
		t.Fatalf("offline with a local AI service: %+v", got)
	}
	// What is missing still says so first.
	missing := sidebar.Readiness{State: sidebar.Missing, Message: noSpeechModel}
	if got := a.privacyReadiness(missing); got.State != sidebar.Missing || got.Text(a.cx) != m.PrecheckNoSpeechModel() {
		t.Fatalf("offline with no speech model: %+v", got)
	}
}

func TestOfflineRefusesWhatWouldReachTheNetwork(t *testing.T) {
	m := testlocale.Messages()
	a, tt, _ := newTestApp(t, cloudTools())
	checked := 0
	a.updates.check = func(*ns.Scope) { checked++ }
	a.SetOffline(true)

	// The AI service in the cloud: the palette does not ask it, and a film that was not translated for it says why.
	if _, why := a.paletteTranslator(func(*ns.Scope) string { return "none" }); why == nil || why(a.cx) != m.PrivacyNoCloudAI() {
		t.Fatal("the palette asks a cloud service offline")
	}
	if reason, section := translationHint(m, media.ErrOffline.Error()); reason != m.PrivacyNoCloudAI() || section != settings.SectionAI {
		t.Fatalf("translation hint %q in %q", reason, section)
	}
	// Uploads, sign-in and the update check.
	if got := a.uploadBlocked(); got != m.PrivacyNoUpload() {
		t.Fatalf("upload blocked %q", got)
	}
	a.CheckForUpdates()
	if checked != 0 || a.lib.notice.Text != m.PrivacyNoUpdate() || a.lib.notice.Section != settings.SectionPrivacy {
		t.Fatalf("update check offline: checked %d, notice %+v", checked, a.lib.notice)
	}
	a.SetOffline(false)
	a.CheckForUpdates()
	if checked != 1 {
		t.Fatal("the update check did not run online")
	}
	a.SetOffline(true)

	// Downloads of a program and of a speech model.
	if _, ok := media.KitFor("ffmpeg"); ok {
		saved := media.OwnToolsDir
		media.OwnToolsDir = t.TempDir()
		t.Cleanup(func() { media.OwnToolsDir = saved })
		a.kits.install = func(context.Context, media.Kit, string, func(int64, int64)) (string, error) {
			t.Error("a program was downloaded offline")
			return "", nil
		}
		a.kitsInstall("ffmpeg")
		if a.kits.running["ffmpeg"] != nil || settings.KitsError(m, a.kits.errs["ffmpeg"]) != m.PrivacyNoNetworkDownload() {
			t.Fatalf("program download offline: running %v, err %v", a.kits.running["ffmpeg"], a.kits.errs["ffmpeg"])
		}
	}
	a.prefs.modelsDir = t.TempDir()
	a.downloadSpeechModel("base")
	if a.prefs.downloading != "" || settings.SpeechDownloadError(m, a.prefs.downloadErr) != m.PrivacyNoNetworkDownload() {
		t.Fatalf("model download offline: downloading %q, err %v", a.prefs.downloading, a.prefs.downloadErr)
	}
	// The AI page's test and model list say why they did not ask.
	if got := settings.LLMTestError(m, media.ErrOffline); got != m.PrivacyNoCloudAI() {
		t.Fatalf("test error %q", got)
	}
	refresh(tt, a)
}

func TestOfflineShowsNoSitePictureNotFetchedYet(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{})
	a.lib.posterDir = t.TempDir()
	fetched := 0
	saved := fetchSiteThumbnail
	fetchSiteThumbnail = func(context.Context, string, string) error { fetched++; return errors.New("no") }
	t.Cleanup(func() { fetchSiteThumbnail = saved })
	a.SetOffline(true)
	a.ensureThumbnail(films.Film{Thumbnail: "https://i.ytimg.com/vi/abc/hq.jpg", Key: "k"})
	if fetched != 0 || a.lib.thumbBusy["https://i.ytimg.com/vi/abc/hq.jpg"] || a.lib.thumbMiss["https://i.ytimg.com/vi/abc/hq.jpg"] {
		t.Fatalf("offline fetched %d, busy %v, miss %v", fetched, a.lib.thumbBusy, a.lib.thumbMiss)
	}
}

func TestClearCacheKeepsTheCopyPlaying(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{})
	root := t.TempDir()
	a.lib.posterDir = filepath.Join(root, "posters")
	a.player.remuxDir = filepath.Join(root, "playback")
	for path, size := range map[string]int{filepath.Join(a.lib.posterDir, "a.jpg"): 1 << 20, filepath.Join(a.player.remuxDir, "x", "old.mp4"): 1 << 20} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.ClearCache()
	if !a.CacheClear().Busy {
		t.Fatal("not busy while clearing")
	}
	waitUntil(t, tt, a, updates, func() bool { return a.CacheClear().Done })
	if got := a.CacheClear(); got.Freed != 2<<20 || got.Err != "" {
		t.Fatalf("cleared %+v", got)
	}
}
