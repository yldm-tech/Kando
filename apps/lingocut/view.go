package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

const (
	pageFilms     = ""
	pageYouTube   = "youtube"
	pageGeneral   = "general"
	pageSpeech    = "speech"
	pageAI        = "ai"
	pageTranslate = "translate"

	appVersion = "0.1.0"

	// windowBackground matches the theme so the hidden title bar is not a light strip.
	windowBackground = "#16120e"

	sidebarWidth float32 = 248
	pagePadX     float32 = 32
	cardMinWidth float32 = 280
	cardGap      float32 = 18
)

func lingoTheme() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex(windowBackground)
	t.Surface = ui.Hex("#2c241c")
	t.SurfaceHover = ui.Hex("#3a3126")
	t.SurfacePressed = ui.Hex("#4a3b2c")
	t.Border = ui.Hex("#3d342b")
	t.Text = ui.Hex("#f6f1e8")
	t.TextMuted = ui.Hex("#b3a38f")
	t.Accent = ui.Hex("#e39b2d")
	t.AccentHover = ui.Hex("#f0b04a")
	t.AccentPressed = ui.Hex("#c4841c")
	t.AccentText = ui.Hex("#1a140e")
	t.Danger = ui.Hex("#ef5350")
	t.Radius = 12
	return t
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(lingoTheme())
	t := c.Theme()
	films := a.films()
	// The traffic lights sit on the dark content. The band under them is the drag region.
	bar := c.TitleBar()
	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if bar.Height > 0 {
			ui.Row(c).Height(bar.Height).Padding(0, bar.Right, 0, bar.Left).DragWindow()
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if a.page != pageFilms || a.openKey == "" {
				a.sidebar(c, t, len(films))
			}
			a.mainColumn(c, t, films)
		})
	})
}

func (a *app) mainColumn(c *ui.Context, t *ui.Theme, films []film) {
	padX, padY := pagePadX, float32(28)
	if a.page == pageFilms && a.openKey != "" {
		padX, padY = 16, 12
	}
	body := ui.Column(c).Grow(1).MinWidth(0).Padding(padY, padX).Gap(20)
	if files := body.DroppedFiles(); len(files) > 0 {
		for _, file := range files {
			if media.IsVideoFile(file) {
				a.importLocal(file)
				break
			}
		}
	}
	if body.FileDragOver() {
		body.Background(t.Surface)
	}
	body.Children(func() {
		switch a.page {
		case pageGeneral, pageSpeech, pageAI, pageTranslate:
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).WidthPercent(100).MaxWidth(920).Gap(22).Children(func() {
					a.settings(c, t)
				})
			})
		default:
			a.libraryPage(c, t, films)
		}
	})
}

func (a *app) sidebar(c *ui.Context, t *ui.Theme, count int) {
	ui.Column(c).Width(sidebarWidth).Shrink(0).Padding(18, 14).Gap(2).Children(func() {
		ui.Row(c).Padding(4, 8, 14, 8).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(28, 28).Radius(8).Background(t.Accent).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, iconMark).Size(16, 16).TextColor(t.AccentText)
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "LingoCut").FontSize(15).Bold()
				ui.Text(c, countLabel(count)).TextColor(t.TextMuted).FontSize(12)
			})
		})
		if libs := a.knownLibraries(); len(libs) > 0 {
			a.section(c, t, "资料库")
			for _, dir := range libs {
				a.libraryNav(c, t, dir)
			}
			a.libraryAction(c, t, "新建资料库", a.beginNewLibrary)
			a.libraryAction(c, t, "导入资料库", a.pickLibrary)
		}
		a.section(c, t, "影片")
		a.nav(c, t, pageFilms, "全部影片", fmt.Sprintf("%d", count))
		a.section(c, t, "来源")
		a.nav(c, t, pageYouTube, "YouTube", "")
		a.section(c, t, "设置")
		a.nav(c, t, pageGeneral, "通用", "")
		a.nav(c, t, pageSpeech, "语音识别", "")
		a.nav(c, t, pageAI, "AI 服务", "")
		a.nav(c, t, pageTranslate, "翻译", "")
		if a.saveErr != "" {
			ui.Text(c, a.saveErr).TextColor(t.Danger).Wrap().Padding(8, 8, 0, 8)
		}
	})
}

