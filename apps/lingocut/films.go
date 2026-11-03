package main

import (
	"os"
	"path/filepath"
	"strings"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/yldm-tech/nodra"
)

func (a *app) welcomeLibrary(c *ui.Context, t *ui.Theme) {
	title := "先准备一个资料库"
	if a.snap().LibraryDir != "" {
		title = "新建资料库"
	}
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(440).Gap(14).Children(func() {
			ui.Text(c, title).FontSize(28).Bold()
			ui.Text(c, "一个资料库是一个 .lingocut 文件夹，里面每个视频一个文件夹，访达里会带上图标。建好后从左上角的菜单切换。").TextColor(t.TextMuted).Wrap()
			a.libraryDraft(c, t, "创建资料库")
			if a.snap().LibraryDir == "" {
				ui.Text(c, "已经有资料库").TextColor(t.TextMuted).FontSize(12)
				if components.Button(c, "导入已有资料库", components.ButtonSecondary).Clicked() {
					a.pickLibrary()
				}
			} else if components.Button(c, "取消新建", components.ButtonSecondary).Clicked() {
				a.edit(func(s *session) { s.MakingLibrary = false })
			}
			if a.saveErr != "" {
				ui.Text(c, a.saveErr).TextColor(t.Danger).Wrap()
			}
		})
	})
}

func (a *app) libraryDraft(c *ui.Context, t *ui.Theme, confirm string) {
	ui.Text(c, "名称").TextColor(t.TextMuted).FontSize(12)
	name, err := nodra.BindField(a.store, func(s session) string { return s.LibraryName }, func(s *session, value string) error {
		s.LibraryName = value
		return nil
	})
	if err != nil {
		ui.Text(c, err.Error()).TextColor(t.Danger)
		return
	}
	_ = name.Edit(func(value *string) bool {
		return components.TextBox(c, value, components.TextBoxOptions{}).Label("资料库名称").Placeholder("影视库").Changed()
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "位置").TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, displayPath(a.libraryPlace())).Grow(1).TextColor(t.Text)
		if components.Button(c, "更改位置", components.ButtonSecondary).Clicked() {
			a.pickLibraryParent()
		}
	})
	if components.Button(c, confirm, components.ButtonPrimary).Clicked() {
		a.createLibrary()
	}
}

func libraryTitle(dir string) string {
	base := filepath.Base(dir)
	base = strings.TrimSuffix(base, media.LibraryExt)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return dir
	}
	return base
}

func (a *app) libraryPage(c *ui.Context, t *ui.Theme, films []film) {
	shelf := a.snap()
	if shelf.LibraryDir == "" || shelf.MakingLibrary {
		a.welcomeLibrary(c, t)
		return
	}
	if a.openKey != "" {
		if item, ok := findFilm(films, a.openKey); ok {
			a.filmPage(c, t, item)
			return
		}
		if item, ok := a.jobFilm(a.openKey); ok {
			a.filmPage(c, t, item)
			return
		}
		a.openKey = ""
	}
	if a.openKey == "" && (a.play != nil || a.playPath != "" || a.playErr != "") {
		a.stopPlayback()
	}
	running := a.running()
	ui.Row(c).Wrap().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MinWidth(240).Height(40).Radius(20).Background(t.Surface).Padding(0, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconLink).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
			field := ui.TextInputBase(c, &a.source).Placeholder("粘贴视频链接，或把本地视频拖进来").Label("视频链接").Grow(1).Disabled(running)
			if field.Submitted() && !running {
				a.start()
			}
		})
		if running {
			c.Key("cancel")
			if components.Button(c, "取消", components.ButtonSecondary).Shrink(0).Clicked() {
				a.stop()
			}
		} else {
			c.Key("files")
			if components.Button(c, "本地文件", components.ButtonSecondary).Shrink(0).Clicked() {
				a.pickVideos()
			}
			c.Key("start")
			if components.Button(c, "开始处理", components.ButtonPrimary).Shrink(0).Clicked() {
				a.start()
			}
		}
	})
	if a.job == nil && a.status != "" {
		color := t.Danger
		if a.status == "已选择本地视频" {
			color = t.TextMuted
		}
		ui.Text(c, a.status).TextColor(color).Wrap()
	}
	shown := films
	if a.page == pageYouTube {
		shown = nil
		for _, item := range films {
			if strings.Contains(item.source, "youtube.com") || strings.Contains(item.source, "youtu.be") {
				shown = append(shown, item)
			}
		}
	}
	if len(shown) == 0 {
		ui.Text(c, "还没有影片").TextColor(t.TextMuted)
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		width, _ := c.Size()
		ui.Grid(c).Columns(filmColumns(width)).Gap(cardGap).AlignItems(ui.Start).Children(func() {
			for _, item := range shown {
				a.card(c, t, item)
			}
		})
	})
}

// filmColumns is how many posters fit across the window: the sidebar and page padding stay fixed, and each poster keeps cardMinWidth.
func filmColumns(windowWidth float32) int {
	content := windowWidth - sidebarWidth - pagePadX*2
	if content < cardMinWidth {
		return 1
	}
	n := int((content + cardGap) / (cardMinWidth + cardGap))
	if n < 1 {
		return 1
	}
	return n
}

