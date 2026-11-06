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

// panelTabs are the side panel's tabs, by the FilmSide each one is.
var panelTabs = []struct{ id, label string }{
	{"", "字幕"},
	{"clips", "片段"},
	{"info", "信息"},
}

// The side panel's surfaces (prototype template 642-876).
var (
	panelCard    = ui.RGBA(255, 255, 255, 0.07)
	panelCurrent = ui.RGBA(255, 255, 255, 0.14)
	panelLine    = ui.RGBA(255, 255, 255, 0.1)
)

// panel is the glass side panel floating over the right of the picture: the subtitle list, the 片段 and the film's files under three tabs, or the subtitle style in their place.
func (s screen) panel(c *ui.Context, t *ui.Theme, item films.Film) {
	box := ui.Column(c).Absolute().Right(16).Top(62).Bottom(18).Width(panelWidth).Radius(22).Clip().Role(ui.RoleGroup).Label("侧栏")
	glassFace(box, ui.RGBA(28, 28, 32, 0.6), 34, 0.2).Shadow(0, 20, 50, 0, ui.RGBA(0, 0, 0, 0.35))
	box.Children(func() {
		if s.StyleView() {
			s.styleView(c)
			return
		}
		ui.Column(c).Padding(14, 14, 10, 14).Children(func() {
			chosen := 0
			for i, tab := range panelTabs {
				if tab.id == s.FilmSide() {
					chosen = i
				}
			}
			c.Key("film-tabs")
			labels := make([]string, len(panelTabs))
			for i, tab := range panelTabs {
				labels[i] = tab.label
			}
			breeze.Tabs(c, &chosen, labels...).Background(ui.RGBA(255, 255, 255, 0.1))
			if panelTabs[chosen].id != s.FilmSide() {
				s.SetFilmSide(panelTabs[chosen].id)
			}
		})
		switch s.FilmSide() {
		case "clips":
			s.clipsTab(c, t)
		case "info":
			s.infoTab(c, item)
		default:
			s.subsTab(c, t, item)
		}
	})
}

// subsTab lists the cues with the time each starts, the playing one lit and kept in view as it changes. A click plays from a cue. The field above it searches both lines.
func (s screen) subsTab(c *ui.Context, t *ui.Theme, item films.Film) {
	subs := s.Subs()
	ready := subs.Key == item.Key && subs.Ready
	ui.Column(c).Padding(0, 14, 8, 14).Gap(8).Children(func() {
		c.Key("film-search")
		field := breeze.SearchField(c, s.Search()).Placeholder("搜索台词  " + breeze.ShortcutText(c, ui.Cmd, ui.KeyF)).Label("搜索台词")
		if focus := s.SearchFocus(); *focus {
			field.Focus()
			*focus = false
		}
		if ready && subs.Err == "" && len(subs.Cues) > 0 {
			breeze.Text(c, fmt.Sprintf("%d 句", len(subs.Cues)), breeze.TextCaption|breeze.TextSecondary)
		}
	})
	body := ui.Column(c).Grow(1).MinHeight(0).Padding(0, 8, 12, 8)
	body.Children(func() {
		switch {
		case !ready:
			panelNote(c, "正在读取字幕…")
		case subs.Err != "":
			panelNote(c, subs.Err).TextColor(t.Danger)
		case item.SubtitlePath == "":
			panelNote(c, "还没有字幕。处理完成后会显示在这里。")
		case len(subs.Cues) == 0:
			panelNote(c, "这份字幕是空的。")
		default:
			s.cueList(c, subs.Cues)
		}
	})
	ui.Row(c).Padding(8, 14, 12, 14).BorderWidth(0.5, 0, 0, 0).BorderColor(panelLine).Children(func() {
		breeze.Text(c, "单击跳到该句", breeze.TextCaption|breeze.TextSecondary)
	})
}

// cueList is the subtitle list, narrowed to the cues that hold the search.
func (s screen) cueList(c *ui.Context, cues []core.Segment) {
	query := strings.ToLower(strings.TrimSpace(*s.Search()))
	shown := make([]int, 0, len(cues))
	for i, cue := range cues {
		if query == "" || strings.Contains(strings.ToLower(cue.Text), query) {
			shown = append(shown, i)
		}
	}
	if len(shown) == 0 {
		panelNote(c, "没有符合条件的字幕。").TextAlign(ui.Center)
		return
	}
	current := -1
	if s.Play() != nil {
		current = cueIndex(cues, s.Play().Head()-s.Delay())
	}
	list := s.SubList()
	if follow := s.Follow(); current != *follow {
		*follow = current
		for row, i := range shown {
			if i == current {
				list.ScrollIntoView(row)
			}
		}
	}
	ui.List(c, list, len(shown), func(row int) {
		i := shown[row]
		s.cueRow(c, cues[i], i == current)
	}).Grow(1).Gap(2)
}