func (a *app) libraryNav(c *ui.Context, t *ui.Theme, dir string) {
	title := libraryTitle(dir)
	row := ui.Row(c).Padding(7, 10).Radius(8).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(title)
	if filepath.Clean(a.libraryDir) == filepath.Clean(dir) {
		row.Background(ui.Hex("#4a3824"))
	}
	if row.Clicked() {
		a.switchLibrary(dir)
	}
	row.Children(func() {
		ui.Icon(c, iconFilm).Size(15, 15).TextColor(t.TextMuted)
		ui.Text(c, title).Grow(1)
	})
}

func (a *app) libraryAction(c *ui.Context, t *ui.Theme, label string, fn func()) {
	row := ui.Row(c).Padding(7, 10).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label)
	if row.Clicked() {
		fn()
	}
	row.Children(func() {
		ui.Text(c, label).TextColor(t.TextMuted)
	})
}

func (a *app) welcomeLibrary(c *ui.Context, t *ui.Theme) {
	title := "先准备一个资料库"
	if a.libraryDir != "" {
		title = "新建资料库"
	}
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(440).Gap(14).Children(func() {
			ui.Text(c, title).FontSize(28).Bold()
			ui.Text(c, "一个资料库是一个 .lingocut 文件夹，里面每个视频一个文件夹，访达里会带上图标。可以建多个，之后在侧边栏切换。").TextColor(t.TextMuted).Wrap()
			a.libraryDraft(c, t, "创建资料库")
			if a.libraryDir == "" {
				ui.Text(c, "已经有资料库").TextColor(t.TextMuted).FontSize(12)
				if ui.Button(c, "导入已有资料库").Clicked() {
					a.pickLibrary()
				}
			} else if ui.Button(c, "取消新建").Clicked() {
				a.makingLibrary = false
			}
			if a.saveErr != "" {
				ui.Text(c, a.saveErr).TextColor(t.Danger).Wrap()
			}
		})
	})
}

func (a *app) libraryDraft(c *ui.Context, t *ui.Theme, confirm string) {
	ui.Text(c, "名称").TextColor(t.TextMuted).FontSize(12)
	ui.TextInput(c, &a.libraryName).Label("资料库名称").Placeholder("影视库")
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "位置").TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, displayPath(a.libraryPlace())).Grow(1).TextColor(t.Text)
		if ui.Button(c, "更改位置").Clicked() {
			a.pickLibraryParent()
		}
	})
	if ui.Button(c, confirm).Clicked() {
		a.createLibrary()
	}
}

func libraryTitle(dir string) string {
	base := filepath.Base(dir)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return dir
	}
	return base
}

func (a *app) section(c *ui.Context, t *ui.Theme, label string) {
	ui.Text(c, label).TextColor(t.TextMuted).FontSize(12).Padding(14, 8, 4, 8)
}

func (a *app) nav(c *ui.Context, t *ui.Theme, page, label, badge string) {
	row := ui.Row(c).Padding(7, 10).Radius(8).Gap(8).AlignItems(ui.Center)
	if a.page == page {
		row.Background(ui.Hex("#4a3824"))
	}
	if row.Clicked() {
		a.page = page
		a.openKey = ""
		a.stopPlayback()
	}
	row.Children(func() {
		color := t.TextMuted
		if a.page == page {
			color = t.Text
		}
		ui.Icon(c, navIcon(page)).Size(15, 15).TextColor(color)
		ui.Text(c, label).Grow(1)
		if badge != "" {
			ui.Text(c, badge).TextColor(t.TextMuted).FontSize(12)
		}
	})
}

