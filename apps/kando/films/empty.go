package films

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// noLibrary is the page before there is a library: what a library is, 新建资料库… and 打开已有资料库….
func noLibrary(c *ui.Context, a App) {
	statusPage(c, libraryArt, "还没有资料库", "资料库是一个 .lingocut 文件夹，每部影片一个子文件夹，视频、封面和字幕都是普通文件，可以直接在访达里管理。", func() {
		c.Key("empty-new")
		if statusButton(breeze.PrimaryButton(c, "新建资料库…")).Clicked() {
			a.BeginNew()
		}
		c.Key("empty-open")
		if statusButton(breeze.Button(c, "打开已有资料库…")).Clicked() {
			a.PickLibrary()
		}
	})
}

// emptyLibrary is the page of a library that holds no film yet: how to add one, 选择本地视频… and 打开已有资料库….
func emptyLibrary(c *ui.Context, a App) {
	statusPage(c, startArt, "从一段视频开始", "在上方粘贴 YouTube、哔哩哔哩等视频链接，或把本地视频拖进窗口。看懂会下载视频，识别语音，再生成双语字幕。", func() {
		c.Key("empty-pick")
		if statusButton(breeze.PrimaryButton(c, "选择本地视频…")).Clicked() {
			a.PickVideos()
		}
		c.Key("empty-open")
		if statusButton(breeze.Button(c, "打开已有资料库…")).Clicked() {
			a.PickLibrary()
		}
	})
}

// emptySource is what a source of the sidebar shows when no film of the library comes from it.
func emptySource(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Padding(80, 28).Children(func() {
		breeze.Text(c, "这个来源下还没有影片。", breeze.TextSecondary)
	})
}

// statusPage is an empty page as the prototype draws it: the colored illustration, the title at 20 points, the description centered in gray, and the actions in a row, in a column 420 wide, 90 points under the message bar.
func statusPage(c *ui.Context, art *ui.SVG, title, description string, actions func()) {
	t := c.Theme()
	ui.Column(c).AlignItems(ui.Center).Padding(90, 28).Children(func() {
		ui.Column(c).MaxWidth(420).AlignItems(ui.Center).Gap(10).Children(func() {
			w, h := art.Size()
			ui.Image(c, art).Size(w, h).Shrink(0)
			ui.Text(c, title).FontSize(20).Bold().Margin(6, 0, 0, 0).Role(ui.RoleHeading).Level(2)
			ui.Text(c, description).FontSize(13).FixedLineHeight(20).TextColor(t.TextMuted).TextAlign(ui.Center).Wrap()
			ui.Row(c).Gap(10).Wrap().Justify(ui.Center).Margin(8, 0, 0, 0).Children(actions)
		})
	})
}

// statusButton sizes an empty page's button as the prototype's, 30 points high and padded 16 at its sides; the platform draws its face.
func statusButton(b ui.Element) ui.Element { return b.Height(30).PaddingX(16) }
