package films

import (
	"fmt"
	"strings"
	"time"

	"kando/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// queueSection is 处理中: a card for every job that is waiting, running, failed or was cancelled, in QueueOrder. Past QueueShown cards it shows the first ones and a button for the rest.
func queueSection(c *ui.Context, t *ui.Theme, a App, queue []Film) {
	ui.Column(c).Margin(26, 0, 0, 0).Gap(12).Children(func() {
		sectionTitle(c, t, "处理中", QueueSummary(queue))
		expanded := a.QueueExpanded()
		shown := queue
		if !*expanded && len(shown) > QueueShown {
			shown = shown[:QueueShown]
		}
		width, _ := c.Size()
		ui.Grid(c).Columns(QueueColumns(width)).Gap(QueueGap).Padding(2, 2, 8, 2).AlignItems(ui.Start).Children(func() {
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

// sectionTitle is the heading of a part of the shelf with a dimmed note after it on the same baseline.
func sectionTitle(c *ui.Context, t *ui.Theme, title, note string) {
	ui.Row(c).Height(26).Gap(10).AlignItems(ui.Baseline).Children(func() {
		ui.Text(c, title).FontSize(19).Bold().LetterSpacing(-0.2).Role(ui.RoleHeading).Level(2)
		if note != "" {
			ui.Text(c, note).FontSize(12).TextColor(t.TextMuted)
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
		// The end keeps clear of the cancel button over the card's corner.
		text.Padding(0, 18, 0, 0)
		text.Children(func() {
			ui.Text(c, filmName(item)).FontSize(13).FontWeight(600).SingleLine()
			if failed {
				failedDetail(c, t, a, item)
				return
			}
			meta := ui.Text(c, QueueMeta(item)).FontSize(11).TextColor(t.TextMuted)
			if RetryWaiting(item) {
				meta.TextColor(retryInk)
			}
			stepsBar(c, t, item)
		})
		if !failed {
			c.Key("queue-cancel")
			if cancelButton(c, t).Clicked() {
				a.Cancel(item.JobID)
			}
		}
	})
}

// retryInk is the orange of a card whose translation waits out a rate limit.
var retryInk = ui.Hex("#A35200")

// cardFill is the face of a processing card, a step darker than the page.
func cardFill(*ui.Theme) ui.Color { return ui.Hex("#F5F5F7") }

// thumbFill is a thumbnail's face before its picture is there, and placeholderInk the glyph on it.
var thumbFill, placeholderInk = ui.Hex("#DCDDE2"), ui.Hex("#8E8E95")

// cancelButton is the round × over a waiting or running card's corner, 22 points, its 10-point glyph in the secondary text's gray.
func cancelButton(c *ui.Context, t *ui.Theme) ui.Element {
	b := ui.ButtonBase(c).Label("取消处理").Absolute().Top(8).Right(8).Size(22, 22).Radius(11).AlignItems(ui.Center).Justify(ui.Center)
	if b.Hovered() {
		b.Background(ui.RGBA(0, 0, 0, 0.06))
	}
	b.Children(func() {
		ui.Icon(c, closeIcon).Size(10, 10).TextColor(t.TextMuted)
	})
	return b
}

// queueThumb is a processing card's 16:9 thumbnail: the poster once there is one, the picture the video's site gives it before that, a download glyph until either is there, and a red mark over a failed job's.
func queueThumb(c *ui.Context, t *ui.Theme, a App, item Film) {
	ui.Box(c).Width(112).Shrink(0).AspectRatio(16.0 / 9).Radius(9).Clip().Background(thumbFill).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		img := a.Poster(item.VideoPath)
		if img == nil {
			img = a.Thumbnail(item)
		}
		if img != nil {
			ui.Image(c, img).Absolute().Top(0).Left(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover)
		} else {
			ui.Icon(c, downloadIcon).Size(22, 22).TextColor(placeholderInk)
		}
		if item.Stage == core.StageFailed && !Canceled(item) {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, 0.45)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Image(c, failedMark).Size(22, 22)
			})
		}
	})
}

// QueueMeta is the line under a waiting or running job's title: the stage and how far it has gone, as its tool reports it, or what does the work until the tool has said.
func QueueMeta(item Film) string {
	p := item.Progress
	switch item.Stage {
	case core.StageQueued:
		return "排队中 · 等待前面的任务"
	case core.StageDownloading:
		if Local(item) {
			return "读取中 · 本地文件"
		}
		if !p.Known {
			return "下载中 · yt-dlp · " + SourceLabel(item)
		}
		parts := []string{"下载中", Percent(p.Fraction)}
		if p.Speed > 0 {
			parts = append(parts, Rate(p.Speed))
		}
		if p.ETA > 0 {
			parts = append(parts, "还剩 "+Clock(p.ETA))
		}
		return strings.Join(parts, " · ")
	case core.StageTranscribing:
		if p.Known {
			return "生成字幕中 · whisper 本机识别 · " + Percent(p.Fraction)
		}
		return "生成字幕中 · whisper 本机识别"
	case core.StageTranslating:
		if RetryWaiting(item) {
			why := "翻译接口限流"
			if p.Unanswered {
				why = "翻译接口没有回应"
			}
			return fmt.Sprintf("%s，%d 秒后重试（%d/%d）", why, int(p.RetryIn.Round(time.Second)/time.Second), p.Attempt, p.Attempts)
		}
		if p.Batches > 0 {
			return fmt.Sprintf("翻译中 · 第 %d/%d 批 · 每批 %d 句", p.Batch, p.Batches, p.BatchSize)
		}
		return "翻译中 · AI 服务"
	case core.StageExporting:
		return "导出中 · 写入 video.srt 与 video.bilingual.srt"
	}
	return StageLabel(item.Stage)
}

// RetryWaiting reports whether the film's translation is waiting to try a request again: one rate-limited, or one that got no usable answer.
func RetryWaiting(item Film) bool {
	return item.Stage == core.StageTranslating && item.Progress.RetryIn > 0
}

// Percent writes a fraction as a whole percentage, such as 42%.
func Percent(f float64) string {
	return fmt.Sprintf("%d%%", int(f*100))
}

// Rate writes a download speed in bytes a second as the Finder writes sizes, such as 3.2 MB/s.
func Rate(bytesPerSecond float64) string {
	const k = 1024.0
	switch {
	case bytesPerSecond >= k*k:
		return fmt.Sprintf("%.1f MB/s", bytesPerSecond/(k*k))
	case bytesPerSecond >= k:
		return fmt.Sprintf("%.0f KB/s", bytesPerSecond/k)
	}
	return fmt.Sprintf("%.0f B/s", bytesPerSecond)
}

// stepsBar shows the stages of a job as segments, as the prototype draws them: those behind it filled, the one it is in filled as far as its tool has said, or tinted while it has not said yet, and those ahead empty.
func stepsBar(c *ui.Context, t *ui.Theme, item Film) {
	fills := StepFills(item)
	total, done := Steps(item)
	running := Running(item)
	value := float64(done)
	if done < total {
		value += fills[done]
	}
	track, fill := t.Text.Alpha(0.09), t.Accent
	known := item.Progress.Known
	ui.Box(c).WidthPercent(100).Height(4).Margin(2, 0, 0, 0).Role(ui.RoleProgress).Label("进度").Range(0, float64(total), value).Draw(func(p *ui.Painter, r ui.Rect) {
		const gap = 3
		w := (r.W - gap*float32(total-1)) / float32(total)
		for i, f := range fills {
			segment := ui.Rect{X: r.X + float32(i)*(w+gap), Y: r.Y, W: w, H: r.H}
			if i == done && running && !known {
				p.Fill(segment, fill.Alpha(0.35), 2)
				continue
			}
			p.Fill(segment, track, 2)
			if f > 0 {
				p.Fill(ui.Rect{X: segment.X, Y: segment.Y, W: max(segment.W*float32(f), r.H), H: r.H}, fill, 2)
			}
		}
	})
}

// Remedy is what a failed card says when the app knows why its download failed, as the prototype words each kind: Headline in red (in the accent while Working, as while yt-dlp updates), Why under it, and the Actions that fix it, the primary one first. 移除 always follows them.
type Remedy struct {
	Headline, Why string
	Working       bool
	Actions       []RemedyAction
}

// RemedyAction is one button of a Remedy; App.RunRemedy hears its ID when it is clicked.
type RemedyAction struct {
	ID, Label         string
	Primary, Disabled bool
}

// failInk is the red of a failed card's reason, and whyInk the dark gray of what to do about it.
var failInk, whyInk = ui.Hex("#C4291C"), ui.Hex("#3A3A3F")

// failedDetail is what a failed or cancelled card says and offers: for a download whose failure the app knows, its remedy (App.FailureRemedy); otherwise why, in red unless it was cancelled, and 打开设置 when a setting fixes it, 重试 and 移除.
func failedDetail(c *ui.Context, t *ui.Theme, a App, item Film) {
	if r, ok := a.FailureRemedy(item); ok && !Canceled(item) {
		remedyDetail(c, t, a, item, r)
		return
	}
	reason, section := a.FailureHint(item)
	if Canceled(item) {
		ui.Text(c, "已取消").FontSize(11).TextColor(t.TextMuted)
	} else {
		ui.Text(c, reason).FontSize(11).TextColor(failInk).Wrap().MaxLines(2)
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

func remedyDetail(c *ui.Context, t *ui.Theme, a App, item Film, r Remedy) {
	headline := ui.Text(c, r.Headline).FontSize(11).TextColor(failInk).Wrap().MaxLines(2)
	if r.Working {
		headline.TextColor(t.Accent)
	}
	if r.Why != "" {
		ui.Text(c, r.Why).FontSize(11).FixedLineHeight(15).TextColor(whyInk).Wrap()
	}
	ui.Row(c).Wrap().Gap(6).Margin(2, 0, 0, 0).Children(func() {
		for _, action := range r.Actions {
			c.Key("queue-remedy-" + action.ID)
			b := breeze.Button
			if action.Primary {
				b = breeze.PrimaryButton
			}
			if b(c, action.Label).Disabled(action.Disabled).Clicked() && !action.Disabled {
				a.RunRemedy(item, action.ID)
			}
		}
		c.Key("queue-remove")
		if breeze.Button(c, "移除").Clicked() {
			a.Remove(item.JobID)
		}
	})
}