func (a *app) libraryPage(c *ui.Context, t *ui.Theme, films []film) {
	if a.libraryDir == "" || a.makingLibrary {
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
			if ui.Button(c, "取消").Shrink(0).Clicked() {
				a.stop()
			}
		} else {
			c.Key("files")
			if ui.Button(c, "本地文件").Shrink(0).Clicked() {
				a.pickVideos()
			}
			c.Key("start")
			if ui.Button(c, "开始处理").Shrink(0).Clicked() {
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
						if ui.Button(c, "移除").Clicked() {
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
		if ui.Button(c, "返回").Shrink(0).Clicked() {
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
			if ui.Button(c, "在访达中显示").Shrink(0).Clicked() {
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
			if ui.Button(c, "取消").Clicked() {
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
			if ui.Button(c, "移除").Clicked() {
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
				ui.Select(c, &face.Family, []string{"默认", "苹方", "冬青黑体"}).Width(120)
			})
			a.cueStepper(c, t, "字号", &face.Size, 2, 12, 72)
			a.cueColors(c, t, face)
			ui.Row(c).MinHeight(36).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "加粗").Grow(1)
				ui.Switch(c, &face.Bold).Label("加粗")
			})
			if ui.Button(c, "使用原文样式").Clicked() {
				a.cue.Translation = a.cue.Original
			}
			ui.Text(c, "双语").TextColor(t.TextMuted).FontSize(12)
			a.cueChoiceRow(c, t, []cueChoice{{"translation", "译文在上"}, {"original", "原文在上"}, {"translation-only", "只要译文"}}, a.cue.Order, func(id string) { a.cue.Order = id })
			ui.Text(c, "位置").TextColor(t.TextMuted).FontSize(12)
			a.cueStepper(c, t, "高底部", &a.cue.Bottom, 4, 24, 160)
			if ui.Button(c, "恢复默认样式").Clicked() {
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
		if ui.Button(c, "−").Label(label+"减小").Clicked() && *value > min {
			*value -= step
		}
		ui.Text(c, fmt.Sprintf("%.0f", *value)).FontSize(13)
		if ui.Button(c, "+").Label(label+"增大").Clicked() && *value < max {
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

func findFilm(films []film, key string) (film, bool) {
	for _, item := range films {
		if item.key == key {
			return item, true
		}
	}
	return film{}, false
}

func (a *app) settings(c *ui.Context, t *ui.Theme) {
	tools, ok := a.pipeline.(media.Tools)
	switch a.page {
	case pageGeneral:
		a.generalSettings(c, t, tools, ok)
	case pageSpeech:
		a.speechSettings(c, t, tools)
	case pageAI:
		a.aiSettings(c, t, tools, ok)
	case pageTranslate:
		a.translateSettings(c, t)
	}
}

func (a *app) translateSettings(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 16).Children(func() {
		ui.Row(c).MinHeight(48).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "默认翻译为").Grow(1)
			c.Key("translate-language")
			choice := ui.Select(c, &a.language, languageChoices(a.language)).Width(160)
			if choice.Changed() {
				a.saveSettings()
			}
		})
		ui.Box(c).Fill().Height(1).Background(t.Border)
		ui.Row(c).MinHeight(56).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "智能翻译")
				ui.Text(c, "翻译时参考前面的句子，同一句里的碎片会按顺序接上。").TextColor(t.TextMuted).FontSize(12).Wrap()
			})
			c.Key("translate-context")
			toggle := ui.Switch(c, &a.translateContext).Label("智能翻译")
			if toggle.Changed() {
				a.saveSettings()
			}
		})
	})
	ui.Text(c, "提示词")
	usingDefault := strings.TrimSpace(a.translatePrompt) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	ui.Column(c).Background(t.Surface).Radius(12).Padding(12, 16).Gap(10).Children(func() {
		c.Key("translate-prompt")
		prompt := ui.TextArea(c, &a.translatePrompt).Height(148).Label("提示词")
		if prompt.Changed() {
			a.saveSettings()
		}
		ui.Box(c).Fill().Height(1).Background(t.Border)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			label := "正在使用自定义提示词"
			if usingDefault {
				label = "正在使用默认提示词"
			}
			ui.Text(c, label).TextColor(t.TextMuted).Grow(1)
			c.Key("translate-reset")
			reset := ui.Button(c, "恢复默认").Disabled(usingDefault)
			if reset.Clicked() {
				a.translatePrompt = media.DefaultSubtitlePrompt
				a.saveSettings()
			}
		})
	})
	ui.Text(c, "用 {{language}} 代表目标语言。可以加上风格要求，例如口语化、人名保留英文。返回的 JSON 行数由 LingoCut 自动要求，不用写在这里。").TextColor(t.TextMuted).FontSize(12).Wrap()
}

