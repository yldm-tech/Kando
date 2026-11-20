package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/addsheet"
	"kando/apps/kando/browse"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
)

// A link to one online video pasted in the link field opens the Add sheet rather than being queued: it says where the link starts, and what the user chooses on it, the original lines, the picture, no sign-in and the chapters, goes onto the job, which opens where the link starts once done. Cancel and Escape close it without adding anything.
func TestAddSheetAddsALinkWithWhatWasChosen(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	msgs := testlocale.Messages()
	a.signin.browser = "chrome"
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw&t=1m24s"

	tt.Type(msgs.FilmsLinkLabel(), link)
	tt.Press(whole(msgs.FilmsStart()))
	if !a.addsheet.open || !a.modalOpen() || len(a.library.Jobs) != 0 || a.queue.source != "" {
		t.Fatalf("sheet open %v, modal %v, jobs %+v, field %q", a.addsheet.open, a.modalOpen(), a.library.Jobs, a.queue.source)
	}
	wantText(t, tt, msgs.AddSheetResumeLink("1:24"))
	wantText(t, tt, msgs.AddSheetUntitled("YouTube"))
	if c := a.addsheet.choice; c.Quality != core.QualityBest || c.Source != core.SubtitlesPlatform || c.Browser != "chrome" || !c.Chapters {
		t.Fatalf("the sheet does not start from the settings: %+v", c)
	}
	tt.Key("escape")
	if a.addsheet.open || a.modalOpen() || len(a.library.Jobs) != 0 {
		t.Fatalf("Escape: sheet open %v, modal %v, jobs %d", a.addsheet.open, a.modalOpen(), len(a.library.Jobs))
	}

	tt.Type(msgs.FilmsLinkLabel(), link)
	tt.Press(whole(msgs.FilmsStart()))
	tt.Press("id:addsheet-source-local")
	tt.Press("id:addsheet-chapters")
	apply(tt, a, func() {
		a.addsheet.choice.Quality = core.Quality720
		a.addsheet.choice.Browser = ""
	})
	tt.Press("id:addsheet-add")
	if a.addsheet.open || a.modalOpen() || len(a.library.Jobs) != 1 {
		t.Fatalf("after Add: sheet open %v, modal %v, jobs %+v", a.addsheet.open, a.modalOpen(), a.library.Jobs)
	}
	job := a.library.Jobs[0]
	if job.Quality != core.Quality720 || job.SubtitleSource != core.SubtitlesLocal || job.Cookies != core.CookiesNone || job.Browser() != "" || job.ChapterUse != core.ChaptersEven || job.ResumeAt != 84*time.Second {
		t.Fatalf("job %+v", job)
	}
}

// A job queued with no sign-in on the sheet borrows none, though Settings › 通用 › 浏览器登录 names a browser; its download is not given that browser as it runs.
func TestAddSheetsNoSignInBorrowsNone(t *testing.T) {
	a := newApp(media.Tools{}, func(fn func()) { fn() })
	a.signin.browser = "chrome"
	job := core.Job{Source: "https://youtu.be/jNQXAC9IVRw", Cookies: core.CookiesNone}
	if tools, ok := a.queuePipeline(job).(media.Tools); !ok || tools.Cookies != "" {
		t.Fatalf("pipeline %+v", a.queuePipeline(job))
	}
}

// A link of a site whose videos are DRM-protected says so on the sheet, whose only button closes it, and nothing is added.
func TestAddSheetRefusesADRMSite(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://www.netflix.com/watch/80100172")
	tt.Press(whole(msgs.FilmsStart()))
	wantText(t, tt, msgs.AddSheetDrm("Netflix"))
	if tt.Exists("id:addsheet-add") || tt.Exists("id:addsheet-quality") {
		t.Fatalf("a DRM site offers to add: %q", tt.Texts())
	}
	tt.Press("id:addsheet-ok")
	if a.addsheet.open || a.modalOpen() || len(a.library.Jobs) != 0 {
		t.Fatalf("sheet open %v, modal %v, jobs %d", a.addsheet.open, a.modalOpen(), len(a.library.Jobs))
	}
}

