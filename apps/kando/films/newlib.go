package films

import (
	"errors"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/media"

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
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) a LabelFunc twin, so the field's accessible name follows the interface language as it changes; it is read once, as the sheet opens https://github.com/nsgo-dev/nsgo/issues/145
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

// LibraryFolder is the folder name of a library called name, its .kando bundle, or why there can be none on goos, in the language of m. On Windows, where the library's folder is made, a name can hold none of the characters Windows refuses in a file's name, < > : " | ? * and the control characters, nor be one of the names it keeps for devices, as CON or COM1, with any extension.
func LibraryFolder(m locales.Messages, goos, name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, media.LibraryExt)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New(m.LibraryNameEmpty())
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", errors.New(m.LibraryNameHasPath())
	}
	if goos == "windows" && !windowsName(name) {
		return "", errors.New(m.LibraryNameNotAllowed())
	}
	return name + media.LibraryExt, nil
}

// windowsName reports whether Windows takes name, followed by an extension, as a file's name: none of < > : " | ? * or a control character, and before its first dot, its trailing spaces aside, none of the device names, whatever their case.
func windowsName(name string) bool {
	if strings.ContainsAny(name, `<>:"|?*`) || strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 }) {
		return false
	}
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		return false
	}
	// COM¹ to COM³ and LPT¹ to LPT³, with a superscript digit, are kept too.
	for _, device := range []string{"COM", "LPT"} {
		if rest, ok := strings.CutPrefix(stem, device); ok && (rest == "¹" || rest == "²" || rest == "³") {
			return false
		}
	}
	return true
}
