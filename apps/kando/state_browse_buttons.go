package main

import (
	"kando/apps/kando/browse"
	"kando/apps/kando/films"
	"kando/apps/kando/sites/catalog"
	kcore "kando/packages/core"
)

// The browser's own buttons in the pages (browse/buttons.go): the page asks what the library has of the videos it shows, the app answers and tells it again as that changes, and a click on one adds its video as 下载视频 does.

// browseButtons is what the app told the page's buttons: their words, and what the library has of each video the page asked about, by its link, with the link's key (queueSourceKey), in the order asked, at most browse.PageVideos of them, the oldest dropped first. The web view's page may have moved on to another document, whose buttons ask again; what the app tells of a video the page no longer shows, the page leaves.
type browseButtons struct {
	text   browse.PageText
	videos map[string]browseButton
	order  []string
}

type browseButton struct {
	key, have string
}

// browsePageText is the words of the page's buttons in the interface's language, as the toolbar's 下载视频 says them.
func (a *app) browsePageText() browse.PageText {
	m := a.messages()
	return browse.PageText{Add: m.BrowseDownload(), Queued: m.BrowseQueued(), Done: m.BrowseInLibrary()}
}

// browseHaves is what the open library has of each video, by the key of its source (queueSourceKey), as browseJob finds it: browse.HaveDone for a film, browse.HaveQueued for a job waiting or being processed, the first job of a video where it has several, and nothing for a failed or cancelled one.
func (a *app) browseHaves() map[string]string {
	root := a.snap().LibraryDir
	out := map[string]string{}
	for _, job := range a.library.Jobs {
		if job.Stage == kcore.StageFailed || !films.InLibrary(root, job) {
			continue
		}
		key := queueSourceKey(job.Source)
		if _, ok := out[key]; ok {
			continue
		}
		out[key] = browse.HaveQueued
		if job.Stage == kcore.StageComplete {
			out[key] = browse.HaveDone
		}
	}
	return out
}

// browsePageAsk answers a page of a site the catalog lists that asks what the library has of links, the videos it shows (browse.ParseAsk), with the buttons' words, and follows those videos, so the page hears when that changes (browsePagePush). A page of another site, which gets no buttons, is not answered.
func (a *app) browsePageAsk(origin string, links []string) {
	if !catalog.Load().On(origin) || len(links) == 0 {
		return
	}
	b := &a.browse.buttons
	if b.videos == nil {
		b.videos = map[string]browseButton{}
	}
	haves := a.browseHaves()
	have := make(map[string]string, len(links))
	for _, u := range links {
		if _, ok := b.videos[u]; !ok {
			b.order = append(b.order, u)
		}
		key := queueSourceKey(u)
		b.videos[u] = browseButton{key: key, have: haves[key]}
		have[u] = haves[key]
	}
	for len(b.order) > browse.PageVideos {
		delete(b.videos, b.order[0])
		b.order = b.order[1:]
	}
	b.text = a.browsePageText()
	a.browse.view.PostMessage(browse.StateData(browse.PageState{Text: b.text, Have: have}))
}

// browsePagePush tells the page what changed for its buttons while the browser's page shows: what the library has of the videos the page asked about, as a job of one is added, finishes or fails, and their words, in another interface language. Off the browser's page it waits, and tells what changed once the page shows again.
func (a *app) browsePagePush() {
	b := &a.browse.buttons
	if len(b.videos) == 0 || a.page != pageBrowse {
		return
	}
	text := a.browsePageText()
	haves := a.browseHaves()
	have := map[string]string{}
	for u, v := range b.videos {
		if now := haves[v.key]; now != v.have {
			v.have = now
			b.videos[u] = v
			have[u] = now
		}
	}
	if len(have) == 0 && text == b.text {
		return
	}
	b.text = text
	a.browse.view.PostMessage(browse.StateData(browse.PageState{Text: text, Have: have}))
}

// browsePageAdd is a click on one of the page's own buttons, which adds the video at link as 下载视频 does (browseAdd), named by the title the page gave it. It takes it only while the browser's page shows, from a page of a site the catalog lists, and only a link to one video of such a site (browse.ParseAdd and kcore.Link.OneVideo), as any script of the page can post what its buttons do.
func (a *app) browsePageAdd(origin, link, title string) {
	if a.page != pageBrowse || !a.browseShowing() || !catalog.Load().On(origin) || !catalog.Load().On(link) {
		return
	}
	if parsed, ok := kcore.ParseLink(link); !ok || !parsed.OneVideo() {
		return
	}
	a.browseAdd(link, title)
}
