package player

import (
	"cmp"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The mini player's size: its bar's height, and the gap between it and the tab bar or the screen's foot.
const (
	miniHeight core.Points = 64
	miniGap    core.Points = 8
)

// MiniBar is the mini player over a tab's pages while the player is minimized: the film's poster, the line playing, its translation over its original, the button that plays or holds it, the one that plays the next line, and how far it has played along its foot. A tap opens the player again. Put it last in the tab's content, over the pages, whose foot keeps clear of it through look.ProvideMini.
func MiniBar(cx *core.Scope, l *Layer) core.Node {
	return core.Show{When: l.Mini, Then: func(cx *core.Scope) core.Node {
		p := l.cur.GetUntracked()
		if p == nil {
			return nil
		}
		return miniBar(cx, l, p)
	}}
}

func miniBar(cx *core.Scope, l *Layer, p *play) core.Node {
	// The bar is light in both appearances, as the design draws it over a dark page too.
	cx.SetSubtreeAppearance(core.AppearanceLight)
	colors := ui.ColorsOf(cx)
	fins := ui.FinOf(cx)
	over := look.OverTabBar(cx)
	// Beside a tablet's sidebar the bar is roomier: a wider poster, larger text and buttons.
	wide := func() bool { return !over() }
	title := func() string { f, _ := p.s.Film(p.key); return f.Film.Title }
	// The line playing, or the last heard: its translation first, its original under it; a film's title where there is no line, or no translation.
	top := func() string {
		c := p.line(p.shown.Get())
		return cmp.Or(c.Translation, c.Original, title())
	}
	under := func() string {
		c := p.line(p.shown.Get())
		if c.Translation != "" {
			return c.Original
		}
		if c.Original != "" {
			return title()
		}
		return ""
	}
	poster := func() core.ImageSource {
		f, _ := p.s.Film(p.key)
		return look.LocalPoster(p.s, f)
	}
	played := func() core.Style {
		length := p.length.Get()
		share := float32(0)
		if length > 0 {
			share = min(max(float32(p.at.Get())/float32(length), 0), 1)
		}
		return core.Style{Width: core.Pct(share * 100), Background: colors.Accent()}
	}
	bar := core.View{TestID: "player-mini", Role: core.RoleGroup, LabelFunc: func() string { return locales.PlayerMini(cx) },
		Style: core.Style{Height: core.Pt(miniHeight), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Padding: core.Edges{Top: 8, Right: 6, Bottom: 8, Left: 8},
			BorderCurve: core.BorderCurveContinuous, BorderWidth: 0.5, Overflow: core.Hidden, PointerEvents: core.PointerEventsAuto},
		StyleFunc: func() core.Style {
			st := fins.Menu().Style()
			st.CornerRadius = 16
			if wide() {
				st.CornerRadius = 18
			}
			return st
		},
		Children: []core.Node{
			core.View{TestID: "player-mini-open", Role: core.RoleButton, LabelFunc: func() string { return locales.PlayerMiniOpen(cx, title()) },
				OnPress: func(core.PressEvent) { l.Play(p.key) },
				Style:   core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Active: &core.Style{Opacity: core.Alpha(0.7)}},
				Children: []core.Node{
					core.View{Style: core.Style{CornerRadius: 10, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, FlexShrink: 0},
						StyleFunc: func() core.Style {
							if wide() {
								return core.Style{Width: core.Pt(76), Height: core.Pt(43), Background: colors.Inset()}
							}
							return core.Style{Width: core.Pt(48), Height: core.Pt(48), Background: colors.Inset()}
						},
						Children: []core.Node{core.Image{SourceFunc: poster, ResizeMode: core.Cover, ElementsHidden: true, Style: fill}}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
						core.Text{TestID: "player-mini-line", ContentFunc: top, MaxLines: 1, Style: core.Style{FontWeight: core.FontWeightBold}, StyleFunc: func() core.Style {
							if wide() {
								return core.Style{FontSize: 15, LineHeight: 20, Color: colors.Text()}
							}
							return core.Style{FontSize: 14, LineHeight: 19, Color: colors.Text()}
						}},
						core.Show{When: func() bool { return under() != "" }, Then: func(cx *core.Scope) core.Node {
							return core.Text{TestID: "player-mini-original", ContentFunc: under, MaxLines: 1, StyleFunc: func() core.Style {
								if wide() {
									return core.Style{FontSize: 13, LineHeight: 17, Color: colors.Muted()}
								}
								return core.Style{FontSize: 12, LineHeight: 16, Color: colors.Muted()}
							}}
						}},
					}},
				}},
			ui.IconButton{TestID: "player-mini-play", Variant: ui.IconButtonGhost, Size: 40, IconSize: 18,
				Icon:      playGlyphOf(p),
				LabelFunc: playLabel(cx, p),
				OnPress:   p.toggle},
			ui.IconButton{TestID: "player-mini-next", Icon: skipNextGlyph, Variant: ui.IconButtonGhost, Size: 40, IconSize: 18, LabelFunc: func() string { return locales.PlayerNextLine(cx) }, OnPress: p.next},
			core.View{PointerEvents: core.PointerEventsNone, AccessibilityElementsHidden: func() bool { return true },
				Style:     core.Style{Position: core.Absolute, Left: core.Pt(14), Right: core.Pt(14), Bottom: core.Pt(0), Height: core.Pt(2), CornerRadius: 1, Overflow: core.Hidden},
				StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.08)} },
				Children:  []core.Node{core.View{TestID: "player-mini-played", Style: core.Style{Height: core.Pct(100)}, StyleFunc: played}}},
		}}
	// Over a tab bar the bar sits above it, as iOS's floats over the pages or another takes its own room; beside a tablet's sidebar, above the screen's foot.
	return core.View{PointerEvents: core.PointerEventsBoxNone,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), ZIndex: 5},
		StyleFunc: func() core.Style {
			st := core.Style{Padding: core.Edges{Right: 8, Left: 8, Bottom: miniGap + ui.TabBarInset(cx)}, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea}}
			if !over() {
				st.Padding = core.Edges{Right: 20, Left: 20, Bottom: 16}
				st.PaddingEnv.Bottom = core.EnvSafeArea
			}
			return st
		},
		Children: []core.Node{bar}}
}
