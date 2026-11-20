package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/playlist"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/subscribe"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// pageSubscription is the page of one subscription (package subscribe), the one subscribeUI.shown names.
const pageSubscription = sidebar.PageSubscription

// subscribeTick is how often the app looks for subscriptions whose hour or day has come while it runs (subscribeArm). A check is due at most this long after its time.
const subscribeTick = 10 * time.Minute

// subscribeUI is the subscriptions' state (package subscribe). store is what subscriptions.json beside settings.json holds; storeErr says why it could not be read, which keeps it from being written over, and saveErr is the sidebar's message (app.saveErr) this area set last. shown is the subscription whose page shows while the page is pageSubscription. opened is when the window came up, which the subscriptions that check each time the app opens compare their last check with (subscribe.Sub.Due). One yt-dlp lists at a time: checking is the subscription being checked, cancel stops it and run tells its report from that of a check stopped before it; due is those waiting their turn, in order, and passed is set when one due was passed over because its library is locked, so unlocking it lines it up again (subscribeAfterChange). notes is the message each page shows about the last thing done to its subscription, which is not saved. ask is the dialog a link is pasted in, and tickStop stops the timer that looks for due checks.
type subscribeUI struct {
	store    subscribe.Store
	storeErr error
	saveErr  string
	shown    string
	opened   time.Time
	checking string
	cancel   context.CancelFunc
	run      int
	due      []string
	passed   bool
	notes    map[string]subscribeNote
	ask      subscribeAsk
	tickStop func() bool
	// offer is the subscription whose next check offers the videos its list held when it was subscribed, which it left out (subscribe.Sub.Take's offer), as its page's 列出来 asks; "" for none.
	offer string
}

// subscribeNote is the message a subscription's page shows about the last thing done to it, and, for a note that says why something did not work, why, worded only when the page reads it (SubscriptionHead), so it is said in the language the page shows then.
type subscribeNote struct {
	subscribe.Note
	why subscribeReason
}

// subscribeReason says why something did not work in the language of m. It keeps the data it needs, not words.
type subscribeReason func(m locales.Messages) string

// subscribeAsk is the dialog a link to subscribe to is pasted in: whether it is open, release, which gives back the window's modal flag it holds (holdModal), and the link typed; while the list is read, reading is set, cancel stops it and run tells its report from that of a dialog closed before it; err says why the link could not be subscribed to. Once the list was read, read is set and the dialog shows it, the list at source of the library at dir, with what the user chooses on its card (start). episode is the link of the one video of the list the link typed named, which 只要这一集 adds alone, "" when the link named the list.
type subscribeAsk struct {
	open    bool
	release func()
	link    string
	reading bool
	cancel  context.CancelFunc
	run     int
	err     string
	read    bool
	source  string
	dir     string
	listing media.Listing
	episode string
	choice  subscribe.Choice
}

// subscribeStart is what a new subscription starts with (subscribeCreate): how many of a channel's most recent videos to add now (recent), or, for a collection or a playlist, only the one video at only rather than all ("" for all); whether its videos are one series, and the picture and the source of original lines its rules name ("" for the settings'); extra gives the jobs it queues now more, as the dialog's sign-in and chapters, nil for nothing. The zero value, as the control socket subscribes with, adds a collection's or a playlist's videos with the settings' choices and none of a channel's.
type subscribeStart struct {
	recent  int
	only    string
	series  bool
	quality kcore.Quality
	source  kcore.SubtitleSource
	extra   func(job *kcore.Job)
}

func newSubscribeUI() subscribeUI { return subscribeUI{notes: map[string]subscribeNote{}} }

// subscribeHooks reads the subscriptions at startup and checks the ones due once the window is up, those that check each time the app opens among them, then looks for due ones every subscribeTick and whenever the app comes to the front, as a timer counts no time the computer sleeps, and once a library whose due ones were passed over is unlocked. The page goes back to the library once its subscription is no longer the open library's, and the subscriptions follow a library whose folder was renamed. Quitting stops a check or a reading that runs, so yt-dlp is not left running.
func (a *app) subscribeHooks() areaHooks {
	return areaHooks{
		restore: func(savedSettings) { a.subscribeLoad() },
		ready: func() {
			a.subs.opened = time.Now()
			a.subscribeDue()
			a.subscribeArm()
		},
		becameActive: a.subscribeDue,
		afterChange:  a.subscribeAfterChange,
		videosMoved:  a.subscribeLibraryMoved,
		quitting: func() {
			a.subscribeStopCheck()
			a.subscribeAskClose()
		},
	}
}

