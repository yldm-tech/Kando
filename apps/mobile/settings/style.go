package settings

import (
	"strconv"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/substyle"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// sizeName is a subtitle size's name.
func sizeName(cx *core.Scope, z store.SubSize) string {
	switch z {
	case store.SubSmall:
		return locales.StyleSmall(cx)
	case store.SubLarge:
		return locales.StyleLarge(cx)
	case store.SubHuge:
		return locales.StyleHuge(cx)
	}
	return locales.StyleMedium(cx)
}

// colorName is a color's name.
func colorName(cx *core.Scope, c store.SubColor) string {
	switch c {
	case store.SubWhite:
		return locales.StyleWhite(cx)
	case store.SubCyan:
		return locales.StyleCyan(cx)
	case store.SubPink:
		return locales.StylePink(cx)
	}
	return locales.StyleYellow(cx)
}

// setStyle keeps change of the subtitle style.
func setStyle(s *session.Session, change func(*store.SubStyle)) {
	p := s.Prefs()
	change(&p.Subtitles)
	s.SetPrefs(p)
}

// StylePage is the page that sets how the player draws subtitles, over a preview that follows each choice: their size, the original line's color and where they show on the picture.
func StylePage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	style := func() store.SubStyle { return s.Prefs().Subtitles }
	ref := &core.Ref{}
	var sizes []core.Node
	for i, z := range store.SubSizes {
		sizes = append(sizes, look.Row{TestID: "style-size-" + sizeTestID(z), Index: func() int { return i },
			Title:   func() string { return sizeName(cx, z) },
			Checked: func() bool { return style().Size == z },
			Trailing: core.Show{When: func() bool { return style().Size == z }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
			}},
			OnPress: func() { setStyle(s, func(st *store.SubStyle) { st.Size = z }) }})
	}
	var swatches []core.Node
	for _, c := range store.SubColors {
		swatches = append(swatches, core.View{TestID: "style-color-" + colorTestID(c), Role: core.RoleRadio, LabelFunc: func() string { return colorName(cx, c) },
			Checked: func() bool { return style().Color == c }, OnPress: func(core.PressEvent) { setStyle(s, func(st *store.SubStyle) { st.Color = c }) },
			Style: core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, BorderWidth: 3, Padding: core.All(3)},
			StyleFunc: func() core.Style {
				ring := ui.Alpha(colors.Text(), 0)
				if style().Color == c {
					ring = colors.Accent()
				}
				return core.Style{BorderColor: ring}
			},
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, BorderWidth: 0.5, Background: substyle.Color(c)},
				StyleFunc: func() core.Style { return core.Style{BorderColor: ui.Alpha(colors.Text(), 0.2)} }}}})
	}
	position := func(i int, top bool, id string, name func() string) core.Node {
		return look.Row{TestID: "style-position-" + id, Index: func() int { return i },
			Title:   name,
			Checked: func() bool { return style().Top == top },
			Trailing: core.Show{When: func() bool { return style().Top == top }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
			}},
			OnPress: func() { setStyle(s, func(st *store.SubStyle) { st.Top = top }) }}
	}
	group := func(title func() string, label func() string, rows ...core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
			look.SectionTitle(cx, title),
			core.View{Role: core.RoleRadioGroup, LabelFunc: label, Children: []core.Node{look.Group{Children: rows}}},
		}}
	}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsSubtitleStyle(cx) }, TestID: "style-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				preview(cx, style),
				group(func() string { return locales.StyleSize(cx) }, func() string { return locales.StyleSize(cx) }, sizes...),
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					look.SectionTitle(cx, func() string { return locales.StyleColor(cx) }),
					core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.StyleColor(cx) },
						Style: core.Style{Direction: core.Row, Gap: 14, Padding: core.Symmetric(4, 4)}, Children: swatches},
				}},
				group(func() string { return locales.StylePosition(cx) }, func() string { return locales.StylePosition(cx) },
					position(0, false, "bottom", func() string { return locales.StyleBottom(cx) }),
					position(1, true, "top", func() string { return locales.StyleTop(cx) })),
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// preview is a picture with a line drawn on it as the player draws it with style, which follows each choice.
func preview(cx *core.Scope, style func() store.SubStyle) core.Node {
	line := func(id string, text func() string, original bool) core.Node {
		return core.View{Style: core.Style{MaxWidth: core.Pct(100), CornerRadius: 6, Padding: core.Symmetric(2, 10), Background: core.RGBA(0x000000, 0.62)}, Children: []core.Node{
			core.Text{TestID: id, ContentFunc: text, StyleFunc: func() core.Style {
				tr, orig := substyle.Sizes(style(), 17, 14)
				if original {
					return core.Style{FontSize: orig, LineHeightMultiple: 1.25, FontWeight: core.FontWeightMedium, Color: substyle.Color(style().Color), TextAlign: core.TextAlignCenter}
				}
				return core.Style{FontSize: tr, LineHeightMultiple: 1.25, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF), TextAlign: core.TextAlignCenter}
			}},
		}}
	}
	return core.View{TestID: "style-preview", Role: core.RoleImage, LabelFunc: func() string { return locales.StylePreview(cx) },
		Style: core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden,
			Background: core.RGB(0x1C2A3A), AlignItems: core.AlignCenter, Gap: 4, Padding: core.All(12)},
		StyleFunc: func() core.Style {
			if style().Top {
				return core.Style{JustifyContent: core.JustifyStart}
			}
			return core.Style{JustifyContent: core.JustifyEnd}
		},
		Children: []core.Node{
			// The picture: a sky over hills, so the lines show on something as they do over a film.
			core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Height: core.Pct(42), Background: core.RGB(0x2F4A3A)}},
			core.View{Style: core.Style{Position: core.Absolute, Right: core.Pct(18), Top: core.Pct(18), Width: core.Pt(36), Height: core.Pt(36), CornerRadius: 18, Background: core.RGB(0xF3D27A)}},
			line("style-preview-translation", func() string { return locales.StylePreviewTranslation(cx) }, false),
			line("style-preview-original", func() string { return locales.StylePreviewOriginal(cx) }, true),
		}}
}

func sizeTestID(z store.SubSize) string {
	switch z {
	case store.SubSmall:
		return "small"
	case store.SubLarge:
		return "large"
	case store.SubHuge:
		return "huge"
	}
	return "medium"
}

func colorTestID(c store.SubColor) string {
	switch c {
	case store.SubWhite:
		return "white"
	case store.SubCyan:
		return "cyan"
	case store.SubPink:
		return "pink"
	}
	return "yellow"
}

// GoalPage is the page that picks the daily goal, in minutes of films played.
func GoalPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	var rows []core.Node
	for i, m := range learning.GoalMinutes {
		rows = append(rows, look.Row{TestID: "settings-goal-" + strconv.Itoa(m), Index: func() int { return i },
			Title:   func() string { return locales.SettingsGoalValue(cx, m) },
			Checked: func() bool { return s.Goal() == m },
			Trailing: core.Show{When: func() bool { return s.Goal() == m }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
			}},
			OnPress: func() {
				p := s.Prefs()
				p.Goal = m
				s.SetPrefs(p)
			}})
	}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsGoal(cx) }, TestID: "settings-goal-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.SettingsGoal(cx) }, Children: []core.Node{look.Group{Children: rows}}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}
