package film

import (
	"fmt"
	"strings"
	"time"

	"lingocut/apps/lingocut/films"
	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// App is the open film the player page reads and the clicks it reports.
type App interface {
	OpenKey() string
	SetOpenKey(string)
	StopPlayback()
	Reveal(path string)
	Running() bool
	Stop()
	StartFilm(item films.Film)
	Remove(id string)
	RetryOpen(id string)
	Play() Player
	PlayPath() string
	PlayErr() string
	Subs() Subs
	SubList() *ui.ListState
	ClipList() *ui.ListState
	FilmSide() string
	SetFilmSide(string)
	Seek(at time.Duration)
	Cue() *Look
	// SubMode is which lines of a bilingual cue show: SubBilingual, SubTranslation, SubOriginal or SubOff.
	SubMode() string
	SetSubMode(mode string)
	// SubOrder is which line of a bilingual cue goes on top: SubTranslationFirst or SubOriginalFirst.
	SubOrder() string
	SetSubOrder(order string)
	UploadView(item films.Film) Upload
	BeginUpload(item films.Film)
	// ToggleFullScreen enters or leaves full screen.
	ToggleFullScreen()
}

// Upload is the manual R2 action on a film that already has a local video.
type Upload struct {
	Show    bool
	Enabled bool
	Label   string
	Detail  string
}

// Player is the picture currently open.
type Player interface {
	Widget() *ui.VideoPlayer
	Playing() bool
	Settle()
	Toggle()
	Head() time.Duration
	Length() time.Duration
}

// Subs is the subtitle file open on the film page.
type Subs struct {
	Key   string
	Ready bool
	Err   string
	Cues  []core.Segment
}

type screen struct{ App }

// Show draws the open film in dark, the player's theme, which stays dark in the light window.
func Show(c *ui.Context, dark *ui.Theme, a App, item films.Film) {
	ui.Themed(c, dark).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(dark.Background).Radius(12).Padding(12).Children(func() {
		screen{a}.filmPage(c, dark, item)
	})
}

func (s screen) filmPage(c *ui.Context, t *ui.Theme, item films.Film) {
	ui.Column(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
		s.filmTop(c, t, item)
		ui.Row(c).Grow(1).MinHeight(0).Gap(16).AlignItems(ui.Stretch).Children(func() {
			s.playerPane(c, t)
			s.subtitlePane(c, t, item)
		})
	})
}

func (s screen) filmTop(c *ui.Context, t *ui.Theme, item films.Film) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		c.Key("film-back")
		if breeze.Button(c, "返回").Shrink(0).Clicked() {
			s.SetOpenKey("")
			s.StopPlayback()
		}
		title := item.Name
		if title == "" {
			title = item.Title
		}
		ui.Text(c, title).FontSize(18).Bold().Grow(1).Wrap()
		if item.Stage != "" {
			color := t.Accent
			if item.Stage == core.StageFailed {
				color = t.Danger
			}
			ui.Text(c, films.StageLabel(item.Stage)).TextColor(color).Bold().Shrink(0)
		}
		path := item.SubtitlePath
		if path == "" {
			path = item.VideoPath
		}
		if path != "" {
			c.Key("film-reveal")
			if breeze.Button(c, "在访达中显示").Shrink(0).Clicked() {
				s.Reveal(path)
			}
		}
	})
	s.filmActions(c, item)
	s.uploadRow(c, t, item)
}

func (s screen) uploadRow(c *ui.Context, t *ui.Theme, item films.Film) {
	view := s.UploadView(item)
	if !view.Show {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		c.Key("film-upload")
		btn := breeze.Button(c, view.Label).Disabled(!view.Enabled)
		if view.Enabled && btn.Clicked() {
			s.BeginUpload(item)
		}
		if view.Detail != "" {
			ui.Text(c, view.Detail).TextColor(t.TextMuted).FontSize(12).Grow(1).Wrap()
		}
	})
}