// cueRow is one cue of the list: its start, the translation and the original under it. A click plays from its start.
func (s screen) cueRow(c *ui.Context, cue core.Segment, current bool) {
	translation, original := splitCue(cue.Text)
	if translation == "" {
		translation, original = original, ""
	}
	row := ui.ButtonBase(c).Row().Gap(10).Padding(8, 10).Radius(10).AlignItems(ui.Start).Label(cueClock(cue.Start) + " " + translation)
	if current {
		row.Background(panelCurrent)
	} else if row.Hovered() {
		row.Background(panelLine)
	}
	if row.Clicked() {
		s.Seek(cue.Start + s.Delay())
	}
	row.Children(func() {
		ui.Text(c, cueClock(cue.Start)).FontSize(11).FontFeatures("tnum").Opacity(0.7).Width(38).Shrink(0).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			main := ui.Text(c, translation).FontSize(14).Wrap()
			if current {
				main.FontWeight(600)
			}
			if original != "" {
				ui.Text(c, original).FontSize(12).Opacity(0.7).Wrap()
			}
		})
	})
}

// clipsTab is the film cut into 片段 of equal time; a click plays from one, and the playing one is lit.
func (s screen) clipsTab(c *ui.Context, t *ui.Theme) {
	subs := s.Subs()
	chapters := Chapters(subs.Cues, s.PlayLength())
	ui.Column(c).Grow(1).MinHeight(0).Padding(0, 14, 14, 14).Gap(8).Children(func() {
		switch {
		case !subs.Ready:
			panelNote(c, "正在读取字幕…")
		case subs.Err != "":
			panelNote(c, subs.Err).TextColor(t.Danger)
		case len(chapters) == 0:
			panelNote(c, "还没有可以分段的字幕。")
		default:
			breeze.Text(c, "按时间把字幕平均切成 4–12 段，每段约 30 秒。", breeze.TextCaption|breeze.TextSecondary).Wrap()
			ui.List(c, s.ClipList(), len(chapters), func(i int) {
				s.chapterRow(c, i, chapters[i])
			}).Grow(1).Gap(8)
		}
	})
}

func (s screen) chapterRow(c *ui.Context, index int, ch Chapter) {
	active := s.chapterActive(ch)
	row := ui.ButtonBase(c).Row().Gap(12).Padding(8).Radius(12).AlignItems(ui.Center).Label(fmt.Sprintf("片段 %d", index+1))
	if active {
		row.Background(ui.RGBA(255, 255, 255, 0.16))
	} else if row.Hovered() {
		row.Background(panelLine)
	}
	if row.Clicked() {
		s.Seek(ch.Start)
	}
	row.Children(func() {
		ui.Box(c).Width(84).AspectRatio(16.0/9).Radius(7).Shrink(0).Background(clipTiles[index%len(clipTiles)]).InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.15)).Children(func() {
			ui.Text(c, cueClock(ch.Start)).FontSize(10).FontWeight(600).FontFeatures("tnum").Absolute().Right(4).Bottom(3)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, fmt.Sprintf("片段 %d · %s–%s", index+1, cueClock(ch.Start), cueClock(ch.End))).FontSize(11).FontWeight(600).Opacity(0.7)
			text := ui.Text(c, ch.Title).FontSize(13).Wrap()
			if active {
				text.Bold()
			}
			if ch.Detail != "" {
				ui.Text(c, ch.Detail).FontSize(11).Opacity(0.6).Wrap()
			}
		})
	})
}

// clipTiles are the colors of the 片段's tiles, the prototype's stand-ins for a frame of each.
var clipTiles = []ui.Color{ui.Hex("#3E6A5B"), ui.Hex("#6A8EA1"), ui.Hex("#4B7A8C"), ui.Hex("#5D7F63"), ui.Hex("#7C8F6B"), ui.Hex("#2E5B6E")}

func (s screen) chapterActive(ch Chapter) bool {
	if s.Play() == nil {
		return false
	}
	at := s.Play().Head()
	return at >= ch.Start && (ch.End <= ch.Start || at < ch.End)
}

