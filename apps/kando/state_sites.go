package main

import (
	"kando/apps/kando/sites/catalog"

	"github.com/egoist/mygo/ui"
)

// sitesUI is the state of the 支持的网站 sheet (sites/): whether it is open, the search, where the grid is scrolled, and the logos decoded so far.
type sitesUI struct {
	open  bool
	query string
	grid  ui.GridState
	// logos holds each logo decoded once its tile showed, by domain; nil marks a logo that would not decode, so the tile shows the initial without decoding it again.
	logos map[string]ui.ImageSource
	// release gives back the window's modal flag (holdModal) the sheet holds while it is open; sitesHooks' afterFrame runs it once the sheet has closed.
	release func()
}

func newSitesUI() sitesUI { return sitesUI{} }

// sitesHooks: the sheet gives back the window's modal flag once it has closed.
func (a *app) sitesHooks() areaHooks {
	return areaHooks{afterFrame: a.sitesModal}
}

// sitesKeys are the sheet's rows of the shortcut table: 支持的网站… in the 帮助 menu, without a key.
func (a *app) sitesKeys() []shortcut {
	return []shortcut{
		{Group: "帮助", Label: "支持的网站…", Run: a.OpenSites},
	}
}

// OpenSites opens the sheet with the search cleared and the grid at its top.
func (a *app) OpenSites() {
	if a.sites.open {
		return
	}
	a.sites.open = true
	a.sites.query = ""
	a.sites.grid = ui.GridState{}
	a.sites.release = a.holdModal()
}

func (a *app) sitesModal() {
	if !a.sites.open && a.sites.release != nil {
		a.sites.release()
		a.sites.release = nil
	}
}

// sitesLogo decodes s's embedded logo the first time its tile shows, and keeps it.
func (a *app) sitesLogo(s catalog.Site) ui.ImageSource {
	if img, ok := a.sites.logos[s.Domain]; ok {
		return img
	}
	var img ui.ImageSource
	if data := catalog.Logo(s); data != nil {
		if decoded, err := ui.DecodeImage(data); err == nil {
			img = decoded
		}
	}
	if a.sites.logos == nil {
		a.sites.logos = map[string]ui.ImageSource{}
	}
	a.sites.logos[s.Domain] = img
	return img
}
