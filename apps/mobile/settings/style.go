package settings

import (
	"strconv"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/substyle"
	"kando/apps/mobile/theme"

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

// sizeShortName is a subtitle size's short name, as the size control says it.
func sizeShortName(cx *core.Scope, z store.SubSize) string {
	switch z {
	case store.SubSmall:
		return locales.StyleSizeShortSmall(cx)
	case store.SubLarge:
		return locales.StyleSizeShortLarge(cx)
	case store.SubHuge:
		return locales.StyleSizeShortHuge(cx)
	}
	return locales.StyleSizeShortMedium(cx)
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

// StylePage is the page that sets how the player draws subtitles, over a preview that follows each choice: their size, the original line's color and where they show on the picture; and whether the original's words show the marks the computer computes for them.
func StylePage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	style := func() store.SubStyle { return s.Prefs().Subtitles }
	ref := &core.Ref{}
	// sizes are the size control's options: the short name shows, in the ink, bold while chosen, and the full name is what assistive technology reads.
	var sizes []ui.Option[store.SubSize]
	for _, z := range store.SubSizes {
		sizes = append(sizes, ui.Option[store.SubSize]{Value: z, LabelFunc: func() string { return sizeName(cx, z) }, TestID: "style-size-" + sizeTestID(z),
			Content: core.Text{ContentFunc: func() string { return sizeShortName(cx, z) }, MaxLines: 1, StyleFunc: func() core.Style {
				st := core.Style{FontSize: 13, LineHeight: 16, FontWeight: core.FontWeightMedium, Color: colors.Text()}
				if style().Size == z {
					st.FontWeight = core.FontWeightBold
				}
				return st
			}}})
	}
	// A swatch is 36 points across; the chosen one is ringed in the accent, 3 points out from it, as the design draws it.
	var swatches []core.Node
	for _, c := range store.SubColors {
		swatches = append(swatches, core.View{TestID: "style-color-" + colorTestID(c), Role: core.RoleRadio, LabelFunc: func() string { return colorName(cx, c) },
			Checked: func() bool { return style().Color == c }, OnPress: func(core.PressEvent) { setStyle(s, func(st *store.SubStyle) { st.Color = c }) },
			Style: core.Style{Width: core.Pt(46), Height: core.Pt(46), CornerRadius: 23, BorderWidth: 2, Padding: core.All(3)},
			StyleFunc: func() core.Style {
				ring := ui.Alpha(colors.Text(), 0)
				if style().Color == c {
					ring = colors.Accent()
				}
				return core.Style{BorderColor: ring}
			},
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 18, BorderWidth: 0.5, Background: substyle.Color(c)},
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
		return core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
			look.GroupHeader(cx, title),
			core.View{Role: core.RoleRadioGroup, LabelFunc: label, Children: []core.Node{look.Group{Children: rows}}},
		}}
	}
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsSubtitleStyle(cx) }, TestID: "style-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				preview(cx, style),
				core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
					look.GroupHeader(cx, func() string { return locales.StyleSize(cx) }),
					// The design's segmented control: its labels in the ink at 500, 700 while chosen, drawn as each option's Content.
					ui.Tabs[store.SubSize]{TestID: "style-size", Variant: ui.TabsSegmented, Equal: true, Size: ui.SizeLarge, LabelFunc: func() string { return locales.StyleSize(cx) }, Options: sizes,
						Style: look.SegmentTrack, TabStyle: look.SegmentTab, IndicatorStyle: look.SegmentThumb,
						Value: func() store.SubSize { return style().Size }, OnChange: func(z store.SubSize) { setStyle(s, func(st *store.SubStyle) { st.Size = z }) }},
				}},
				core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
					look.GroupHeader(cx, func() string { return locales.StyleColor(cx) }),
					core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.StyleColor(cx) },
						Style:     core.Style{Direction: core.Row, Gap: 4, Padding: core.Symmetric(9, 11)},
						StyleFunc: func() core.Style { return look.Card(cx) }, Children: swatches},
				}},
				group(func() string { return locales.StylePosition(cx) }, func() string { return locales.StylePosition(cx) },
					position(0, false, "bottom", func() string { return locales.StyleBottom(cx) }),
					position(1, true, "top", func() string { return locales.StyleTop(cx) })),
				look.Group{Children: []core.Node{look.Row{TestID: "style-marks", TitleLines: 1,
					Title:  func() string { return locales.StyleMarks(cx) },
					Detail: func() string { return locales.StyleMarksDetail(cx) },
					Trailing: look.Switch{TestID: "style-marks-switch", Label: func() string { return locales.StyleMarks(cx) },
						Checked: func() bool { return !style().NoMarks }, OnChange: func(on bool) { setStyle(s, func(st *store.SubStyle) { st.NoMarks = !on }) }}}}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// preview is a picture with a line drawn on it as the player draws it with style, which follows each choice: no box behind the lines, which a shadow sets off from the picture.
func preview(cx *core.Scope, style func() store.SubStyle) core.Node {
	line := func(id string, text func() string, original bool) core.Node {
		return core.Text{TestID: id, ContentFunc: text, Style: core.Style{MaxWidth: core.Pct(100), TextShadowColor: core.RGBA(0x000000, 0.9), TextShadowRadius: 3, TextShadowOffset: core.Point{Y: 1}},
			StyleFunc: func() core.Style {
				tr, orig := substyle.Sizes(style(), 17, 13)
				if original {
					return core.Style{FontSize: orig, LineHeightMultiple: 1.25, FontWeight: core.FontWeightMedium, Color: substyle.Color(style().Color), TextAlign: core.TextAlignCenter}
				}
				return core.Style{FontSize: tr, LineHeightMultiple: 1.25, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF), TextAlign: core.TextAlignCenter}
			}}
	}
	return core.View{TestID: "style-preview", Role: core.RoleImage, LabelFunc: func() string { return locales.StylePreview(cx) },
		Style: core.Style{Width: core.Pct(100), AspectRatio: 358.0 / 200, CornerRadius: look.CardRadius, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden,
			Background: theme.PreviewBase, AlignItems: core.AlignCenter, Gap: 2, Padding: core.Symmetric(10, 12)},
		StyleFunc: func() core.Style {
			if style().Top {
				return core.Style{JustifyContent: core.JustifyStart}
			}
			return core.Style{JustifyContent: core.JustifyEnd}
		},
		Children: []core.Node{
			// The picture, the design's river under mountains at 0.9 over black, so the lines show on something as they do over a film; the box takes the picture's proportions, so it fills it.
			core.SVGView{SVG: riverScene, Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Opacity: core.Alpha(0.9)}},
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
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsGoal(cx) }, TestID: "settings-goal-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.SettingsGoal(cx) }, Children: []core.Node{look.Group{Children: rows}}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}
