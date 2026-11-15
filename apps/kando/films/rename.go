package films

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// renameDialog is the sheet of 重命名…: the title Kando shows for a film, which starts as the one it shows now, and 取消 or 重命名. The keyboard is in the field as it opens; Enter there renames, Escape and 取消 keep the title as it was. Left empty, the field shows the title the film has without one given in Kando, which 重命名 gives it back. Nothing on disk is renamed. A press outside it keeps it, so what was typed is not lost.
type renameDialog struct{ a App }

// Render implements core.Node.
func (r renameDialog) Render(cx *core.Scope) core.Node {
	a := r.a
	return ui.Dialog{
		TestID: "rename",
		Open: live.Func(a, func() bool {
			open, _, _ := a.Renaming()
			return *open
		}),
		NoCloseOnBackdropPress: true,
		Bare:                   true,
		Style:                  core.Style{Width: core.Pt(420)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CloseRename()
				a.Changed()
			}
		},
		Children: []core.Node{renameBody{a}},
	}
}

// renameBody is what the rename sheet holds, built as it opens.
type renameBody struct{ a App }

// Render implements core.Node.
func (r renameBody) Render(cx *core.Scope) core.Node {
	a := r.a
	colors := ui.ColorsOf(cx)
	name := live.Func(a, func() string {
		_, name, _ := a.Renaming()
		return *name
	})
	original := live.Func(a, func() string {
		_, _, original := a.Renaming()
		return original
	})
	// Only an open dialog renames: Enter and 重命名 after it closed do nothing.
	rename := live.Do(a, func() {
		if open, _, _ := a.Renaming(); *open {
			a.RenameFilm()
		}
	})
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) a LabelFunc twin, so the field's accessible name follows the interface language as it changes; it is read once, as the sheet opens https://github.com/nsgo-dev/nsgo/issues/145
	field := ui.Input{
		TestID:          "rename-name",
		Label:           locales.FilmsRenameField(cx),
		AutoFocus:       true,
		Value:           name,
		PlaceholderFunc: original,
		OnChange: live.On(a, func(text string) {
			_, name, _ := a.Renaming()
			*name = text
		}),
		OnSubmit: func(string) { rename() },
	}
	head := core.View{Style: core.Style{Gap: 3}, Children: []core.Node{
		heading(1, core.Text{ContentFunc: func() string { return locales.FilmsRenameTitle(cx) }, Style: ui.TypeTitle, StyleFunc: ui.Colored(colors.Text)}),
		core.Text{ContentFunc: func() string { return locales.FilmsRenameNote(cx) }, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	footer := core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd}, Children: []core.Node{
		textButton(locales.FilmsCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "rename-cancel", OnPress: live.Do(a, a.CloseRename)}),
		textButton(locales.FilmsRenameSave, ui.Button{TestID: "rename-save", OnPress: rename}),
	}}
	return core.View{Style: core.Style{Padding: core.All(20), Gap: 14}, Children: []core.Node{head, field, footer}}
}
