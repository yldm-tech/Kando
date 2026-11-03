package main

import (
	"fmt"
	"strings"
	"time"

	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

func (a *app) filmPage(c *ui.Context, t *ui.Theme, item film) {
	ui.Column(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
		a.filmTop(c, t, item)
		ui.Row(c).Grow(1).MinHeight(0).Gap(16).AlignItems(ui.Stretch).Children(func() {
			a.playerPane(c, t)
			a.subtitlePane(c, t, item)
		})
	})
}

func (a *app) filmTop(c *ui.Context, t *ui.Theme, item film) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		c.Key("film-back")
		if components.Button(c, "返回", components.ButtonSecondary).Shrink(0).Clicked() {
			a.openKey = ""
			a.stopPlayback()
		}
		title := item.name
		if title == "" {
			title = item.title
		}
		ui.Text(c, title).FontSize(18).Bold().Grow(1).Wrap()
		if item.stage != "" {
			color := t.Accent
			if item.stage == core.StageFailed {
				color = t.Danger
			}
			ui.Text(c, stageLabel(item.stage)).TextColor(color).Bold().Shrink(0)
		}
		path := item.subtitlePath
		if path == "" {
			path = item.videoPath
		}
		if path != "" {
			c.Key("film-reveal")
			if components.Button(c, "在访达中显示", components.ButtonSecondary).Shrink(0).Clicked() {
				a.reveal(path)
			}
		}
	})
	a.filmActions(c, item)
}

func (a *app) filmActions(c *ui.Context, item film) {
	showCancel := item.jobID != "" && item.stage != "" && !item.stage.Done()
	showStart := item.jobID == ""
	showFailed := item.jobID != "" && item.stage == core.StageFailed && !a.running()
	if !showCancel && !showStart && !showFailed {
		return
	}
	ui.Row(c).Gap(8).Children(func() {
		if showCancel {
			c.Key("film-cancel")
			if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
				a.stop()
			}
		}
		if showStart {
			c.Key("film-start")
			if ui.PrimaryButton(c, "开始处理").Disabled(a.running()).Clicked() {
				a.startFilm(item)
			}
		}
		if showFailed {
			c.Key("film-remove")
			if components.Button(c, "移除", components.ButtonSecondary).Clicked() {
				a.remove(item.jobID)
			}
			c.Key("film-retry")
			if ui.PrimaryButton(c, "重试").Clicked() {
				if job, ok := a.library.find(item.jobID); ok {
					a.retry(job)
					if a.job != nil {
						a.openKey = a.job.ID
						a.loadSubtitles(a.job.ID, a.job.SubtitlePath)
					}
				}
			}
		}
	})
}

func (a *app) playerPane(c *ui.Context, t *ui.Theme) {
	if a.play == nil && a.playPath == "" && a.playErr == "" {
		return
	}
	stage := ui.Box(c).Grow(1).MinWidth(0).MinHeight(240).Radius(12).Clip().Background(ui.Hex("#000000")).Cursor(ui.CursorPointer)
	if a.play != nil && stage.Clicked() {
		a.togglePlay(c)
	}
	stage.Children(func() {
		switch {
		case a.play != nil:
			ui.Video(c, a.play.player).Fill().Fit(ui.Contain)
			a.play.settlePlay()
			if !a.play.player.Playing() {
				a.playButton(c, t)
			}
			a.playCue(c, t)
			a.playTransport(c, t)
		case a.playErr != "":
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, a.playErr).TextColor(t.Text)
			})
		default:
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, "正在打开视频…").TextColor(t.TextMuted)
			})
		}
	})
}

// togglePlay starts or holds the picture. The button is built after the video, so the frame that handles the click has already drawn a paused frame; the extra frame is what starts the clock.
func (a *app) togglePlay(c *ui.Context) {
	if a.play == nil {
		return
	}
	a.play.toggle()
	c.Invalidate()
}

// playButton is the round control on the picture. It shows while the video is held.
func (a *app) playButton(c *ui.Context, t *ui.Theme) {
	label := "播放"
	glyph := iconPlay
	if a.play.player.Playing() {
		label = "暂停"
		glyph = iconPause
	}
	ui.Row(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Center).Justify(ui.Center).PassThrough().Children(func() {
		btn := ui.Row(c).Size(68, 68).Radius(34).Background(ui.RGBA(180, 180, 180, 0.42)).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(label)
		if btn.Clicked() {
			a.togglePlay(c)
		}
		btn.Children(func() {
			ui.Icon(c, glyph).Size(30, 30).TextColor(t.Text)
		})
	})
}

