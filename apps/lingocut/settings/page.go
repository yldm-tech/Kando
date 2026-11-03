package settings

import (
	"fmt"
	"path/filepath"
	"strings"

	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Preset is one translation service the settings page can select.
type Preset struct {
	ID, Name, Blurb, Group string
}

// App is the settings the page reads and the edits it reports.
type App interface {
	Page() string
	Tools() (media.Tools, bool)
	Language() *string
	TranslateContext() *bool
	TranslatePrompt() *string
	Save() error
	LibraryDir() string
	Reveal(path string)
	LibraryMenu(m *ui.Menu)
	Redetect()
	ToolVersion(path string) string
	SpeechDirs() []string
	UseSpeechModel(path string)
	Downloading() string
	DownloadWritten() int64
	DownloadTotal() int64
	DownloadErr() string
	CancelDownload()
	DownloadModel(id string)
	CurrentPreset() (Preset, bool)
	Presets() []Preset
	LLMProvider() string
	SetLLMProvider(id string)
	ProviderID(base string) string
	LLMBase() *string
	LLMKey() *string
	LLMModel() *string
	ShowLLMKey() bool
	ToggleLLMKey()
	ApplyLLM()
	TestingLLM() bool
	TestLLM()
	LLMResult() string
	SelectLLM(id string)
}

type screen struct {
	App
	version string
}

// Show draws the settings page for the current section.
func Show(c *ui.Context, t *ui.Theme, version string, a App) {
	screen{App: a, version: version}.settings(c, t)
}

func (s screen) settings(c *ui.Context, t *ui.Theme) {
	tools, ok := s.Tools()
	switch s.Page() {
	case sidebar.PageGeneral:
		s.generalSettings(c, t, tools, ok)
	case sidebar.PageSpeech:
		s.speechSettings(c, t, tools)
	case sidebar.PageAI:
		s.aiSettings(c, t, tools, ok)
	case sidebar.PageTranslate:
		s.translateSettings(c, t)
	}
}

func (s screen) translateSettings(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 16).Children(func() {
		ui.Row(c).MinHeight(48).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "默认翻译为").Grow(1)
			c.Key("translate-language")
			choice := choiceSelect(c, s.Language(), languageChoices(*s.Language())).Width(160)
			if choice.Changed() {
				s.Save()
			}
		})
		ui.Box(c).Fill().Height(1).Background(t.Border)
		ui.Row(c).MinHeight(56).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "智能翻译")
				ui.Text(c, "翻译时参考前面的句子，同一句里的碎片会按顺序接上。").TextColor(t.TextMuted).FontSize(12).Wrap()
			})
			c.Key("translate-context")
			before := *s.TranslateContext()
			components.Switch(c, s.TranslateContext(), "智能翻译", components.SwitchOptions{HideLabel: true})
			if *s.TranslateContext() != before {
				s.Save()
			}
		})
	})
	ui.Text(c, "提示词")
	usingDefault := strings.TrimSpace(*s.TranslatePrompt()) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	ui.Column(c).Background(t.Surface).Radius(12).Padding(12, 16).Gap(10).Children(func() {
		c.Key("translate-prompt")
		prompt := components.TextBox(c, s.TranslatePrompt(), components.TextBoxOptions{Multiline: true}).Height(148).Label("提示词")
		if prompt.Changed() {
			s.Save()
		}
		ui.Box(c).Fill().Height(1).Background(t.Border)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			label := "正在使用自定义提示词"
			if usingDefault {
				label = "正在使用默认提示词"
			}
			ui.Text(c, label).TextColor(t.TextMuted).Grow(1)
			c.Key("translate-reset")
			reset := components.Button(c, "恢复默认", components.ButtonSecondary).Disabled(usingDefault)
			if reset.Clicked() {
				*s.TranslatePrompt() = media.DefaultSubtitlePrompt
				s.Save()
			}
		})
	})
	ui.Text(c, "用 {{language}} 代表目标语言。可以加上风格要求，例如口语化、人名保留英文。返回的 JSON 行数由 LingoCut 自动要求，不用写在这里。").TextColor(t.TextMuted).FontSize(12).Wrap()
}