// subscribeKeys is the File menu's 添加订阅…, which has no key.
func (a *app) subscribeKeys(cx *core.Scope) []shortcut {
	return []shortcut{{Group: groupFile, Label: locales.SubscribeAddMenu(cx), Run: a.AskSubscription}}
}

// subscribeOverlay is the dialog a link to subscribe to is pasted in, over the window while it is open (state_overlay.go).
func (a *app) subscribeOverlay(*core.Scope) core.Node {
	return subscribe.Ask{
		Open:     live.Func(a, func() bool { return a.subs.ask.open }),
		Link:     live.Func(a, func() string { return a.subs.ask.link }),
		OnLink:   live.On(a, func(text string) { a.subs.ask.link, a.subs.ask.err = text, "" }),
		Reading:  live.Func(a, func() bool { return a.subs.ask.reading }),
		Err:      live.Func(a, func() string { return a.subs.ask.err }),
		Preview:  live.Func(a, a.subscribeAskPreview),
		Choice:   live.Func(a, func() subscribe.Choice { return a.subs.ask.choice }),
		OnChoice: live.On(a, func(c subscribe.Choice) { a.subs.ask.choice = c }),
		Adding:   live.Func(a, func() int { return len(a.subscribeAskAdding()) }),
		OnSubmit: live.Do(a, a.subscribeAskSubmit),
		OnCancel: live.Do(a, a.subscribeAskClose),
	}
}

// subscribeColumn is the page of the subscription shown, in the content column while the page is pageSubscription (view.go).
func (a *app) subscribeColumn(*core.Scope) core.Node { return subscribe.Page{App: a} }

// subscribeLoad reads subscriptions.json beside settings.json. Without a settings file, as in tests, the subscriptions are kept in memory alone. A file that cannot be read is left as it is: the sidebar says so, and nothing is written over it (subscribeSave).
func (a *app) subscribeLoad() {
	if a.settingsPath == "" {
		return
	}
	store, err := subscribe.Load(filepath.Join(filepath.Dir(a.settingsPath), "subscriptions.json"))
	a.subs.store, a.subs.storeErr = store, err
	if err != nil {
		a.subs.saveErr = a.messages().SubscribeLoadFailed(err.Error())
		a.saveErr = a.subs.saveErr
	}
}

// subscribeSave writes subscriptions.json, unless it could not be read at startup; the sidebar says why it could not be written, until it is.
func (a *app) subscribeSave() {
	if a.subs.storeErr != nil {
		return
	}
	if err := a.subs.store.Save(); err != nil {
		a.subs.saveErr = a.messages().SubscribeSaveFailed(err.Error())
		a.saveErr = a.subs.saveErr
		return
	}
	if a.subs.saveErr != "" && a.saveErr == a.subs.saveErr {
		a.saveErr = ""
	}
	a.subs.saveErr = ""
}

// subscribeFind is the subscription with id.
func (a *app) subscribeFind(id string) (*subscribe.Sub, bool) {
	if id == "" {
		return nil, false
	}
	return a.subs.store.Find(id)
}

// subscribeOfOpen reports whether sub adds its videos to the open library.
func (a *app) subscribeOfOpen(sub subscribe.Sub) bool {
	dir := a.snap().LibraryDir
	return dir != "" && filepath.Clean(sub.Library) == filepath.Clean(dir)
}

// subscribeShow shows the page of the subscription with id, closing the open film.
func (a *app) subscribeShow(id string) {
	a.Select(pageSubscription)
	a.subs.shown = id
}

// subscribeAfterChange keeps the page on a subscription of the open library (subscribeFollowPage) and, once a check was passed over because its library was locked and none runs, looks for due checks again, as the library may have been unlocked since. It reports whether either changed what a binding reads.
func (a *app) subscribeAfterChange() bool {
	changed := a.subscribeFollowPage()
	if a.subs.passed && a.subs.checking == "" {
		a.subs.passed = false
		a.subscribeDue()
		changed = a.subs.checking != "" || changed
	}
	return changed
}

