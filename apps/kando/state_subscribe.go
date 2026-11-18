package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films"
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

// subscribeUI is the subscriptions' state (package subscribe). store is what subscriptions.json beside settings.json holds; storeErr says why it could not be read, which keeps it from being written over, and saveErr is the sidebar's message (app.saveErr) this area set last. shown is the subscription whose page shows while the page is pageSubscription. One yt-dlp lists at a time: checking is the subscription being checked, cancel stops it and run tells its report from that of a check stopped before it; due is those waiting their turn, in order. notes is the message each page shows about the last thing done to its subscription, which is not saved. ask is the dialog a link is pasted in, and tickStop stops the timer that looks for due checks.
type subscribeUI struct {
	store    subscribe.Store
	storeErr error
	saveErr  string
	shown    string
	checking string
	cancel   context.CancelFunc
	run      int
	due      []string
	notes    map[string]subscribe.Note
	ask      subscribeAsk
	tickStop func() bool
}

// subscribeAsk is the dialog a link to subscribe to is pasted in: whether it is open, release, which gives back the window's modal flag it holds (holdModal), and the link typed; while the list is read, reading is set, cancel stops it and run tells its report from that of a dialog closed before it; err says why the link could not be subscribed to.
type subscribeAsk struct {
	open    bool
	release func()
	link    string
	reading bool
	cancel  context.CancelFunc
	run     int
	err     string
}

func newSubscribeUI() subscribeUI { return subscribeUI{notes: map[string]subscribe.Note{}} }

// subscribeHooks reads the subscriptions at startup and checks the ones due once the window is up, those that check each time the app opens among them, then looks for due ones every subscribeTick and whenever the app comes to the front, as a timer counts no time the computer sleeps. The page goes back to the library once its subscription is no longer the open library's, and the subscriptions follow a library whose folder was renamed. Quitting stops a check or a reading that runs, so yt-dlp is not left running.
func (a *app) subscribeHooks() areaHooks {
	return areaHooks{
		restore: func(savedSettings) { a.subscribeLoad() },
		ready: func() {
			a.subscribeDue(true)
			a.subscribeArm()
		},
		becameActive: func() { a.subscribeDue(false) },
		afterChange:  a.subscribeFollowPage,
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
		a.subscribeDue(false)
		a.subscribeArm()
	}))
}

