package films

import "github.com/egoist/mygo/ui"

// dropZone is drawn over the library while files are dragged over it: a dashed frame in the accent and a card saying what letting go does. It lets the drop through to the page under it.
func dropZone(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Top(12).Left(12).Right(12).Bottom(12).PassThrough().Radius(18).Border(2, t.Accent).BorderStyle(ui.BorderDashed).Background(t.Accent.Alpha(0.07)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).AlignItems(ui.Center).Gap(8).Padding(18, 28).Radius(16).Background(t.Background.Alpha(0.92)).Shadow(0, 10, 30, 0, ui.RGBA(10, 60, 140, 0.18)).Children(func() {
			ui.Icon(c, dropIcon).Size(34, 34).TextColor(t.Accent)
			ui.Text(c, "松开以添加本地视频").FontSize(15).Bold().TextColor(ui.Hex("#0052B0"))
			ui.Text(c, "本地文件不用下载，直接生成字幕").FontSize(12).TextColor(t.TextMuted)
		})
	})
}