// subscribeFollowPage goes back to the library page when the page shows a subscription that is gone or is not the open library's, as after another library opened (afterChange in subscribeHooks).
func (a *app) subscribeFollowPage() bool {
	if a.page != pageSubscription {
		return false
	}
	if sub, ok := a.subscribeFind(a.subs.shown); ok && a.subscribeOfOpen(*sub) && !a.lockShut() {
		return false
	}
	a.page, a.subs.shown = pageFilms, ""
	return true
}

// subscribeLibraryMoved points the subscriptions of a library whose folder was renamed at its new path (videosMoved in subscribeHooks). move maps the paths of videos, so a path inside each library tells where the library went.
func (a *app) subscribeLibraryMoved(move func(path string) (string, bool)) {
	changed := false
	for _, dir := range a.subscribeLibraries() {
		if next, ok := move(filepath.Join(dir, "subscribed")); ok && filepath.Dir(next) != filepath.Clean(dir) {
			changed = a.subs.store.MoveLibrary(dir, filepath.Dir(next)) || changed
		}
	}
	if changed {
		a.subscribeSave()
	}
}

// subscribeLibraries is the libraries the subscriptions add to, each once.
func (a *app) subscribeLibraries() []string {
	var dirs []string
	for _, sub := range a.subs.store.Subs {
		if dir := filepath.Clean(sub.Library); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// subscribeArm arms the timer that looks for due checks every subscribeTick while the app runs, and arms it again each time it fires.
func (a *app) subscribeArm() {
	if a.subs.tickStop != nil {
		a.subs.tickStop()
	}
	if a.cx == nil {
		return
	}
	a.subs.tickStop = a.cx.AfterFunc(subscribeTick, live.Do(a, func() {
		a.subscribeDue()
		a.subscribeArm()
	}))
}

// subscribeDue lines up the checks of the subscriptions due now (subscribe.Sub.Due), and starts the first.
func (a *app) subscribeDue() {
	now := time.Now()
	for _, sub := range a.subs.store.Subs {
		if sub.Due(now, a.subs.opened) && sub.ID != a.subs.checking && !slices.Contains(a.subs.due, sub.ID) {
			a.subs.due = append(a.subs.due, sub.ID)
		}
	}
	a.subscribeNext()
}

// subscribeLister is the pipeline's yt-dlp to list videos with, borrowing the browser's sign-in as the playlist dialog does (playlistAsk), or nil when there is none.
func (a *app) subscribeLister() media.Lister {
	lister, ok := a.pipeline.(media.Lister)
	if !ok {
		return nil
	}
	if tools, isTools := lister.(media.Tools); isTools {
		if tools.YTDLP == "" {
			return nil
		}
		tools.Cookies = a.signin.browser
		lister = tools
	}
	return lister
}

// subscribeNext starts the next check lined up when none runs. A subscription that no longer checks, or whose library was forgotten or is locked, is passed over: it is lined up again by a later look for due checks, the next one after its library is unlocked (subscribeAfterChange), or after the library was added back. Without yt-dlp none can run, and the line is emptied.
func (a *app) subscribeNext() {
	if a.subs.checking != "" {
		return
	}
	for len(a.subs.due) > 0 {
		id := a.subs.due[0]
		a.subs.due = a.subs.due[1:]
		sub, ok := a.subscribeFind(id)
		if !ok || !sub.Checks() || !a.subscribeKnown(sub.Library) {
			continue
		}
		if a.LibraryLocked(sub.Library) {
			a.subs.passed = true
			continue
		}
		lister := a.subscribeLister()
		if lister == nil {
			a.subs.due = nil
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		a.subs.run++
		run, source := a.subs.run, sub.Source
		a.subs.checking, a.subs.cancel = id, cancel
		go func() {
			list, err := playlist.ReadOne(ctx, lister, source)
			a.update(func() { a.subscribeChecked(run, id, list.Listing, err) })
		}()
		return
	}
}

// subscribeStopCheck stops the check that runs, whose report is then dropped.
func (a *app) subscribeStopCheck() {
	if a.subs.cancel != nil {
		a.subs.cancel()
	}
	a.subs.run++
	a.subs.checking, a.subs.cancel = "", nil
}

// subscribeChecked takes the report of the check run of the subscription with id (subscribeTook), and starts the next check lined up.
func (a *app) subscribeChecked(run int, id string, listing media.Listing, err error) {
	if run != a.subs.run {
		return
	}
	a.subs.checking, a.subs.cancel = "", nil
	defer a.subscribeNext()
	if errors.Is(err, context.Canceled) {
		return
	}
	a.subscribeTook(id, listing, err)
}

// subscribeTook folds what a check of the subscription with id found into it: what the list holds, or why it could not be listed. The videos the list held when it was subscribed, which an old subscription left out, are offered only by the check its page's 列出来 asked for (subscribeUI.offer). New videos that fit the rules are queued, at most subscribe.AddPerCheck of them, or held for the user when the subscription asks first, when they are past that many, or when they could not be queued; the page says what happened. It returns the subscription, false when it is gone.
func (a *app) subscribeTook(id string, listing media.Listing, err error) (*subscribe.Sub, bool) {
	sub, ok := a.subscribeFind(id)
	if !ok {
		return nil, false
	}
	now := time.Now()
	offer := a.subs.offer == id
	if offer {
		a.subs.offer = ""
	}
	if err != nil {
		sub.Fail(err, now)
		if sub.Gone {
			a.subs.notes[id] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteGone}}
		} else {
			if sub.Err != "" {
				sub.Err = jobfailure.ToolMessage(sub.Err)
			}
			a.subs.notes[id] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteFailed}, why: subscribeFailure(*sub)}
		}
		a.subscribeSave()
		return sub, true
	}
	found := sub.Take(listing, now, offer)
	note := subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteChecked, Count: len(found.Fresh), Skipped: found.Skipped}}
	// The videos a list held when it was subscribed, before Kando added them (Found.Earlier), wait for the user whatever the mode, as it did not ask for them; those the library has already are recorded as added.
	if len(found.Earlier) > 0 {
		var had, offer []subscribe.Video
		for _, v := range found.Earlier {
			if a.subscribeHas(sub.Library, v.URL) {
				had = append(had, v)
			} else {
				offer = append(offer, v)
			}
		}
		sub.Queued(had)
		sub.Hold(offer)
		note.Earlier = len(offer)
	}
	switch {
	case len(found.Fresh) == 0:
	case sub.Mode == subscribe.ModeAsk:
		sub.Hold(found.Fresh)
		note.Held = true
	default:
		queue, over := subscribe.Cap(found.Fresh)
		failed, why := a.subscribeEnqueue(sub, queue, nil)
		// Held in the list's order: those that could not be queued came before those past the cap.
		sub.Hold(failed)
		sub.Hold(over)
		note.Over = len(over)
		if len(failed) > 0 {
			// The counts stay for the control socket (remoteSubscriptionCheck); the page says only why.
			note.Kind, note.why = subscribe.NoteNotQueued, why
		}
	}
	a.subs.notes[id] = note
	a.subscribeSave()
	return sub, true
}

