package main

import (
	"context"
	"net/url"
	"strings"
	"time"

	"kando/apps/kando/addsheet"
	"kando/apps/kando/films/sitename"
	"kando/apps/kando/live"
	"kando/apps/kando/sites/catalog"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// The Add sheet (package addsheet): a link to one online video, pasted in the library's link field or sent by the browser extension or the bookmarklet, opens it before its job is made, so the user chooses for this link where its original lines come from, the picture, the sign-in and the chapters, or to add the whole playlist instead, as a series or not. As it opens, the video's page is read off the engine's goroutine (addSheetRead), so the sheet names it, says how long it plays, how many videos its playlist holds and which subtitles its site has before it is added. What it chose goes onto the job as it is queued (jobQueue.stamp), where the queue keeps it rather than give the job the settings' (queueRun). A link that names where the video was, as the browser's playback time or a link's t=, has the done film open there (core.Job.ResumeAt, addSheetHooks).

// addSheetUI is the sheet's state: whether it is open, and release, which gives back the window's modal flag it holds (holdModal); the link it is open for, what it shows of it and what the user chose. startAtOnce is Settings › 通用 › 浏览器扩展's 链接到达后立即开始: a link from the browser extension is then added at once, without the sheet.
type addSheetUI struct {
	open    bool
	release func()
	source  string
	look    addsheet.Look
	choice  addsheet.Choice
	// meta is what was read of the video since the sheet opened (addSheetRead); stopRead stops the reading, nil while none goes on, and run tells the reading of this sheet from that of one closed before it.
	meta     addsheet.Meta
	stopRead context.CancelFunc
	run      int
	// startAtOnce is off until the user turns it on, as the prototype has it.
	startAtOnce bool
}

// savedAddSheet is the sheet's part of settings.json: whether a link from the browser extension starts at once.
type savedAddSheet struct {
	LinkStartAtOnce bool `json:"linkStartAtOnce,omitempty"`
}

func (s *savedAddSheet) clean() {}

// addSheetHooks: whether a link from the browser starts at once is saved and taken back at startup. As a job completes, where its link said to open the film (core.Job.ResumeAt) becomes the film's resume position, unless the film has one already, so it opens there; the job then forgets it.
func (a *app) addSheetHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedAddSheet = savedAddSheet{LinkStartAtOnce: a.addsheet.startAtOnce}
		},
		restore: func(saved savedSettings) { a.addsheet.startAtOnce = saved.LinkStartAtOnce },
		// A reading still going as the app quits is stopped, so yt-dlp is not left running.
		quitting: a.addSheetClose,
		shelfChanged: func() {
			if a.addSheetResume() {
				a.playerPositionsChanged()
			}
		},
	}
}

// addSheetResume keeps where each job that has just completed was asked to open as its film's resume position, unless the film has one already, and clears it from the job. It reports whether a position was kept.
func (a *app) addSheetResume() bool {
	kept, changed := false, false
	for _, job := range a.library.Jobs {
		if job.ResumeAt <= 0 || job.Stage != kcore.StageComplete || job.VideoPath == "" {
			continue
		}
		if a.player.positions[job.VideoPath] <= 0 {
			if a.player.positions == nil {
				a.player.positions = map[string]time.Duration{}
			}
			a.player.positions[job.VideoPath] = job.ResumeAt
			kept = true
		}
		job.ResumeAt = 0
		a.library.Upsert(job)
		changed = true
	}
	if changed {
		if err := a.library.Save(); err != nil {
			a.saveErr = a.shelfSaveErr(err, a.messages().QueueSaveFailed)
		}
	}
	return kept
}