// playCue draws the current bilingual line on the picture, above the transport.
func (a *app) playCue(c *ui.Context, _ *ui.Theme) {
	if a.play == nil || a.subs.key != a.openKey || !a.subs.ready {
		return
	}
	cue, ok := cueAt(a.subs.cues, a.play.playhead())
	if !ok {
		return
	}
	lines := a.styledLines(cue.Text)
	if len(lines) == 0 {
		return
	}
	block := ui.Column(c).Absolute().Left(32).Right(32).Bottom(a.cue.Bottom).Gap(2).Padding(8, 14).Radius(8).AlignItems(ui.Center).PassThrough()
	if a.cue.Box {
		block.Background(ui.RGBA(0, 0, 0, 0.55))
	}
	block.Children(func() {
		for _, line := range lines {
			text := ui.Text(c, line.text).FontSize(line.face.Size).TextColor(ui.Hex(line.face.Color)).TextAlign(ui.Center).Wrap()
			if line.face.Bold {
				text.Bold()
			}
			if family := fontFamily(line.face.Family); family != "" {
				text.Font(family)
			}
			if a.cue.Shadow {
				text.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.95))
			}
		}
	})
}

func (a *app) playTransport(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(16).Right(16).Bottom(14).Gap(12).AlignItems(ui.Center).Children(func() {
		glyph := iconPlay
		btn := ui.Row(c).Size(28, 28).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
		if a.play.player.Playing() {
			glyph = iconPause
			btn.Label("暂停")
		}
		if btn.Clicked() {
			a.togglePlay(c)
		}
		btn.Children(func() {
			ui.Icon(c, glyph).Size(16, 16).TextColor(t.Text)
		})
		clock := cueClock(a.play.playhead())
		if a.play.duration > 0 {
			clock += " / " + cueClock(a.play.duration)
		}
		ui.Text(c, clock).FontSize(12).Shrink(0)
		a.playTrack(c, t)
	})
}

func (a *app) playTrack(c *ui.Context, t *ui.Theme) {
	frac := float32(0)
	if a.play.duration > 0 {
		frac = float32(a.play.playhead()) / float32(a.play.duration)
		if frac > 1 {
			frac = 1
		}
		if frac < 0 {
			frac = 0
		}
	}
	ui.Box(c).Grow(1).Height(4).Radius(2).Background(ui.RGBA(255, 255, 255, 0.28)).Children(func() {
		if frac > 0 {
			ui.Box(c).WidthPercent(frac * 100).Height(4).Radius(2).Background(t.Accent)
		}
	})
}

func (a *app) subtitlePane(c *ui.Context, t *ui.Theme, item film) {
	pane := ui.Column(c).MinHeight(0).Gap(10)
	if a.play != nil || a.playPath != "" || a.playErr != "" {
		pane.Width(340).Shrink(0)
	} else {
		pane.Grow(1)
	}
	pane.Children(func() {
		a.filmTabs(c, t)
		switch a.filmSide {
		case "clips":
			a.chapterPane(c, t)
		case "style":
			a.cueStylePane(c, t)
		default:
			a.transcriptPane(c, t, item)
		}
	})
}

func (a *app) filmTabs(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(4).Children(func() {
		for _, tab := range []struct{ id, label string }{
			{"", "文稿"},
			{"clips", "精彩片段"},
			{"style", "字幕样式"},
		} {
			a.filmTab(c, tab.id, tab.label)
		}
	})
}

func (a *app) filmTab(c *ui.Context, id, label string) {
	tab := ui.Row(c).Padding(6, 10).Radius(8).Label(label).Cursor(ui.CursorPointer)
	if a.filmSide == id {
		tab.Background(ui.Hex("#4a3824"))
	}
	if tab.Clicked() {
		a.filmSide = id
	}
	tab.Children(func() {
		ui.Text(c, label).FontSize(13)
	})
}