// contentWidth is the main column inside the page padding.
func contentWidth(windowWidth float32) float32 {
	w := windowWidth - sidebarWidth - pagePadX*2
	if w < 1 {
		return 1
	}
	return w
}

func (a *app) card(c *ui.Context, t *ui.Theme, item film) {
	a.ensurePoster(item.videoPath)
	col := ui.Column(c).WidthPercent(100).Gap(8).Cursor(ui.CursorPointer)
	if col.Clicked() {
		a.openFilm(item)
	}
	col.Children(func() {
		frame := ui.Box(c).Height(168).Radius(10).Clip().Background(posterColor(item.key))
		frame.Children(func() {
			if img := a.posterOf(item.videoPath); img != nil {
				ui.Image(c, img).WidthPercent(100).Height(168).Fit(ui.Cover)
			}
			ui.Column(c).Absolute().Left(10).Right(10).Bottom(10).Gap(6).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if label := posterLabel(item); label != "" {
						color := t.Text
						if item.stage == core.StageFailed && label != "已取消" {
							color = t.Danger
						}
						ui.Text(c, label).TextColor(color).Bold()
					}
					if item.jobID != "" && item.stage == core.StageFailed && !a.running() {
						if components.Button(c, "移除", components.ButtonSecondary).Clicked() {
							a.remove(item.jobID)
						}
						if ui.PrimaryButton(c, "重试").Clicked() {
							if job, ok := a.library.find(item.jobID); ok {
								a.retry(job)
							}
						}
					}
				})
				if note := posterNote(item); note != "" {
					ui.Text(c, note).FontSize(12).TextColor(t.Text).Wrap()
				}
			})
		})
		if item.jobID != "" && item.stage == core.StageComplete {
			ui.Text(c, "翻译").TextColor(t.Accent).FontSize(12)
		}
		name := item.name
		if name == "" {
			name = item.title
		}
		ui.Text(c, name).Wrap()
	})
}

func posterLabel(item film) string {
	if item.stage == core.StageFailed && item.detail == "已取消" {
		return "已取消"
	}
	if item.stage == "" {
		return ""
	}
	return stageLabel(item.stage)
}

func posterNote(item film) string {
	if item.stage != core.StageFailed || item.detail == "" || item.detail == "已取消" {
		return ""
	}
	detail := strings.TrimPrefix(item.detail, "失败：")
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
		return shorten(detail, 42)
	}
}

func (a *app) openFilm(item film) {
	a.openKey = item.key
	a.loadSubtitles(item.key, item.subtitlePath)
	a.startPlayback(item.videoPath)
}

func findFilm(films []film, key string) (film, bool) {
	for _, item := range films {
		if item.key == key {
			return item, true
		}
	}
	return film{}, false
}

func displayPath(path string) string {
	if path == "" {
		return "未设置"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + path[len(prefix):]
	}
	return path
}

type film struct {
	key          string
	title        string
	name         string
	source       string
	detail       string
	stage        core.Stage
	jobID        string
	videoPath    string
	subtitlePath string
}

func (a *app) films() []film {
	shelf := a.snap()
	videos := shelf.Videos
	used := map[string]bool{}
	out := make([]film, 0, len(a.library.jobs)+len(videos))
	for i := range a.library.jobs {
		job := a.library.jobs[i]
		if !a.jobInLibrary(job) {
			continue
		}
		if job.VideoPath != "" {
			used[job.VideoPath] = true
		}
		name := jobName(job)
		out = append(out, film{
			key: job.ID, title: shorten(name, 22), name: name, source: job.Source,
			detail: statusText(job), stage: job.Stage, jobID: job.ID,
			videoPath: job.VideoPath, subtitlePath: job.SubtitlePath,
		})
	}
	root := shelf.LibraryDir
	for _, video := range videos {
		if used[video.Path] || !pathInDir(root, video.Path) {
			continue
		}
		name := strings.TrimSuffix(video.Name, filepath.Ext(video.Name))
		out = append(out, film{
			key: video.Path, title: shorten(name, 22), name: name, detail: video.Path, videoPath: video.Path,
		})
	}
	return out
}

// jobInLibrary reports whether job belongs on the open library's shelf. A finished job follows its video file. A job that has not downloaded yet follows the library it was started in.
func (a *app) jobInLibrary(job core.Job) bool {
	root := a.snap().LibraryDir
	if root == "" {
		return false
	}
	if job.VideoPath != "" {
		return pathInDir(root, job.VideoPath)
	}
	if job.LibraryDir != "" {
		return filepath.Clean(job.LibraryDir) == filepath.Clean(root)
	}
	return !job.Stage.Done()
}

func (a *app) jobFilm(id string) (film, bool) {
	job, ok := a.library.find(id)
	if !ok {
		return film{}, false
	}
	name := jobName(job)
	return film{
		key: job.ID, title: shorten(name, 22), name: name, source: job.Source,
		detail: statusText(job), stage: job.Stage, jobID: job.ID,
		videoPath: job.VideoPath, subtitlePath: job.SubtitlePath,
	}, true
}

func pathInDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func posterColor(key string) ui.Color {
	colors := []ui.Color{ui.Hex("#3a2c1e"), ui.Hex("#243044"), ui.Hex("#1e3330"), ui.Hex("#3a2430")}
	n := 0
	for _, r := range key {
		n += int(r)
	}
	return colors[n%len(colors)]
}
