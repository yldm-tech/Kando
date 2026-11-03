package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

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
			choice := choiceSelect(c, &a.language, languageChoices(a.language)).Width(160)
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
			before := a.translateContext
			components.Switch(c, &a.translateContext, "智能翻译", components.SwitchOptions{HideLabel: true})
			if a.translateContext != before {
				a.saveSettings()
			}
		})
	})
	ui.Text(c, "提示词")
	usingDefault := strings.TrimSpace(a.translatePrompt) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	ui.Column(c).Background(t.Surface).Radius(12).Padding(12, 16).Gap(10).Children(func() {
		c.Key("translate-prompt")
		prompt := components.TextBox(c, &a.translatePrompt, components.TextBoxOptions{Multiline: true}).Height(148).Label("提示词")
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
			reset := components.Button(c, "恢复默认", components.ButtonSecondary).Disabled(usingDefault)
			if reset.Clicked() {
				a.translatePrompt = media.DefaultSubtitlePrompt
				a.saveSettings()
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

func (a *app) generalSettings(c *ui.Context, t *ui.Theme, tools media.Tools, known bool) {
	settingSection(c, t, "资料库", "每部影片是资料库里的一个文件夹，视频、封面和字幕都是普通文件。切换资料库会停止进行中的任务。从列表拿掉一个资料库不会删除里面的文件。", func() {
		settingCard(c, t, func() {
			a.currentLibraryRow(c, t)
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
				if components.Button(c, "重新检测", components.ButtonSecondary).Clicked() {
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
		if components.Button(c, label, components.ButtonSecondary).Clicked() {
			a.cancelDownload()
		}
		return
	}
	if installed != "" {
		ui.Text(c, "已下载").TextColor(t.TextMuted)
		return
	}
	c.Key(model.ID + "-download")
	if components.Button(c, "下载", components.ButtonSecondary).Clicked() {
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
					input := fitControl(components.TextBox(c, &a.llmBase, components.TextBoxOptions{}).Label("Base URL"))
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
					input := fitControl(components.TextBox(c, &a.llmKey, components.TextBoxOptions{}).Label("API Key"))
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
					if components.Button(c, label, components.ButtonSecondary).Shrink(0).Clicked() {
						a.showLLMKey = !a.showLLMKey
					}
				})
				ui.Box(c).Fill().Height(1).Background(t.Border)
				a.llmField(c, t, "模型", "", func() {
					c.Key("llm-model")
					input := fitControl(components.TextBox(c, &a.llmModel, components.TextBoxOptions{}).Label("模型"))
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
					if components.Button(c, label, components.ButtonSecondary).Clicked() && !a.testingLLM {
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

func (a *app) currentLibraryRow(c *ui.Context, t *ui.Theme) {
	dir := a.snap().LibraryDir
	path := "还没有资料库"
	if dir != "" {
		path = displayPath(dir)
	}
	ui.Row(c).MinHeight(48).Padding(8, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "当前资料库").Shrink(0)
		ui.Text(c, path).Grow(1).TextColor(t.TextMuted).FontSize(13).Wrap()
		folder := ui.Row(c).Size(28, 28).Radius(6).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Shrink(0).Label("打开所在文件夹")
		if dir == "" {
			folder.Cursor(ui.CursorDefault)
		} else if folder.Clicked() {
			a.reveal(dir)
		}
		folder.Children(func() {
			components.Icon(c, lucide.Folder).Size(16, 16).TextColor(t.TextMuted)
		})
		components.Button(c, "切换", components.ButtonSecondary).Shrink(0).PopoverMenuButton(func(m *ui.Menu) { a.libraryMenu(m) })
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