func (a *app) transcriptPane(c *ui.Context, t *ui.Theme, item film) {
	ready := a.subs.key == item.key && a.subs.ready
	if ready && a.subs.err == "" && item.subtitlePath != "" {
		ui.Text(c, fmt.Sprintf("%d 句", len(a.subs.cues))).TextColor(t.TextMuted).FontSize(12)
	}
	switch {
	case !ready:
		ui.Text(c, "正在读取字幕…").TextColor(t.TextMuted)
	case a.subs.err != "":
		ui.Text(c, a.subs.err).TextColor(t.Danger).Wrap()
	case item.subtitlePath == "":
		ui.Text(c, "还没有字幕。处理完成后会显示在这里。").TextColor(t.TextMuted).Wrap()
	case len(a.subs.cues) == 0:
		ui.Text(c, "这份字幕是空的。").TextColor(t.TextMuted)
	default:
		ui.List(c, &a.subs.list, len(a.subs.cues), func(i int) {
			a.cueRow(c, t, a.subs.cues[i])
		}).Grow(1).Gap(14)
	}
}

func (a *app) chapterPane(c *ui.Context, t *ui.Theme) {
	ready := a.subs.ready
	chapters := filmChapters(a.subs.cues)
	if ready && a.subs.err == "" && len(chapters) > 0 {
		end := chapters[len(chapters)-1].End
		ui.Text(c, fmt.Sprintf("%d 段 · 共 %s", len(chapters), cueClock(end))).TextColor(t.TextMuted).FontSize(12)
	}
	switch {
	case !ready:
		ui.Text(c, "正在读取字幕…").TextColor(t.TextMuted)
	case a.subs.err != "":
		ui.Text(c, a.subs.err).TextColor(t.Danger).Wrap()
	case len(chapters) == 0:
		ui.Text(c, "还没有可以分段的字幕。").TextColor(t.TextMuted).Wrap()
	default:
		ui.List(c, &a.subs.clips, len(chapters), func(i int) {
			a.chapterRow(c, t, i, chapters[i])
		}).Grow(1).Gap(8)
	}
}

func (a *app) chapterRow(c *ui.Context, t *ui.Theme, index int, ch filmChapter) {
	row := ui.Column(c).Padding(8, 10).Radius(8).Gap(4).Cursor(ui.CursorPointer)
	if a.chapterActive(ch) {
		row.Background(ui.Hex("#4a3824"))
	}
	if row.Clicked() {
		a.seekPlayback(ch.Start)
	}
	row.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(22, 22).Radius(11).Shrink(0).Background(t.Surface).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("%d", index+1)).FontSize(12)
			})
			ui.Text(c, ch.Title).Grow(1).Wrap()
		})
		if ch.Detail != "" {
			ui.Text(c, ch.Detail).TextColor(t.TextMuted).FontSize(12).Wrap()
		}
		ui.Text(c, cueClock(ch.Start)+" – "+cueClock(ch.End)).TextColor(t.TextMuted).FontSize(12)
	})
}

func (a *app) chapterActive(ch filmChapter) bool {
	if a.play == nil {
		return false
	}
	at := a.play.playhead()
	return at >= ch.Start && (ch.End <= ch.Start || at < ch.End)
}

func (a *app) cueStylePane(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).WidthPercent(100).Gap(14).Children(func() {
			ui.Text(c, "预设").TextColor(t.TextMuted).FontSize(12)
			ui.Row(c).Gap(8).Children(func() {
				a.cuePresetCard(c, t, "outline", "描边")
				a.cuePresetCard(c, t, "box", "底框")
				a.cuePresetCard(c, t, "large", "大字")
			})
			ui.Text(c, "文字").TextColor(t.TextMuted).FontSize(12)
			a.cueChoiceRow(c, t, []cueChoice{{"translation", "译文"}, {"original", "原文"}}, a.cue.Edit, func(id string) { a.cue.Edit = id })
			face := a.editedFace()
			a.cueField(c, t, "字体", func() {
				c.Key("cue-font-" + a.cue.Edit)
				choiceSelect(c, &face.Family, []string{"默认", "苹方", "冬青黑体"}).Width(120)
			})
			a.cueStepper(c, t, "字号", &face.Size, 2, 12, 72)
			a.cueColors(c, t, face)
			ui.Row(c).MinHeight(36).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "加粗").Grow(1)
				components.Switch(c, &face.Bold, "加粗", components.SwitchOptions{HideLabel: true})
			})
			if components.Button(c, "使用原文样式", components.ButtonSecondary).Clicked() {
				a.cue.Translation = a.cue.Original
			}
			ui.Text(c, "双语").TextColor(t.TextMuted).FontSize(12)
			a.cueChoiceRow(c, t, []cueChoice{{"translation", "译文在上"}, {"original", "原文在上"}, {"translation-only", "只要译文"}}, a.cue.Order, func(id string) { a.cue.Order = id })
			ui.Text(c, "位置").TextColor(t.TextMuted).FontSize(12)
			a.cueStepper(c, t, "高底部", &a.cue.Bottom, 4, 24, 160)
			if components.Button(c, "恢复默认样式", components.ButtonSecondary).Clicked() {
				a.cue = defaultCueLook()
			}
		})
	})
}