// subscribeFailure says why the last check of sub failed, as its page says it: yt-dlp took too long, or what it said.
func subscribeFailure(sub subscribe.Sub) subscribeReason {
	timedOut, text := sub.TimedOut, sub.Err
	return func(m locales.Messages) string {
		if timedOut {
			return m.PlaylistTimedOut()
		}
		return text
	}
}

// subscribeKnown reports whether the library at dir is one the app knows, open or listed in the library menu; one that was forgotten is not.
func (a *app) subscribeKnown(dir string) bool {
	return slices.ContainsFunc(a.knownLibraries(), func(known string) bool { return filepath.Clean(known) == filepath.Clean(dir) })
}

// subscribeEnqueue queues videos, in order, in the library sub adds to, and records those queued on sub; a video the library has a job for already counts as queued. Each new job takes the picture and the source of original lines sub's rules name (subscribeStamp), then what extra gives it, nil for nothing more. It returns the videos it could not queue and why: the library is no longer known or is locked, no language to translate into is chosen, a tool or a setting a job needs is missing (precheckTools), or the queue refused one.
func (a *app) subscribeEnqueue(sub *subscribe.Sub, videos []subscribe.Video, extra func(job *kcore.Job)) (failed []subscribe.Video, why subscribeReason) {
	name := sidebar.Title(sub.Library)
	open := a.subscribeOfOpen(*sub)
	switch {
	case !a.subscribeKnown(sub.Library):
		return videos, func(m locales.Messages) string { return m.SubscribeLibraryMissing(name) }
	case a.LibraryLocked(sub.Library):
		return videos, func(m locales.Messages) string { return m.LockLibraryIsLocked(name) }
	case strings.TrimSpace(a.prefs.language) == "":
		return videos, func(m locales.Messages) string { return m.QueueNoLanguage() }
	}
	// The library's own checks are the ones above, for the library sub adds to, which need not be the open one; what is left is the same for every library.
	if say := a.precheckTools(); say != nil {
		return videos, func(m locales.Messages) string { msg, _ := say(m); return msg }
	}
	stamp := subscribeStamp(sub.Rules, extra)
	a.queue.stamp = stamp
	defer func() { a.queue.stamp = nil }()
	var queued []subscribe.Video
	for _, v := range videos {
		var err error
		if open {
			_, err = a.queueTake(v.URL)
		} else {
			err = a.subscribeQueueIn(sub.Library, v.URL, stamp)
		}
		var dup *queueDuplicate
		if err == nil || errors.As(err, &dup) {
			queued = append(queued, v)
			continue
		}
		failed = append(failed, v)
		if why == nil {
			// What is left is core.NewJob's, about the link, which the app shows in English everywhere.
			text := err.Error()
			why = func(locales.Messages) string { return text }
		}
	}
	sub.Queued(queued)
	a.nextKeepSubscribed(sub, queued)
	return failed, why
}

