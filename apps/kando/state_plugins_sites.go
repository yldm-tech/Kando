package main

import (
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	kcore "kando/packages/core"
)

// Site packs (plugins.KindSite, docs/plugin-system.md E8): sites a third party says are video sites, with the shapes of their videos' links, which the built-in browser's 下载视频 in its toolbar then knows as it knows the videos of Kando's own sites (kcore.VideoSites): on a page of an allowed site, 下载视频 adds the page only when its link is one of a video, and is off on the site's other pages, which it never reads as a playlist or a channel either. Each domain works only once the user allowed it, on its own, in the install confirmation or the pack's details (plugins.State.Allowed), and only while the pack is turned on.
//
// The sites widen no gate of the app. They are never added to kcore.VideoSites or the catalog (sites/catalog), which the browser extension's socket, the remote API, the command line and the MCP server trust; only the toolbar's 下载视频 consults them (browseNotVideo, browseList, browseEnqueue), never the pages' own buttons (browsePageAsk, browsePageAdd), the extension (browserExtVideo) or any other way a link comes in. The pages' buttons do not cover them: the browser's page script (browse.Script) holds the sites it knows as it is handed to the web view, once, when the web view is made (core.WebView.InjectedJavaScriptBeforeContent), and the web view lives for the whole launch, so it could not follow a pack installed or a domain allowed since. The jobs 下载视频 adds from these sites borrow no browser's sign-in and none of the pages' cookies (core.Job.PluginSite, queuePipeline).

// pluginsSite is a site pack turned on, with the sites of it the user allowed, as 下载视频 matches links against them.
type pluginsSite struct {
	id    string
	sites *kcore.VideoMatcher
}

// pluginsSyncSites matches 下载视频's links against the site packs turned on, in the order the settings keep, each with the domains the user allowed, once the packs, their switches or the domains allowed changed. A pack none of whose domains is allowed is left out.
func (a *app) pluginsSyncSites() {
	var out []pluginsSite
	for _, pack := range a.pluginsOn(plugins.KindSite) {
		var allowed []kcore.VideoSite
		for _, s := range pack.Site.VideoSites() {
			if a.plugins.state.Allowed(pack.ID, s.Domain) {
				allowed = append(allowed, s)
			}
		}
		if len(allowed) == 0 {
			continue
		}
		// The pack passed plugins.Validate, which checks its sites as NewVideoMatcher does, as it was read; a pack that does not is left out all the same.
		if m, err := kcore.NewVideoMatcher(allowed); err == nil {
			out = append(out, pluginsSite{id: pack.ID, sites: m})
		}
	}
	a.plugins.sites = out
}

// pluginSiteOf is the site pack one of whose allowed sites u is on, "" for none, and whether u is a link to one video of it.
func (a *app) pluginSiteOf(u string) (id string, video bool) {
	for _, s := range a.plugins.sites {
		if domain, video := s.sites.Find(u); domain != "" {
			return s.id, video
		}
	}
	return "", false
}

// pluginsStampSite has the job 下载视频 queues for u, a link to one video of a site pack's allowed site, borrow no browser's sign-in (core.Job.PluginSite, core.CookiesNone), after whatever else stamps it (jobQueue.stamp); it returns what puts the stamp back as it was, which the caller runs once it has queued. Any other link is left alone.
func (a *app) pluginsStampSite(u string) (restore func()) {
	id, video := a.pluginSiteOf(u)
	before := a.queue.stamp
	if id == "" || !video {
		return func() {}
	}
	a.queue.stamp = func(job *kcore.Job) {
		if before != nil {
			before(job)
		}
		job.PluginSite, job.Cookies = id, kcore.CookiesNone
	}
	return func() { a.queue.stamp = before }
}

// pluginsSiteRemedy is r, the card of a failed download from a site pack's site (core.Job.PluginSite), without what would borrow a browser's sign-in, which its download never does (queuePipeline): those buttons go, and the card says why instead.
func pluginsSiteRemedy(m locales.Messages, r films.Remedy) films.Remedy {
	var actions []films.RemedyAction
	for _, act := range r.Actions {
		if strings.HasPrefix(act.ID, remedyBorrow) || act.ID == remedyFindBrowsers || act.ID == remedyChooseBrowser || act.ID == remedyDiskAccess {
			continue
		}
		actions = append(actions, act)
	}
	if len(actions) == len(r.Actions) {
		return r
	}
	if len(actions) > 0 && !slices.ContainsFunc(actions, func(act films.RemedyAction) bool { return act.Primary }) {
		actions[0].Primary = true
	}
	r.Why, r.Actions = m.QueueRemedyPluginSite(), actions
	return r
}

// pluginsAllowSite allows or no longer allows domain of the site pack id, saving state.json off the UI thread as a switch does (pluginsApplyState); 下载视频 follows at once.
func (a *app) pluginsAllowSite(id, domain string, on bool) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if a.pluginsRoot() == "" || i < 0 || a.plugins.busy || !slices.Contains(a.plugins.packs[i].SiteDomains(), domain) {
		return
	}
	hash := a.plugins.packs[i].Hash
	next := a.pluginsNextState(func(s *plugins.State) {
		e, ok := s.Entry(id)
		if !ok {
			e = plugins.PluginState{ID: id, Enabled: s.Enabled(id)}
		}
		e.Hash = hash
		e.Sites = slices.DeleteFunc(slices.Clone(e.Sites), func(d string) bool { return d == domain })
		if on {
			e.Sites = append(e.Sites, domain)
		}
		s.Set(e)
	})
	a.pluginsApplyState(next)
}

// pluginsInstallSites are the domains of the pack of the confirmation p the user ticked, in the pack's order; none for another kind.
func pluginsInstallSites(p *pluginsPending) []string {
	var out []string
	for _, d := range p.pack.SiteDomains() {
		if slices.Contains(p.sites, d) {
			out = append(out, d)
		}
	}
	return out
}

// pluginsTickSite ticks or unticks domain of the site pack in the open confirmation, which installs it with the domains ticked; nothing while it installs.
func (a *app) pluginsTickSite(domain string, on bool) {
	p := a.plugins.pending
	if p == nil || p.err != nil || a.plugins.busy || !slices.Contains(p.pack.SiteDomains(), domain) {
		return
	}
	p.sites = slices.DeleteFunc(p.sites, func(d string) bool { return d == domain })
	if on {
		p.sites = append(p.sites, domain)
	}
}

// pluginsTickedBefore are the domains of pack, a site pack about to be installed, that the user allowed for the pack of its id installed now, as the confirmation of a new version starts with them ticked; a domain the new version no longer names, and every domain of a new pack, starts unticked.
func (a *app) pluginsTickedBefore(pack plugins.Pack) []string {
	var out []string
	for _, d := range pack.SiteDomains() {
		if a.plugins.state.Allowed(pack.ID, d) {
			out = append(out, d)
		}
	}
	return out
}
