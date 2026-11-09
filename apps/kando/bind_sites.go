package main

import (
	"kando/apps/kando/sites"
	"kando/apps/kando/sites/catalog"

	"github.com/egoist/mygo/ui"
)

// sitesOverlay draws the 支持的网站 sheet while it is open; overlays calls it.
func (a *app) sitesOverlay(c *ui.Context, t *ui.Theme) {
	if a.sites.open {
		sites.Show(c, t, a, &a.sites.open)
	}
}

// The sheet's reads (sites.App).

func (a *app) SitesQuery() *string                    { return &a.sites.query }
func (a *app) SitesGrid() *ui.GridState               { return &a.sites.grid }
func (a *app) SiteLogo(s catalog.Site) ui.ImageSource { return a.sitesLogo(s) }