func (s screen) filmActions(c *ui.Context, item films.Film) {
	showCancel := item.JobID != "" && item.Stage != "" && !item.Stage.Done()
	showStart := item.JobID == ""
	showFailed := item.JobID != "" && item.Stage == core.StageFailed && !s.Running()
	if !showCancel && !showStart && !showFailed {
		return
	}
	ui.Row(c).Gap(8).Children(func() {
		if showCancel {
			c.Key("film-cancel")
			if breeze.Button(c, "取消").Clicked() {
				s.Stop()
			}
		}
		if showStart {
			c.Key("film-start")
			if breeze.PrimaryButton(c, "开始处理").Disabled(s.Running()).Clicked() {
				s.StartFilm(item)
			}
		}
		if showFailed {
			c.Key("film-remove")
			if breeze.Button(c, "移除").Clicked() {
				s.Remove(item.JobID)
			}
			c.Key("film-retry")
			if breeze.PrimaryButton(c, "重试").Clicked() {
				s.RetryOpen(item.JobID)
			}
		}
	})
}

func (s screen) playerPane(c *ui.Context, t *ui.Theme) {
	if s.Play() == nil && s.PlayPath() == "" && s.PlayErr() == "" {
		return
	}
	stage := ui.Box(c).Grow(1).MinWidth(0).MinHeight(240).Radius(12).Clip().Background(ui.Hex("#000000")).Cursor(ui.CursorPointer)
	if s.Play() != nil && stage.Clicked() {
		s.togglePlay(c)
	}
	stage.Children(func() {
		switch {
		case s.Play() != nil:
			ui.Video(c, s.Play().Widget()).Fill().Fit(ui.Contain)
			s.Play().Settle()
			if !s.Play().Playing() {
				s.playButton(c, t)
			}
			s.playCue(c, t)
			s.playTransport(c, t)
		case s.PlayErr() != "":
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, s.PlayErr()).TextColor(t.Text)
			})
		default:
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, "正在打开视频…").TextColor(t.TextMuted)
			})
		}
	})
}

// togglePlay starts or holds the picture. The button is built after the video, so the frame that handles the click has already drawn a paused frame; the extra frame is what starts the clock.
func (s screen) togglePlay(c *ui.Context) {
	if s.Play() == nil {
		return
	}
	s.Play().Toggle()
	c.Invalidate()
}

// playButton is the round control on the picture. It shows while the video is held.
func (s screen) playButton(c *ui.Context, t *ui.Theme) {
	label := "播放"
	glyph := iconPlay
	if s.Play().Playing() {
		label = "暂停"
		glyph = iconPause
	}
	ui.Row(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Center).Justify(ui.Center).PassThrough().Children(func() {
		btn := ui.Row(c).Size(68, 68).Radius(34).Background(ui.RGBA(180, 180, 180, 0.42)).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(label)
		if btn.Clicked() {
			s.togglePlay(c)
		}
		btn.Children(func() {
			ui.Icon(c, glyph).Size(30, 30).TextColor(t.Text)
		})
	})
}

// playCue draws the current bilingual line on the picture, above the transport.
func (s screen) playCue(c *ui.Context, _ *ui.Theme) {
	if s.Play() == nil || s.Subs().Key != s.OpenKey() || !s.Subs().Ready || s.SubMode() == SubOff {
		return
	}
	cue, ok := CueAt(s.Subs().Cues, s.Play().Head())
	if !ok {
		return
	}
	lines := s.styledLines(cue.Text)
	if len(lines) == 0 {
		return
	}
	block := ui.Column(c).Absolute().Left(32).Right(32).Bottom(s.Cue().Bottom).Gap(2).Padding(8, 14).Radius(8).AlignItems(ui.Center).PassThrough()
	if s.Cue().Box {
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
			if s.Cue().Shadow {
				text.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.95))
			}
		}
	})
}

func (s screen) playTransport(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(16).Right(16).Bottom(14).Gap(12).AlignItems(ui.Center).Children(func() {
		glyph := iconPlay
		btn := ui.Row(c).Size(28, 28).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
		if s.Play().Playing() {
			glyph = iconPause
			btn.Label("暂停")
		}
		if btn.Clicked() {
			s.togglePlay(c)
		}
		btn.Children(func() {
			ui.Icon(c, glyph).Size(16, 16).TextColor(t.Text)
		})
		clock := cueClock(s.Play().Head())
		if s.Play().Length() > 0 {
			clock += " / " + cueClock(s.Play().Length())
		}
		ui.Text(c, clock).FontSize(12).Shrink(0)
		s.playTrack(c, t)
	})
}