func choiceSelect(c *ui.Context, selected *string, options []string) ui.Element {
	sel := components.Select(c, selected)
	current := *selected
	sel.Trigger.Children(func() { components.Text(c, current).SingleLine() })
	sel.Popup(func(ui.Element) {
		for _, opt := range options {
			opt := opt
			sel.Item(opt).Children(func() { components.Text(c, opt).SingleLine() })
		}
	})
	return sel.Trigger
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

func (s screen) generalSettings(c *ui.Context, t *ui.Theme, tools media.Tools, known bool) {
	settingSection(c, t, "资料库", "每部影片是资料库里的一个文件夹，视频、封面和字幕都是普通文件。切换资料库会停止进行中的任务。从列表拿掉一个资料库不会删除里面的文件。", func() {
		settingCard(c, t, func() {
			s.currentLibraryRow(c, t)
		})
	})
	settingSection(c, t, "语言", "", func() {
		settingCard(c, t, func() {
			settingLine(c, t, "界面语言", "简体中文", nil)
		})
	})
	settingSection(c, t, "更新", "", func() {
		settingCard(c, t, func() {
			settingLine(c, t, "当前版本", s.version, nil)
		})
	})
	settingSection(c, t, "组件", "", func() {
		settingCard(c, t,
			func() { s.toolLine(c, t, "yt-dlp", tools.YTDLP) },
			func() { s.toolLine(c, t, "FFmpeg", tools.FFmpeg) },
			func() { s.toolLine(c, t, "whisper-cli", tools.Whisper) },
		)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "这些程序来自本机，LingoCut 不负责安装或更新。").TextColor(t.TextMuted).FontSize(12).Grow(1).Wrap()
			if known {
				c.Key("redetect")
				if components.Button(c, "重新检测", components.ButtonSecondary).Clicked() {
					s.Redetect()
				}
			}
		})
	})
}

func (s screen) speechSettings(c *ui.Context, t *ui.Theme, tools media.Tools) {
	dirs := s.SpeechDirs()
	models := media.SpeechModels()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "Whisper")
		ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 12).Children(func() {
			for i, model := range models {
				if i > 0 {
					ui.Box(c).Fill().Height(1).Background(t.Border)
				}
				installed := media.ResolveSpeechModel(tools.WhisperModel, dirs, model.File)
				s.speechModelRow(c, t, model, installed, installed != "" && filepath.Clean(installed) == filepath.Clean(tools.WhisperModel))
			}
		})
		ui.Text(c, "自动识别语言，支持智能断句。越大越准，也越慢、越占空间；不确定就用推荐的。").TextColor(t.TextMuted).FontSize(12).Wrap()
		if s.DownloadErr() != "" {
			ui.Text(c, s.DownloadErr()).TextColor(t.Danger).FontSize(12).Wrap()
		}
	})
}

func (s screen) speechModelRow(c *ui.Context, t *ui.Theme, model media.SpeechModel, installed string, selected bool) {
	c.Key(model.ID)
	row := ui.Row(c).Padding(12, 4).Gap(12).AlignItems(ui.Center)
	if installed != "" && row.Clicked() {
		s.UseSpeechModel(installed)
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
		s.speechModelAction(c, t, model, installed)
	})
}

