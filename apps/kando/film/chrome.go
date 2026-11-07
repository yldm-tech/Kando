package film

import (
	"fmt"
	"math"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/sidebar"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// topBar is the player's title bar over the picture, after the prototype's (template 556-579): the back button and the film's actions on the glass of the player's small buttons, and between them the film's title over where it stands. It is transparent, so the picture shows through it, and moves the window when dragged. It keeps clear of the traffic lights when it lies under them, and in the content column beside the sidebar it starts at the column's edge, the traffic lights being over the sidebar. It fades with the transport.
func (s screen) topBar(c *ui.Context, item films.Film, fade float32, shown bool) {
	c.Key("film-top")
	bar := ui.Row(c).Absolute().Left(0).Top(0).Right(0).Gap(12).AlignItems(ui.Center).TextColor(onGlass).Opacity(fade).DragWindow().Role(ui.RoleGroup).Label(title(item))
	bar.Padding(14, 16, 0, barStart(c, bar))
	if !shown {
		bar.PassThrough()
	}
	bar.Children(func() {
		c.Key("film-back")
		back := glyphButton(c, iconChevronLeft, "返回资料库", 34, 34, 10, 16)
		glassFace(back, ui.RGBA(255, 255, 255, 0.12), 20, 0.25, 17)
		if back.Clicked() {
			s.SetOpenKey("")
			s.StopPlayback()
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, title(item)).FontSize(14).Bold().SingleLine()
			ui.Text(c, s.status(item)).FontSize(11).Opacity(0.8).SingleLine()
		})
		s.actions(c, item)
	})
}

// barStart is where the title bar's content starts: 87 points in, 7 past the room of the traffic lights (MyGo's 80 points), where the prototype puts the back button after its traffic lights, when the bar lies under them; else 20, the prototype's padding. It reads where the previous frame laid the bar out, as a bar in the content column beside the sidebar is not under them, and asks for another frame when the bar is drawn where it lies under other room than it kept clear of, as breeze's HeaderBar does.
func barStart(c *ui.Context, bar ui.Element) float32 {
	room := c.TitleBar()
	under := func(r ui.Rect) float32 {
		if r.Y >= room.Height {
			return 0
		}
		return max(room.Left-r.X, 0)
	}
	kept := room.Left
	if r := bar.LayoutBounds(); r.W > 0 || r.H > 0 {
		kept = under(r)
	}
	s := c.Services()
	bar.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		if under(r) != kept {
			s.Invalidate()
		}
	})
	if kept > 0 {
		return kept + 7
	}
	return 20
}

// status is the line under the title: the library, then how much of the film's subtitles is translated, as the prototype says it ("字幕 94%"), or how far its job is while it has no subtitles to show.
func (s screen) status(item films.Film) string {
	parts := []string{}
	if dir := s.LibraryDir(); dir != "" {
		parts = append(parts, sidebar.Title(dir))
	}
	if subs := s.Subs(); subs.Key == item.Key && subs.Ready && subs.Err == "" && len(subs.Cues) > 0 {
		parts = append(parts, fmt.Sprintf("字幕 %d%%", translatedPercent(subs.Cues)))
	} else if item.Stage != "" {
		parts = append(parts, films.StageLabel(item.Stage))
	}
	return strings.Join(parts, " · ")
}

// translatedPercent is the share of cues that have a translation, rounded to a whole percent.
func translatedPercent(cues []core.Segment) int {
	n := 0
	for _, cue := range cues {
		if translation, _ := splitCue(cue.Text); translation != "" {
			n++
		}
	}
	return int(math.Round(float64(n) * 100 / float64(len(cues))))
}

// actions are the film's buttons at the end of the title bar: the job's own (start, cancel, retry), then 在访达中显示, the upload and the ⋯ menu. The job buttons act on this film's job only and do not wait for whatever else runs: a job started or retried while another runs waits in the queue behind it.
func (s screen) actions(c *ui.Context, item films.Film) {
	running := item.JobID != "" && item.Stage != "" && !item.Stage.Done()
	failed := item.JobID != "" && item.Stage == core.StageFailed
	switch {
	case running:
		c.Key("film-cancel")
		if breeze.Button(c, "取消").Clicked() {
			s.Cancel(item.JobID)
		}
	case item.JobID == "":
		c.Key("film-start")
		if breeze.PrimaryButton(c, "开始处理").Clicked() {
			s.StartFilm(item)
		}
	case failed:
		c.Key("film-remove")
		if breeze.Button(c, "移除").Clicked() {
			s.Remove(item.JobID)
		}
		c.Key("film-retry")
		if breeze.PrimaryButton(c, "重试").Clicked() {
			s.RetryOpen(item.JobID)
		}
	}
	path := item.SubtitlePath
	if path == "" {
		path = item.VideoPath
	}
	upload := s.UploadView(item)
	more := item.JobID != "" && item.Stage == core.StageComplete
	if path == "" && !upload.Show && !more {
		return
	}
	// The film's own buttons share a capsule of glass, 34 high, as the prototype's 在访达中显示, 导出, 上传字幕 and ⋯.
	capsule := ui.Row(c).Height(34).Padding(0, 3).AlignItems(ui.Center).Shrink(0)
	glassFace(capsule, ui.RGBA(255, 255, 255, 0.12), 20, 0.25, 17).Children(func() {
		if path != "" {
			c.Key("film-reveal")
			if glyphButton(c, iconFolder, "在访达中显示", 34, 28, 18, 16).Clicked() {
				s.Reveal(path)
			}
		}
		if upload.Show {
			c.Key("film-upload")
			btn := breeze.FlatButton(c, "").Label(upload.Label).Height(28).Padding(0, 10).Gap(0).TextColor(onGlass)
			btn.Children(func() {
				ui.Icon(c, iconUpload).Size(15, 17).Shrink(0).Margin(0, 6, 0, 0)
				ui.Text(c, upload.Label).FontSize(13).SingleLine()
			})
			if btn.Clicked() {
				*s.UploadNote() = true
				if upload.Enabled {
					s.BeginUpload(item)
				}
			}
		}
		// A finished film can be processed again from the start, as the prototype's ⋯ menu offers; a failed one has 重试 already.
		if more {
			c.Key("film-more")
			glyphButton(c, iconMore, "更多操作", 34, 28, 14, 4).Menu(func(m *ui.Menu) {
				if m.Item("反馈这部影片的字幕…").Chosen() {
					s.FeedbackSubtitles()
				}
				if m.Item("重新处理整部影片").Chosen() {
					s.RetryOpen(item.JobID)
				}
			})
		}
	})
}
