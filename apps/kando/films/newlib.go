package films

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// newLibrary is the sheet that creates a library: its name, the folder it goes in, the path it will have, and 取消 or 创建. It shows while the app is making a library; Escape and 取消 stop that.
func newLibrary(c *ui.Context, t *ui.Theme, a App) {
	open := a.MakingLibrary()
	if !open {
		return
	}
	breeze.Sheet(c, &open, func() {
		ui.Column(c).Width(440).MaxWidth(440).Gap(14).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
				ui.Image(c, libraryArt).Size(52, 44).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					breeze.Text(c, locales.FilmsNewLibraryTitle(), breeze.TextHeading).FontSize(15).Role(ui.RoleHeading).Level(1)
					breeze.Text(c, locales.FilmsNewLibraryNote(breeze.PlatformOf(c).String()), breeze.TextCaption, breeze.TextSecondary).FontSize(12).Wrap()
				})
			})
			var bindErr error
			breeze.Card(c, func() {
				bindErr = a.BindName(func(value *string) bool {
					return breeze.EntryRow(c, locales.FilmsNewLibraryName(), value).Changed()
				})
				breeze.ActionRow(c, locales.FilmsNewLibraryPlace(), DisplayPath(a.LibraryPlace()), func() {
					c.Key("newlib-place")
					if breeze.Button(c, locales.FilmsChange()).Clicked() {
						a.PickLibraryParent()
					}
				})
			})
			path, pathErr := a.NewLibraryPath()
			switch {
			case bindErr != nil:
				ui.Text(c, bindErr.Error()).FontSize(12).TextColor(t.Danger).Wrap()
			case pathErr != nil:
				ui.Text(c, pathErr.Error()).FontSize(12).TextColor(t.Danger).Wrap()
			default:
				breeze.Text(c, DisplayPath(path), breeze.TextSecondary).FontSize(12).Font("Menlo").Wrap().Selectable()
			}
			if err := a.SaveErr(); err != "" {
				ui.Text(c, err).FontSize(12).TextColor(t.Danger).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				c.Key("newlib-cancel")
				if breeze.Button(c, locales.FilmsCancel()).Clicked() {
					a.CancelMaking()
				}
				c.Key("newlib-create")
				if breeze.PrimaryButton(c, locales.FilmsCreate()).Disabled(pathErr != nil).Clicked() {
					a.CreateLibrary()
				}
			})
		})
	})
	if !open {
		a.CancelMaking()
	}
}