// A site Kando does not know is added all the same, after the sheet says that yt-dlp's generic extractor tries it; it offers no choice of the site's subtitles or chapters, which only YouTube and bilibili have.
func TestAddSheetWarnsOfAnUnknownSite(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://videos.example.org/watch/42")
	tt.Press(whole(msgs.FilmsStart()))
	wantText(t, tt, msgs.AddSheetUnknown("videos.example.org"))
	if tt.Exists("id:addsheet-source-platform") || tt.Exists("id:addsheet-chapters") {
		t.Fatalf("an unknown site offers the site's subtitles or chapters: %q", tt.Texts())
	}
	tt.Press("id:addsheet-add")
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].SubtitleSource != "" || a.library.Jobs[0].ChapterUse != "" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// Several links, a playlist, and a link precheck refuses are added as before, without the sheet.
func TestAddSheetOnlyForOneOnlineVideo(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	msgs := testlocale.Messages()
	a.queue.source = "https://youtu.be/TRWgBxVAb1M https://vimeo.com/76979871"
	refresh(tt, a)
	tt.Press(whole(msgs.FilmsStart()))
	if a.addsheet.open || len(a.library.Jobs) != 2 {
		t.Fatalf("several links: sheet open %v, jobs %d", a.addsheet.open, len(a.library.Jobs))
	}

	b, _, _ := newTestApp(t, media.Tools{})
	if b.addSheetAsk("https://youtu.be/jNQXAC9IVRw", addsheet.FromField, "", 0) {
		t.Fatal("a link precheck refuses opened the sheet")
	}
	if b.addSheetAsk("file:///videos/talk.mp4", addsheet.FromField, "", 0) || b.addSheetAsk(testPlaylist, addsheet.FromField, "", 0) {
		t.Fatal("a local video or a playlist opened the sheet")
	}
}

// A video of a playlist offers to add the whole playlist instead, which the playlist dialog lists before adding.
func TestAddSheetAddsTheWholePlaylist(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8")
	tt.Press(whole(msgs.FilmsStart()))
	wantText(t, tt, msgs.AddSheetKindInList())
	tt.Press("id:addsheet-whole-list")
	wantText(t, tt, msgs.AddSheetListVideos())
	tt.Press("id:addsheet-add")
	if got := <-p.calls; got.source != testPlaylist {
		t.Fatalf("listed %+v", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.playlist.open && !a.playlist.loading })
	if a.addsheet.open || len(a.library.Jobs) != 0 {
		t.Fatalf("sheet open %v, jobs %d", a.addsheet.open, len(a.library.Jobs))
	}
	tt.Press("id:playlist-add")
	if got := len(a.library.Jobs); got != 3 {
		t.Fatalf("%d jobs, want 3", got)
	}
}

// A video the browser extension sends opens the sheet in front, saying where the browser was, and the extension's popup says to confirm in Kando; once added and done, the film opens there, and the job forgets it. With 链接到达后立即开始 on, it is added at once, with where the browser was all the same.
func TestAddSheetFromTheBrowserExtension(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	remoteServe(t, a)
	msgs := testlocale.Messages()
	added, err := remoteCall[remote.BrowserAdded](t, updates, remote.MethodBrowserAdd, remote.BrowserAddParams{URL: "https://youtu.be/jNQXAC9IVRw", Title: "Me at the zoo", At: 84.6})
	if err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	if added != (remote.BrowserAdded{Message: msgs.AddSheetConfirmInKando()}) || !a.addsheet.open || len(a.library.Jobs) != 0 {
		t.Fatalf("added %+v, sheet open %v, jobs %d", added, a.addsheet.open, len(a.library.Jobs))
	}
	wantText(t, tt, "Me at the zoo")
	wantText(t, tt, msgs.AddSheetResumeBrowser("1:24"))
	tt.Press("id:addsheet-add")
	settle(t, a, tt, updates)
	job := a.library.Jobs[0]
	if job.Stage != core.StageComplete || job.ResumeAt != 0 || a.ResumeAt(job.VideoPath) != 84*time.Second {
		t.Fatalf("job %+v, resumes at %v", job, a.ResumeAt(job.VideoPath))
	}

	b, tb, bupdates := newTestApp(t, &fakePipeline{block: true})
	remoteServe(t, b)
	b.addsheet.startAtOnce = true
	added, err = remoteCall[remote.BrowserAdded](t, bupdates, remote.MethodBrowserAdd, remote.BrowserAddParams{URL: "https://youtu.be/jNQXAC9IVRw", At: 30})
	if err != nil {
		t.Fatal(err)
	}
	refresh(tb, b)
	if b.addsheet.open || len(b.library.Jobs) != 1 || b.library.Jobs[0].ResumeAt != 30*time.Second || added.Have != browse.HaveQueued {
		t.Fatalf("at once: added %+v, sheet open %v, jobs %+v", added, b.addsheet.open, b.library.Jobs)
	}
}