// subscribeDue lines up the checks of the subscriptions due now (subscribe.Sub.Due), launch being the app's start, and starts the first.
func (a *app) subscribeDue(launch bool) {
	now := time.Now()
	for _, sub := range a.subs.store.Subs {
		if sub.Due(now, launch) && sub.ID != a.subs.checking && !slices.Contains(a.subs.due, sub.ID) {
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

// subscribeNext starts the next check lined up when none runs. A subscription that no longer checks, or whose library is locked, is passed over; without yt-dlp none can run, and the line is emptied.
func (a *app) subscribeNext() {
	if a.subs.checking != "" {
		return
	}
	for len(a.subs.due) > 0 {
		id := a.subs.due[0]
		a.subs.due = a.subs.due[1:]
		sub, ok := a.subscribeFind(id)
		if !ok || !sub.Checks() || a.LibraryLocked(sub.Library) {
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

// subscribeChecked takes the report of the check run of the subscription with id: what the list holds, or why it could not be listed. New videos that fit the rules are queued, or held for the user when the subscription asks first or they could not be queued; the page says what happened, and the next check lined up starts.
func (a *app) subscribeChecked(run int, id string, listing media.Listing, err error) {
	if run != a.subs.run {
		return
	}
	a.subs.checking, a.subs.cancel = "", nil
	defer a.subscribeNext()
	sub, ok := a.subscribeFind(id)
	if !ok || errors.Is(err, context.Canceled) {
		return
	}
	now := time.Now()
	if err != nil {
		sub.Fail(err, now)
		if sub.Gone {
			a.subs.notes[id] = subscribe.Note{Kind: subscribe.NoteGone}
		} else {
			sub.Err = a.subscribeError(err)
			a.subs.notes[id] = subscribe.Note{Kind: subscribe.NoteFailed, Text: sub.Err}
		}
		a.subscribeSave()
		return
	}
	found := sub.Take(listing, now)
	note := subscribe.Note{Kind: subscribe.NoteChecked, Count: len(found.Fresh), Skipped: found.Skipped}
	switch {
	case len(found.Fresh) == 0:
	case sub.Mode == subscribe.ModeAsk:
		sub.Hold(found.Fresh)
		note.Held = true
	default:
		if failed, reason := a.subscribeEnqueue(sub, found.Fresh); len(failed) > 0 {
			sub.Hold(failed)
			note = subscribe.Note{Kind: subscribe.NoteNotQueued, Text: reason}
		}
	}
	a.subs.notes[id] = note
	a.subscribeSave()
}

// subscribeError says why a list could not be read: yt-dlp took too long, or the first line of what it said.
func (a *app) subscribeError(err error) string {
	if errors.Is(err, media.ErrListTimeout) {
		return a.messages().PlaylistTimedOut()
	}
	return toolMessage(err.Error())
}

// subscribeEnqueue queues videos, in order, in the library sub adds to, and records those queued on sub; a video the library has a job for already counts as queued. It returns the videos it could not queue and why, in the interface's language: the library is no longer known or is locked, a tool or a setting a job needs is missing (precheckReady), or the queue refused one.
func (a *app) subscribeEnqueue(sub *subscribe.Sub, videos []subscribe.Video) (failed []subscribe.Video, reason string) {
	m := a.messages()
	name := sidebar.Title(sub.Library)
	known := slices.ContainsFunc(a.knownLibraries(), func(dir string) bool { return filepath.Clean(dir) == filepath.Clean(sub.Library) })
	open := a.subscribeOfOpen(*sub)
	switch {
	case !known:
		return videos, m.SubscribeLibraryMissing(name)
	case a.LibraryLocked(sub.Library):
		return videos, m.LockLibraryIsLocked(name)
	}
	// What precheckReady says of a locked open library is not about another library this one adds to.
	if msg, _ := a.precheckReady(); msg != "" && (open || !a.lockShut()) {
		return videos, msg
	}
	var queued []subscribe.Video
	for _, v := range videos {
		var err error
		if open {
			_, err = a.queueTake(v.URL)
		} else {
			err = a.subscribeQueueIn(sub.Library, v.URL)
		}
		var dup *queueDuplicate
		if err == nil || errors.As(err, &dup) {
			queued = append(queued, v)
			continue
		}
		failed = append(failed, v)
		if reason == "" {
			reason = err.Error()
		}
	}
	sub.Queued(queued)
	return failed, reason
}

// subscribeQueueIn queues the video at source in the library at dir, which is not the open one, as newJob and queueTake do in the open library: a video that library has a job for already, but for one that failed, is not queued again.
func (a *app) subscribeQueueIn(dir, source string) error {
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
	a.queueAdd(job)
	return nil
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

// subscribeAskSubmit subscribes to the link typed in the dialog: one that is not a channel, a collection or a playlist, or one with no library to add to, is refused with why; one already followed in the open library shows its page; any other is read off the engine's goroutine (subscribeAskRead).
func (a *app) subscribeAskSubmit() {
	ask := &a.subs.ask
	if !ask.open || ask.reading {
		return
	}
	m := a.messages()
	source := strings.TrimSpace(ask.link)
	if link, ok := kcore.ParseLink(source); !ok || !link.IsList() {
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
		a.subs.notes[sub.ID] = subscribe.Note{Kind: subscribe.NoteAlready}
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

// subscribeAskRead takes what the dialog's reading found: why the list could not be read, which the dialog says, or the list, which becomes a subscription of the library at dir whose page then shows, the videos the list holds now counting as seen.
func (a *app) subscribeAskRead(run int, source, dir string, listing media.Listing, err error) {
	ask := &a.subs.ask
	if ask.run != run || !ask.open {
		return
	}
	if err != nil {
		ask.reading, ask.cancel, ask.err = false, nil, playlistError(a.messages(), err)
		return
	}
	sub := subscribe.New(source, dir, listing, time.Now())
	a.subs.store.Subs = append(a.subs.store.Subs, sub)
	a.subscribeSave()
	a.subscribeAskClose()
	a.subscribeShow(sub.ID)
	a.subs.notes[sub.ID] = subscribe.Note{Kind: subscribe.NoteSubscribed, Text: sub.Name}
}