func (s screen) speechModelAction(c *ui.Context, t *ui.Theme, model media.SpeechModel, installed string) {
	if s.Downloading() == model.ID {
		label := "取消"
		if s.DownloadTotal() > 0 {
			label = fmt.Sprintf("%d%%", s.DownloadWritten()*100/s.DownloadTotal())
		}
		c.Key(model.ID + "-cancel")
		if components.Button(c, label, components.ButtonSecondary).Clicked() {
			s.CancelDownload()
		}
		return
	}
	if installed != "" {
		ui.Text(c, "已下载").TextColor(t.TextMuted)
		return
	}
	c.Key(model.ID + "-download")
	if components.Button(c, "下载", components.ButtonSecondary).Clicked() {
		s.DownloadModel(model.ID)
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

func (s screen) aiSettings(c *ui.Context, t *ui.Theme, _ media.Tools, known bool) {
	if !known {
		settingSection(c, t, "翻译服务", "", func() {
			settingCard(c, t, func() { settingLine(c, t, "服务", "未配置", nil) })
		})
		return
	}
	preset, _ := s.CurrentPreset()
	// The provider list sits beside the form while the page is wide enough, and above it when it is not.
	width, _ := c.Size()
	wide := films.ContentWidth(width) >= 640
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
		s.llmList(c, t, listWidth)
		ui.Column(c).Grow(1).MinWidth(0).Gap(18).Children(func() {
			ui.Row(c).Background(t.Surface).Radius(12).Padding(14, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				if mark := providerMark(preset.ID); mark != nil {
					ui.Image(c, mark).Size(28, 28).Shrink(0)
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, preset.Name).Bold()
					if preset.Blurb != "" {
						ui.Text(c, preset.Blurb).TextColor(t.TextMuted).FontSize(12).Wrap()
					}
				})
			})
			ui.Text(c, "连接")
			ui.Column(c).Background(t.Surface).Radius(12).Padding(4, 16).Children(func() {
				s.llmField(c, t, "Base URL", "任何兼容 OpenAI Chat Completions 的服务都可以。", func() {
					c.Key("llm-base")
					input := fitControl(components.TextBox(c, s.LLMBase(), components.TextBoxOptions{}).Label("Base URL"))
					if input.Changed() {
						s.SetLLMProvider(s.ProviderID(*s.LLMBase()))
						s.ApplyLLM()
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				hint := "只保存在本机设置文件里。"
				if s.LLMProvider() == "everyapi" && strings.TrimSpace(*s.LLMKey()) == "" {
					hint = "留空则使用本机 everyapi 登录。"
				}
				s.llmField(c, t, "API Key", hint, func() {
					c.Key("llm-key")
					input := fitControl(components.TextBox(c, s.LLMKey(), components.TextBoxOptions{}).Label("API Key"))
					if !s.ShowLLMKey() {
						input.Password()
					}
					if input.Changed() {
						s.ApplyLLM()
					}
					c.Key("llm-reveal")
					label := "显示"
					if s.ShowLLMKey() {
						label = "隐藏"
					}
					if components.Button(c, label, components.ButtonSecondary).Shrink(0).Clicked() {
						s.ToggleLLMKey()
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				s.llmField(c, t, "模型", "", func() {
					c.Key("llm-model")
					input := fitControl(components.TextBox(c, s.LLMModel(), components.TextBoxOptions{}).Label("模型"))
					if input.Changed() {
						s.ApplyLLM()
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				s.llmField(c, t, "连接", "", func() {
					c.Key("llm-test")
					label := "测试"
					if s.TestingLLM() {
						label = "正在测试…"
					}
					if components.Button(c, label, components.ButtonSecondary).Clicked() && !s.TestingLLM() {
						s.TestLLM()
					}
				})
			})
			if s.LLMResult() != "" {
				color := t.TextMuted
				if s.LLMResult() != "连接成功" {
					color = t.Danger
				}
				ui.Text(c, s.LLMResult()).TextColor(color).FontSize(12).Wrap()
			}
		})
	})
}

func (s screen) llmList(c *ui.Context, t *ui.Theme, width float32) {
	col := ui.Column(c).Gap(2).Shrink(0)
	if width > 0 {
		col.Width(width)
	}
	col.Children(func() {
		group := ""
		for _, preset := range s.Presets() {
			if preset.Group != group {
				group = preset.Group
				ui.Text(c, group).TextColor(t.TextMuted).FontSize(12).Padding(12, 8, 4, 8)
			}
			c.Key("llm-" + preset.ID)
			row := ui.Row(c).Padding(7, 10).Radius(8).Gap(8).AlignItems(ui.Center)
			if s.LLMProvider() == preset.ID {
				row.Background(ui.Hex("#4a3824"))
			}
			if row.Clicked() {
				s.SelectLLM(preset.ID)
			}
			id := preset.ID
			name := preset.Name
			row.Children(func() {
				if mark := providerMark(id); mark != nil {
					ui.Image(c, mark).Size(16, 16).Shrink(0)
				}
				ui.Text(c, name)
			})
		}
	})
}

func fitControl(e ui.Element) ui.Element {
	return e.MinWidth(120).MaxWidth(320).Basis(200)
}

func (s screen) llmField(c *ui.Context, t *ui.Theme, label, hint string, control func()) {
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

func (s screen) toolLine(c *ui.Context, t *ui.Theme, name, path string) {
	detail := "未安装"
	color := t.Danger
	if path != "" {
		color = ui.Hex("#3d9a6a")
		detail = films.DisplayPath(path)
		if version := s.ToolVersion(path); version != "" {
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

func (s screen) currentLibraryRow(c *ui.Context, t *ui.Theme) {
	dir := s.LibraryDir()
	path := "还没有资料库"
	if dir != "" {
		path = films.DisplayPath(dir)
	}
	ui.Row(c).MinHeight(48).Padding(8, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "当前资料库").Shrink(0)
		ui.Text(c, path).Grow(1).TextColor(t.TextMuted).FontSize(13).Wrap()
		folder := ui.Row(c).Size(28, 28).Radius(6).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Shrink(0).Label("打开所在文件夹")
		if dir == "" {
			folder.Cursor(ui.CursorDefault)
		} else if folder.Clicked() {
			s.Reveal(dir)
		}
		folder.Children(func() {
			components.Icon(c, lucide.Folder).Size(16, 16).TextColor(t.TextMuted)
		})
		components.Button(c, "切换", components.ButtonSecondary).Shrink(0).PopoverMenuButton(func(m *ui.Menu) { s.LibraryMenu(m) })
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
