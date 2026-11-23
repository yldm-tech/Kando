package category

import (
	"errors"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Overlay is what the categories draw over the window: the dialog that names a category and the confirmation that deletes one.
func Overlay(a App) core.Node {
	return core.Group{nameDialog{a}, deleteDialog{a}}
}

// nameDialog is the sheet of 新建分类… and 重命名…: the category's name, empty for a new one, and 取消 or 新建 (重命名). The keyboard is in the field as it opens; Enter there submits, Escape and 取消 change nothing. A name that is empty or that another category has is refused, and a line under the field says why, the sheet staying open. A press outside it keeps it, so what was typed is not lost.
type nameDialog struct{ a App }

// Render implements core.Node.
func (d nameDialog) Render(cx *core.Scope) core.Node {
	a := d.a
	return ui.Dialog{
		TestID:                 "category-name",
		Open:                   live.Func(a, func() bool { return a.CategoryNaming().Open }),
		NoCloseOnBackdropPress: true,
		Bare:                   true,
		Style:                  core.Style{Width: core.Pt(380)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CloseCategoryName()
				a.Changed()
			}
		},
		Children: []core.Node{nameBody{a}},
	}
}

// nameBody is what the naming sheet holds, built as it opens.
type nameBody struct{ a App }

// Render implements core.Node.
func (n nameBody) Render(cx *core.Scope) core.Node {
	a := n.a
	colors := ui.ColorsOf(cx)
	renaming := a.CategoryNaming().ID != ""
	// Only an open dialog submits: Enter and its button after it closed do nothing.
	submit := live.Do(a, func() {
		if a.CategoryNaming().Open {
			a.SubmitCategoryName()
		}
	})
	problem := live.Func(a, func() string {
		naming := a.CategoryNaming()
		switch {
		case errors.Is(naming.Err, ErrNameEmpty):
			return locales.CategoryNameEmpty(cx)
		case errors.Is(naming.Err, ErrNameTaken):
			return locales.CategoryNameTaken(cx, naming.Name)
		}
		return ""
	})
	field := ui.Input{
		TestID:          "category-name-field",
		LabelFunc:       func() string { return locales.CategoryNameField(cx) },
		AutoFocus:       true,
		Value:           live.Func(a, func() string { return a.CategoryNaming().Name }),
		PlaceholderFunc: func() string { return locales.CategoryNamePlaceholder(cx) },
		OnChange: live.On(a, func(text string) {
			naming := a.CategoryNaming()
			naming.Name, naming.Err = text, nil
		}),
		OnSubmit: func(string) { submit() },
	}
	title, button := locales.CategoryNewTitle, locales.CategoryCreate
	if renaming {
		title, button = locales.CategoryRenameTitle, locales.CategoryRenameSave
	}
	head := core.View{Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }, Children: []core.Node{
		core.Text{ContentFunc: func() string { return title(cx) }, Style: ui.TypeTitle, StyleFunc: ui.Colored(colors.Text)},
	}}
	note := core.Show{When: func() bool { return problem() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "category-name-problem", ContentFunc: problem, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	footer := core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd}, Children: []core.Node{
		textButton(locales.CategoryCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "category-name-cancel", OnPress: live.Do(a, a.CloseCategoryName)}),
		textButton(button, ui.Button{TestID: "category-name-save", OnPress: submit}),
	}}
	return core.View{Style: core.Style{Padding: core.All(20), Gap: 12}, Children: []core.Node{head, core.View{Style: core.Style{Gap: 6}, Children: []core.Node{field, note}}, footer}}
}

// deleteDialog confirms 删除分类… before the category is deleted. Its films stay in the library and in their other categories. 取消 is the default answer, so Enter keeps the category, as do Escape and closing the dialog.
type deleteDialog struct{ a App }

// Render implements core.Node.
func (d deleteDialog) Render(cx *core.Scope) core.Node {
	a := d.a
	colors := ui.ColorsOf(cx)
	open := live.Func(a, func() bool {
		_, ok := a.CategoryDeleting()
		return ok
	})
	name := live.Func(a, func() string {
		c, _ := a.CategoryDeleting()
		return c.Name
	})
	cancel := live.Do(a, a.CloseDeleteCategory)
	remove := live.Do(a, func() {
		if _, ok := a.CategoryDeleting(); ok {
			a.DeleteCategory()
		}
	})
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		// Enter answers 取消, the default answer, wherever the keyboard focus is in the dialog; the binding goes when the dialog does.
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, OverModals: true}, cancel)
		return ui.Dialog{
			TestID:                 "category-delete",
			Role:                   core.RoleAlertDialog,
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc:              func() string { return locales.CategoryDeleteConfirm(cx, name()) },
			DescriptionFunc:        func() string { return locales.CategoryDeleteMessage(cx) },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Footer: []core.Node{
				textButton(locales.CategoryCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "category-delete-cancel", OnPress: cancel}),
				textButton(locales.CategoryDeleteButton, ui.Button{Color: colors.Danger, TestID: "category-delete-confirm", OnPress: remove}),
			},
		}
	}}
}

// textButton is b saying text in the language the app shows: text is read in the button's binding, so it follows the language without building the button again, which would take the keyboard focus off it.
func textButton(text func(cx *core.Scope) string, b ui.Button) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		b.TextFunc = func() string { return text(cx) }
		return b
	})
}
