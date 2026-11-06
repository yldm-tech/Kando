package films

import (
	"strings"
	"time"

	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// App is the shelf the films page reads and the clicks it reports.
type App interface {
	LibraryDir() string
	MakingLibrary() bool
	SaveErr() string
	Page() string
	OpenKey() string
	SetOpenKey(string)
	Source() *string
	Status() string
	HasJob() bool
	PlaybackOpen() bool
	StopPlayback()
	PickLibrary()
	CancelMaking()
	LibraryPlace() string
	PickLibraryParent()
	CreateLibrary()
	BindName(edit func(*string) bool) error
	Start()
	PickVideos()
	Poster(path string) ui.ImageSource
	EnsurePoster(path string)
	Open(item Film)
	Remove(id string)
	Retry(id string)
	JobFilm(id string) (Film, bool)
	// SourceFilter is the sidebar source the page shows, sidebar.SourceAll or another sidebar.Source value.
	SourceFilter() string
	// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name, and runs them one after another. The error is why a source was not added.
	Enqueue(sources ...string) error
	// Cancel stops job id if it is waiting or running. Its card stays, marked 已取消. Enqueue, Cancel and RunningJob are the contract of the job queue; queue.go in the main package says how much of it runs today. Job control is per job: a page cancels the job it shows, never "whatever runs".
	Cancel(id string)
	// RunningJob is the ID of the job being processed now, or "" when none is. A job waiting in the queue is not running; its Film.Stage is core.StageQueued.
	RunningJob() string
	// OpenSettings shows the settings at section, one of the settings package's Section values.
	OpenSettings(section string)
	// ResumeAt is where the video at path stopped playing last time, or 0.
	ResumeAt(path string) time.Duration
}

// Show draws the shelf. showFilm draws the open film; the page stays here so the shelf does not import the player.
func Show(c *ui.Context, t *ui.Theme, a App, items []Film, showFilm func(*ui.Context, *ui.Theme, Film)) {
	if a.LibraryDir() == "" || a.MakingLibrary() {
		welcome(c, t, a)
		return
	}
	if key := a.OpenKey(); key != "" {
		if item, ok := Find(items, key); ok {
			showFilm(c, t, item)
			return
		}
		if item, ok := a.JobFilm(key); ok {
			showFilm(c, t, item)
			return
		}
		a.SetOpenKey("")
	}
	if a.OpenKey() == "" && a.PlaybackOpen() {
		a.StopPlayback()
	}
	running := a.RunningJob() != ""
	ui.Row(c).Wrap().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MinWidth(240).Height(40).Radius(20).Background(t.Surface).Padding(0, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, linkIcon).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
			field := ui.TextInputBase(c, a.Source()).Placeholder("粘贴视频链接，或把本地视频拖进来").Label("视频链接").Grow(1).Disabled(running)
			if field.Submitted() && !running {
				a.Start()
			}
		})
		if running {
			c.Key("cancel")
			if breeze.Button(c, "取消").Shrink(0).Clicked() {
				a.Cancel(a.RunningJob())
			}
		} else {
			c.Key("files")
			if breeze.Button(c, "本地文件").Shrink(0).Clicked() {
				a.PickVideos()
			}
			c.Key("start")
			if breeze.PrimaryButton(c, "开始处理").Shrink(0).Clicked() {
				a.Start()
			}
		}
	})
	if !a.HasJob() && a.Status() != "" {
		color := t.Danger
		if a.Status() == "已选择本地视频" {
			color = t.TextMuted
		}
		ui.Text(c, a.Status()).TextColor(color).Wrap()
	}
	source := a.SourceFilter()
	if a.Page() == sidebar.PageYouTube {
		source = sidebar.SourceYouTube
	}
	shown := BySource(items, source)
	if len(shown) == 0 {
		ui.Text(c, "还没有影片").TextColor(t.TextMuted)
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		width, _ := c.Size()
		ui.Grid(c).Columns(Columns(width)).Gap(sidebar.CardGap).AlignItems(ui.Start).Children(func() {
			for _, item := range shown {
				card(c, t, a, item)
			}
		})
	})
}