// infoTab shows where the film is: its folder, the files in it and where it came from.
func (s screen) infoTab(c *ui.Context, item films.Film) {
	info := s.Info()
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).WidthPercent(100).Padding(0, 14, 16, 14).Gap(8).Children(func() {
			if info.Folder != "" {
				panelHeading(c, "影片文件夹")
				panelGroup(c, func() {
					ui.Column(c).Padding(10, 12).Gap(8).Children(func() {
						ui.Text(c, films.DisplayPath(info.Folder)).FontSize(12).Font("Menlo").Opacity(0.9).Wrap()
						c.Key("film-info-reveal")
						if breeze.Button(c, "在访达中显示").AlignSelf(ui.Start).Clicked() {
							s.Reveal(item.VideoPath)
						}
					})
				})
			}
			if len(info.Files) > 0 {
				panelHeading(c, "文件")
				panelGroup(c, func() {
					for i, f := range info.Files {
						fileRow(c, f, i > 0)
					}
				})
			}
			panelHeading(c, "来源")
			panelGroup(c, func() {
				ui.Column(c).Padding(10, 12).Gap(6).Children(func() {
					source := item.Source
					if source == "" || strings.HasPrefix(source, "file://") {
						source = "本地文件"
					}
					ui.Text(c, source).FontSize(12).Opacity(0.9).Wrap()
					ui.Text(c, "任务记录保存在 ~/Library/Application Support/LingoCut/library.json").FontSize(12).Opacity(0.65).Wrap()
				})
			})
		})
	})
}

// fileRow is a file of the film's folder: a tile with its kind, its name and its size.
func fileRow(c *ui.Context, f File, divided bool) {
	row := ui.Row(c).Gap(10).MinHeight(44).Padding(6, 12).AlignItems(ui.Center)
	if divided {
		row.BorderWidth(0.5, 0, 0, 0).BorderColor(panelLine)
	}
	row.Children(func() {
		tag, color := fileTag(f.Name)
		ui.Box(c).Size(28, 28).Radius(7).Shrink(0).Background(color).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, tag).FontSize(9).FontWeight(800)
		})
		ui.Column(c).MinWidth(0).Children(func() {
			ui.Text(c, f.Name).FontSize(12).Font("Menlo").Wrap()
			ui.Text(c, fileSize(f.Size)).FontSize(11).Opacity(0.65)
		})
	})
}

// fileTag is the kind a file's tile shows and its color, after the prototype's (script 3261-3266).
func fileTag(name string) (string, ui.Color) {
	ext := strings.ToUpper(strings.TrimPrefix(fileExt(name), "."))
	switch ext {
	case "SRT", "VTT", "ASS":
		return ext, ui.Hex("#6B5A3A")
	case "JPG", "JPEG", "PNG", "WEBP":
		return "JPG", ui.Hex("#4C6B4C")
	case "":
		return "—", ui.Hex("#4A4A50")
	}
	if len(ext) > 4 {
		ext = ext[:4]
	}
	return ext, ui.Hex("#3A5A8C")
}

func fileExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[i:]
	}
	return ""
}

func fileSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d 字节", n)
}

func panelHeading(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(11).FontWeight(700).Opacity(0.6).Padding(4, 4, 0, 4)
}

// panelGroup is a card of the panel, a lighter glass holding rows.
func panelGroup(c *ui.Context, fn func()) {
	ui.Column(c).Radius(14).Background(panelCard).Children(fn)
}

func panelNote(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(12).Opacity(0.6).Wrap().Margin(30, 10)
}

// Chapter is one of the 片段 the film is cut into.
type Chapter struct {
	Start, End    time.Duration
	Title, Detail string
}

// Chapters cuts the film into 4 to 12 片段 of equal time, about 30 seconds each, from the start to length, or to the last cue's end when that is later or length is not known. Each is named after the cue playing as it starts, or the first one after it: the translation, with the original as its detail (prototype script 3248-3259).
func Chapters(cues []core.Segment, length time.Duration) []Chapter {
	if len(cues) == 0 {
		return nil
	}
	end := max(length, cues[len(cues)-1].End, cues[len(cues)-1].Start)
	if end <= 0 {
		return nil
	}
	n := int((end + 15*time.Second) / (30 * time.Second))
	n = min(max(n, 4), 12)
	out := make([]Chapter, 0, n)
	for k := range n {
		from := end * time.Duration(k) / time.Duration(n)
		to := end * time.Duration(k+1) / time.Duration(n)
		out = append(out, chapterAt(cues, from, to))
	}
	return out
}

// chapterAt is the 片段 from from to to, named after the cue that started last by from, or the first one.
func chapterAt(cues []core.Segment, from, to time.Duration) Chapter {
	cue := cues[0]
	for _, c := range cues {
		if c.Start > from {
			break
		}
		cue = c
	}
	title, detail := splitCue(cue.Text)
	if title == "" {
		title, detail = detail, ""
	}
	return Chapter{Start: from, End: to, Title: films.Shorten(title, 36), Detail: films.Shorten(detail, 48)}
}
