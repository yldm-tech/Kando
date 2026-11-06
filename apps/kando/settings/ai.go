package settings

import (
	"strings"

	"kando/apps/kando/settings/form"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// ai is the AI 服务 page: the service that translates and how Kando reaches it, in one card as the prototype's, with what the fields mean in the footnote under it. The prototype's page starts 6 points lower than one that opens with a section label.
func (s screen) ai(c *ui.Context, t *ui.Theme) {
	if _, known := s.Tools(); !known {
		form.Card(c, func() {
			form.Row(c, "翻译服务", "", func() { form.Value(c, "未配置", 13) })
		}).Margin(6, 0, 0, 0)
		return
	}
	preset, _ := s.CurrentPreset()
	form.Card(c, func() {
		form.Row(c, "服务", "", func() {
			if mark := providerMark(preset.ID); mark != nil {
				ui.Image(c, mark).Size(16, 16).Shrink(0)
			}
			s.providerSelect(c)
		})
		form.Separator(c, true)
		form.Row(c, "Base URL", "", func() {
			c.Key("llm-base")
			if field(breeze.TextInput(c, s.LLMBase()).Label("Base URL")).Font("monospace").FontSize(12).Changed() {
				s.SetLLMProvider(s.ProviderID(*s.LLMBase()))
				s.ApplyLLM()
			}
		})
		form.Separator(c, true)
		form.Row(c, "模型", "", func() {
			c.Key("llm-model")
			if field(breeze.TextInput(c, s.LLMModel()).Label("模型")).Changed() {
				s.ApplyLLM()
			}
		})
		form.Separator(c, true)
		form.Row(c, "API Key", "", func() {
			c.Key("llm-key")
			if field(breeze.PasswordField(c, s.LLMKey()).Label("API Key")).Changed() {
				s.ApplyLLM()
			}
		})
		ui.Row(c).Shrink(0).MinHeight(40).Padding(6, 14).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { s.testStatus(c, t) })
			c.Key("llm-test")
			if form.Button(c, "测试连接", false).Disabled(s.TestingLLM()).Clicked() && !s.TestingLLM() {
				s.TestLLM()
			}
		})
	}).Margin(6, 0, 0, 0)
	hint := "API Key 只保存在本机设置文件里。"
	if s.LLMProvider() == "everyapi" && strings.TrimSpace(*s.LLMKey()) == "" {
		hint = "不填 API Key 时使用本机 everyapi 登录。"
	}
	note := hint + "任何兼容 OpenAI Chat Completions 的服务都可以；测试连接会发一个很短的请求。"
	if preset.Blurb != "" {
		note = preset.Blurb + "。" + note
	}
	form.Note(c, note, 0)
}

// providerSelect chooses the service among the presets. The drop-down shows their names, the ones not in the cloud after their group, as 本机 · Ollama; the setting keeps the id. Grouped drop-downs (mygo-breeze-ui B10) are not planned, so the group stays in the name.
func (s screen) providerSelect(c *ui.Context) {
	presets := s.Presets()
	labels := make([]string, len(presets))
	chosen := ""
	for i, preset := range presets {
		labels[i] = preset.Name
		if preset.Group == "本机" {
			labels[i] = preset.Group + " · " + preset.Name
		}
		if preset.ID == s.LLMProvider() {
			chosen = labels[i]
		}
	}
	c.Key("llm-provider")
	if form.Select(c, &chosen, labels).Label("选择服务").Changed() {
		for i, label := range labels {
			if label == chosen {
				s.SelectLLM(presets[i].ID)
			}
		}
	}
}

// testStatus is the connection test's state: a dot and a word, the error when it failed.
func (s screen) testStatus(c *ui.Context, t *ui.Theme) {
	dot, color, text := form.Idle, form.Secondary, "尚未测试"
	switch result := s.LLMResult(); {
	case s.TestingLLM():
		text = "正在发送测试请求…"
	case result == "连接成功":
		dot, color, text = form.OK, form.SuccessText, result
	case result != "":
		dot, color, text = t.Danger, t.Danger, result
	}
	form.Status(c, dot, color, text)
}

// field gives a text field in a row the prototype's room: 330 points, the width of the column the fields line up in, and 26 high.
func field(e ui.Element) ui.Element {
	return e.Width(330).Height(26)
}
