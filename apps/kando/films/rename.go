package films

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// renameDialog is the sheet of 重命名…: the title Kando shows for a film, which starts as the one it shows now, and 取消 or 重命名. The keyboard is in the field as it opens; Enter there renames, Escape and 取消 keep the title as it was. Left empty, the field shows the title the film has without one given in Kando, which 重命名 gives it back. Nothing on disk is renamed.
func renameDialog(c *ui.Context, a App) {
	open, name, original := a.Renaming()
	if !*open {
		return
	}
	breeze.Sheet(c, open, func() {
		ui.Column(c).Width(420).MaxWidth(420).Gap(14).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				breeze.Text(c, locales.FilmsRenameTitle(), breeze.TextHeading).FontSize(15).Role(ui.RoleHeading).Level(1)
				breeze.Text(c, locales.FilmsRenameNote(), breeze.TextCaption, breeze.TextSecondary).FontSize(12).Wrap()
			})
			c.Key("rename-name")
			field := breeze.TextInput(c, name).Label(locales.FilmsRenameField()).Placeholder(original).AutoFocus()
			submitted := field.Submitted()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				c.Key("rename-cancel")
				if breeze.Button(c, locales.FilmsCancel()).Clicked() {
					*open = false
				}
				c.Key("rename-save")
				if breeze.PrimaryButton(c, locales.FilmsRenameSave()).Clicked() {
					submitted = true
				}
			})
			if submitted && *open {
				a.RenameFilm()
			}
		})
	})
}