func welcome(c *ui.Context, t *ui.Theme, a App) {
	title := "先准备一个资料库"
	if a.LibraryDir() != "" {
		title = "新建资料库"
	}
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(440).Gap(14).Children(func() {
			ui.Text(c, title).FontSize(28).Bold()
			ui.Text(c, "一个资料库是一个 .lingocut 文件夹，里面每个视频一个文件夹，访达里会带上图标。建好后从左上角的菜单切换。").TextColor(t.TextMuted).Wrap()
			draft(c, t, a, "创建资料库")
			if a.LibraryDir() == "" {
				ui.Text(c, "已经有资料库").TextColor(t.TextMuted).FontSize(12)
				if breeze.Button(c, "导入已有资料库").Clicked() {
					a.PickLibrary()
				}
			} else if breeze.Button(c, "取消新建").Clicked() {
				a.CancelMaking()
			}
			if a.SaveErr() != "" {
				ui.Text(c, a.SaveErr()).TextColor(t.Danger).Wrap()
			}
		})
	})
}

func draft(c *ui.Context, t *ui.Theme, a App, confirm string) {
	ui.Text(c, "名称").TextColor(t.TextMuted).FontSize(12)
	if err := a.BindName(func(value *string) bool {
		return breeze.TextInput(c, value).Label("资料库名称").Placeholder("影视库").Changed()
	}); err != nil {
		ui.Text(c, err.Error()).TextColor(t.Danger)
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "位置").TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, DisplayPath(a.LibraryPlace())).Grow(1).TextColor(t.Text)
		if breeze.Button(c, "更改位置").Clicked() {
			a.PickLibraryParent()
		}
	})
	if breeze.PrimaryButton(c, confirm).Clicked() {
		a.CreateLibrary()
	}
}

func card(c *ui.Context, t *ui.Theme, a App, item Film) {
	a.EnsurePoster(item.VideoPath)
	col := ui.Column(c).WidthPercent(100).Gap(8).Cursor(ui.CursorPointer)
	if col.Clicked() {
		a.Open(item)
	}
	col.Children(func() {
		frame := ui.Box(c).Height(168).Radius(10).Clip().Background(posterColor(item.Key))
		frame.Children(func() {
			if img := a.Poster(item.VideoPath); img != nil {
				ui.Image(c, img).WidthPercent(100).Height(168).Fit(ui.Cover)
			}
			ui.Column(c).Absolute().Left(10).Right(10).Bottom(10).Gap(6).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if label := posterLabel(item); label != "" {
						color := onPoster
						if item.Stage == core.StageFailed && label != "已取消" {
							color = t.Danger
						}
						ui.Text(c, label).TextColor(color).Bold()
					}
					if item.JobID != "" && item.Stage == core.StageFailed && a.RunningJob() == "" {
						if breeze.Button(c, "移除").Clicked() {
							a.Remove(item.JobID)
						}
						if breeze.PrimaryButton(c, "重试").Clicked() {
							a.Retry(item.JobID)
						}
					}
				})
				if note := posterNote(item); note != "" {
					ui.Text(c, note).FontSize(12).TextColor(onPoster).Wrap()
				}
			})
		})
		if item.JobID != "" && item.Stage == core.StageComplete {
			ui.Text(c, "翻译").TextColor(t.Accent).FontSize(12)
		}
		name := item.Name
		if name == "" {
			name = item.Title
		}
		ui.Text(c, name).Wrap()
	})
}

// onPoster is the text on a poster, which is a picture or a dark color in either theme.
var onPoster = ui.Hex("#F5F5F7")

func posterLabel(item Film) string {
	if item.Stage == core.StageFailed && item.Detail == "已取消" {
		return "已取消"
	}
	if item.Stage == "" {
		return ""
	}
	return StageLabel(item.Stage)
}

func posterNote(item Film) string {
	if item.Stage != core.StageFailed || item.Detail == "" || item.Detail == "已取消" {
		return ""
	}
	detail := strings.TrimPrefix(item.Detail, "失败：")
	switch {
	case strings.Contains(detail, "LINGOCUT_WHISPER_MODEL"), strings.Contains(detail, "whisper model"):
		return "先在「语音识别」里选一个模型"
	case strings.Contains(detail, "is not installed"):
		return "本机还没装好处理视频的程序"
	case strings.Contains(detail, "EveryAPI is not signed in"):
		return "先登录 EveryAPI，或在「AI 服务」里填 Key"
	case strings.Contains(detail, "LINGOCUT_LLM_MODEL"):
		return "先在「AI 服务」里填模型"
	default:
		return Shorten(detail, 42)
	}
}
