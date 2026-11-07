package onboarding

import (
	"fmt"
	"slices"

	"kando/apps/kando/settings/sectionid"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// card is a white card with the prototype's hairline and soft shadow, its rows stacked.
func card(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).AlignItems(ui.Stretch).Radius(12).Background(white).Border(0.5, cardRing).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04)).Children(fn)
}

// sectionLabel is the 12 point label over a group of the step's body.
func sectionLabel(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(12).FontWeight(600).TextColor(muted)
}

// note is the 11 point gray line under a group.
func note(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(11).FixedLineHeight(16).TextColor(gray).Wrap()
}

// languagesStep is the first step: the language subtitles are translated into, and tiles for the languages of the videos the user watches.
func languagesStep(c *ui.Context, a App) {
	card(c, func() {
		ui.Row(c).Height(44).Padding(0, 8, 0, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "字幕翻译成").FontSize(13).Grow(1)
			c.Key("onboarding-language")
			if breeze.Select(c, a.Language(), a.LanguageChoices()).Label("字幕翻译成").Changed() {
				a.OnboardingSave()
			}
		})
	})
	target := *a.Language()
	chosen := Effective(a.OnboardingLearning(), target)
	ui.Column(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
		sectionLabel(c, "常看哪些语言的视频 · 可多选")
		ui.Grid(c).Columns(3).GapX(8).GapY(8).Role(ui.RoleGroup).Label("常看哪些语言的视频").Children(func() {
			for _, code := range tiles(target) {
				c.Key("onboarding-tile-" + code)
				if tile(c, code, slices.Contains(chosen, code)).Clicked() {
					a.OnboardingToggle(code)
				}
			}
		})
	})
	note(c, "没选的语言也能识别：whisper 会自己听出视频说的是哪种语言。选好的语言决定下一步推荐哪个模型。")
}

// tile is a language's tile: its own name over its Chinese name, and a check circle at its top right, blue and ringed in blue while chosen.
func tile(c *ui.Context, code string, on bool) ui.Element {
	name := names[code]
	b := ui.ButtonBase(c).Label(name[0]).Checked(on).Column().Height(52).Padding(0, 12).Gap(1).Radius(10).Justify(ui.Center).AlignItems(ui.Start)
	if on {
		b.Background(ui.RGBA(10, 102, 216, 0.07)).InsetShadow(0, 0, 0, 1.5, blue)
	} else {
		b.Background(white).Border(0.5, cardRing).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
		if b.Hovered() {
			b.Background(ui.Hex("#FAFAFC"))
		}
	}
	b.Children(func() {
		ui.Text(c, name[0]).FontSize(13).FontWeight(600).TextColor(ink).Lang(code).SingleLine()
		ui.Text(c, name[1]).FontSize(11).TextColor(gray).SingleLine()
		mark := checkOff
		if on {
			mark = checkOn
		}
		ui.Image(c, mark).Size(16, 16).Absolute().Top(9).Right(9)
	})
	return b
}

// modelsStep is the second step: the speech models recommended for the languages chosen, each with its download, and the translation service.
func modelsStep(c *ui.Context, a App) {
	models := a.OnboardingModels()
	busy := a.OnboardingDownloading()
	card(c, func() {
		ui.Row(c).Height(38).Padding(0, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, modelsHead(models)).FontSize(12).TextColor(muted).Grow(1)
		})
		for _, m := range models {
			ui.Box(c).Height(0.5).Shrink(0).Background(rowRule)
			modelRow(c, a, m, busy)
		}
	})
	note(c, modelsNote(models))
	ai := a.OnboardingAI()
	card(c, func() {
		ui.Row(c).MinHeight(46).Padding(5, 14).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "翻译 · AI 服务").FontSize(13).TextColor(ink)
				color := muted
				if !ai.Ready {
					color = ui.Hex("#B25000")
				}
				ui.Text(c, ai.Text).FontSize(11).TextColor(color).Wrap()
			})
			if ai.Ready {
				ui.Image(c, readyMark).Size(16, 16).Shrink(0)
			}
			c.Key("onboarding-ai")
			if breeze.Button(c, "设置…").Height(24).Shrink(0).Clicked() {
				a.OpenSettings(sectionid.AI)
			}
		})
	})
}

