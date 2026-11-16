package films

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// barFade is the top bar's face: the page's color, nearly opaque at the top and fading out at its bottom, so what scrolls under it fades into it, with no blur and no line, as the prototype's toolbar.
func barFade(page ns.Color) ns.Gradient {
	return ns.Gradient{Kind: ns.GradientLinear, Angle: 180, Stops: []ns.ColorStop{
		{Color: page.Faded(0.96), At: ns.Pct(0)},
		{Color: page.Faded(0.86), At: ns.Pct(60)},
		{Color: page.Faded(0), At: ns.Pct(100)},
	}}
}

// pill is the look of the pills of the link field and 本地视频, which lift them off what scrolls under the bar: the theme's card color, translucent, with the shade's hairline (theme.Of).
func pill(k theme.Colors) ns.Style {
	return ns.Style{Background: k.Card().Faded(0.75), BorderColor: k.Shade(0.12), Shadows: pillShadow}
}

var pillShadow = []ns.BoxShadow{{Offset: ns.Point{Y: 2}, Blur: 8, Color: ns.RGBA(0x000000, 0.06)}}

// topBar is the bar along the top of the library: the source being shown, the link field, 浏览网站, which opens the built-in browser to find a video in, 本地视频 and 开始处理. Enter in the field adds the link as 开始处理 does. It lies over the page's scroll view, which scrolls under it, and drags the window. It takes the prototype's toolbar: 65 points high, its controls 10 from its top and 14 from its end, and its title 24 from its start at 15 points; on macOS the sidebar holds the traffic lights, so none of its room is the window controls', and elsewhere it keeps clear of those at its end and is at least as tall as the title bar.
type topBar struct {
	a      App
	source func() string
}

// Render implements core.Node.
func (b topBar) Render(cx *ns.Scope) ns.Node {
	a := b.a
	colors := ui.ColorsOf(cx)
	title := func() string { return PageTitle(locales.For(cx), b.source()) }
	// The sidebar's switcher names the open library, so the bar names only the source the page shows.
	head := ns.View{Style: ns.Style{FlexShrink: 1, MinWidth: ns.Pt(0), JustifyContent: ns.JustifyCenter}, Children: []ns.Node{
		heading(1, ns.Text{ContentFunc: title, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 15, FontWeight: ui.Bold, LetterSpacing: -0.1}, StyleFunc: ui.Colored(colors.Text)}),
	}}
	// The prototype's toolbar pills are 34 points high; 开始处理 is the platform's primary button, its accent's shadow under it.
	start := ns.View{Style: ns.Style{CornerRadius: 17}, StyleFunc: func() ns.Style {
		return ns.Style{Shadows: []ns.BoxShadow{{Offset: ns.Point{Y: 2}, Blur: 8, Color: colors.Accent().Faded(0.3)}}}
	}, Children: []ns.Node{textButton(locales.FilmsStart, ui.Button{Size: ui.SizeLarge, TestID: "films-start", OnPress: live.Do(a, a.Start)})}}
	tools := ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 10, FlexShrink: 0, Margin: ns.Edges{Left: 2}},
		Children: []ns.Node{browseButton{a}, localButton{a}, start}}
	end := ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 10, FlexShrink: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{linkField{a}, tools}}
	return ns.View{
		TestID:    "films-top-bar",
		Role:      ns.RoleToolbar,
		LabelFunc: title,
		Desktop:   ns.DesktopViewProps{WindowDrag: ns.WindowDragOn},
		Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(0), Left: ns.Pt(0), Right: ns.Pt(0), Direction: ns.Row, AlignItems: ns.AlignStretch, JustifyContent: ns.JustifySpaceBetween,
			Gap: 16},
		StyleFunc: func() ns.Style {
			bar := nsdesktop.TitleBar(cx)
			return ns.Style{MinHeight: ns.Pt(max(65, bar.Top)), Padding: ns.Edges{Top: 10, Right: 14 + bar.Right, Bottom: 18, Left: 24}, Gradients: []ns.Gradient{barFade(colors.Background())}}
		},
		Children: []ns.Node{head, end},
	}
}

// browseButton is 浏览网站 in a pill as 本地视频's, with the compass of the sidebar's 浏览, whose page it opens.
type browseButton struct{ a App }

