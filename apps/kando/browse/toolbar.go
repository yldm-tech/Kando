package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// toolbarHeight is the toolbar's height on macOS, where it lies under the window's hidden title bar beside the sidebar's card and drags the window, as a unified toolbar does.
const toolbarHeight core.Points = 52

var (
	// toolbarLine is the hairline under the toolbar.
	toolbarLine = core.RGBA(0x000000, 0.1)
	// The address field's pill: white over the toolbar, a hairline around it and a soft shadow under it.
	addressFill   = core.RGBA(0xFFFFFF, 0.75)
	addressBorder = core.RGBA(0x000000, 0.12)
	addressShadow = core.RGBA(0x000000, 0.06)
)

// toolbar is the row along the top of the page: back, forward, reload (stop while the page loads) and home, the address field growing between them and 下载视频 at the end. On macOS it is the window's title bar beside the sidebar and drags the window; elsewhere the content's drag band lies above it.
type toolbar struct{ a App }

// Render implements core.Node.
func (t toolbar) Render(cx *core.Scope) core.Node {
	a := t.a
	colors := ui.ColorsOf(cx)
	state := live.Memo(cx, a, a.BrowseState)
	bar := core.View{
		TestID: "browse-toolbar",
		Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 9, Right: 14, Bottom: 9, Left: 14},
			Borders: core.Edges{Bottom: 0.5}, BorderColor: toolbarLine},
		StyleFunc: ui.Filled(colors.Background),
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
				Then: func(*core.Scope) core.Node { return nav("browse-stop", iconStop, locales.BrowseStop, always, a.BrowseStop) },
				Else: func(*core.Scope) core.Node {
					return nav("browse-reload", iconReload, locales.BrowseReload, func(s State) bool { return s.Page }, a.BrowseReload)
				},
			},
			nav("browse-home", iconHome, locales.BrowseHome, func(s State) bool { return s.Page }, a.BrowseHome),
		},
	}
	// The library's toolbar pills are as high as the large button; the platform draws their faces.
	download := ui.Tooltip{
		TextFunc: func() string { return locales.BrowseDownloadTooltip(cx) },
		Style:    core.Style{FlexShrink: 0},
		// The button's words are set as it is built, so it is built again for another language.
		Trigger: core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Button{Text: locales.BrowseDownload(cx), Size: ui.SizeLarge, TestID: "browse-download",
				Disabled: func() bool { return !state().Page }, OnPress: live.Do(a, a.BrowseDownload)}
		}),
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
	// TODO(nsgo): give core.Input (base.Input, ui.Input) LabelFunc and AccessibilityDescriptionFunc twins, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it
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
			Background: addressFill, BorderWidth: 0.5, BorderColor: addressBorder, Shadows: []core.BoxShadow{{Offset: core.Point{Y: 2}, Blur: 8, Color: addressShadow}}},
		StyleFunc: func() core.Style {
			if !focused.Get() {
				return core.Style{}
			}
			return core.Style{OutlineWidth: 3, OutlineOffset: 1, OutlineColor: ui.Alpha(colors.Accent(), 0.5)}
		},
		Children: []core.Node{
			core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{ui.Icon(svg(iconGlobe), 15, colors.Muted)}},
			input,
		},
	}
}