// modelRow is a speech model: its name and size, and what can be done with it: 下载, a progress bar and 取消 while it downloads, 使用 once it is here, and a green check while it is in use.
func modelRow(c *ui.Context, a App, m Model, busy bool) {
	ui.Row(c).MinHeight(46).Padding(5, 14).Gap(10).AlignItems(ui.Center).Label(m.Name).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "语音识别 · "+m.Name).FontSize(13).TextColor(ink).SingleLine()
			sub, color := modelSub(m)
			ui.Text(c, sub).FontSize(11).TextColor(color).Wrap()
		})
		switch m.State {
		case ModelInUse:
			ui.Image(c, readyMark).Size(16, 16).Shrink(0).Label("正在使用")
		case ModelReady:
			c.Key("onboarding-use-" + m.ID)
			if breeze.Button(c, "使用").Height(24).Shrink(0).Clicked() {
				a.OnboardingUseModel(m.ID)
			}
		case ModelDownloading:
			breeze.Progress(c, m.Progress).Width(72).Shrink(0).Label("下载进度")
			c.Key("onboarding-cancel-" + m.ID)
			if breeze.Button(c, "取消").Height(24).Shrink(0).Clicked() {
				a.OnboardingCancelDownload()
			}
		default:
			label := "下载"
			if m.Err != "" {
				label = "重试"
			}
			c.Key("onboarding-download-" + m.ID)
			if breeze.Button(c, label).Height(24).Shrink(0).Disabled(busy).Tooltip("下载 "+m.Name+"（"+m.Size+"）").Clicked() && !busy {
				a.OnboardingDownload(m.ID)
			}
		}
	})
}

// modelSub is the line under a model's name and its color: the download's progress in blue while it runs, why it failed in red, otherwise its size and what it suits.
func modelSub(m Model) (string, ui.Color) {
	switch m.State {
	case ModelInUse:
		return "已下载 · 正在使用", muted
	case ModelReady:
		return "已下载 · " + m.Blurb, muted
	case ModelDownloading:
		if m.Progress < 0 {
			return "正在连接… · 共 " + m.Size, ui.Hex("#0052B0")
		}
		return fmt.Sprintf("下载中 %d%% · 共 %s", int(m.Progress*100), m.Size), ui.Hex("#0052B0")
	}
	if m.Err != "" {
		return "下载失败：" + m.Err, ui.Hex("#C4291C")
	}
	return "约 " + m.Size + " · " + m.Blurb, muted
}

// modelsHead is the card's head: what is in use, or what is missing.
func modelsHead(models []Model) string {
	for _, m := range models {
		if m.State == ModelInUse {
			return "语音识别已就绪"
		}
	}
	for _, m := range models {
		if m.State == ModelDownloading {
			return "正在下载 " + m.Name
		}
	}
	return "还没有语音识别模型，下载一个才能生成字幕"
}

// modelsNote is the line under the models.
func modelsNote(models []Model) string {
	for _, m := range models {
		if m.State == ModelDownloading {
			return "下载会在后台继续，可以先进入下一步。"
		}
	}
	for _, m := range models {
		if m.State == ModelInUse {
			return "以后可以在 设置 › 语音识别 换别的模型。"
		}
	}
	return "也可以以后再下：设置 › 语音识别 里随时可以下载。"
}

// linkStep is the third step: the field for the first link and what it reads as.
func linkStep(c *ui.Context, a App) {
	value := a.OnboardingLink()
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		field := ui.Row(c).Height(44).Radius(10).Padding(0, 14, 0, 13).Gap(8).AlignItems(ui.Center).Background(white)
		field.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.05)).Border(0.5, ui.RGBA(0, 0, 0, 0.14))
		focused := false
		field.Children(func() {
			ui.Icon(c, linkIcon).Size(15, 15).TextColor(gray).Shrink(0)
			c.Key("onboarding-link")
			in := ui.TextInputBase(c, value).Placeholder("粘贴 YouTube 或哔哩哔哩链接").Label("第一个视频的链接").Grow(1).MinWidth(0).FontSize(14).FocusRing(false).AutoFocus()
			if in.Submitted() {
				if _, ok := LinkHint(*value); ok {
					a.OnboardingNext()
				}
			}
			focused = in.Focused()
		})
		if focused {
			field.Shadow(0, 0, 0, 3, blue.Alpha(0.5))
		}
		hint, ok := LinkHint(*value)
		color := ui.Hex("#1B6B30")
		switch {
		case a.OnboardingLinkError() != "" && *value != "":
			hint, color = a.OnboardingLinkError(), ui.Hex("#C4291C")
		case hint == "":
			hint, color = "回车开始；没有链接也可以先跳过，之后把本地视频拖进窗口。", gray
		case !ok:
			color = ui.Hex("#C4291C")
		}
		ui.Text(c, hint).FontSize(12).TextColor(color).MinHeight(16).Wrap()
	})
}
