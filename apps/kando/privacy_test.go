package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"kando/apps/kando/cloud"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
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

// TestClearCacheKeepsTheCopyBeingMade clears while the open film's copy for the system's player is still being made: the part ffmpeg writes and the copy it becomes stay, other copies go.
func TestClearCacheKeepsTheCopyBeingMade(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{})
	root := t.TempDir()
	a.player.remuxDir = filepath.Join(root, "playback")
	video := filepath.Join(root, "talk.mkv")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest, err := film.RemuxPath(a.player.remuxDir, video, film.RemuxCopy)
	if err != nil {
		t.Fatal(err)
	}
	part, other := dest+"-1.part", filepath.Join(a.player.remuxDir, "other-1.mp4")
	for _, path := range []string{part, other} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("copy"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// playerRemux leaves source empty until the copy is whole.
	a.player.play = &filmPlay{path: video, remux: film.RemuxCopy}
	t.Cleanup(func() { a.player.play = nil })
	a.ClearCache()
	waitUntil(t, tt, a, updates, func() bool { return a.CacheClear().Done })
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("the copy being made is gone: %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("another copy is still there: %v", err)
	}
}

// TestClearCacheFindsThePostersAgain clears the posters folder: the posters and site pictures the app held for files that went are let go, so they are made again rather than shown from memory while their files are missing.
func TestClearCacheFindsThePostersAgain(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{})
	a.lib.posterDir = filepath.Join(t.TempDir(), "posters")
	video, address := "/videos/talk.mp4", "https://i.ytimg.com/vi/abc/hq.jpg"
	for _, path := range []string{posterDest(a.lib.posterDir, video), siteThumbnailPath(a.lib.posterDir, address)} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.lib.posters = map[string]ns.ImageSource{video: {Data: []byte("jpeg")}}
	a.lib.thumbs = map[string]ns.ImageSource{address: {Data: []byte("jpeg")}}
	a.ClearCache()
	waitUntil(t, tt, a, updates, func() bool { return a.CacheClear().Done })
	if _, ok := a.lib.posters[video]; ok {
		t.Fatal("the poster whose file went is still held")
	}
	if _, ok := a.lib.thumbs[address]; ok {
		t.Fatal("the site picture whose file went is still held")
	}
}

// TestOfflineReachesAJobUnderWay turns offline mode on after a job took its copy of the tools: the copy refuses the download and the cloud AI service from then on, so a job transcribing as offline mode comes on goes on but sends nothing.
func TestOfflineReachesAJobUnderWay(t *testing.T) {
	a, _, _ := newTestApp(t, cloudTools())
	// As the start does (privacyHooks' restore).
	a.privacyApply()
	held, ok := a.queuePipeline(core.Job{Source: "https://example.com/a"}).(media.Tools)
	if !ok {
		t.Fatal("the job's pipeline is not the tools")
	}
	if err := held.CheckTranslate(); err != nil {
		t.Fatalf("online: %v", err)
	}
	a.SetOffline(true)
	ctx := context.Background()
	if err := held.CheckTranslate(); !errors.Is(err, media.ErrOffline) {
		t.Fatalf("translating after offline mode came on: %v", err)
	}
	if _, err := held.Proofread(ctx, []core.Segment{{Text: "x"}}, "t"); !errors.Is(err, media.ErrOffline) {
		t.Fatalf("proofreading after offline mode came on: %v", err)
	}
	if _, err := held.Download(ctx, "https://example.com/a"); !errors.Is(err, media.ErrOffline) {
		t.Fatalf("downloading after offline mode came on: %v", err)
	}
	a.SetOffline(false)
	if err := held.CheckTranslate(); err != nil {
		t.Fatalf("after offline mode went off: %v", err)
	}
}

// checkGate is a gatePipeline whose first check, the job's, waits, so a link's job is under way but has not downloaded yet. The app's own checks of what is installed (checkReadiness) go through.
type checkGate struct {
	*gatePipeline
	checking chan struct{}
	proceed  chan struct{}
	first    *atomic.Bool
}

func (g checkGate) Check() error {
	if g.first.CompareAndSwap(false, true) {
		g.checking <- struct{}{}
		<-g.proceed
	}
	return nil
}

// TestOfflineStopsALinkAboutToDownload turns offline mode on while a link's job checks what it needs, before it downloads: it stops, as one downloading does.
func TestOfflineStopsALinkAboutToDownload(t *testing.T) {
	g := checkGate{gatePipeline: newGatePipeline(), checking: make(chan struct{}, 1), proceed: make(chan struct{}), first: new(atomic.Bool)}
	a, _, updates := newTestApp(t, g)
	t.Cleanup(func() { close(g.release) })
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	<-g.checking
	id := jobIDs(a)["https://example.com/a"]
	if a.queue.active != id || stageOf(t, a, id).Stage != core.StageQueued {
		t.Fatalf("active %q, job %+v", a.queue.active, stageOf(t, a, id))
	}
	a.SetOffline(true)
	close(g.proceed)
	for a.queue.active == id {
		(<-updates)()
	}
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("the link about to download after offline mode came on: %+v", job)
	}
}