// addSheetAsk opens the sheet for source, a link to one online video, from where it came, with the title the page that sent it gave it and where the browser was in it (at, 0 for nowhere), and reports whether it did. It does not for anything else, a local video, a playlist or a channel, or several links, nor for a link precheck refuses or a video the library has, which the caller adds as it would without the sheet, so the message bar says why. A sheet already open is closed first.
func (a *app) addSheetAsk(source string, from addsheet.From, title string, at time.Duration) bool {
	source = strings.TrimSpace(source)
	link, ok := kcore.ParseLink(source)
	if !ok || (link.Platform == kcore.PlatformLocal) || !link.OneVideo() {
		return false
	}
	if u, err := url.Parse(link.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if !link.DRM() {
		if msg, _ := a.precheck([]string{source}); msg != "" {
			return false
		}
	}
	// A video the library has waiting, running or done is not asked about: the message bar says so, and offers to open it.
	if job, ok := a.queueFindSource(source); ok && job.Stage != kcore.StageFailed {
		return false
	}
	if at <= 0 {
		at = kcore.StartAt(link.URL)
	}
	known := link.Platform == kcore.PlatformYouTube || link.Platform == kcore.PlatformBilibili
	look := addsheet.Look{
		Site:          sitename.Of(a.prefs.interfaceLanguage, link.Site),
		Host:          link.Host,
		Title:         strings.TrimSpace(title),
		Link:          link.URL,
		From:          from,
		ResumeAt:      at,
		Unknown:       link.Site == "" && !catalog.Load().On(link.URL),
		DRM:           link.DRM(),
		InList:        a.playlistWhole(source) != "",
		SiteSubtitles: known,
		Chapters:      known,
		Bilibili:      link.Platform == kcore.PlatformBilibili,
		Streams:       media.StreamsHere(),
	}
	for _, b := range a.SignIn().Browsers {
		look.Browsers = append(look.Browsers, addsheet.Browser{ID: b.ID, Name: b.Name})
	}
	a.addSheetClose()
	a.addsheet.open, a.addsheet.release = true, a.holdModal()
	a.addsheet.source, a.addsheet.look = link.URL, look
	a.addsheet.choice = addsheet.Choice{Quality: a.onlineQuality(), Source: a.onlineSource(), Browser: a.signin.browser, Chapters: a.ImportChapters(), Series: true}
	if !look.DRM {
		a.addSheetRead(link.URL, look.InList)
	}
	return true
}

// addSheetRead reads what the video at source is off the engine's goroutine, as the sheet shows it (addsheet.Meta): its title, who put it on the site, its length and which subtitles its site has (media.Previewer), then, for a video of a playlist (inList), how many videos the playlist holds (media.Lister). It borrows the sign-in downloads borrow, as a video only a signed-in account sees is read so. A video whose page could not be read shows what the sheet knew; one without the uploader's subtitles has its original lines heard, unless the user chose otherwise meanwhile. Closing the sheet stops the reading.
func (a *app) addSheetRead(source string, inList bool) {
	previewer, ok := a.pipeline.(media.Previewer)
	if !ok {
		return
	}
	lister, _ := a.pipeline.(media.Lister)
	if tools, isTools := a.pipeline.(media.Tools); isTools {
		tools.Cookies = a.signin.browser
		previewer, lister = tools, tools
	}
	whole := ""
	if inList && lister != nil {
		whole = a.playlistWhole(source)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := a.addsheet.run + 1
	a.addsheet.run, a.addsheet.stopRead, a.addsheet.meta = run, cancel, addsheet.Meta{Reading: true}
	update := a.update
	go func() {
		defer cancel()
		preview, err := previewer.Preview(ctx, source)
		// The playlist is counted next, unless the sheet closed meanwhile; this is decided here, as the update may run once the reading is over.
		counting := whole != "" && ctx.Err() == nil
		update(func() {
			if a.addsheet.run != run || !a.addsheet.open {
				return
			}
			a.addsheet.meta.Reading = counting
			if err != nil {
				return
			}
			a.addsheet.meta = addsheet.Meta{Reading: a.addsheet.meta.Reading, Known: true, Title: preview.Title, Uploader: preview.Uploader, Length: preview.Duration, Subtitles: preview.Subtitles, AutoCaptions: preview.AutoCaptions}
			if !preview.Subtitles && a.addsheet.choice.Source == kcore.SubtitlesPlatform {
				a.addsheet.choice.Source = kcore.SubtitlesLocal
			}
		})
		if !counting {
			return
		}
		listing, err := lister.ListVideos(ctx, whole, 0)
		update(func() {
			if a.addsheet.run != run || !a.addsheet.open {
				return
			}
			a.addsheet.meta.Reading = false
			if err == nil {
				a.addsheet.meta.Episodes = len(listing.Videos)
			}
		})
	}()
}

// addSheetClose closes the sheet and gives back the window's modal flag. Cancel, Escape, the DRM note's OK and Add close it.
func (a *app) addSheetClose() {
	if a.addsheet.stopRead != nil {
		a.addsheet.stopRead()
	}
	if a.addsheet.release != nil {
		a.addsheet.release()
	}
	a.addsheet = addSheetUI{startAtOnce: a.addsheet.startAtOnce, run: a.addsheet.run}
}

// addSheetAdd adds the link with what the user chose, and closes the sheet: the whole playlist, which the playlist dialog then lists before adding its videos, or the video, as the link field adds it (libraryEnqueue), whose message bar says what became of it.
func (a *app) addSheetAdd() {
	sheet := a.addsheet
	if !sheet.open {
		return
	}
	a.addSheetClose()
	if sheet.look.DRM {
		return
	}
	if sheet.choice.Whole {
		if whole := a.playlistWhole(sheet.source); whole != "" {
			a.libraryEnqueue(whole)
			// The playlist dialog that lists its videos starts from the sheet's 作为剧集处理.
			if a.playlist.open {
				a.playlist.series = sheet.choice.Series
			}
			return
		}
	}
	a.addSheetEnqueue(sheet.source, func(job *kcore.Job) { a.addSheetStamp(job, sheet) })
}

// addSheetEnqueue adds source as the link field does (libraryEnqueue), stamping the job it queues with stamp (jobQueue.stamp).
func (a *app) addSheetEnqueue(source string, stamp func(job *kcore.Job)) bool {
	a.queue.stamp = stamp
	defer func() { a.queue.stamp = nil }()
	return a.libraryEnqueue(source)
}

// addSheetStamp gives job what the sheet chose for its link: the picture, the original lines where the site has subtitles to take, a sign-in other than the settings', none included (kcore.CookiesNone), the chapters where the user chose otherwise than the setting, and where the film opens once done.
func (a *app) addSheetStamp(job *kcore.Job, sheet addSheetUI) {
	c, l := sheet.choice, sheet.look
	if c.Quality.Known() {
		job.Quality = c.Quality
	}
	if l.SiteSubtitles && c.Source.Known() {
		job.SubtitleSource = c.Source
	}
	// The browser Settings › 通用 › 浏览器登录 names is left to the queue, which gives it to the job as it first runs.
	if c.Browser != a.signin.browser {
		job.Cookies = c.Browser
		if c.Browser == "" {
			job.Cookies = kcore.CookiesNone
		}
	}
	if l.Chapters && c.Chapters != a.ImportChapters() {
		job.ChapterUse = kcore.ChaptersEven
		if c.Chapters {
			job.ChapterUse = kcore.ChaptersPlatform
		}
	}
	if l.ResumeAt > 0 {
		job.ResumeAt = l.ResumeAt
	}
}

// addSheetResumeStamp gives a job only where its film opens once done, at, for a link added without the sheet, as one the browser extension sends while 链接到达后立即开始 is on.
func addSheetResumeStamp(at time.Duration) func(job *kcore.Job) {
	return func(job *kcore.Job) {
		if at > 0 {
			job.ResumeAt = at
		}
	}
}

// addSheetOverlay is the sheet over the window while it is open (state_overlay.go).
func (a *app) addSheetOverlay(*core.Scope) core.Node {
	return addsheet.Sheet{
		Open:     live.Func(a, func() bool { return a.addsheet.open }),
		Look:     live.Func(a, func() addsheet.Look { return a.addsheet.look }),
		Meta:     live.Func(a, func() addsheet.Meta { return a.addsheet.meta }),
		Choice:   live.Func(a, func() addsheet.Choice { return a.addsheet.choice }),
		OnChoice: live.On(a, func(c addsheet.Choice) { a.addsheet.choice = c }),
		OnAdd:    live.Do(a, a.addSheetAdd),
		OnCancel: live.Do(a, a.addSheetClose),
	}
}