// subscribeStamp gives a job a subscription queues the picture and the source of original lines its rules name, where they name one, then what extra gives it.
func subscribeStamp(r subscribe.Rules, extra func(job *kcore.Job)) func(job *kcore.Job) {
	return func(job *kcore.Job) {
		if r.Quality.Known() {
			job.Quality = r.Quality
		}
		if r.Subtitles.Known() {
			job.SubtitleSource = r.Subtitles
		}
		if extra != nil {
			extra(job)
		}
	}
}

// subscribeChoiceStamp gives the jobs the subscribe dialog queues now the sign-in and the chapters its card chose, where they differ from the settings', as the Add sheet does (addSheetStamp).
func (a *app) subscribeChoiceStamp(c subscribe.Choice) func(job *kcore.Job) {
	return func(job *kcore.Job) {
		if c.Browser != a.signin.browser {
			job.Cookies = c.Browser
			if c.Browser == "" {
				job.Cookies = kcore.CookiesNone
			}
		}
		if c.Chapters != a.ImportChapters() {
			job.ChapterUse = kcore.ChaptersEven
			if c.Chapters {
				job.ChapterUse = kcore.ChaptersPlatform
			}
		}
	}
}

// subscribeHas reports whether the library at dir has a job for the video at source that did not fail.
func (a *app) subscribeHas(dir, source string) bool {
	key := queueSourceKey(source)
	return slices.ContainsFunc(a.library.Jobs, func(job kcore.Job) bool {
		return job.Stage != kcore.StageFailed && films.InLibrary(dir, job) && queueSourceKey(job.Source) == key
	})
}

// subscribeCreate subscribes the library at dir to the list at source, which listing is what was read of it, with start's series, picture and source of original lines as its rules', and adds the videos it holds now (subscribe.Sub.Start): a collection's or a playlist's every one that fits the rules, or only start.only, a channel's start.recent most recent; those take what start.extra gives them too. Those that could not be queued, as when the library is locked or a tool is missing, wait on its page, which says why and how many were queued. It returns the subscription, whose note says what it added.
func (a *app) subscribeCreate(source, dir string, listing media.Listing, start subscribeStart) *subscribe.Sub {
	made := subscribe.New(source, dir, listing, time.Now())
	made.Series, made.Quality, made.Subtitles = start.series, start.quality, start.source
	a.subs.store.Subs = append(a.subs.store.Subs, made)
	sub, _ := a.subs.store.Find(made.ID)
	note := subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteSubscribed, Text: sub.Name}}
	videos := sub.Start(listing, start.recent)
	if start.only != "" {
		videos = slices.DeleteFunc(videos, func(v subscribe.Video) bool { return queueSourceKey(v.URL) != queueSourceKey(start.only) })
	}
	if len(videos) > 0 {
		failed, why := a.subscribeEnqueue(sub, videos, start.extra)
		sub.Hold(failed)
		note.Count = len(videos) - len(failed)
		if len(failed) > 0 {
			// How many were queued stays, as a check's counts do; the page says it, and why the rest were not.
			note.Kind, note.Queued, note.why = subscribe.NoteNotQueued, note.Count, why
		}
	}
	if start.series {
		a.glossarySubscriptionSeries(*sub)
	}
	a.subs.notes[sub.ID] = note
	a.subscribeSave()
	return sub
}