func (s screen) playTrack(c *ui.Context, t *ui.Theme) {
	frac := float32(0)
	if s.Play().Length() > 0 {
		frac = float32(s.Play().Head()) / float32(s.Play().Length())
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

func (s screen) subtitlePane(c *ui.Context, t *ui.Theme, item films.Film) {
	pane := ui.Column(c).MinHeight(0).Gap(10)
	if s.Play() != nil || s.PlayPath() != "" || s.PlayErr() != "" {
		pane.Width(340).Shrink(0)
	} else {
		pane.Grow(1)
	}
	pane.Children(func() {
		s.filmTabs(c, t)
		switch s.FilmSide() {
		case "clips":
			s.chapterPane(c, t)
		case "style":
			s.cueStylePane(c, t)
		default:
			s.transcriptPane(c, t, item)
		}
	})
}

func (s screen) filmTabs(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(4).Children(func() {
		for _, tab := range []struct{ id, label string }{
			{"", "文稿"},
			{"clips", "精彩片段"},
			{"style", "字幕样式"},
		} {
			s.filmTab(c, tab.id, tab.label)
		}
	})
}

func (s screen) filmTab(c *ui.Context, id, label string) {
	tab := ui.Row(c).Padding(6, 10).Radius(8).Label(label).Cursor(ui.CursorPointer)
	if s.FilmSide() == id {
		tab.Background(ui.Hex("#4a3824"))
	}
	if tab.Clicked() {
		s.SetFilmSide(id)
	}
	tab.Children(func() {
		ui.Text(c, label).FontSize(13)
	})
}

func (s screen) transcriptPane(c *ui.Context, t *ui.Theme, item films.Film) {
	ready := s.Subs().Key == item.Key && s.Subs().Ready
	if ready && s.Subs().Err == "" && item.SubtitlePath != "" {
		ui.Text(c, fmt.Sprintf("%d 句", len(s.Subs().Cues))).TextColor(t.TextMuted).FontSize(12)
	}
	switch {
	case !ready:
		ui.Text(c, "正在读取字幕…").TextColor(t.TextMuted)
	case s.Subs().Err != "":
		ui.Text(c, s.Subs().Err).TextColor(t.Danger).Wrap()
	case item.SubtitlePath == "":
		ui.Text(c, "还没有字幕。处理完成后会显示在这里。").TextColor(t.TextMuted).Wrap()
	case len(s.Subs().Cues) == 0:
		ui.Text(c, "这份字幕是空的。").TextColor(t.TextMuted)
	default:
		ui.List(c, s.SubList(), len(s.Subs().Cues), func(i int) {
			s.cueRow(c, t, s.Subs().Cues[i])
		}).Grow(1).Gap(14)
	}
}

func (s screen) chapterPane(c *ui.Context, t *ui.Theme) {
	ready := s.Subs().Ready
	chapters := Chapters(s.Subs().Cues)
	if ready && s.Subs().Err == "" && len(chapters) > 0 {
		end := chapters[len(chapters)-1].End
		ui.Text(c, fmt.Sprintf("%d 段 · 共 %s", len(chapters), cueClock(end))).TextColor(t.TextMuted).FontSize(12)
	}
	switch {
	case !ready:
		ui.Text(c, "正在读取字幕…").TextColor(t.TextMuted)
	case s.Subs().Err != "":
		ui.Text(c, s.Subs().Err).TextColor(t.Danger).Wrap()
	case len(chapters) == 0:
		ui.Text(c, "还没有可以分段的字幕。").TextColor(t.TextMuted).Wrap()
	default:
		ui.List(c, s.ClipList(), len(chapters), func(i int) {
			s.chapterRow(c, t, i, chapters[i])
		}).Grow(1).Gap(8)
	}
}

func (s screen) chapterRow(c *ui.Context, t *ui.Theme, index int, ch Chapter) {
	row := ui.Column(c).Padding(8, 10).Radius(8).Gap(4).Cursor(ui.CursorPointer)
	if s.chapterActive(ch) {
		row.Background(ui.Hex("#4a3824"))
	}
	if row.Clicked() {
		s.Seek(ch.Start)
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

func (s screen) chapterActive(ch Chapter) bool {
	if s.Play() == nil {
		return false
	}
	at := s.Play().Head()
	return at >= ch.Start && (ch.End <= ch.Start || at < ch.End)
}

func (s screen) cueStylePane(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).WidthPercent(100).Gap(14).Children(func() {
			ui.Text(c, "预设").TextColor(t.TextMuted).FontSize(12)
			ui.Row(c).Gap(8).Children(func() {
				s.cuePresetCard(c, t, "outline", "描边")
				s.cuePresetCard(c, t, "box", "底框")
				s.cuePresetCard(c, t, "large", "大字")
			})
			ui.Text(c, "文字").TextColor(t.TextMuted).FontSize(12)
			s.cueChoiceRow(c, t, []Choice{{"translation", "译文"}, {"original", "原文"}}, s.Cue().Edit, func(id string) { s.Cue().Edit = id })
			face := s.editedFace()
			s.cueField(c, t, "字体", func() {
				c.Key("cue-font-" + s.Cue().Edit)
				choiceSelect(c, &face.Family, []string{"默认", "苹方", "冬青黑体"}).Width(120)
			})
			s.cueStepper(c, t, "字号", &face.Size, 2, 12, 72)
			s.cueColors(c, t, face)
			ui.Row(c).MinHeight(36).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "加粗").Grow(1)
				breeze.Switch(c, &face.Bold).Label("加粗")
			})
			if breeze.Button(c, "使用原文样式").Clicked() {
				s.Cue().Translation = s.Cue().Original
			}
			ui.Text(c, "双语").TextColor(t.TextMuted).FontSize(12)
			s.cueChoiceRow(c, t, []Choice{{SubTranslationFirst, "译文在上"}, {SubOriginalFirst, "原文在上"}, {SubTranslation, "只要译文"}}, s.linesChoice(), s.chooseLines)
			ui.Text(c, "位置").TextColor(t.TextMuted).FontSize(12)
			s.cueStepper(c, t, "高底部", &s.Cue().Bottom, 4, 24, 160)
			if breeze.Button(c, "恢复默认样式").Clicked() {
				*s.Cue() = DefaultLook()
				s.chooseLines(SubTranslationFirst)
			}
		})
	})
}