func languageChoices(current string) []string {
	choices := []string{"简体中文", "繁体中文", "English", "日本語", "한국어", "Español", "Français", "Deutsch"}
	for _, choice := range choices {
		if choice == current {
			return choices
		}
	}
	if strings.TrimSpace(current) == "" {
		return choices
	}
	return append([]string{current}, choices...)
}

func (a *app) generalSettings(c *ui.Context, t *ui.Theme, tools media.Tools, known bool) {
	settingSection(c, t, "资料库", "每个资料库是一个 .lingocut 文件夹，访达里带自己的图标。切换会停止进行中的任务。移出只从列表去掉，不删除文件。", func() {
		libs := a.knownLibraries()
		if len(libs) > 0 {
			lines := make([]func(), 0, len(libs))
			for _, dir := range libs {
				dir := dir
				lines = append(lines, func() { a.librarySettingRow(c, t, dir) })
			}
			settingCard(c, t, lines...)
		}
		if len(libs) == 0 {
			a.libraryDraft(c, t, "确认创建")
		}
		ui.Row(c).Gap(8).Children(func() {
			if len(libs) > 0 {
				if ui.Button(c, "新建资料库").Clicked() {
					a.beginNewLibrary()
				}
			}
			if ui.Button(c, "导入已有资料库").Clicked() {
				a.pickLibrary()
			}
		})
	})
	settingSection(c, t, "语言", "", func() {
		settingCard(c, t, func() {
			settingLine(c, t, "界面语言", "简体中文", nil)
		})
	})
	settingSection(c, t, "更新", "", func() {
		settingCard(c, t, func() {
			settingLine(c, t, "当前版本", appVersion, nil)
		})
	})
	settingSection(c, t, "组件", "", func() {
		settingCard(c, t,
			func() { a.toolLine(c, t, "yt-dlp", tools.YTDLP) },
			func() { a.toolLine(c, t, "FFmpeg", tools.FFmpeg) },
			func() { a.toolLine(c, t, "whisper-cli", tools.Whisper) },
		)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "这些程序来自本机，LingoCut 不负责安装或更新。").TextColor(t.TextMuted).FontSize(12).Grow(1).Wrap()
			if known {
				c.Key("redetect")
				if ui.Button(c, "重新检测").Clicked() {
					a.redetectTools()
				}
			}
		})
	})
}

func (a *app) speechSettings(c *ui.Context, t *ui.Theme, tools media.Tools) {
	dirs := a.speechSearchDirs()
	models := media.SpeechModels()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "Whisper")
		ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 12).Children(func() {
			for i, model := range models {
				if i > 0 {
					ui.Box(c).Fill().Height(1).Background(t.Border)
				}
				installed := media.ResolveSpeechModel(tools.WhisperModel, dirs, model.File)
				a.speechModelRow(c, t, model, installed, installed != "" && filepath.Clean(installed) == filepath.Clean(tools.WhisperModel))
			}
		})
		ui.Text(c, "自动识别语言，支持智能断句。越大越准，也越慢、越占空间；不确定就用推荐的。").TextColor(t.TextMuted).FontSize(12).Wrap()
		if a.downloadErr != "" {
			ui.Text(c, a.downloadErr).TextColor(t.Danger).FontSize(12).Wrap()
		}
	})
}

func (a *app) speechModelRow(c *ui.Context, t *ui.Theme, model media.SpeechModel, installed string, selected bool) {
	c.Key(model.ID)
	row := ui.Row(c).Padding(12, 4).Gap(12).AlignItems(ui.Center)
	if installed != "" && row.Clicked() {
		a.useSpeechModel(installed)
	}
	row.Children(func() {
		mark := ui.Box(c).Size(16, 16).Radius(8)
		if selected {
			mark.Background(t.Accent)
		} else {
			mark.Border(1.5, t.TextMuted)
		}
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, model.Name).Bold()
				if model.Recommended {
					ui.Text(c, "推荐").FontSize(12).TextColor(t.Accent)
				}
			})
			ui.Text(c, model.Blurb).TextColor(t.TextMuted).FontSize(12).Wrap()
			ui.Text(c, fmt.Sprintf("速度 %s  准确 %s  %s", stars(model.Speed), stars(model.Accuracy), model.SizeLabel)).TextColor(t.TextMuted).FontSize(12)
		})
		a.speechModelAction(c, t, model, installed)
	})
}

