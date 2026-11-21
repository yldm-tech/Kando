// Package chrome is the main window's title bar where Kando draws one of its own: Windows' 34-point title bar with the app's mark, its name, the button that opens the command palette and the caption buttons NSGo draws, and Linux's 46-point header bar in the manner of GNOME's, with the palette's button, the name in the middle and round minimize, maximize and close buttons of its own, as GTK draws none in a window whose title bar the app hides. On macOS the system's menu bar carries the commands and the traffic lights sit in the sidebar's card, so there is no bar. Neither Windows nor Linux shows an in-window menu bar: with the title bar hidden NSGo draws none (desktop.SetTitleBar), and the command palette reaches every menu command instead.
package chrome

import (
	"kando/apps/kando/hotkey"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// Kind is the title bar a system's main window gets.
type Kind int

const (
	// None is macOS's: the app draws no bar, and the traffic lights sit where the app puts them.
	None Kind = iota
	// Windows is the 34-point title bar, NSGo's caption buttons at its right end.
	Windows
	// Linux is the 46-point header bar with the app's own round window buttons.
	Linux
)

// KindFor is the bar of the system os: Windows' and Linux's own, none elsewhere.
func KindFor(os core.OS) Kind {
	switch os {
	case core.OSWindows:
		return Windows
	case core.OSLinux:
		return Linux
	}
	return None
}

// KindOf is the bar of the system cx runs on.
func KindOf(cx *core.Scope) Kind { return KindFor(cx.Env().OS()) }

// Height is how tall k's bar is, the height the window's hidden title bar is given (desktop.SetTitleBar): the design's 34 points on Windows and 46 on Linux, and 0, the platform's, without a bar.
func Height(k Kind) core.Points {
	switch k {
	case Windows:
		return 34
	case Linux:
		return 46
	}
	return 0
}

// SetUp hides the system's title bar of the window cx is in, for the app's own: Windows' bar keeps NSGo's caption buttons, which Snap Layouts and the keyboard know, and centers them in its height; Linux's draws its own; macOS's traffic lights go where lights says.
func SetUp(cx *core.Scope, lights nsdesktop.WindowControls) {
	k := KindOf(cx)
	nsdesktop.SetTitleBar(cx, nsdesktop.TitleBarHidden, Height(k))
	switch k {
	case None:
		nsdesktop.SetWindowControls(cx, lights)
	case Linux:
		nsdesktop.SetWindowControls(cx, nsdesktop.WindowControls{Hidden: true})
	}
}

// Shows reports whether the bar shows where cx is: on Windows and Linux, out of full screen, which has no title bar.
func Shows(cx *core.Scope) bool { return KindOf(cx) != None && !nsdesktop.FullScreen(cx) }

// Covered is what the window's hidden title bar covers of the content under the app's bar, as desktop.TitleBar reports it for a window without one: nothing on Windows and Linux, where the bar takes the title bar's room and its window controls above the content, and the title bar's room and the traffic lights' widths on macOS. The parts at the top of the window keep clear of these rather than of desktop.TitleBar.
func Covered(cx *core.Scope) core.Edges {
	if KindOf(cx) != None {
		return core.Edges{}
	}
	return nsdesktop.TitleBar(cx)
}

// App is what the bar reads and does: the command palette's key and opening it, and the app's icon, the mark at the start of Windows' bar.
type App interface {
	live.State
	PaletteKey() string
	TogglePalette()
	Logo() core.ImageSource
}

// Bar is the main window's title bar: Windows' or Linux's while it shows (Shows), nothing on macOS. Title is the window's title, which Windows' bar shows after the mark and Linux's in its middle. The bar moves the window, and the window's controls stay clear of what it holds.
type Bar struct {
	App   App
	Title func() string
}

// Render implements core.Node.
func (b Bar) Render(cx *core.Scope) core.Node {
	switch KindOf(cx) {
	case Windows:
		return core.Show{When: func() bool { return Shows(cx) }, Then: b.windows}
	case Linux:
		return core.Show{When: func() bool { return Shows(cx) }, Then: b.linux}
	}
	return nil
}

// windows is the design's Windows bar: 34 points high, 6 in from the left, the app's 16-point icon, its name in 12 points and the palette's button, and past them the room the caption buttons take (desktop.TitleBar's Right). It shows the window's material, Mica, where the window shows one, as Windows 11's title bars do, and the sidebar's gutter where it does not.
func (b Bar) windows(cx *core.Scope) core.Node {
	k := theme.Of(cx)
	mark := core.Image{Source: b.App.Logo(), Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0, Margin: core.Edges{Left: 4, Right: 6}}}
	title := core.Text{TestID: "chrome-title", ContentFunc: b.Title, MaxLines: 1,
		Style: core.Style{FontSize: 12, FlexShrink: 0, Margin: core.Edges{Right: 4}}, StyleFunc: ui.Colored(k.Strong)}
	return core.View{
		TestID:  "chrome-bar",
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		Style:   core.Style{Direction: core.Row, Height: core.Pt(Height(Windows)), FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 2},
		StyleFunc: func() core.Style {
			s := core.Style{Padding: core.Edges{Left: 6, Right: nsdesktop.TitleBar(cx).Right}}
			if !nsdesktop.ShowsMaterial(cx) {
				s.Background = k.Gutter()
			}
			return s
		},
		Children: []core.Node{mark, title, search{a: b.App, height: 26, radius: 4, margin: 6}},
	}
}

