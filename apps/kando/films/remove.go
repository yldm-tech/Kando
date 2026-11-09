package films

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// removeDialog confirms 从资料库移除 before the film's folder, or only its video and subtitles, goes to the Trash. 取消 is the default, so Enter keeps the film.
func removeDialog(c *ui.Context, a App) {
	open, item, folder := a.Removing()
	if !*open {
		return
	}
	message := locales.FilmsRemoveFiles()
	switch {
	case !InDir(a.LibraryDir(), item.VideoPath):
		message = locales.FilmsRemoveOutside()
	case folder:
		message = locales.FilmsRemoveFolder()
	}
	if breeze.DestructiveAlertDialog(c, open, locales.FilmsRemoveConfirm(filmName(item)), message, 0, locales.FilmsRemove(), locales.FilmsCancel()) == 0 {
		a.TrashFilm(item)
	}
}