// A film that was watched already keeps its own resume position when a job for it completes again.
func TestAddSheetKeepsAPositionTheFilmHas(t *testing.T) {
	testVideosFor(t)
	a := newApp(media.Tools{}, func(fn func()) { fn() })
	a.player.positions = map[string]time.Duration{testVideos + "/talk.mp4": 5 * time.Second}
	a.library.Upsert(core.Job{ID: "j", Source: "https://youtu.be/jNQXAC9IVRw", Stage: core.StageComplete, VideoPath: testVideos + "/talk.mp4", ResumeAt: 84 * time.Second})
	if a.addSheetResume() {
		t.Fatal("kept a position over the film's own")
	}
	if job, _ := a.library.Find("j"); job.ResumeAt != 0 || a.ResumeAt(testVideos+"/talk.mp4") != 5*time.Second {
		t.Fatalf("job %+v, resumes at %v", job, a.ResumeAt(testVideos+"/talk.mp4"))
	}
}

// The bookmarklet's link opens the sheet in front for the page it names, whatever 链接到达后立即开始 says; a link of another shape is left alone, and a page that is no video goes into the link field.
func TestAddSheetFromTheBookmarklet(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	msgs := testlocale.Messages()
	a.addsheet.startAtOnce = true
	page := "https://www.bilibili.com/video/BV1nx4y1x7t1"
	apply(tt, a, func() {
		a.browserExtOpenLinks([]string{"kando://open?url=" + url.QueryEscape(page), "kando://add?url=" + url.QueryEscape(page) + "&title=" + url.QueryEscape("爆旋陀螺") + "&t=75"})
	})
	if !a.addsheet.open || a.addsheet.source != page || len(a.library.Jobs) != 0 {
		t.Fatalf("sheet open %v for %q, jobs %d", a.addsheet.open, a.addsheet.source, len(a.library.Jobs))
	}
	wantText(t, tt, "爆旋陀螺")
	wantText(t, tt, msgs.AddSheetResumeBrowser("1:15"))
	tt.Press("id:addsheet-cancel")

	channel := "https://www.youtube.com/@veritasium"
	apply(tt, a, func() { a.browserExtOpenLinks([]string{"kando://add?url=" + url.QueryEscape(channel)}) })
	if a.addsheet.open || a.queue.source != channel || len(a.library.Jobs) != 0 {
		t.Fatalf("a channel: sheet open %v, field %q, jobs %d", a.addsheet.open, a.queue.source, len(a.library.Jobs))
	}
}

// The bookmarklet's link reaches the add sheet when the system opens Kando with it, through the one handler areasReady registers (areasListenLinks): on macOS, and on Windows and Linux, where a second launch hands it to the running copy.
func TestAddSheetFromTheSystemsLink(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	page := "https://www.bilibili.com/video/BV1nx4y1x7t1"
	onEngine(a, a.areasListenLinks)
	tt.OpenLinks("kando://add?url=" + url.QueryEscape(page) + "&title=" + url.QueryEscape("爆旋陀螺"))
	refresh(tt, a)
	if !a.addsheet.open || a.addsheet.source != page || len(a.library.Jobs) != 0 {
		t.Fatalf("sheet open %v for %q, jobs %d", a.addsheet.open, a.addsheet.source, len(a.library.Jobs))
	}
	wantText(t, tt, "爆旋陀螺")
	tt.Press("id:addsheet-cancel")
}

// 链接到达后立即开始 is off until it is turned on in Settings › 通用, which saves it in settings.json, and takes it back at startup.
func TestAddSheetStartAtOnceIsSaved(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = t.TempDir() + "/settings.json"
	if a.LinkStartAtOnce() {
		t.Fatal("on by default")
	}
	st := settingsOpenAt(a, tt, settings.SectionGeneral)
	st.Press("id:settings-browser-start-at-once")
	if !a.LinkStartAtOnce() {
		t.Fatal("the switch did not turn it on")
	}
	data, _ := json.Marshal(a.settingsFile())
	if !strings.Contains(string(data), `"linkStartAtOnce":true`) {
		t.Fatalf("not saved: %s", data)
	}
	var saved savedSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	b := newApp(media.Tools{}, func(fn func()) { fn() })
	b.addSheetHooks().restore(saved)
	if !b.LinkStartAtOnce() {
		t.Fatal("not taken back")
	}
}

