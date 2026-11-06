package films

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Stand-ins for breeze widgets whose pull requests are not merged yet. Each has the signature of the widget it stands in for, so swapping it is a rename at the call sites and the removal of the helper.

// flatIconButton stands in for breeze.FlatIconButton (mygo-breeze-ui#15): until it lands the button keeps its border.
func flatIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	return breeze.IconButton(c, icon, label)
}

// iconLabelButton stands in for breeze.IconLabelButton (mygo-breeze-ui#14): until it lands the button shows its label without the icon.
func iconLabelButton(c *ui.Context, _ *ui.SVG, label string) ui.Element {
	return breeze.Button(c, label)
}

// headerBar stands in for breeze.HeaderBar placed in the content pane (mygo-breeze-ui#13), which today keeps clear of the window controls as if it spanned the window: the title and subtitle at the start, end's controls at the end, and the room between them a handle that moves the window.
func headerBar(c *ui.Context, title, subtitle string, end func()) ui.Element {
	bar := ui.Row(c).Shrink(0).Gap(10).AlignItems(ui.Center)
	bar.Children(func() {
		ui.Column(c).Shrink(0).MinWidth(110).Gap(1).DragWindow().Children(func() {
			breeze.Text(c, title, breeze.TextHeading).FontSize(15).SingleLine()
			if subtitle != "" {
				breeze.Text(c, subtitle, breeze.TextCaption, breeze.TextSecondary).FontSize(11).SingleLine()
			}
		})
		ui.Row(c).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).DragWindow()
		end()
	})
	return bar
}

// infoBarActions stands in for breeze.InfoBarActions (mygo-breeze-ui#19): until it lands the actions sit after the bar in the same row rather than inside it.
func infoBarActions(c *ui.Context, kind, title, message string, open *bool, actions func()) ui.Element {
	if open != nil && !*open {
		return ui.Element{}
	}
	row := ui.Row(c).AlignSelf(ui.Stretch).Gap(8).AlignItems(ui.Center)
	row.Children(func() {
		breeze.InfoBar(c, kind, title, message, open).Grow(1).MinWidth(0)
		if actions != nil {
			actions()
		}
	})
	return row
}

// sheet stands in for breeze.Sheet (mygo-breeze-ui#18), a dialog that a click outside leaves open and Escape closes. breeze.Modal closes on either, so it gets an open of its own every frame, which its backdrop may clear, and only Escape on the panel closes the sheet.
func sheet(c *ui.Context, open *bool, fn func()) ui.Element {
	if !*open {
		return ui.Element{}
	}
	shown := true
	panel := breeze.Modal(c, &shown, fn)
	if panel.OverlayShortcut(0, ui.KeyEscape) {
		*open = false
	}
	return panel
}

// destructiveAlertDialog stands in for breeze.DestructiveAlertDialog (mygo-breeze-ui#18), whose destructive button is never the default. breeze.AlertDialog makes the last button the default, so until it lands the destructive button, buttons[destructive], must not be the last one, and is drawn as a plain button.
func destructiveAlertDialog(c *ui.Context, open *bool, title, message string, _ int, buttons ...string) int {
	return breeze.AlertDialog(c, open, title, message, buttons...)
}