func (a *app) cuePresetCard(c *ui.Context, t *ui.Theme, id, label string) {
	card := ui.Column(c).Width(96).Grow(1).Padding(10, 6).Radius(10).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label)
	if a.cue.Preset == id {
		card.Background(ui.Hex("#4a3824"))
	} else {
		card.Background(t.Surface)
	}
	if card.Clicked() {
		a.applyCuePreset(id)
	}
	card.Children(func() {
		switch id {
		case "box":
			ui.Row(c).Padding(2, 6).Radius(4).Background(ui.RGBA(0, 0, 0, 0.55)).Children(func() {
				ui.Text(c, "字幕").FontSize(13)
			})
		case "large":
			ui.Text(c, "字幕").FontSize(18).Bold()
		default:
			ui.Text(c, "字幕").FontSize(13).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.95))
		}
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
	})
}

type cueChoice struct {
	id, label string
}

func (a *app) cueChoiceRow(c *ui.Context, t *ui.Theme, choices []cueChoice, current string, set func(string)) {
	ui.Row(c).Gap(4).Children(func() {
		for _, choice := range choices {
			chip := ui.Row(c).Grow(1).Padding(6, 8).Radius(8).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(choice.label)
			if current == choice.id {
				chip.Background(t.Accent)
			} else {
				chip.Background(t.Surface)
			}
			if chip.Clicked() {
				set(choice.id)
			}
			chip.Children(func() {
				color := t.Text
				if current == choice.id {
					color = t.AccentText
				}
				ui.Text(c, choice.label).FontSize(12).TextColor(color)
			})
		}
	})
}

func (a *app) cueField(c *ui.Context, t *ui.Theme, label string, control func()) {
	ui.Row(c).MinHeight(36).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1)
		_ = t
		control()
	})
}

func (a *app) cueStepper(c *ui.Context, t *ui.Theme, label string, value *float32, step, min, max float32) {
	ui.Row(c).MinHeight(36).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1)
		if components.Button(c, "−", components.ButtonSecondary).Label(label+"减小").Clicked() && *value > min {
			*value -= step
		}
		ui.Text(c, fmt.Sprintf("%.0f", *value)).FontSize(13)
		if components.Button(c, "+", components.ButtonSecondary).Label(label+"增大").Clicked() && *value < max {
			*value += step
		}
		_ = t
	})
}

func (a *app) cueColors(c *ui.Context, t *ui.Theme, face *cueFace) {
	ui.Row(c).MinHeight(36).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "颜色").Grow(1)
		for _, color := range []struct{ name, hex string }{{"白色", "#f6f1e8"}, {"黄色", "#f5d76e"}, {"橙色", "#e39b2d"}} {
			dot := ui.Row(c).Size(18, 18).Radius(9).Background(ui.Hex(color.hex)).Cursor(ui.CursorPointer).Label(color.name)
			if face.Color == color.hex {
				dot.Size(22, 22).Radius(11)
			}
			if dot.Clicked() {
				face.Color = color.hex
			}
		}
		_ = t
	})
}

func (a *app) cueRow(c *ui.Context, t *ui.Theme, seg core.Segment) {
	lines := a.styledLines(seg.Text)
	row := ui.Column(c).Gap(2).Padding(8, 10).Radius(8)
	if a.cueActive(seg) {
		row.Background(ui.Hex("#4a3824"))
	}
	row.Children(func() {
		ui.Text(c, cueClock(seg.Start)).TextColor(t.TextMuted).FontSize(12)
		for i, line := range lines {
			text := ui.Text(c, line.text).Wrap()
			if i > 0 {
				text.TextColor(t.TextMuted)
			}
		}
	})
}