// Render implements core.Node.
func (b browseButton) Render(cx *ns.Scope) ns.Node {
	a := b.a
	colors := ui.ColorsOf(cx)
	label := func() string { return locales.FilmsBrowseSites(cx) }
	button := base.Button{
		TestID:    "films-browse",
		LabelFunc: label,
		OnPress:   live.On(a, func(ns.PressEvent) { a.Select(sidebar.PageBrowse) }),
		Style: ns.Style{Direction: ns.Row, FlexGrow: 1, Height: ns.Pt(28), CornerRadius: 14, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, Gap: 6,
			Padding: ns.Edges{Left: 10, Right: 12}, Cursor: ns.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() ns.Style {
			return ns.Style{Hover: &ns.Style{Background: colors.Hover()}, Active: &ns.Style{Background: colors.Pressed()}}
		},
		Children: []ns.Node{
			ui.Icon(lucide.Compass, 15, colors.Text),
			ns.Text{ContentFunc: label, MaxLines: 1, Style: ns.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Text)},
		},
	}
	k := theme.Of(cx)
	return ns.View{Style: ns.Style{Direction: ns.Row, FlexShrink: 0, Height: ns.Pt(34), Padding: ns.Symmetric(0, 3), CornerRadius: 17, AlignItems: ns.AlignCenter,
		BorderWidth: 0.5}, StyleFunc: func() ns.Style { return pill(k) }, Children: []ns.Node{button}}
}

// localButton is 本地视频 as the prototype groups it: a 28-point button in a white pill at least 104 wide, wider for a longer label, that lifts it off what scrolls under the bar. Its tooltip names the shortcut that does the same, as the platform writes it (⌘O on macOS, Ctrl+O elsewhere).
type localButton struct{ a App }

// Render implements core.Node.
func (l localButton) Render(cx *ns.Scope) ns.Node {
	a := l.a
	colors := ui.ColorsOf(cx)
	face := ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 6}, Children: []ns.Node{
		ui.Icon(svg(addVideoIcon), 16, colors.Text),
		ns.Text{ContentFunc: func() string { return locales.FilmsLocalVideo(cx) }, MaxLines: 1, Style: ns.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Text)},
	}}
	button := base.Button{
		TestID:    "films-local",
		LabelFunc: func() string { return locales.FilmsLocalVideo(cx) },
		OnPress:   live.On(a, func(ns.PressEvent) { a.PickVideos() }),
		Style: ns.Style{Direction: ns.Row, FlexGrow: 1, Height: ns.Pt(28), CornerRadius: 14, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter,
			Padding: ns.Edges{Left: 10, Right: 12}, Cursor: ns.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() ns.Style {
			return ns.Style{Hover: &ns.Style{Background: colors.Hover()}, Active: &ns.Style{Background: colors.Pressed()}}
		},
		Children: []ns.Node{face},
	}
	tip := ui.Tooltip{TextFunc: func() string { return locales.FilmsLocalVideoTooltip(cx, cx.KeyLabel(a.PickVideosShortcut())) }, Trigger: button,
		Style: ns.Style{FlexGrow: 1}}
	k := theme.Of(cx)
	return ns.View{Style: ns.Style{Direction: ns.Row, MinWidth: ns.Pt(104), FlexShrink: 0, Height: ns.Pt(34), Padding: ns.Symmetric(0, 3), CornerRadius: 17, AlignItems: ns.AlignCenter,
		BorderWidth: 0.5}, StyleFunc: func() ns.Style { return pill(k) }, Children: []ns.Node{tip}}
}

// linkField is the pill the link is pasted into, with the link's platform at its end once it reads as one, and the prototype's focus ring around it while it has the focus: a 3-point ring of the accent at half its strength, a point outside the pill.
type linkField struct{ a App }

