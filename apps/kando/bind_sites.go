package main

import (
	"kando/apps/kando/live"
	"kando/apps/kando/sites"
	"kando/apps/kando/sites/catalog"

	"github.com/nsgo-dev/nsgo/core"
)

// sitesOverlay is the 支持的网站 sheet over the window while it is open; overlays calls it. The sheet is built as it opens, so it opens with the keyboard in its search and its grid at the top.
func (a *app) sitesOverlay(*core.Scope) core.Node {
	return core.Show{
		When: live.Func(a, func() bool { return a.sites.open }),
		Then: func(*core.Scope) core.Node { return sites.Sheet{App: a, OnClose: live.Do(a, a.sitesClose)} },
	}
}

// The sheet's reads (sites.App).

func (a *app) SitesQuery() *string { return &a.sites.query }

func (a *app) SiteLogo(s catalog.Site) []byte { return a.sitesLogo(s) }