// subscribeQueueIn queues the video at source in the library at dir, which is not the open one, as newJob and queueTake do in the open library, the job given what stamp gives it: a video that library has a job for already, but for one that failed, is not queued again.
func (a *app) subscribeQueueIn(dir, source string, stamp func(job *kcore.Job)) error {
	job, err := kcore.NewJob(source)
	if err != nil {
		return err
	}
	language := strings.TrimSpace(a.prefs.language)
	if language == "" {
		return errors.New(a.messages().QueueNoLanguage())
	}
	key := queueSourceKey(job.Source)
	for _, old := range a.library.Jobs {
		if old.Stage != kcore.StageFailed && films.InLibrary(dir, old) && queueSourceKey(old.Source) == key {
			return &queueDuplicate{ID: old.ID, Stage: old.Stage, m: a.messages()}
		}
	}
	job.Language, job.LibraryDir = language, dir
	if stamp != nil {
		stamp(&job)
	}
	a.queueAdd(job)
	return nil
}

// subscribeFollow has the open library subscribe to the list at source, whose videos listing holds now count as seen, as the playlist dialog's switch asks when it adds them, and returns its ID; the dialog queues the videos itself, and then records them on it (subscribeAdopt). A list the library follows already, a library that is locked or none open, is left as it is, and the ID is "".
func (a *app) subscribeFollow(source string, listing media.Listing) string {
	dir := a.snap().LibraryDir
	if strings.TrimSpace(dir) == "" || a.lockShut() {
		return ""
	}
	if _, ok := a.subs.store.Following(source, dir); ok {
		return ""
	}
	sub := subscribe.New(source, dir, listing, time.Now())
	a.subs.store.Subs = append(a.subs.store.Subs, sub)
	a.subscribeSave()
	a.subs.notes[sub.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteSubscribed, Text: sub.Name}}
	return sub.ID
}

// subscribeAdopt records on the subscription with id the videos of listing its library has a job for, as the playlist dialog queued them, so its page lists them as added and they are not queued again. Those of meant, the videos the dialog was to queue, that have no job, as when the queue refused them for want of a language or a tool, wait on its page to be added or skipped, so a collection subscribed to this way is never left with its episodes neither added nor waiting.
func (a *app) subscribeAdopt(id string, listing media.Listing, meant []string) {
	sub, ok := a.subscribeFind(id)
	if !ok {
		return
	}
	adding := map[string]bool{}
	for _, source := range meant {
		adding[queueSourceKey(source)] = true
	}
	var have, held []subscribe.Video
	for _, lv := range listing.Videos {
		v := subscribe.Video{URL: lv.URL, Title: lv.Title, Duration: lv.Duration}
		switch {
		case a.subscribeHas(sub.Library, lv.URL):
			have = append(have, v)
		case adding[queueSourceKey(lv.URL)]:
			held = append(held, v)
		}
	}
	if len(have) == 0 && len(held) == 0 {
		return
	}
	sub.Queued(have)
	sub.Hold(held)
	a.subs.notes[id] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteSubscribed, Text: sub.Name, Count: len(have)}}
	a.subscribeSave()
}

// subscribeAskClose closes the dialog a link is pasted in, stops its reading and gives back the window's modal flag.
func (a *app) subscribeAskClose() {
	ask := a.subs.ask
	if ask.cancel != nil {
		ask.cancel()
	}
	if ask.release != nil {
		ask.release()
	}
	a.subs.ask = subscribeAsk{run: ask.run}
}