func (s screen) cuePresetCard(c *ui.Context, t *ui.Theme, id, label string) {
	card := ui.Column(c).Width(96).Grow(1).Padding(10, 6).Radius(10).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label)
	if s.Cue().Preset == id {
		card.Background(ui.Hex("#4a3824"))
	} else {
		card.Background(t.Surface)
	}
	if card.Clicked() {
		s.applyCuePreset(id)
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

type Choice struct {
	id, label string
}

func (s screen) cueChoiceRow(c *ui.Context, t *ui.Theme, choices []Choice, current string, set func(string)) {
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

func (s screen) cueField(c *ui.Context, t *ui.Theme, label string, control func()) {
	ui.Row(c).MinHeight(36).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1)
		_ = t
		control()
	})
}

func (s screen) cueStepper(c *ui.Context, t *ui.Theme, label string, value *float32, step, min, max float32) {
	ui.Row(c).MinHeight(36).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1)
		if breeze.Button(c, "−").Label(label+"减小").Clicked() && *value > min {
			*value -= step
		}
		ui.Text(c, fmt.Sprintf("%.0f", *value)).FontSize(13)
		if breeze.Button(c, "+").Label(label+"增大").Clicked() && *value < max {
			*value += step
		}
		_ = t
	})
}

func (s screen) cueColors(c *ui.Context, t *ui.Theme, face *Face) {
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

func (s screen) cueRow(c *ui.Context, t *ui.Theme, seg core.Segment) {
	lines := s.styledLines(seg.Text)
	row := ui.Column(c).Gap(2).Padding(8, 10).Radius(8)
	if s.cueActive(seg) {
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

func (s screen) cueActive(seg core.Segment) bool {
	if s.Play() == nil || s.Subs().Key != s.OpenKey() || !s.Subs().Ready {
		return false
	}
	cur, ok := CueAt(s.Subs().Cues, s.Play().Head())
	return ok && cur.Start == seg.Start && cur.Text == seg.Text
}

type Chapter struct {
	Start, End    time.Duration
	Title, Detail string
}

// Chapters groups the transcript into a handful of stretches, using the lines already in the file.
func Chapters(cues []core.Segment) []Chapter {
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
	out := make([]Chapter, 0, want)
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

func chapterFrom(group []core.Segment) Chapter {
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
	return Chapter{Start: group[0].Start, End: end, Title: films.Shorten(title, 36), Detail: films.Shorten(detail, 48)}
}

type Face struct {
	Family string
	Size   float32
	Color  string
	Bold   bool
}

// Look is how subtitles are drawn. Which lines show and which goes on top are not part of it: they are the app's SubMode and SubOrder.
type Look struct {
	Preset, Edit string
	Bottom       float32
	Box, Shadow  bool
	Translation  Face
	Original     Face
}

// Subtitle modes: which lines of a bilingual cue the player shows. V steps through them in this order.
const (
	SubBilingual   = "bilingual"
	SubTranslation = "translation"
	SubOriginal    = "original"
	SubOff         = "off"
)

// Subtitle orders: which line of a bilingual cue goes on top.
const (
	SubTranslationFirst = "translation-first"
	SubOriginalFirst    = "original-first"
)

// What the player shows for a cue that has no translation yet.
const (
	UntranslatedDim   = "dim"
	UntranslatedBlank = "blank"
)

type styledLine struct {
	text string
	face Face
}

func DefaultLook() Look {
	return Look{
		Preset: "box", Edit: "translation", Bottom: 56, Box: true,
		Translation: Face{Family: "默认", Size: 18, Color: "#f6f1e8", Bold: true},
		Original:    Face{Family: "默认", Size: 14, Color: "#b3a38f"},
	}
}

func (s screen) applyCuePreset(id string) {
	s.Cue().Preset = id
	switch id {
	case "outline":
		s.Cue().Box, s.Cue().Shadow = false, true
		s.Cue().Translation.Size, s.Cue().Original.Size = 18, 14
	case "large":
		s.Cue().Box, s.Cue().Shadow = true, false
		s.Cue().Translation.Size, s.Cue().Original.Size = 32, 22
	default:
		s.Cue().Preset = "box"
		s.Cue().Box, s.Cue().Shadow = true, false
		s.Cue().Translation.Size, s.Cue().Original.Size = 18, 14
	}
}

func (s screen) editedFace() *Face {
	if s.Cue().Edit == "original" {
		return &s.Cue().Original
	}
	return &s.Cue().Translation
}

// styledLines is the lines of a cue as SubMode and SubOrder pick them, for the picture and the transcript. SubOff counts as bilingual here: it hides only the line on the picture (playCue), and the transcript keeps both.
func (s screen) styledLines(text string) []styledLine {
	translation, original := splitCue(text)
	add := func(lines []styledLine, s string, face Face) []styledLine {
		if s == "" {
			return lines
		}
		return append(lines, styledLine{text: s, face: face})
	}
	switch s.SubMode() {
	case SubTranslation:
		lines := add(nil, translation, s.Cue().Translation)
		if len(lines) == 0 {
			lines = add(lines, original, s.Cue().Original)
		}
		return lines
	case SubOriginal:
		return add(nil, original, s.Cue().Original)
	}
	if s.SubOrder() == SubOriginalFirst {
		return add(add(nil, original, s.Cue().Original), translation, s.Cue().Translation)
	}
	return add(add(nil, translation, s.Cue().Translation), original, s.Cue().Original)
}

// linesChoice is the chip of the 双语 row that is on: 只要译文 in SubTranslation mode, else the order.
func (s screen) linesChoice() string {
	if s.SubMode() == SubTranslation {
		return SubTranslation
	}
	return s.SubOrder()
}

// chooseLines applies a chip of the 双语 row: 只要译文 switches to SubTranslation, an order shows both lines in that order.
func (s screen) chooseLines(id string) {
	if id == SubTranslation {
		s.SetSubMode(SubTranslation)
		return
	}
	s.SetSubOrder(id)
	s.SetSubMode(SubBilingual)
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

// CueAt is the subtitle line covering at. A zero end means the line stays until the next one.
func CueAt(cues []core.Segment, at time.Duration) (core.Segment, bool) {
	var found core.Segment
	ok := false
	for _, cue := range cues {
		if cue.Start > at {
			break
		}
		if cue.End > at || cue.End == 0 {
			found = cue
			ok = true
		}
	}
	return found, ok
}

func choiceSelect(c *ui.Context, selected *string, options []string) ui.Element {
	return breeze.Select(c, selected, options)
}
