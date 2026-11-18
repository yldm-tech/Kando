// Package films is the Library tab: the films on the phone, then under each paired computer its libraries, which pull to refresh reads again; before any computer is paired, what pairing brings and the way to pair.
package films

import (
	"maps"
	"slices"

	"kando/apps/mobile/computers"
	"kando/apps/mobile/downloads"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/onboarding"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the tab leads.
type Actions struct {
	// Pair opens pairing.
	Pair func()
	// Film opens a film's page, of one on the phone.
	Film func(store.Film)
	// Library opens a computer's library.
	Library func(desktop string, lib sync.Library)
}

// Page is the tab's root page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	title := func() string { return locales.FilmsTitle(cx) }
	// gen counts the pulls to refresh, which the computers' sections read their libraries again on; loading is which of them are still reading.
	gen := core.NewSignal(cx, 0)
	loading := core.NewSignalFunc(cx, map[string]bool{}, nil)
	pulled := core.NewSignal(cx, false)
	cx.Effect(func() {
		if !pulled.Get() {
			return
		}
		busy := loading.Get()
		if !slices.ContainsFunc(s.Desktops(), func(d store.Desktop) bool { return busy[d.ID] }) {
			pulled.Set(false)
		}
	})
	ref := &core.Ref{}
	body := core.Scroll{TestID: "films-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		OnRefresh:  func() { pulled.Set(true); gen.Update(func(n int) int { return n + 1 }) },
		Refreshing: pulled.Get,
		Children: []core.Node{core.View{StyleFunc: look.Column(cx), Children: []core.Node{
			core.Show{When: func() bool { return len(s.Desktops()) == 0 }, Then: func(cx *core.Scope) core.Node { return unpaired(cx, act.Pair) }},
			core.Show{When: func() bool { return len(s.Films()) > 0 }, Then: func(cx *core.Scope) core.Node { return downloads.Section(cx, s, act.Film) }},
			core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
				return computers.Section(cx, s, d, gen.Get, computers.Actions{Pair: act.Pair, Library: act.Library}, func(busy bool) {
					id := d().ID
					loading.Update(func(m map[string]bool) map[string]bool { m = maps.Clone(m); m[id] = busy; return m })
				})
			}),
		}}}}
	return look.TabRoot(cx, base.NavPage{TestID: "films-page", Children: []core.Node{body}}, title, nil, nil)
}

// unpaired is the card the tab starts with while no computer is paired: what pairing brings, the way to pair, and where to get the desktop app.
func unpaired(cx *core.Scope, pair func()) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	return core.View{TestID: "films-unpaired", Style: core.Style{CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Padding: core.Edges{Top: 28, Right: 20, Bottom: 12, Left: 20}, AlignItems: core.AlignCenter, Gap: 12},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045)} },
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				StyleFunc: ui.Filled(tints.Accent), Children: []core.Node{ui.Icon(lucide.Laptop, 32, colors.Accent)}},
			core.Text{ContentFunc: func() string { return locales.FilmsUnpairedTitle(cx) }, Role: core.RoleHeader, Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.FilmsUnpairedBody(cx) }, Style: look.Subhead.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
			core.View{Style: core.Style{AlignSelf: core.AlignStretch, Gap: 4, Padding: core.Edges{Top: 8}}, Children: []core.Node{
				look.Button{Kind: look.Primary, Text: func() string { return locales.FilmsPair(cx) }, TestID: "films-pair", OnPress: pair},
				look.Button{Kind: look.Plain, Text: func() string { return locales.FilmsNoDesktop(cx) }, TestID: "films-desktop-app", OnPress: func() { cx.OpenURL(onboarding.DownloadURL, nil) }},
			}},
		}}
}