// The playlist dialog subscribes to the one list it read as it adds its videos, when its switch is on: the videos it holds now count as seen, and the subscription lists those the dialog queued as added, without queuing them again.
func TestPlaylistDialogSubscribes(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	a.queue.source = testPlaylist
	refresh(tt, a)
	tt.Press(whole(testlocale.Messages().FilmsStart()))
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return a.playlist.open && !a.playlist.loading })
	tt.Press("id:playlist-subscribe")
	tt.Press("id:playlist-add")
	if len(a.subs.store.Subs) != 1 || a.subs.store.Subs[0].Source != testPlaylist || len(a.library.Jobs) != 3 {
		t.Fatalf("subscriptions %+v, jobs %d", a.subs.store.Subs, len(a.library.Jobs))
	}
	if sub := a.subs.store.Subs[0]; len(sub.Added) != 3 || sub.Added[0].URL != testListing.Videos[0].URL || len(sub.Pending) != 0 || len(sub.Earlier()) != 0 {
		t.Fatalf("subscription %+v", sub)
	}
}

// previewPipeline is a listingPipeline that also reads a link's video before it is added, as media.Tools does with yt-dlp -J (media.Previewer): it answers preview, or err, and reports each source on previewed; with holdPreview set it waits until its context ends, and halted receives its error.
type previewPipeline struct {
	*listingPipeline
	preview     media.Preview
	err         error
	holdPreview bool
	// gate, when set, holds the answer until it is closed.
	gate      chan struct{}
	previewed chan string
	halted    chan error
}

func newPreviewPipeline(preview media.Preview) *previewPipeline {
	return &previewPipeline{listingPipeline: newListingPipeline(map[string]media.Listing{testPlaylist: testListing}), preview: preview, previewed: make(chan string, 8), halted: make(chan error, 8)}
}

func (p *previewPipeline) Preview(ctx context.Context, source string) (media.Preview, error) {
	p.previewed <- source
	if p.holdPreview {
		<-ctx.Done()
		p.halted <- ctx.Err()
		return media.Preview{}, ctx.Err()
	}
	if p.gate != nil {
		<-p.gate
	}
	return p.preview, p.err
}

// A link pasted in the link field has its video read as the sheet opens, off the engine's goroutine: the sheet says it reads it, then names it with the page's title, says who put it on the site, how long it plays, how many videos its playlist holds and that it has no subtitles of the uploader's, so the original lines are heard on this computer and the uploader's subtitles cannot be chosen; the site's own captions can, and go onto the job.
func TestAddSheetReadsTheVideoFirst(t *testing.T) {
	p := newPreviewPipeline(media.Preview{Title: "Why Glaciers Move Faster in Summer", Uploader: "Earth Lab", Duration: 754 * time.Second, AutoCaptions: true})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8")
	tt.Press(whole(msgs.FilmsStart()))
	if got := <-p.previewed; got != "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8" {
		t.Fatalf("read %q", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.addsheet.meta.Reading })
	if got := <-p.calls; got != (listCall{testPlaylist, 0}) {
		t.Fatalf("counted %+v, want the whole playlist", got)
	}
	wantText(t, tt, "Why Glaciers Move Faster in Summer")
	wantText(t, tt, strings.Join([]string{"Earth Lab", "12:34", msgs.AddSheetEpisodesCount(3), msgs.AddSheetNoSubtitles()}, " · "))
	if a.addsheet.choice.Source != core.SubtitlesLocal {
		t.Fatalf("a video without the uploader's subtitles takes them: %+v", a.addsheet.choice)
	}
	if !tt.Find("id:addsheet-source-platform").Props.Disabled || tt.Find("id:addsheet-source-auto").Props.Disabled {
		t.Fatal("the choices do not follow the subtitles the site has")
	}
	tt.Press("id:addsheet-source-auto")
	tt.Press("id:addsheet-add")
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].SubtitleSource != core.SubtitlesAuto {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// Before the video is read the sheet says so, and closing the sheet stops the reading, which then changes nothing; a page that cannot be read leaves the sheet as it was, every choice open.
func TestAddSheetStopsReadingAsItCloses(t *testing.T) {
	p := newPreviewPipeline(media.Preview{})
	p.holdPreview = true
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://youtu.be/jNQXAC9IVRw")
	tt.Press(whole(msgs.FilmsStart()))
	<-p.previewed
	wantText(t, tt, msgs.AddSheetReading())
	tt.Key("escape")
	select {
	case err := <-p.halted:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stopped with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("closing the sheet did not stop the reading")
	}
	refresh(tt, a)
	if a.addsheet.open || a.addsheet.meta != (addsheet.Meta{}) {
		t.Fatalf("after Escape: %+v", a.addsheet)
	}

	p.holdPreview, p.err = false, errors.New("yt-dlp: exit status 1: ERROR: Video unavailable")
	tt.Type(msgs.FilmsLinkLabel(), "https://youtu.be/jNQXAC9IVRw")
	tt.Press(whole(msgs.FilmsStart()))
	<-p.previewed
	waitUntil(t, tt, a, updates, func() bool { return !a.addsheet.meta.Reading })
	if a.addsheet.meta.Known || tt.Exists("id:addsheet-meta") || tt.Find("id:addsheet-source-platform").Props.Disabled || tt.Find("id:addsheet-source-auto").Props.Disabled {
		t.Fatalf("a page that could not be read: %+v, %q", a.addsheet.meta, tt.Texts())
	}
	wantText(t, tt, msgs.AddSheetUntitled("YouTube"))
}

// Under the whole playlist, 作为剧集处理 is on; turned off, the playlist dialog starts with it off and the films it adds are no series.
func TestAddSheetLeavesThePlaylistOutOfASeries(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8")
	tt.Press(whole(msgs.FilmsStart()))
	if tt.Exists("id:addsheet-series") {
		t.Fatal("the series switch shows for one video")
	}
	tt.Press("id:addsheet-whole-list")
	wantText(t, tt, msgs.AddSheetSeries())
	if !a.addsheet.choice.Series {
		t.Fatal("作为剧集处理 starts off")
	}
	tt.Press("id:addsheet-series")
	tt.Press("id:addsheet-add")
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return a.playlist.open && !a.playlist.loading })
	if a.playlist.series || !tt.Exists("id:playlist-series") {
		t.Fatalf("the dialog's series switch: on %v, texts %q", a.playlist.series, tt.Texts())
	}
	tt.Press("id:playlist-add")
	if len(a.library.Jobs) != 3 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	for _, job := range a.library.Jobs {
		if job.Series != "" {
			t.Fatalf("%s is in series %q", job.Source, job.Series)
		}
	}
}

