package films

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// removeDialog confirms 从资料库移除 before the film's folder, or only its video and subtitles, goes to the Trash. 取消 is the default, so Enter keeps the film.
func removeDialog(c *ui.Context, a App) {
	open, item, folder := a.Removing()
	if !*open {
		return
	}
	message := "视频和它的字幕会移到废纸篓，可以从废纸篓找回。所在文件夹里的其他文件留在原处。"
	switch {
	case !InDir(a.LibraryDir(), item.VideoPath):
		message = "视频不在资料库文件夹里，会留在原处，只是不再出现在资料库中。"
	case folder:
		message = "影片文件夹会移到废纸篓，里面的视频、封面和字幕都可以从废纸篓找回。"
	}
	if breeze.DestructiveAlertDialog(c, open, "要从资料库移除「"+filmName(item)+"」吗？", message, 0, "移除", "取消") == 0 {
		a.TrashFilm(item)
	}
}
