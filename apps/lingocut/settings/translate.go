package settings

import (
	"strings"

	"lingocut/apps/lingocut/settings/form"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// translate is the 翻译 page: the language subtitles are translated into, whether each batch takes the sentences before it, and the prompt. The prototype's page starts 6 points lower than one that opens with a section label.
func (s screen) translate(c *ui.Context, t *ui.Theme) {
	form.Card(c, func() {
		form.Row(c, "默认目标语言", "", func() {
			c.Key("translate-language")
			if form.Select(c, s.Language(), languageChoices(*s.Language())).Label("默认目标语言").Changed() {
				s.Save()
			}
		})
		form.Separator(c, true)
		c.Key("translate-context")
		if form.SwitchRow(c, "智能翻译", "翻译时参考前面的句子，同一句里的碎片会按顺序接上。", s.TranslateContext()) {
			s.Save()
		}
	}).Margin(6, 0, 0, 0)
	form.Note(c, "新任务译成这种语言。", 0)
	usingDefault := strings.TrimSpace(*s.TranslatePrompt()) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	form.Label(c, "提示词", 12, nil)
	form.Card(c, func() {
		ui.Column(c).Padding(10, 14, 12, 14).Gap(8).AlignItems(ui.Stretch).Children(func() {
			c.Key("translate-prompt")
			if breeze.TextArea(c, s.TranslatePrompt()).Height(106).Font("monospace").FontSize(12).Label("翻译提示词").Changed() {
				s.Save()
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := "正在使用自定义提示词"
				if usingDefault {
					label = "正在使用默认提示词"
				}
				ui.Text(c, label+" · 用 {{language}} 代表目标语言。").FontSize(11).TextColor(form.Secondary).Grow(1).Wrap()
				c.Key("translate-reset")
				if form.Button(c, "恢复默认", true).Disabled(usingDefault).Clicked() && !usingDefault {
					*s.TranslatePrompt() = media.DefaultSubtitlePrompt
					s.Save()
				}
			})
		})
	})
	form.Note(c, "可以加上风格要求，例如口语化、人名保留英文。返回的 JSON 行数由 LingoCut 自动要求，不用写在这里。", 0)
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