// A bilibili video downloaded without a sign-in is 480p at best, which the sheet's best picture says; with a browser's sign-in it is the best again.
func TestAddSheetSaysBilibiliIs480pSignedOut(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{block: true})
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://www.bilibili.com/video/BV1GJ411x7h7")
	tt.Press(whole(msgs.FilmsStart()))
	if !a.addsheet.open {
		t.Fatal("the sheet did not open")
	}
	wantText(t, tt, msgs.AddSheetQualityBest480())
	wantText(t, tt, msgs.AddSheetSourceAutoBilibili())
	apply(tt, a, func() { a.addsheet.choice.Browser = "chrome" })
	if hasText(tt, msgs.AddSheetQualityBest480()) || !hasText(tt, msgs.SettingsGeneralQualityBest()) {
		t.Fatalf("signed in: %q", tt.Texts())
	}
	tt.Key("escape")

	tt.Type(msgs.FilmsLinkLabel(), "https://youtu.be/jNQXAC9IVRw")
	tt.Press(whole(msgs.FilmsStart()))
	if hasText(tt, msgs.AddSheetQualityBest480()) || !hasText(tt, msgs.AddSheetSourceAuto()) {
		t.Fatalf("YouTube signed out: %q", tt.Texts())
	}
}

// The site's captions chosen before the video is read, on a video the site made none for, are not left chosen behind a disabled radio: the film is heard.
func TestAddSheetDropsCaptionsTheVideoLacks(t *testing.T) {
	p := newPreviewPipeline(media.Preview{Title: "No captions", Subtitles: true})
	p.gate = make(chan struct{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	tt.Type(msgs.FilmsLinkLabel(), "https://youtu.be/jNQXAC9IVRw")
	tt.Press(whole(msgs.FilmsStart()))
	<-p.previewed
	tt.Press("id:addsheet-source-auto")
	if a.addsheet.choice.Source != core.SubtitlesAuto {
		t.Fatalf("choice %+v", a.addsheet.choice)
	}
	close(p.gate)
	waitUntil(t, tt, a, updates, func() bool { return a.addsheet.meta.Known })
	if a.addsheet.choice.Source != core.SubtitlesLocal || !tt.Find("id:addsheet-source-auto").Props.Disabled {
		t.Fatalf("choice %+v", a.addsheet.choice)
	}
}