func (a *app) cueActive(seg core.Segment) bool {
	if a.play == nil || a.subs.key != a.openKey || !a.subs.ready {
		return false
	}
	cur, ok := cueAt(a.subs.cues, a.play.playhead())
	return ok && cur.Start == seg.Start && cur.Text == seg.Text
}

type filmChapter struct {
	Start, End    time.Duration
	Title, Detail string
}

// filmChapters groups the transcript into a handful of stretches, using the lines already in the file.
func filmChapters(cues []core.Segment) []filmChapter {
	if len(cues) == 0 {
		return nil
	}
	span := cues[len(cues)-1].End - cues[0].Start
	if span < 0 {
		span = 0
	}
	want := int(span / (30 * time.Second))
	if want < 4 {
		want = 4
	}
	if want > 12 {
		want = 12
	}
	if want > len(cues) {
		want = len(cues)
	}
	base, extra := len(cues)/want, len(cues)%want
	out := make([]filmChapter, 0, want)
	i := 0
	for n := 0; n < want; n++ {
		size := base
		if n < extra {
			size++
		}
		out = append(out, chapterFrom(cues[i:i+size]))
		i += size
	}
	return out
}

func chapterFrom(group []core.Segment) filmChapter {
	title, detail := splitCue(group[0].Text)
	if title == "" {
		title, detail = detail, ""
	}
	if title == "" {
		title = "这一段"
	}
	if detail == "" && len(group) > 1 {
		next, original := splitCue(group[1].Text)
		if next != "" {
			detail = next
		} else {
			detail = original
		}
	}
	end := group[len(group)-1].End
	if end < group[0].Start {
		end = group[0].Start
	}
	return filmChapter{Start: group[0].Start, End: end, Title: shorten(title, 36), Detail: shorten(detail, 48)}
}

type cueFace struct {
	Family string
	Size   float32
	Color  string
	Bold   bool
}

type cueLook struct {
	Preset, Edit, Order string
	Bottom              float32
	Box, Shadow         bool
	Translation         cueFace
	Original            cueFace
}

type styledLine struct {
	text string
	face cueFace
}

func defaultCueLook() cueLook {
	return cueLook{
		Preset: "box", Edit: "translation", Order: "translation", Bottom: 56, Box: true,
		Translation: cueFace{Family: "默认", Size: 18, Color: "#f6f1e8", Bold: true},
		Original:    cueFace{Family: "默认", Size: 14, Color: "#b3a38f"},
	}
}

func (a *app) applyCuePreset(id string) {
	a.cue.Preset = id
	switch id {
	case "outline":
		a.cue.Box, a.cue.Shadow = false, true
		a.cue.Translation.Size, a.cue.Original.Size = 18, 14
	case "large":
		a.cue.Box, a.cue.Shadow = true, false
		a.cue.Translation.Size, a.cue.Original.Size = 32, 22
	default:
		a.cue.Preset = "box"
		a.cue.Box, a.cue.Shadow = true, false
		a.cue.Translation.Size, a.cue.Original.Size = 18, 14
	}
}

func (a *app) editedFace() *cueFace {
	if a.cue.Edit == "original" {
		return &a.cue.Original
	}
	return &a.cue.Translation
}

func (a *app) styledLines(text string) []styledLine {
	translation, original := splitCue(text)
	add := func(lines []styledLine, s string, face cueFace) []styledLine {
		if s == "" {
			return lines
		}
		return append(lines, styledLine{text: s, face: face})
	}
	switch a.cue.Order {
	case "original":
		return add(add(nil, original, a.cue.Original), translation, a.cue.Translation)
	case "translation-only":
		lines := add(nil, translation, a.cue.Translation)
		if len(lines) == 0 {
			lines = add(lines, original, a.cue.Original)
		}
		return lines
	default:
		return add(add(nil, translation, a.cue.Translation), original, a.cue.Original)
	}
}

func fontFamily(name string) string {
	switch name {
	case "苹方":
		return "PingFang SC"
	case "冬青黑体":
		return "Hiragino Sans GB"
	default:
		return ""
	}
}

func splitCue(text string) (translation, original string) {
	text = strings.TrimSpace(text)
	parts := strings.SplitN(text, "\n", 2)
	if len(parts) == 1 {
		return "", strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func cueClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h := ms / 3_600_000
	m := ms / 60_000 % 60
	s := ms / 1000 % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
