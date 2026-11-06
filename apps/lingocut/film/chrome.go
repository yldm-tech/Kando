package film

import (
	"fmt"
	"strings"

	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// topBar is the player's title bar over the picture: the back button, the film's title and where it stands, and the film's actions. It is the platform's header bar without its face, so the picture shows through it; it keeps clear of the window controls and moves the window when dragged. In the content column beside the sidebar it is paneHeaderBar, which starts at the column's edge. It fades with the transport.
func (s screen) topBar(c *ui.Context, item films.Film, fade float32, shown bool) {
	holder := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Opacity(fade)
	if !shown {
		holder.PassThrough()
	}
	holder.Children(func() {
		c.Key("film-top")
		header := breeze.HeaderBar
		if !s.FillsWindow() {
			header = paneHeaderBar
		}
		bar := header(c, title(item), s.status(item), func() {
			c.Key("film-back")
			if flatIconButton(c, iconChevronLeft, "返回资料库").Clicked() {
				s.SetOpenKey("")
				s.StopPlayback()
			}
		}, func() {
			s.actions(c, item)
		})
		bar.Background(ui.Transparent).Draw(func(*ui.Painter, ui.Rect) {})
	})
}

// status is the line under the title: the library, then how far the film's job is and how many lines its subtitles have.
func (s screen) status(item films.Film) string {
	parts := []string{}
	if dir := s.LibraryDir(); dir != "" {
		parts = append(parts, sidebar.Title(dir))
	}
	if item.Stage != "" {
		parts = append(parts, films.StageLabel(item.Stage))
	}
	if subs := s.Subs(); subs.Key == item.Key && subs.Ready && subs.Err == "" && len(subs.Cues) > 0 {
		parts = append(parts, fmt.Sprintf("%d 句", len(subs.Cues)))
	}
	return strings.Join(parts, " · ")
}

// actions are the film's buttons at the end of the title bar: the job's own (start, cancel, retry), then 在访达中显示, the upload and the ⋯ menu.
func (s screen) actions(c *ui.Context, item films.Film) {
	running := item.JobID != "" && item.Stage != "" && !item.Stage.Done()
	failed := item.JobID != "" && item.Stage == core.StageFailed && s.RunningJob() == ""
	switch {
	case running:
		c.Key("film-cancel")
		if breeze.Button(c, "取消").Clicked() {
			s.Cancel(item.JobID)
		}
	case item.JobID == "":
		c.Key("film-start")
		if breeze.PrimaryButton(c, "开始处理").Disabled(s.RunningJob() != "").Clicked() {
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
	if path != "" {
		c.Key("film-reveal")
		if flatIconButton(c, iconFolder, "在访达中显示").Clicked() {
			s.Reveal(path)
		}
	}
	if view := s.UploadView(item); view.Show {
		c.Key("film-upload")
		if iconLabelButton(c, nil, view.Label).Clicked() {
			*s.UploadNote() = true
			if view.Enabled {
				s.BeginUpload(item)
			}
		}
	}
	// A finished film can be processed again from the start, as the prototype's ⋯ menu offers; a failed one has 重试 already.
	if item.JobID != "" && item.Stage == core.StageComplete && s.RunningJob() == "" {
		c.Key("film-more")
		flatIconButton(c, iconMore, "更多操作").Menu(func(m *ui.Menu) {
			if m.Item("重新处理整部影片").Chosen() {
				s.RetryOpen(item.JobID)
			}
		})
	}
}