func (a *app) speechModelAction(c *ui.Context, t *ui.Theme, model media.SpeechModel, installed string) {
	if a.downloading == model.ID {
		label := "取消"
		if a.downloadTotal > 0 {
			label = fmt.Sprintf("%d%%", a.downloadWritten*100/a.downloadTotal)
		}
		c.Key(model.ID + "-cancel")
		if ui.Button(c, label).Clicked() {
			a.cancelDownload()
		}
		return
	}
	if installed != "" {
		ui.Text(c, "已下载").TextColor(t.TextMuted)
		return
	}
	c.Key(model.ID + "-download")
	if ui.Button(c, "下载").Clicked() {
		a.downloadSpeechModel(model.ID)
	}
}

func stars(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 5 {
		n = 5
	}
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}

func (a *app) aiSettings(c *ui.Context, t *ui.Theme, _ media.Tools, known bool) {
	if !known {
		settingSection(c, t, "翻译服务", "", func() {
			settingCard(c, t, func() { settingLine(c, t, "服务", "未配置", nil) })
		})
		return
	}
	preset, _ := llmPresetByID(a.llmProvider)
	// The provider list sits beside the form while the page is wide enough, and above it when it is not.
	width, _ := c.Size()
	wide := contentWidth(width) >= 640
	listWidth := float32(196)
	if !wide {
		listWidth = 0
	}
	place := func(children func()) {
		if wide {
			ui.Row(c).Gap(16).AlignItems(ui.Start).Children(children)
			return
		}
		ui.Column(c).Gap(16).Children(children)
	}
	place(func() {
		a.llmList(c, t, listWidth)
		ui.Column(c).Grow(1).MinWidth(0).Gap(18).Children(func() {
			ui.Column(c).Background(t.Surface).Radius(12).Padding(14, 16).Gap(4).Children(func() {
				ui.Text(c, preset.Name).Bold()
				if preset.Blurb != "" {
					ui.Text(c, preset.Blurb).TextColor(t.TextMuted).FontSize(12).Wrap()
				}
			})
			ui.Text(c, "连接")
			ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 16).Children(func() {
				a.llmField(c, t, "Base URL", "任何兼容 OpenAI Chat Completions 的服务都可以。", func() {
					c.Key("llm-base")
					input := fitControl(ui.TextInput(c, &a.llmBase).Label("Base URL"))
					if input.Changed() {
						a.llmProvider = llmProviderID(a.llmBase)
						a.applyLLM()
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				hint := "只保存在本机设置文件里。"
				if a.llmProvider == "everyapi" && strings.TrimSpace(a.llmKey) == "" {
					hint = "留空则使用本机 everyapi 登录。"
				}
				a.llmField(c, t, "API Key", hint, func() {
					c.Key("llm-key")
					input := fitControl(ui.TextInput(c, &a.llmKey).Label("API Key"))
					if !a.showLLMKey {
						input.Password()
					}
					if input.Changed() {
						a.applyLLM()
					}
					c.Key("llm-reveal")
					label := "显示"
					if a.showLLMKey {
						label = "隐藏"
					}
					if ui.Button(c, label).Shrink(0).Clicked() {
						a.showLLMKey = !a.showLLMKey
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				a.llmField(c, t, "模型", "", func() {
					c.Key("llm-model")
					input := fitControl(ui.TextInput(c, &a.llmModel).Label("模型"))
					if input.Changed() {
						a.applyLLM()
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				a.llmField(c, t, "连接", "", func() {
					c.Key("llm-test")
					label := "测试"
					if a.testingLLM {
						label = "正在测试…"
					}
					if ui.Button(c, label).Clicked() && !a.testingLLM {
						a.testLLM()
					}
				})
			})
			if a.llmTestResult != "" {
				color := t.TextMuted
				if a.llmTestResult != "连接成功" {
					color = t.Danger
				}
				ui.Text(c, a.llmTestResult).TextColor(color).FontSize(12).Wrap()
			}
		})
	})
}

func (a *app) llmList(c *ui.Context, t *ui.Theme, width float32) {
	col := ui.Column(c).Gap(2).Shrink(0)
	if width > 0 {
		col.Width(width)
	}
	col.Children(func() {
		group := ""
		for _, preset := range llmPresets() {
			if preset.Group != group {
				group = preset.Group
				ui.Text(c, group).TextColor(t.TextMuted).FontSize(12).Padding(12, 8, 4, 8)
			}
			c.Key("llm-" + preset.ID)
			row := ui.Row(c).Padding(7, 10).Radius(8).Gap(8).AlignItems(ui.Center)
			if a.llmProvider == preset.ID {
				row.Background(ui.Hex("#4a3824"))
			}
			if row.Clicked() {
				a.selectLLM(preset.ID)
			}
			id := preset.ID
			name := preset.Name
			row.Children(func() {
				if a.llmProvider == id {
					ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex("#3d9a6a"))
				}
				ui.Text(c, name)
			})
		}
	})
}

func fitControl(e ui.Element) ui.Element {
	return e.MinWidth(120).MaxWidth(320).Basis(200)
}

func (a *app) llmField(c *ui.Context, t *ui.Theme, label, hint string, control func()) {
	ui.Row(c).Wrap().MinHeight(52).Padding(10, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label)
			if hint != "" {
				ui.Text(c, hint).TextColor(t.TextMuted).FontSize(12).Wrap()
			}
		})
		control()
	})
}

func (a *app) toolLine(c *ui.Context, t *ui.Theme, name, path string) {
	detail := "未安装"
	color := t.Danger
	if path != "" {
		color = ui.Hex("#3d9a6a")
		detail = displayPath(path)
		if version := a.toolVersions[path]; version != "" {
			detail = version + " · " + detail
		}
	}
	ui.Row(c).Padding(10, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(8, 8).Radius(4).Background(color)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, name)
			ui.Text(c, detail).TextColor(t.TextMuted).FontSize(12).Wrap().Selectable()
		})
	})
}

