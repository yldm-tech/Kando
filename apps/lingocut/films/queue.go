package films

import (
	"fmt"

	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// queueSection is 处理中: a card for every job that is waiting, running, failed or was cancelled, in QueueOrder. Past QueueShown cards it shows the first ones and a button for the rest.
func queueSection(c *ui.Context, t *ui.Theme, a App, queue []Film) {
	ui.Column(c).Gap(12).Children(func() {
		sectionTitle(c, "处理中", QueueSummary(queue))
		expanded := a.QueueExpanded()
		shown := queue
		if !*expanded && len(shown) > QueueShown {
			shown = shown[:QueueShown]
		}
		width, _ := c.Size()
		ui.Grid(c).Columns(QueueColumns(width)).Gap(QueueGap).AlignItems(ui.Start).Children(func() {
			for _, item := range shown {
				c.Key("queue-" + item.Key)
				queueCard(c, t, a, item)
			}
		})
		if len(queue) > QueueShown {
			label := fmt.Sprintf("显示全部 %d 个", len(queue))
			if *expanded {
				label = "收起"
			}
			c.Key("queue-more")
			if breeze.Button(c, label).AlignSelf(ui.Start).Clicked() {
				*expanded = !*expanded
			}
		}
	})
}

// sectionTitle is the heading of a part of the shelf with a dimmed note after it.
func sectionTitle(c *ui.Context, title, note string) {
	ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
		breeze.Text(c, title, breeze.TextSubtitle).Bold().Role(ui.RoleHeading).Level(2)
		if note != "" {
			breeze.Text(c, note, breeze.TextCaption, breeze.TextSecondary).FontSize(12).Margin(0, 0, 2, 0)
		}
	})
}

// queueCard is one job in 处理中: its thumbnail, its title, what it is doing or why it failed, the stages behind it, and what can be done about it.
func queueCard(c *ui.Context, t *ui.Theme, a App, item Film) {
	failed := item.Stage == core.StageFailed
	card := ui.Row(c).WidthPercent(100).MinWidth(0).Gap(12).Padding(10).Radius(16).Background(cardFill(t)).Border(0.5, t.Text.Alpha(0.06))
	if failed {
		card.AlignItems(ui.Start)
	} else {
		card.AlignItems(ui.Center)
	}
	card.Role(ui.RoleGroup).Label(filmName(item))
	card.Children(func() {
		queueThumb(c, t, a, item)
		text := ui.Column(c).Grow(1).MinWidth(0).Gap(4)
		if !failed {
			// The end keeps clear of the cancel button over the card's corner.
			text.Padding(0, 34, 0, 0)
		}
		text.Children(func() {
			ui.Text(c, filmName(item)).FontSize(13).FontWeight(600).SingleLine()
			if failed {
				failedDetail(c, t, a, item)
				return
			}
			ui.Text(c, QueueMeta(item)).FontSize(11).TextColor(t.TextMuted)
			stepsBar(c, t, item)
		})
		if !failed {
			c.Key("queue-cancel")
			if flatIconButton(c, closeIcon, "取消处理").Absolute().Top(8).Right(8).Clicked() {
				a.Cancel(item.JobID)
			}
		}
	})
}

// cardFill is the face of a processing card, a step darker than the page.
func cardFill(t *ui.Theme) ui.Color { return t.Background.Mix(t.Text, 0.04) }

// queueThumb is a processing card's 16:9 thumbnail: the poster once there is one, a download glyph until then, and a red mark over a failed job's.
func queueThumb(c *ui.Context, t *ui.Theme, a App, item Film) {
	ui.Box(c).Width(112).Shrink(0).AspectRatio(16.0 / 9).Radius(9).Clip().Background(t.Background.Mix(t.Text, 0.12)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if img := a.Poster(item.VideoPath); img != nil {
			ui.Image(c, img).Absolute().Top(0).Left(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover)
		} else {
			ui.Icon(c, downloadIcon).Size(22, 22).TextColor(t.TextMuted)
		}
		if item.Stage == core.StageFailed && !Canceled(item) {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, 0.45)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Image(c, failedMark).Size(22, 22)
			})
		}
	})
}

// QueueMeta is the line under a waiting or running job's title: the stage and what does the work.
func QueueMeta(item Film) string {
	switch item.Stage {
	case core.StageQueued:
		return "排队中 · 等待前面的任务"
	case core.StageDownloading:
		if Local(item) {
			return "读取中 · 本地文件"
		}
		return "下载中 · yt-dlp · " + SourceLabel(item)
	case core.StageTranscribing:
		return "转录中 · whisper 本机识别"
	case core.StageTranslating:
		return "翻译中 · AI 服务"
	case core.StageExporting:
		return "导出中 · 写入 video.srt 与 video.bilingual.srt"
	}
	return StageLabel(item.Stage)
}

// stepsBar shows the stages of a job as segments: those behind it filled, the one it is in half filled.
func stepsBar(c *ui.Context, t *ui.Theme, item Film) {
	total, done := Steps(item)
	running := Running(item)
	track, fill := t.Text.Alpha(0.09), t.Accent
	ui.Box(c).WidthPercent(100).Height(4).Margin(2, 0, 0, 0).Role(ui.RoleProgress).Label("进度").Range(0, float64(total), float64(done)).Draw(func(p *ui.Painter, r ui.Rect) {
		const gap = 3
		w := (r.W - gap*float32(total-1)) / float32(total)
		for i := range total {
			color := track
			switch {
			case i < done:
				color = fill
			case i == done && running:
				color = fill.Alpha(0.35)
			}
			p.Fill(ui.Rect{X: r.X + float32(i)*(w+gap), Y: r.Y, W: w, H: r.H}, color, 2)
		}
	})
}

// failedDetail is what a failed or cancelled card says and offers: why, in red unless it was cancelled, and 打开设置 when a setting fixes it, 重试 and 移除.
func failedDetail(c *ui.Context, t *ui.Theme, a App, item Film) {
	reason, section := a.FailureHint(item)
	if Canceled(item) {
		ui.Text(c, "已取消").FontSize(11).TextColor(t.TextMuted)
	} else {
		ui.Text(c, reason).FontSize(11).TextColor(t.Danger).Wrap().MaxLines(2)
	}
	ui.Row(c).Wrap().Gap(6).Margin(2, 0, 0, 0).Children(func() {
		if section != "" {
			c.Key("queue-settings")
			if breeze.Button(c, "打开设置").Clicked() {
				a.OpenSettings(section)
			}
		}
		c.Key("queue-retry")
		if breeze.PrimaryButton(c, "重试").Clicked() {
			a.Retry(item.JobID)
		}
		c.Key("queue-remove")
		if breeze.Button(c, "移除").Clicked() {
			a.Remove(item.JobID)
		}
	})
}
