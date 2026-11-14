package films

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// barFade is the top bar's face: the page's white, nearly opaque at the top and fading out at its bottom, so what scrolls under it fades into it, with no blur and no line, as the prototype's toolbar.
var barFade = ns.Gradient{Kind: ns.GradientLinear, Angle: 180, Stops: []ns.ColorStop{
	{Color: ns.RGBA(0xFFFFFF, 0.96), At: ns.Pct(0)},
	{Color: ns.RGBA(0xFFFFFF, 0.86), At: ns.Pct(60)},
	{Color: ns.RGBA(0xFFFFFF, 0), At: ns.Pct(100)},
}}

// The white pills of the link field and 本地视频, which lift them off what scrolls under the bar.
var (
	pillFill     = ns.RGBA(0xFFFFFF, 0.75)
	pillHairline = ns.RGBA(0x000000, 0.12)
	pillShadow   = []ns.BoxShadow{{Offset: ns.Point{Y: 2}, Blur: 8, Color: ns.RGBA(0x000000, 0.06)}}
)

// topBar is the bar along the top of the library: the source being shown and the library's name, the link field, 本地视频 and 开始处理. Enter in the field adds the link as 开始处理 does. It lies over the page's scroll view, which scrolls under it, and drags the window. It takes the prototype's toolbar: 65 points high, its controls 10 from its top and 14 from its end, and its title 24 from its start at 15 points over the library's name; on macOS the sidebar holds the traffic lights, so none of its room is the window controls', and elsewhere it keeps clear of those at its end and is at least as tall as the title bar.
type topBar struct {
	a      App
	source func() string
}

// Render implements core.Node.
func (b topBar) Render(cx *ns.Scope) ns.Node {
	a := b.a
	colors := ui.ColorsOf(cx)
	title := func() string { return PageTitle(locales.For(cx), b.source()) }
	subtitle := live.Func(a, func() string { return LibraryName(a.LibraryDir()) })
	head := ns.View{Style: ns.Style{Gap: 3, FlexShrink: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{
		heading(1, ns.Text{ContentFunc: title, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 15, FontWeight: ui.Bold, LetterSpacing: -0.1}, StyleFunc: ui.Colored(colors.Text)}),
		ns.Show{When: func() bool { return subtitle() != "" }, Then: func(*ns.Scope) ns.Node {
			return ns.Text{ContentFunc: subtitle, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Muted)}
		}},
	}}
	// The prototype's toolbar pills are 34 points high; 开始处理 is the platform's primary button, its accent's shadow under it.
	start := ns.View{Style: ns.Style{CornerRadius: 17}, StyleFunc: func() ns.Style {
		return ns.Style{Shadows: []ns.BoxShadow{{Offset: ns.Point{Y: 2}, Blur: 8, Color: colors.Accent().Faded(0.3)}}}
	}, Children: []ns.Node{textButton(locales.FilmsStart, ui.Button{Size: ui.SizeLarge, TestID: "films-start", OnPress: live.Do(a, a.Start)})}}
	tools := ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 10, FlexShrink: 0, Margin: ns.Edges{Left: 2}},
		Children: []ns.Node{localButton{a}, start}}
	end := ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 10, FlexShrink: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{linkField{a}, tools}}
	return ns.View{
		TestID:    "films-top-bar",
		Role:      ns.RoleToolbar,
		LabelFunc: title,
		Desktop:   ns.DesktopViewProps{WindowDrag: ns.WindowDragOn},
		Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(0), Left: ns.Pt(0), Right: ns.Pt(0), Direction: ns.Row, AlignItems: ns.AlignStart, JustifyContent: ns.JustifySpaceBetween,
			Gap: 16, Gradients: []ns.Gradient{barFade}},
		StyleFunc: func() ns.Style {
			bar := nsdesktop.TitleBar(cx)
			return ns.Style{MinHeight: ns.Pt(max(65, bar.Top)), Padding: ns.Edges{Top: 10, Right: 14 + bar.Right, Bottom: 18, Left: 24}}
		},
		Children: []ns.Node{head, end},
	}
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
			Padding: ns.Edges{Left: 10, Right: 12}, Cursor: ns.CursorDefault},
		StyleFunc: func() ns.Style {
			return ns.Style{Hover: &ns.Style{Background: colors.Hover()}, Active: &ns.Style{Background: colors.Pressed()}}
		},
		Children: []ns.Node{face},
	}
	tip := ui.Tooltip{TextFunc: func() string { return locales.FilmsLocalVideoTooltip(cx, cx.KeyLabel(a.PickVideosShortcut())) }, Trigger: button,
		Style: ns.Style{FlexGrow: 1}}
	return ns.View{Style: ns.Style{Direction: ns.Row, MinWidth: ns.Pt(104), FlexShrink: 0, Height: ns.Pt(34), Padding: ns.Symmetric(0, 3), CornerRadius: 17, AlignItems: ns.AlignCenter,
		Background: pillFill, BorderWidth: 0.5, BorderColor: pillHairline, Shadows: pillShadow}, Children: []ns.Node{tip}}
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
	// TODO(nsgo): give core.Input (base.Input, ui.Input) a LabelFunc twin, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it
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
			AlignItems: ns.AlignCenter, Background: pillFill, BorderWidth: 0.5, BorderColor: pillHairline, Shadows: pillShadow},
		StyleFunc: func() ns.Style {
			if !focused.Get() {
				return ns.Style{}
			}
			// outline: 3px solid rgba(10,102,216,.5); outline-offset: 1px.
			return ns.Style{OutlineWidth: 3, OutlineOffset: 1, OutlineColor: colors.Accent().Faded(0.5)}
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

// linkChip names the platform of a link being typed, in the language of m and in the platform's color: YouTube's red, 哔哩哔哩's blue, and gray for another site. A list or a channel says so after the name.
func linkChip(m locales.Messages, raw string) (label string, color ns.Color, ok bool) {
	link, ok := core.ParseLink(strings.TrimSpace(raw))
	if !ok || link.Platform == core.PlatformLocal {
		return "", 0, false
	}
	label = PlatformLabel(m, link)
	switch link.Kind {
	case core.LinkPlaylist:
		label = m.FilmsLinkPlaylist(label)
	case core.LinkChannel:
		label = m.FilmsLinkChannel(label)
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