func (a *app) librarySettingRow(c *ui.Context, t *ui.Theme, dir string) {
	title := libraryTitle(dir)
	current := filepath.Clean(a.libraryDir) == filepath.Clean(dir)
	row := ui.Row(c).MinHeight(44).Padding(8, 0).Gap(12).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(title)
	if row.Clicked() {
		a.switchLibrary(dir)
	}
	row.Children(func() {
		text := ui.Text(c, title)
		if current {
			text.Bold()
		}
		ui.Text(c, displayPath(dir)).Grow(1).TextColor(t.TextMuted).FontSize(12)
		if ui.Button(c, "移出").Label("移出 " + title).Clicked() {
			a.forgetLibrary(dir)
		}
	})
}

func settingSection(c *ui.Context, t *ui.Theme, title, note string, body func()) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title)
		body()
		if note != "" {
			ui.Text(c, note).TextColor(t.TextMuted).FontSize(12).Wrap()
		}
	})
}

func settingCard(c *ui.Context, t *ui.Theme, lines ...func()) {
	ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 16).Children(func() {
		for i, line := range lines {
			if i > 0 {
				ui.Box(c).Fill().Height(1).Background(t.Border)
			}
			line()
		}
	})
}

func settingLine(c *ui.Context, t *ui.Theme, label, value string, trailing func()) {
	ui.Row(c).MinHeight(44).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label)
		ui.Row(c).Grow(1)
		if value != "" {
			ui.Text(c, value).MaxWidthPercent(55).TextColor(t.TextMuted).Wrap().Selectable()
		}
		if trailing != nil {
			trailing()
		}
	})
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
	used := map[string]bool{}
	out := make([]film, 0, len(a.library.jobs)+len(a.videos))
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
	for _, video := range a.videos {
		if used[video.Path] || !pathInDir(a.libraryDir, video.Path) {
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
	if a.libraryDir == "" {
		return false
	}
	if job.VideoPath != "" {
		return pathInDir(a.libraryDir, job.VideoPath)
	}
	if job.LibraryDir != "" {
		return filepath.Clean(job.LibraryDir) == filepath.Clean(a.libraryDir)
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

func countLabel(n int) string {
	return fmt.Sprintf("%d 部影片", n)
}
