package main

import (
	"context"
	"errors"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/playlist"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// playlistUI is the dialog that adds the videos of playlist and channel links (package playlist): whether it is open, and release, which gives back the window's modal flag it holds (holdModal); from, the page that opened it, whose message bar says what became of the videos; while loading, cancel stops the reading, and run tells the reading of this dialog from that of one closed before it; then lists, what was read, or err, why it could not be.
type playlistUI struct {
	open    bool
	release func()
	from    string
	loading bool
	cancel  context.CancelFunc
	run     int
	lists   []playlist.List
	err     error
}

// The pages that open the dialog, whose message bar says what became of the videos it adds.
const (
	playlistFromLibrary = "library"
	playlistFromBrowse  = "browse"
)

func newPlaylistUI() playlistUI { return playlistUI{} }

// playlistHooks stops a reading still going as the app quits, so yt-dlp is not left running.
func (a *app) playlistHooks() areaHooks {
	return areaHooks{quitting: a.playlistClose}
}

// playlistCan reports whether the pipeline can list a playlist's or a channel's videos (media.Lister), as Tools does with yt-dlp; without it such a link is refused as one that is not one video (precheckWhole).
func (a *app) playlistCan() bool {
	_, ok := a.pipeline.(media.Lister)
	return ok
}

// playlistSplit sorts sources into the videos the queue takes as they are and the playlist and channel links whose videos the dialog lists (playlist.Split); all are videos when the pipeline cannot list (playlistCan).
func (a *app) playlistSplit(sources []string) (videos, lists []string) {
	if !a.playlistCan() {
		return sources, nil
	}
	return playlist.Split(sources)
}

// playlistWhole is the whole playlist the video at source is a video of (core.Link.WholeList), which a message bar offers to add once the video is added, or "" when it is in none or the pipeline cannot list one (playlistCan).
func (a *app) playlistWhole(source string) string {
	if !a.playlistCan() {
		return ""
	}
	link, _ := kcore.ParseLink(source)
	return link.WholeList()
}

// playlistAsk opens the dialog for sources, playlist and channel links, from the page from, and reads their videos off the engine's goroutine; the dialog says how many there are once they are read, and adds them only when asked. A dialog already open is closed first, its reading stopped.
func (a *app) playlistAsk(sources []string, from string) {
	lister, ok := a.pipeline.(media.Lister)
	if !ok || len(sources) == 0 {
		return
	}
	// A list only a signed-in account sees, as Watch Later, is read with the sign-in downloads borrow (Settings › 通用 › 浏览器登录).
	if tools, isTools := lister.(media.Tools); isTools {
		tools.Cookies = a.signin.browser
		lister = tools
	}
	a.playlistClose()
	ctx, cancel := context.WithCancel(context.Background())
	run := a.playlist.run + 1
	a.playlist = playlistUI{open: true, release: a.holdModal(), from: from, loading: true, cancel: cancel, run: run}
	go func() {
		lists, err := playlist.Read(ctx, lister, sources)
		a.update(func() {
			if a.playlist.run != run || !a.playlist.open {
				return
			}
			a.playlist.loading, a.playlist.lists, a.playlist.err = false, lists, err
		})
	}()
}

// playlistClose closes the dialog, stops its reading and gives back the window's modal flag. Cancel, Escape, Add and the app quitting close it.
func (a *app) playlistClose() {
	if a.playlist.cancel != nil {
		a.playlist.cancel()
	}
	if a.playlist.release != nil {
		a.playlist.release()
	}
	a.playlist = playlistUI{run: a.playlist.run}
}

// playlistNew is the links of the videos read, each once and in order, and of them those the open library has no job for yet that is waiting, running or done (queueFindSource): a failed or cancelled one is queued again on its card, as the link field does. The dialog reads it on every change while it is open, as jobs come and go, so the library's jobs are looked through once rather than once a video.
func (a *app) playlistNew() (all, fresh []string) {
	if len(a.playlist.lists) == 0 {
		return nil, nil
	}
	root := a.snap().LibraryDir
	have := map[string]bool{}
	for _, job := range a.library.Jobs {
		if job.Stage != kcore.StageFailed && films.InLibrary(root, job) {
			have[queueSourceKey(job.Source)] = true
		}
	}
	seen := map[string]bool{}
	for _, list := range a.playlist.lists {
		for _, v := range list.Videos {
			key := queueSourceKey(v.URL)
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, v.URL)
			if !have[key] {
				fresh = append(fresh, v.URL)
			}
		}
	}
	return all, fresh
}

// playlistAdd queues the videos read that the library does not have yet, in their order, and closes the dialog; the message bar of the page that opened it says what became of them.
func (a *app) playlistAdd() {
	_, fresh := a.playlistNew()
	from := a.playlist.from
	a.playlistClose()
	if len(fresh) == 0 {
		return
	}
	if from == playlistFromBrowse {
		a.browseEnqueueAll(fresh)
		return
	}
	a.libraryEnqueue(fresh...)
}

// playlistLook is what the dialog shows of what was read, in the interface's language.
func (a *app) playlistLook() playlist.Look {
	p := a.playlist
	l := playlist.Look{Loading: p.loading, Lists: len(p.lists)}
	if p.err != nil {
		l.Err = playlistError(a.messages(), p.err)
		return l
	}
	for _, list := range p.lists {
		if list.Channel {
			l.Channels++
		}
	}
	if len(p.lists) == 1 {
		l.Title = p.lists[0].Title
	}
	all, fresh := a.playlistNew()
	l.Videos, l.Have = len(all), len(all)-len(fresh)
	return l
}

// playlistError says in the language of m why the videos could not be read: yt-dlp took too long, or what it said went wrong.
func playlistError(m locales.Messages, err error) string {
	if errors.Is(err, media.ErrListTimeout) {
		return m.PlaylistTimedOut()
	}
	return m.PlaylistFailed(toolMessage(err.Error()))
}

// playlistOverlay is the dialog over the window while it is open (state_overlay.go).
func (a *app) playlistOverlay(*core.Scope) core.Node {
	return playlist.Confirm{
		Open:     live.Func(a, func() bool { return a.playlist.open }),
		Look:     live.Func(a, a.playlistLook),
		OnAdd:    live.Do(a, a.playlistAdd),
		OnCancel: live.Do(a, a.playlistClose),
	}
}