// linux is the design's GNOME header bar: 46 points high, the palette's button at the start, the window's title in bold in its middle, and the round minimize, maximize and close buttons at the end, 10 apart, with a hairline under it.
func (b Bar) linux(cx *core.Scope) core.Node {
	k := theme.Of(cx)
	colors := ui.ColorsOf(cx)
	// The title is centered in the whole bar, under the buttons, whatever they take at either end.
	title := core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children: []core.Node{core.Text{TestID: "chrome-title", ContentFunc: b.Title, MaxLines: 1, Style: core.Style{FontSize: 13, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)}}}
	spacer := core.View{Style: core.Style{FlexGrow: 1}}
	maximized := func() bool { return nsdesktop.Maximized(cx) }
	buttons := core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter, Padding: core.Edges{Right: 4}, FlexShrink: 0},
		Children: []core.Node{
			Round{TestID: "chrome-minimize", Icon: svg(iconMinimize), Label: func() string { return locales.ChromeMinimize(cx) }, OnPress: func() { nsdesktop.MinimizeWindow(cx) }},
			Round{TestID: "chrome-maximize",
				Icon: func() *core.SVG {
					if maximized() {
						return iconRestore
					}
					return iconMaximize
				},
				Label: func() string {
					if maximized() {
						return locales.ChromeRestore(cx)
					}
					return locales.ChromeMaximize(cx)
				},
				OnPress: func() { nsdesktop.ToggleMaximize(cx) }},
			Round{TestID: "chrome-close", Icon: svg(iconClose), Label: func() string { return locales.ChromeClose(cx) }, OnPress: func() { nsdesktop.CloseWindow(cx) }},
		}}
	return core.View{
		TestID:  "chrome-bar",
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		Style:   core.Style{Direction: core.Row, Height: core.Pt(Height(Linux)), FlexShrink: 0, AlignItems: core.AlignCenter, Padding: core.Symmetric(0, 8), Borders: core.Edges{Bottom: 1}},
		StyleFunc: func() core.Style {
			return core.Style{Background: k.Gutter(), BorderColors: core.EdgeColors{Bottom: k.Shade(0.1)}}
		},
		Children: []core.Node{title, search{a: b.App, height: 30, radius: 6}, spacer, buttons},
	}
}

// search is the bar's button that opens the command palette: a magnifier, 搜索命令和台词 and the palette's key as the platform writes it, muted, on a light wash, height points high with corners of radius, margin points after what is before it.
type search struct {
	a              App
	height, radius core.Points
	margin         core.Points
}

// Render implements core.Node.
func (s search) Render(cx *core.Scope) core.Node {
	a := s.a
	k := theme.Of(cx)
	label := func() string { return locales.ChromeSearch(cx) }
	key := func() string { return hotkey.Label(cx, a.PaletteKey()) }
	return base.Button{
		TestID:    "chrome-search",
		LabelFunc: label,
		OnPress:   live.On(a, func(core.PressEvent) { a.TogglePalette() }),
		Style: core.Style{Direction: core.Row, Height: core.Pt(s.height), Padding: core.Symmetric(0, 10), Gap: 8, AlignItems: core.AlignCenter, FlexShrink: 0,
			CornerRadius: s.radius, Margin: core.Edges{Left: s.margin}, Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() core.Style {
			return core.Style{Background: k.Shade(0.06), Hover: &core.Style{Background: k.Shade(0.1)}, Active: &core.Style{Background: k.Shade(0.14)}}
		},
		Children: []core.Node{
			ui.Icon(svg(iconSearch), 12, k.Strong),
			core.Text{ContentFunc: label, MaxLines: 1, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(k.Strong)},
			core.Text{TestID: "chrome-search-key", ContentFunc: key, MaxLines: 1, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(k.Placeholder)},
		},
	}
}

// Round is one of the round window buttons of GNOME's header bars: 24 points across, a gray wash under a 10-point glyph, a darker one under the pointer. Linux's header bar has three; a window of its own that GTK draws no controls for, as the settings window, takes the close one.
type Round struct {
	TestID  string
	Icon    func() *core.SVG
	Label   func() string
	OnPress func()
}

// Render implements core.Node.
func (r Round) Render(cx *core.Scope) core.Node {
	k := theme.Of(cx)
	return base.Button{
		TestID:    r.TestID,
		LabelFunc: r.Label,
		OnPress:   func(core.PressEvent) { r.OnPress() },
		Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), CornerRadius: 12, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() core.Style {
			return core.Style{Background: k.Shade(0.08), Hover: &core.Style{Background: k.Shade(0.14)}, Active: &core.Style{Background: k.Shade(0.2)}}
		},
		Children: []core.Node{ui.IconOf(r.Icon, 10, ui.ColorsOf(cx).Text)},
	}
}

// Close is the round close button of a window of the app's own on Linux, which GTK draws no controls for while its title bar is hidden: it closes the window cx is in as its close button would (desktop.CloseWindow). Elsewhere the system's controls are there, and it is nil.
func Close(cx *core.Scope) core.Node {
	if KindOf(cx) != Linux {
		return nil
	}
	return Round{TestID: "chrome-window-close", Icon: svg(iconClose), Label: func() string { return locales.ChromeClose(cx) }, OnPress: func() { nsdesktop.CloseWindow(cx) }}
}