// Render implements core.Node.
func (l linkField) Render(cx *ns.Scope) ns.Node {
	a := l.a
	colors := ui.ColorsOf(cx)
	field := &ns.Ref{}
	// A request for the focus, as 换个链接's, goes to the field once and is dropped.
	cx.Effect(func() {
		a.Track()
		if focus := a.LinkFocus(); *focus {
			*focus = false
			field.Focus()
		}
	})
	focused := ns.NewSignal(cx, false)
	value := live.Func(a, func() string { return *a.Source() })
	chip := live.Memo(cx, a, func() chipLook {
		label, color, ok := linkChip(locales.For(cx), *a.Source())
		return chipLook{label, color, ok}
	})
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) a LabelFunc twin, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
	input := base.Input{
		Ref: field, TestID: "films-link",
		Label:           locales.FilmsLinkLabel(cx),
		Value:           value,
		PlaceholderFunc: func() string { return locales.FilmsLinkPlaceholder(cx) },
		OnChange:        live.On(a, func(text string) { *a.Source() = text }),
		OnSubmit:        live.On(a, func(string) { a.Start() }),
		OnFocus:         func() { focused.Set(true) },
		OnBlur:          func() { focused.Set(false) },
		Style:           ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0), FontSize: 13, Height: ns.Pt(30)},
		StyleFunc: func() ns.Style {
			return ns.Style{Color: colors.Text(), PlaceholderColor: colors.Faint(), CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection}
		},
	}
	badge := ns.Show{When: func() bool { return chip().ok }, Then: func(*ns.Scope) ns.Node {
		return ns.View{Style: ns.Style{CornerRadius: 10, Padding: ns.Symmetric(3, 8), Margin: ns.Edges{Left: 16}, FlexShrink: 0},
			StyleFunc: func() ns.Style { return ns.Style{Background: chip().color} },
			Children:  []ns.Node{ns.Text{ContentFunc: func() string { return chip().label }, MaxLines: 1, Style: ns.Style{FontSize: 11, FontWeight: ui.Bold, Color: ns.RGB(0xFFFFFF)}}}}
	}}
	return ns.View{
		Style: ns.Style{Direction: ns.Row, Width: ns.Pt(380), FlexShrink: 1, MinWidth: ns.Pt(200), Height: ns.Pt(34), CornerRadius: 17, Padding: ns.Edges{Left: 12, Right: 8}, Gap: 7,
			AlignItems: ns.AlignCenter, BorderWidth: 0.5},
		StyleFunc: func() ns.Style {
			s := pill(theme.Of(cx))
			if focused.Get() {
				// outline: 3px solid rgba(10,102,216,.5); outline-offset: 1px.
				s.OutlineWidth, s.OutlineOffset, s.OutlineColor = 3, 1, colors.Accent().Faded(0.5)
			}
			return s
		},
		Children: []ns.Node{ui.Icon(svg(linkIcon), 15, colors.Muted), input, badge},
	}
}

// chipLook is what the chip at the end of the link field shows: the link's platform in its color, nothing while ok is false.
type chipLook struct {
	label string
	color ns.Color
	ok    bool
}

// linkChip names the platform of a link being typed, in the language of m and in the platform's color: YouTube's red, 哔哩哔哩's blue, and gray for another site. A list or a channel says so after the name; a link to a video of a list, as watch?v=…&list=…, is that video and says nothing more. Several links, between spaces or on lines of their own, are counted, in gray.
func linkChip(m locales.Messages, raw string) (label string, color ns.Color, ok bool) {
	if fields := strings.Fields(raw); len(fields) > 1 {
		return m.FilmsLinkCount(len(fields)), ns.RGB(0x6A6A70), true
	}
	link, ok := core.ParseLink(strings.TrimSpace(raw))
	if !ok || link.Platform == core.PlatformLocal {
		return "", 0, false
	}
	label = PlatformLabel(m, link)
	switch {
	case link.Kind == core.LinkChannel:
		label = m.FilmsLinkChannel(label)
	case link.IsList():
		label = m.FilmsLinkPlaylist(label)
	}
	switch link.Platform {
	case core.PlatformYouTube:
		color = ns.RGB(0xE62117)
	case core.PlatformBilibili:
		color = ns.RGB(0x00A1D6)
	default:
		color = ns.RGB(0x6A6A70)
	}
	return label, color, true
}

// PageTitle is the library page's title for the source it shows, in the language of m.
func PageTitle(m locales.Messages, source string) string {
	switch source {
	case sidebar.SourceYouTube:
		return "YouTube"
	case sidebar.SourceBilibili:
		return m.SourceBilibili()
	case sidebar.SourceWeb:
		return m.SourceWeb()
	case sidebar.SourceLocal:
		return m.SourceLocal()
	}
	return m.SourceAll()
}

// LibraryName is a library folder's name without its .kando suffix.
func LibraryName(dir string) string {
	if dir == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(dir), media.LibraryExt)
}