// subscribeAskPreview is what the dialog shows of the list read, not Ready before it was.
func (a *app) subscribeAskPreview() subscribe.Preview {
	ask := a.subs.ask
	if !ask.read {
		return subscribe.Preview{}
	}
	link, _ := kcore.ParseLink(ask.source)
	p := subscribe.Preview{Ready: true, Name: subscribe.NameOf(ask.source, ask.listing), Platform: link.Platform, Kind: subscribe.KindOf(link), Videos: len(ask.listing.Videos),
		Episode: ask.episode != "" && link.Kind != kcore.LinkChannel, Streams: media.StreamsHere()}
	for _, b := range a.SignIn().Browsers {
		p.Browsers = append(p.Browsers, subscribe.Browser{ID: b.ID, Name: b.Name})
	}
	return p
}

// subscribeAskAdding is the videos the dialog's button adds now (subscribe.Sub.Start), with the rules a new subscription starts with: a channel's most recent the card chose, a collection's or a playlist's every one, or only the one the link named.
func (a *app) subscribeAskAdding() []subscribe.Video {
	ask := a.subs.ask
	if !ask.read {
		return nil
	}
	videos := subscribe.Sub{Source: ask.source, Rules: subscribe.DefaultRules()}.Start(ask.listing, ask.choice.Recent)
	if only := a.subscribeAskOnly(); only != "" {
		videos = slices.DeleteFunc(videos, func(v subscribe.Video) bool { return queueSourceKey(v.URL) != queueSourceKey(only) })
	}
	return videos
}

// subscribeAskOnly is the one video the dialog adds when 只要这一集 is chosen, "" for all of them.
func (a *app) subscribeAskOnly() string {
	if ask := a.subs.ask; ask.choice.Only && ask.episode != "" {
		return ask.episode
	}
	return ""
}

// subscribeAskEpisode is the link of the video of listing that typed, the link typed in the dialog, names, "" when it names the list itself or no video it lists, as a bilibili video's page names one part of its upload.
func subscribeAskEpisode(typed string, listing media.Listing) string {
	link, ok := kcore.ParseLink(strings.TrimSpace(typed))
	if !ok || !link.OneVideo() {
		return ""
	}
	key := queueSourceKey(link.URL)
	for _, v := range listing.Videos {
		if queueSourceKey(v.URL) == key {
			return v.URL
		}
	}
	return ""
}

// subscribeAskSubmit subscribes to the link typed in the dialog: one that is not a channel, a collection or a playlist, or one with no library to add to, is refused with why; one already followed in the open library shows its page; any other is read off the engine's goroutine (subscribeAskRead). Once it was read, it subscribes as the dialog's card chose (subscribeAskConfirm).
func (a *app) subscribeAskSubmit() {
	ask := &a.subs.ask
	if !ask.open || ask.reading {
		return
	}
	if ask.read {
		a.subscribeAskConfirm()
		return
	}
	m := a.messages()
	source, ok := subscribe.Source(ask.link)
	if !ok {
		ask.err = m.SubscribeAddNotList()
		return
	}
	dir := a.snap().LibraryDir
	switch {
	case strings.TrimSpace(dir) == "":
		ask.err = m.PrecheckNoLibrary()
		return
	case a.lockShut():
		ask.err = m.LockLibraryIsLocked(sidebar.Title(dir))
		return
	}
	if sub, ok := a.subs.store.Following(source, dir); ok {
		a.subscribeAskClose()
		a.subscribeShow(sub.ID)
		a.subs.notes[sub.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteAlready}}
		return
	}
	lister := a.subscribeLister()
	if lister == nil {
		ask.err = m.SubscribeAddCannotList()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ask.run++
	run := ask.run
	ask.reading, ask.cancel, ask.err = true, cancel, ""
	go func() {
		list, err := playlist.ReadOne(ctx, lister, source)
		a.update(func() { a.subscribeAskRead(run, source, dir, list.Listing, err) })
	}()
}

// subscribeAskRead takes what the dialog's reading found: why the list could not be read, which the dialog says, or the list, which the dialog then shows to ask what to add now (subscribeAskConfirm); if the library follows it already by then, that subscription's page shows instead.
func (a *app) subscribeAskRead(run int, source, dir string, listing media.Listing, err error) {
	ask := &a.subs.ask
	if ask.run != run || !ask.open {
		return
	}
	if err != nil {
		ask.reading, ask.cancel, ask.err = false, nil, subscribeReadError(a.messages(), err)
		return
	}
	// The control socket may have subscribed to it while yt-dlp read it (remoteSubscribe).
	if have, ok := a.subs.store.Following(source, dir); ok {
		a.subscribeAskClose()
		a.subscribeShow(have.ID)
		a.subs.notes[have.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteAlready}}
		return
	}
	ask.reading, ask.cancel = false, nil
	ask.read, ask.source, ask.dir, ask.listing = true, source, dir, listing
	ask.episode = subscribeAskEpisode(ask.link, listing)
	// The card starts subscribed, with a channel's latest few, a collection's every episode as one series, and the settings' picture, lines, sign-in and chapters.
	ask.choice = subscribe.Choice{Subscribe: true, Recent: subscribe.RecentDefault, Series: true,
		Quality: a.onlineQuality(), Source: a.onlineSource(), Browser: a.signin.browser, Chapters: a.ImportChapters()}
}

