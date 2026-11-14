package films

import (
	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// newLibrary is the sheet that creates a library: its name, the folder it goes in, the path it will have, and 取消 or 创建. It shows while the app is making a library; Escape and 取消 stop that. A press outside it keeps it, so what was typed is not lost.
type newLibrary struct{ a App }

// Render implements core.Node.
func (n newLibrary) Render(cx *core.Scope) core.Node {
	a := n.a
	return ui.Dialog{
		TestID:                 "newlib",
		Open:                   live.Func(a, a.MakingLibrary),
		NoCloseOnBackdropPress: true,
		// The sheet draws its own heading, which follows the language; Bare leaves it the room and sets its width.
		Bare:  true,
		Style: core.Style{Width: core.Pt(440)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CancelMaking()
				a.Changed()
			}
		},
		Children: []core.Node{newLibraryBody{a}},
	}
}

// newLibraryBody is what the new-library sheet holds, built as it opens.
type newLibraryBody struct{ a App }

// pathLook is the line under the sheet's card: the path the library will have, or why it cannot be made.
type pathLook struct {
	path, err string
}

// Render implements core.Node.
func (n newLibraryBody) Render(cx *core.Scope) core.Node {
	a := n.a
	colors := ui.ColorsOf(cx)
	// bindErr is why the name typed could not be kept, "" while it could.
	bindErr := core.NewSignal(cx, "")
	look := live.Memo(cx, a, func() pathLook {
		path, err := a.NewLibraryPath()
		if err != nil {
			return pathLook{err: err.Error()}
		}
		return pathLook{path: path}
	})
	head := core.View{Style: core.Style{Direction: core.Row, Gap: 14, AlignItems: core.AlignStart}, Children: []core.Node{
		core.SVGView{SVG: libraryArt, Style: core.Style{Width: core.Pt(52), Height: core.Pt(44), FlexShrink: 0}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 3}, Children: []core.Node{
			heading(1, core.Text{ContentFunc: func() string { return locales.FilmsNewLibraryTitle(cx) }, Style: ui.TypeTitle, StyleFunc: ui.Colored(colors.Text)}),
			core.Text{ContentFunc: func() string { return locales.FilmsNewLibraryNote(cx, desktop.Platform(cx)) }, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)},
		}},
	}}
	rowStyle := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 12)}
	rowTitle := func(text func(*core.Scope) string) core.Node {
		return core.Text{ContentFunc: func() string { return text(cx) }, MaxLines: 1, Style: core.Style{FontSize: 13, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Text)}
	}
	// TODO(nsgo): give core.Input (base.Input, ui.Input) a LabelFunc twin, so the field's accessible name follows the interface language as it changes; it is read once, as the sheet opens
	name := base.Input{
		TestID: "newlib-name",
		Label:  locales.FilmsNewLibraryName(cx),
		Value:  live.Func(a, a.NewLibraryName),
		OnChange: live.On(a, func(text string) {
			err := a.BindName(func(value *string) bool {
				if *value == text {
					return false
				}
				*value = text
				return true
			})
			if err != nil {
				bindErr.Set(err.Error())
				return
			}
			bindErr.Set("")
		}),
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pt(28), FontSize: 13, TextAlign: core.TextAlignEnd},
		StyleFunc: func() core.Style {
			return core.Style{Color: colors.Text(), PlaceholderColor: colors.Faint(), SelectionColor: cx.Palette().Selection, CaretColor: colors.Accent()}
		},
	}
	place := core.View{Style: rowStyle, Children: []core.Node{
		rowTitle(locales.FilmsNewLibraryPlace),
		core.Text{ContentFunc: live.Func(a, func() string { return DisplayPath(locales.For(cx), a.LibraryPlace()) }), MaxLines: 1, EllipsizeMode: core.EllipsizeHead,
			Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), TextAlign: core.TextAlignEnd}, StyleFunc: ui.Colored(colors.Muted)},
		textButton(locales.FilmsChange, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "newlib-place", OnPress: live.Do(a, a.PickLibraryParent)}),
	}}
	card := ui.FormSection{Children: []core.Node{
		core.View{Style: rowStyle, Children: []core.Node{rowTitle(locales.FilmsNewLibraryName), name}},
		place,
	}}
	danger := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Danger)}
	}
	// The line under the card: why the name cannot be kept, why the library cannot be made there, or the path it will have, which can be selected and copied.
	status := core.Show{When: func() bool { return bindErr.Get() != "" }, Then: func(*core.Scope) core.Node { return danger(bindErr.Get) },
		Else: func(*core.Scope) core.Node {
			return core.Show{When: func() bool { return look().err != "" }, Then: func(*core.Scope) core.Node { return danger(func() string { return look().err }) },
				Else: func(*core.Scope) core.Node {
					return core.Text{ContentFunc: func() string { return DisplayPath(locales.For(cx), look().path) }, Selectable: core.SelectableOn,
						Style: core.Style{FontSize: 12, FontFamily: core.FontMonospace}, StyleFunc: ui.Colored(colors.Muted)}
				}}
		}}
	saveErr := live.Func(a, a.SaveErr)
	failed := core.Show{When: func() bool { return saveErr() != "" }, Then: func(*core.Scope) core.Node { return danger(saveErr) }}
	footer := core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd}, Children: []core.Node{
		textButton(locales.FilmsCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "newlib-cancel", OnPress: live.Do(a, a.CancelMaking)}),
		textButton(locales.FilmsCreate, ui.Button{TestID: "newlib-create", Disabled: func() bool { return look().err != "" }, OnPress: live.Do(a, a.CreateLibrary)}),
	}}
	return core.View{Style: core.Style{Padding: core.All(20), Gap: 14}, Children: []core.Node{head, card, status, failed, footer}}
}
