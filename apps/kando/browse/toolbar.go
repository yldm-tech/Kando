package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// toolbarHeight is the toolbar's height on macOS, where it lies under the window's hidden title bar beside the sidebar's card and drags the window, as a unified toolbar does.
const toolbarHeight core.Points = 52

// addressShadow is the soft shadow under the address field's pill. The pill is the theme's card color, translucent, over the toolbar with the shade's hairline around it, and the toolbar's hairline under it is the shade too (theme.Of).
var addressShadow = core.RGBA(0x000000, 0.06)

// toolbar is the row along the top of the page: back, forward, reload (stop while the page loads) and home, the address field growing between them and 下载视频 at the end. On macOS it is the window's title bar beside the sidebar and drags the window; elsewhere the content's drag band lies above it.
type toolbar struct{ a App }

// Render implements core.Node.
func (t toolbar) Render(cx *core.Scope) core.Node {
	a := t.a
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	state := live.Memo(cx, a, a.BrowseState)
	bar := core.View{
		TestID: "browse-toolbar",
		Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 9, Right: 14, Bottom: 9, Left: 14},
			Borders: core.Edges{Bottom: 0.5}},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Background(), BorderColor: k.Shade(0.1)} },
	}
	if core.Untracked(cx, func() bool { return cx.Env().OS() == core.OSMacOS }) {
		bar.Style.MinHeight = core.Pt(toolbarHeight)
		bar.Desktop = core.DesktopViewProps{WindowDrag: core.WindowDragOn}
	}
	nav := func(test string, icon *core.SVG, label func(*core.Scope) string, enabled func(State) bool, run func()) core.Node {
		return ui.IconButton{Icon: svg(icon), LabelFunc: func() string { return label(cx) }, TestID: test,
			Disabled: func() bool { return !enabled(state()) }, OnPress: live.Do(a, run)}
	}
	always := func(State) bool { return true }
	buttons := core.View{
		Style: core.Style{Direction: core.Row, FlexShrink: 0, Gap: 2, AlignItems: core.AlignCenter},
		Children: []core.Node{
			nav("browse-back", iconBack, locales.BrowseBack, func(s State) bool { return s.CanGoBack }, a.BrowseBack),
			nav("browse-forward", iconForward, locales.BrowseForward, func(s State) bool { return s.CanGoForward }, a.BrowseForward),
			core.Show{
				When: func() bool { return state().Loading },
				Then: func(*core.Scope) core.Node {
					return nav("browse-stop", iconStop, locales.BrowseStop, always, a.BrowseStop)
				},
				Else: func(*core.Scope) core.Node {
					return nav("browse-reload", iconReload, locales.BrowseReload, func(s State) bool { return s.Page }, a.BrowseReload)
				},
			},
			nav("browse-home", iconHome, locales.BrowseHome, func(s State) bool { return s.Page }, a.BrowseHome),
		},
	}
	download := ui.Tooltip{
		TextFunc: func() string { return downloadTooltip(cx, state()) },
		Style:    core.Style{FlexShrink: 0},
		Trigger:  downloadPill{a: a, state: state},
	}
	bar.Children = []core.Node{buttons, address{a}, download}
	return bar
}

// address is the pill the address is typed into, with the globe at its start and the accent's ring around it while it has the keyboard. Enter opens what it holds, and ⌘L (BrowseAddressFocus) puts the keyboard in it.
type address struct{ a App }