// subscribeAskConfirm subscribes the library the list was read for to it, adding what the dialog's card chose (subscribeCreate), closes the dialog and shows the subscription's page; with the card's switch off it only adds the videos (subscribeAskJustAdd). The library may have changed while the card showed: one no longer open, or locked, is refused with why; one that follows the list already by then shows that subscription's page.
func (a *app) subscribeAskConfirm() {
	ask := &a.subs.ask
	m := a.messages()
	switch {
	case filepath.Clean(a.snap().LibraryDir) != filepath.Clean(ask.dir):
		ask.err = m.PrecheckNoLibrary()
		return
	case a.lockShut():
		ask.err = m.LockLibraryIsLocked(sidebar.Title(ask.dir))
		return
	}
	if !ask.choice.Subscribe {
		a.subscribeAskJustAdd()
		return
	}
	if have, ok := a.subs.store.Following(ask.source, ask.dir); ok {
		a.subscribeAskClose()
		a.subscribeShow(have.ID)
		a.subs.notes[have.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteAlready}}
		return
	}
	link, _ := kcore.ParseLink(ask.source)
	c := ask.choice
	start := subscribeStart{recent: c.Recent, only: a.subscribeAskOnly(), series: c.Series && link.Kind != kcore.LinkChannel, quality: c.Quality, extra: a.subscribeChoiceStamp(c)}
	// Only YouTube's and bilibili's subtitles can be taken; elsewhere the films are heard, whatever the card said.
	if link.Platform == kcore.PlatformYouTube || link.Platform == kcore.PlatformBilibili {
		start.source = c.Source
	}
	sub := a.subscribeCreate(ask.source, ask.dir, ask.listing, start)
	id := sub.ID
	a.subscribeAskClose()
	a.subscribeShow(id)
}

// subscribeAskJustAdd adds the videos the dialog's card chose to the open library as the link field does (libraryEnqueue), with the card's picture, lines, sign-in and chapters, without subscribing, and closes the dialog; the library's message bar says what became of them. They play one after another in the list's order, and a collection's or a playlist's are one series unless the card said otherwise, as the playlist dialog adds them.
func (a *app) subscribeAskJustAdd() {
	ask := a.subs.ask
	videos := a.subscribeAskAdding()
	if len(videos) == 0 {
		return
	}
	link, _ := kcore.ParseLink(ask.source)
	c := ask.choice
	rules := subscribe.Rules{Quality: c.Quality}
	if link.Platform == kcore.PlatformYouTube || link.Platform == kcore.PlatformBilibili {
		rules.Subtitles = c.Source
	}
	sources := make([]string, len(videos))
	for i, v := range videos {
		sources[i] = v.URL
	}
	lists := []playlist.List{{Source: ask.source, Channel: link.Kind == kcore.LinkChannel, Listing: ask.listing}}
	a.subscribeAskClose()
	a.Select(pageFilms)
	a.queue.stamp = subscribeStamp(rules, a.subscribeChoiceStamp(c))
	a.libraryEnqueue(sources...)
	a.queue.stamp = nil
	a.nextKeepLists(lists)
	if c.Series && link.Kind != kcore.LinkChannel {
		a.glossaryPlaylistSeries(lists)
	}
}

// subscribeReadError says in the language of m why a link to subscribe to could not be read: a bilibili video's page whose upload is one part is no list (subscribe.Source), and anything else as a list's reading says it (playlistError).
func subscribeReadError(m locales.Messages, err error) string {
	if errors.Is(err, media.ErrOneVideo) {
		return m.SubscribeAddNotList()
	}
	return playlistError(m, err)
}
