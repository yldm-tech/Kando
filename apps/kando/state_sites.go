package main

import (
	"kando/apps/kando/locales"
	"kando/apps/kando/sites/catalog"

	"github.com/nsgo-dev/nsgo/core"
)

// sitesUI is the state of the 支持的网站 sheet (sites/): whether it is open, the search, and the logos read so far. The grid starts at its top each time, as the sheet is built anew as it opens.
type sitesUI struct {
	open  bool
	query string
	// logos holds each logo's bytes once its tile showed, by domain, so a tile hands its image the same bytes each time; nil marks a site without one, which shows its initial.
	logos map[string][]byte
	// release gives back the window's modal flag (holdModal) the sheet holds while it is open; closing the sheet runs it (sitesClose).
	release func()
}

func newSitesUI() sitesUI { return sitesUI{} }

// sitesHooks: the sheet has nothing to do at the moments the areas share; it gives back the window's modal flag as it closes (sitesClose).
func (a *app) sitesHooks() areaHooks { return areaHooks{} }

// sitesKeys are the sheet's rows of the shortcut table: 支持的网站… in the 帮助 menu, without a key.
func (a *app) sitesKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupHelp, Label: locales.SitesMenuItem(cx), Run: a.OpenSites},
	}
}

// OpenSites opens the sheet with the search cleared.
func (a *app) OpenSites() {
	if a.sites.open {
		return
	}
	a.sites.open = true
	a.sites.query = ""
	a.sites.release = a.holdModal()
}

// sitesClose closes the sheet, as 完成, Escape and closing it do, and gives back the window's modal flag.
func (a *app) sitesClose() {
	a.sites.open = false
	if a.sites.release != nil {
		a.sites.release()
		a.sites.release = nil
	}
}

// sitesOpen opens s in the browser, as a tile of the sheet or of the browser's start does: the sheet closes if it is open, the window goes to the browser's page, closing the film open, and the page loads the site's home.
func (a *app) sitesOpen(s catalog.Site) {
	if a.sites.open {
		a.sitesClose()
	}
	a.Select(pageBrowse)
	a.browseLoad(s.Home())
}

// sitesLogo reads s's embedded logo the first time its tile shows, and keeps it.
func (a *app) sitesLogo(s catalog.Site) []byte {
	if data, ok := a.sites.logos[s.Domain]; ok {
		return data
	}
	data := catalog.Logo(s)
	if a.sites.logos == nil {
		a.sites.logos = map[string][]byte{}
	}
	a.sites.logos[s.Domain] = data
	return data
}
