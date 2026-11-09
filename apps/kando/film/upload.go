package film

import (
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// The upload capsule's dots (prototype template 583-595).
var uploadDots = map[string]ui.Color{
	UploadBusy:    ui.Hex("#64D2FF"),
	UploadDone:    ui.Hex("#30D158"),
	UploadFailed:  ui.Hex("#FFD60A"),
	UploadBlocked: ui.Hex("#FFD60A"),
}

// uploadCapsule says how the film's upload goes, under the title bar: while it runs with a bar of its progress, then whether it worked, or, when the upload button found nowhere to upload, why and a button to 账号 to fix it. It shows while an upload of this film runs, and after the upload button was clicked until closed.
func (s screen) uploadCapsule(c *ui.Context, item films.Film, panelSpace float32) {
	view := s.UploadView(item)
	note := s.UploadNote()
	if !view.Show || view.Detail == "" || (!*note && view.Status != UploadBusy) {
		return
	}
	// It is centered in the room the side panel leaves, as the big play button is: over the whole width, a wide capsule in a narrow window reached under the panel, which hid its close button.
	right := panelSpace
	ui.Row(c).Absolute().Left(0).Right(right).Top(64).Justify(ui.Center).PassThrough().Children(func() {
		capsule := ui.Row(c).MaxWidthPercent(70).Wrap().Gap(10).Padding(8, 8, 8, 16).Radius(18).AlignItems(ui.Center).TextColor(onGlass).Role(ui.RoleStatus)
		glassFace(capsule, ui.RGBA(30, 30, 34, 0.72), 24, 0.2, 18).Children(func() {
			dot, ok := uploadDots[view.Status]
			if !ok {
				dot = uploadDots[UploadDone]
			}
			ui.Box(c).Size(8, 8).Radius(4).Background(dot).Shrink(0)
			ui.Text(c, view.Detail).Shrink(1).Wrap()
			if view.Status == UploadBusy {
				breeze.Progress(c, view.Progress).Width(120).Height(4).Shrink(0)
			}
			if view.Fix != "" {
				c.Key("film-upload-fix")
				if breeze.Button(c, view.Fix).Shrink(0).Clicked() {
					*note = false
					s.OpenSettings(sectionid.Account)
				}
			}
			c.Key("film-upload-close")
			if breeze.FlatIconButton(c, iconClose, locales.FilmDismiss()).Shrink(0).Clicked() {
				*note = false
			}
		})
	})
}