// TestOfflineHoldsALinkWhoseDownloadDidNotMerge holds a link's job whose video is there but whose download left parts beside it, as the pipeline downloads that one again.
func TestOfflineHoldsALinkWhoseDownloadDidNotMerge(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{})
	a.SetOffline(true)
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := core.Job{Source: "https://example.com/a", VideoPath: video}
	if a.privacyHolds(job) {
		t.Fatal("a downloaded video is held")
	}
	if err := os.WriteFile(filepath.Join(dir, "video.f137.mp4.part"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !a.privacyHolds(job) {
		t.Fatal("a download that did not merge is not held")
	}
}

// TestOfflineStopsASignInAndForgetsItsRefusals turns offline mode on during a sign-in, which stops and says why, and off again, which takes away what offline mode refused.
func TestOfflineStopsASignInAndForgetsItsRefusals(t *testing.T) {
	m := testlocale.Messages()
	t.Setenv("KANDO_GOOGLE_CLIENT_ID", "client")
	a, tt, updates := newTestApp(t, cloudTools())
	a.cloud.googleLogin = func(ctx context.Context, _ cloud.GoogleConfig) (cloud.Account, error) {
		<-ctx.Done()
		return cloud.Account{}, ctx.Err()
	}
	a.SignInGoogle()
	if !a.cloud.googleBusy {
		t.Fatal("sign-in did not start")
	}
	a.SetOffline(true)
	waitUntil(t, tt, a, updates, func() bool { return !a.cloud.googleBusy })
	if got := a.GoogleErr(); got != m.PrivacyNoSignIn() {
		t.Fatalf("sign-in stopped by offline mode says %q", got)
	}
	a.kits.errs["ffmpeg"] = media.ErrOffline
	a.prefs.downloadErr = media.ErrOffline
	a.prefs.llmTestErr = media.ErrOffline
	a.prefs.llmModelsFor, a.prefs.llmModelsErr = llmModelsKey(a.pipeline.(media.Tools).Translator), media.ErrOffline
	a.SetOffline(false)
	if a.GoogleErr() != "" || a.kits.errs["ffmpeg"] != nil || a.prefs.downloadErr != nil || a.prefs.llmTestErr != nil || a.prefs.llmModelsFor != "" {
		t.Fatalf("after offline mode went off: sign-in %q, program %v, model %v, test %v, models for %q", a.GoogleErr(), a.kits.errs["ffmpeg"], a.prefs.downloadErr, a.prefs.llmTestErr, a.prefs.llmModelsFor)
	}
}

// TestOfflineKeepsTheBrowserOnItsStart turns offline mode on while a page shows: the browser goes to its start, and neither 后退, 前进, 重新载入, an address nor a page the web view goes to by itself, as a swipe back does, opens a page until offline mode is off.
func TestOfflineKeepsTheBrowserOnItsStart(t *testing.T) {
	m := testlocale.Messages()
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/a")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/a", CanGoBack: true, CanGoForward: true})
	a.SetOffline(true)
	refresh(tt, a)
	if a.browseShowing() || tt.FindMounted(browsePage).Web.Source.Src != browseBlank {
		t.Fatalf("offline mode left the page at %q", a.browse.url)
	}
	browseReport(tt, ns.WebNavigation{URL: browseBlank, CanGoBack: true, CanGoForward: true})
	a.BrowseBack()
	a.BrowseForward()
	a.browseLoad("https://example.com/b")
	refresh(tt, a)
	for _, kind := range []nsgotest.WebCommandKind{nsgotest.WebGoBack, nsgotest.WebGoForward, nsgotest.WebReload} {
		if n := len(browseCommands(tt, kind)); n != 0 {
			t.Fatalf("command %v sent %d times offline", kind, n)
		}
	}
	if a.browse.source != browseBlank || a.browse.notice.Text != m.PrivacyNoBrowse() || a.browse.notice.Section != settings.SectionPrivacy {
		t.Fatalf("source %q, notice %+v", a.browse.source, a.browse.notice)
	}
	// A swipe back reaches the page before: the browser leaves it for the start at once.
	scripts := len(browseScripts(tt))
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/a", CanGoBack: true, CanGoForward: true})
	if a.browseShowing() || len(browseScripts(tt)) == scripts {
		t.Fatalf("a page the web view went to offline stays: url %q", a.browse.url)
	}
	a.SetOffline(false)
	a.BrowseBack()
	refresh(tt, a)
	if n := len(browseCommands(tt, nsgotest.WebGoBack)); n != 1 {
		t.Fatalf("back after offline mode went off went %d pages back", n)
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
