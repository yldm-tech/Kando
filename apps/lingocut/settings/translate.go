package settings

import (
	"strings"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// translate is the 翻译 page: the language subtitles are translated into, how much context goes with each batch, and the prompt.
func (s screen) translate(c *ui.Context, t *ui.Theme) {
	breeze.PreferencesGroup(c, "翻译", "", nil, func() {
		c.Key("translate-language")
		if breeze.ComboRow(c, "默认目标语言", "新任务译成这种语言。", s.Language(), languageChoices(*s.Language())).Changed() {
			s.Save()
		}
		c.Key("translate-context")
		if breeze.SwitchRow(c, "智能翻译", "翻译时参考前面的句子，同一句里的碎片会按顺序接上。", s.TranslateContext()).Changed() {
			s.Save()
		}
	})
	usingDefault := strings.TrimSpace(*s.TranslatePrompt()) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	breeze.PreferencesGroup(c, "", "用 {{language}} 代表目标语言。可以加上风格要求，例如口语化、人名保留英文。返回的 JSON 行数由 LingoCut 自动要求，不用写在这里。", nil, func() {
		breeze.ContentRow(c, "翻译提示词", func() {
			c.Key("translate-prompt")
			if breeze.TextArea(c, s.TranslatePrompt()).Height(148).Label("提示词").Changed() {
				s.Save()
			}
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				label := "正在使用自定义提示词"
				if usingDefault {
					label = "正在使用默认提示词"
				}
				breeze.Text(c, label, breeze.TextSecondary).Grow(1)
				c.Key("translate-reset")
				if breeze.Button(c, "恢复默认").Disabled(usingDefault).Clicked() && !usingDefault {
					*s.TranslatePrompt() = media.DefaultSubtitlePrompt
					s.Save()
				}
			})
		})
	})
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