// Render implements core.Node.
func (f address) Render(cx *core.Scope) core.Node {
	a := f.a
	colors := ui.ColorsOf(cx)
	field := &core.Ref{}
	focused := core.NewSignal(cx, false)
	cx.Effect(func() {
		a.Track()
		if focus := a.BrowseAddressFocus(); *focus {
			*focus = false
			field.Focus()
		}
	})
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) LabelFunc and AccessibilityDescriptionFunc twins, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
	input := base.Input{
		Ref: field, TestID: "browse-address",
		Label:           locales.BrowseAddressLabel(cx),
		KeyboardType:    core.KeyboardURL,
		Value:           live.Func(a, func() string { return *a.BrowseAddress() }),
		PlaceholderFunc: func() string { return locales.BrowseAddressPlaceholder(cx) },
		OnChange:        live.On(a, func(text string) { *a.BrowseAddress() = text }),
		OnSubmit:        live.On(a, func(string) { a.BrowseGo() }),
		OnFocus: live.Do(a, func() {
			focused.Set(true)
			a.BrowseAddressFocused(true)
		}),
		OnBlur: live.Do(a, func() {
			focused.Set(false)
			a.BrowseAddressFocused(false)
		}),
		Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0), FontSize: 13},
		StyleFunc: func() core.Style {
			return core.Style{Color: colors.Text(), PlaceholderColor: colors.Faint(), CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection}
		},
	}
	return core.View{
		Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(160), Height: core.Pt(34), CornerRadius: 17,
			Padding: core.Edges{Left: 12, Right: 12}, Gap: 8, AlignItems: core.AlignCenter,
			BorderWidth: 0.5, Shadows: []core.BoxShadow{{Offset: core.Point{Y: 2}, Blur: 8, Color: addressShadow}}},
		StyleFunc: func() core.Style {
			k := theme.Of(cx)
			s := core.Style{Background: k.Card().Faded(0.75), BorderColor: k.Shade(0.12)}
			if focused.Get() {
				s.OutlineWidth, s.OutlineOffset, s.OutlineColor = 3, 1, ui.Alpha(colors.Accent(), 0.5)
			}
			return s
		},
		Children: []core.Node{
			core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{ui.Icon(svg(iconGlobe), 15, colors.Muted)}},
			input,
		},
	}
}

// downloadTooltip is what 下载视频's tooltip says in state: why it is off on a page that lists videos, what it does on a playlist's or a channel's page, and else what pressing it does to the page's video, named when its title is known.
func downloadTooltip(cx *core.Scope, state State) string {
	switch {
	case state.NotVideo:
		return locales.BrowseDownloadNotVideo(cx)
	case state.List:
		return locales.BrowseDownloadListTooltip(cx)
	case state.Have == HaveDone && state.Title != "":
		return locales.BrowseInLibraryTitled(cx, state.Title)
	case state.Have == HaveDone:
		return locales.BrowseInLibraryTooltip(cx)
	case state.Have == HaveQueued && state.Title != "":
		return locales.BrowseQueuedTitled(cx, state.Title)
	case state.Have == HaveQueued:
		return locales.BrowseQueuedTooltip(cx)
	case state.Title != "":
		return locales.BrowseDownloadTitled(cx, state.Title)
	}
	return locales.BrowseDownloadTooltip(cx)
}

// downloadLabel is 下载视频's words in state: Download Videos on a playlist's or a channel's page, and 已在资料库中 or 已在队列中 where the library has the page's video already, which pressing it then shows.
func downloadLabel(cx *core.Scope, state State) string {
	if state.List {
		return locales.BrowseDownloadList(cx)
	}
	switch state.Have {
	case HaveDone:
		return locales.BrowseInLibrary(cx)
	case HaveQueued:
		return locales.BrowseQueued(cx)
	}
	return locales.BrowseDownload(cx)
}

// downloadPill is 下载视频, off before a page opens and on a page that lists videos without being one (State.NotVideo), a pill as high as the library's toolbar pills (34 points) filled with the accent, as MyGo's primary button was there. Where the library has the page's video already it says so and shows it (downloadLabel). ui.Button keeps to its sizes, so the pill is drawn on base.Button with ui.Button's primary colors.
type downloadPill struct {
	a     App
	state func() State
}

// Render implements core.Node.
func (d downloadPill) Render(cx *core.Scope) core.Node {
	a, state := d.a, d.state
	label := func() string { return downloadLabel(cx, state()) }
	return base.Button{
		TestID:    "browse-download",
		LabelFunc: label,
		Disabled:  func() bool { s := state(); return !s.Page || s.NotVideo },
		OnPress:   live.On(a, func(core.PressEvent) { a.BrowseDownload() }),
		Style: core.Style{Height: core.Pt(34), CornerRadius: 17, BorderCurve: core.BorderCurveContinuous, Padding: core.Symmetric(0, 18),
			Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
		StyleFunc: func() core.Style {
			p := cx.Palette()
			return core.Style{Background: p.Accent, Hover: &core.Style{Background: p.AccentHover}, Active: &core.Style{Background: p.AccentPressed}, FocusVisible: ui.FocusRing(cx, 2)}
		},
		Children: []core.Node{core.Text{
			ContentFunc: label,
			MaxLines:    1,
			Style:       core.Style{FontSize: 14, LineHeight: 20},
			StyleFunc:   func() core.Style { return core.Style{Color: cx.Palette().AccentText} },
		}},
	}
}
