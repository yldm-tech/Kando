package main

import (
	"reflect"
	"slices"
	"time"

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
	for _, job := range a.lockVisible(a.library.Jobs) {
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

// browsePageVideo reports whether u is a link to one video the page's buttons know at all: of Kando's sites (browse.VideoLink) or of an allowed site of a site pack turned on, as the app's own matcher of those sites decides it, never the page (pluginSiteOf). Which of them a page may ask about and add depends on its site (browsePageTakes).
func (a *app) browsePageVideo(u string) bool {
	if browse.VideoLink(u) {
		return true
	}
	id, video := a.pluginSiteOf(u)
	return id != "" && video
}

// browsePageTakes is what a page of origin may ask about and add, nil for a page of a site that gets no buttons: a page of a site the catalog lists, links to one video of Kando's sites (browse.VideoLink), as before; a page of an allowed site of a site pack, links to one video of that pack's allowed sites alone, so a pack's page never adds a video of Kando's sites, which would borrow a sign-in, nor another pack's.
func (a *app) browsePageTakes(origin string) func(u string) bool {
	if catalog.Load().On(origin) {
		return browse.VideoLink
	}
	pack, _ := a.pluginSiteOf(origin)
	if pack == "" {
		return nil
	}
	return func(u string) bool {
		id, video := a.pluginSiteOf(u)
		return id == pack && video
	}
}

// browseSitesChanged gives the pages' buttons sites, the allowed sites of the site packs turned on, besides Kando's own (browse.ScriptWith), as the web view takes their script (browseTakeScript).
func (a *app) browseSitesChanged(sites []kcore.VideoSite) {
	if reflect.DeepEqual(sites, a.browse.sites) {
		return
	}
	a.browse.sites = sites
	a.browse.next = browse.ScriptWith(sites)
	a.browseTakeScript()
}

// browseTakeScript gives the web view the page script of the sites the user allowed (browseSitesChanged) and reports whether it did. A changed script has the same web view load its source again with it at once, in place of the page it shows, keeping the history before it (core.WebView.InjectedJavaScriptBeforeContentFunc, browse/page.go), so the source becomes the page it shows, which the user may have gone on to from the source, or the blank page under the start; a load under way says nothing of being cut short (browseCutShort), and what the buttons were told goes, as the page loaded again asks again. A web view hidden behind another page of the window, as while the user allows a site in the settings, takes the script once the browser's page shows again (browseAfterChange): the page it loads again would play its videos unpaused and with sound (browse.QuietScript).
func (a *app) browseTakeScript() bool {
	if a.browse.next == a.browse.script || a.browse.source != "" && a.page != pageBrowse {
		return false
	}
	a.browse.script = a.browse.next
	if a.browse.source == "" {
		return true
	}
	if a.browseShowing() {
		a.browse.source = a.browse.url
	} else {
		a.browse.source = browseBlank
	}
	a.browse.buttons = browseButtons{}
	a.browseCutShort()
	return true
}

// browsePageAsk answers a page of a site the catalog lists, or of an allowed site of a site pack, that asks what the library has of links, the videos it shows (browse.ParseAskWith with browsePageVideo) that its site may ask about (browsePageTakes), with the buttons' words, and follows those videos, so the page hears when that changes (browsePagePush). A page of another site, which gets no buttons, is not answered.
func (a *app) browsePageAsk(origin string, links []string) {
	takes := a.browsePageTakes(origin)
	if takes == nil {
		return
	}
	links = slices.DeleteFunc(slices.Clone(links), func(u string) bool { return !takes(u) })
	if len(links) == 0 {
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

// browsePageAdd is a click on one of the page's own buttons, which adds the video at link as 下载视频 does (browseAdd), named by the title the page gave it. It takes it only while the browser's page shows, from a page of a site the catalog lists or an allowed site of a site pack, and only a link its site may add (browsePageTakes): of Kando's sites, a link to one video of a site the catalog lists (kcore.Link.OneVideo) from such a page; of a site pack's, one to a video of that pack's sites from its page, which borrows no sign-in (browseEnqueue, pluginsStampSite). Any script of the page can post what its buttons do.
func (a *app) browsePageAdd(origin, link, title string, at time.Duration) {
	takes := a.browsePageTakes(origin)
	if a.page != pageBrowse || !a.browseShowing() || takes == nil || !takes(link) {
		return
	}
	if catalog.Load().On(origin) {
		if !catalog.Load().On(link) {
			return
		}
		if parsed, ok := kcore.ParseLink(link); !ok || !parsed.OneVideo() {
			return
		}
	}
	// 下载视频 on a page that plays the video says where it was, so the film opens there once done (core.Job.ResumeAt).
	a.queue.stamp = addSheetResumeStamp(at)
	defer func() { a.queue.stamp = nil }()
	a.browseAdd(link, title)
}
