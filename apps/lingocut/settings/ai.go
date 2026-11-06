package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// ai is the AI 服务 page: the service that translates and how LingoCut reaches it.
func (s screen) ai(c *ui.Context, t *ui.Theme) {
	if _, known := s.Tools(); !known {
		preferencesGroup(c, "服务", "", nil, func() {
			breeze.ActionRow(c, "翻译服务", "", func() {
				breeze.Text(c, "未配置", breeze.TextSecondary)
			})
		})
		return
	}
	preset, _ := s.CurrentPreset()
	preferencesGroup(c, "服务", "", nil, func() {
		breeze.ActionRow(c, "服务", preset.Blurb, func() {
			if mark := providerMark(preset.ID); mark != nil {
				ui.Image(c, mark).Size(16, 16).Shrink(0)
			}
			s.providerSelect(c)
		})
	})
	hint := "只保存在本机设置文件里。"
	if s.LLMProvider() == "everyapi" && strings.TrimSpace(*s.LLMKey()) == "" {
		hint = "留空则使用本机 everyapi 登录。"
	}
	preferencesGroup(c, "连接", "任何兼容 OpenAI Chat Completions 的服务都可以。", nil, func() {
		breeze.ActionRow(c, "Base URL", "", func() {
			c.Key("llm-base")
			if fieldWidth(breeze.TextInput(c, s.LLMBase()).Label("Base URL")).Changed() {
				s.SetLLMProvider(s.ProviderID(*s.LLMBase()))
				s.ApplyLLM()
			}
		})
		breeze.ActionRow(c, "模型", "", func() {
			c.Key("llm-model")
			if fieldWidth(breeze.TextInput(c, s.LLMModel()).Label("模型")).Changed() {
				s.ApplyLLM()
			}
		})
		breeze.ActionRow(c, "API Key", hint, func() {
			c.Key("llm-key")
			if fieldWidth(breeze.PasswordField(c, s.LLMKey()).Label("API Key")).Changed() {
				s.ApplyLLM()
			}
		})
		breeze.ActionRow(c, "测试连接", "发一个很短的请求，看服务能不能用。", func() {
			s.testStatus(c, t)
			c.Key("llm-test")
			if breeze.Button(c, "测试连接").Disabled(s.TestingLLM()).Clicked() && !s.TestingLLM() {
				s.TestLLM()
			}
		})
	})
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
	if breeze.Select(c, &chosen, labels).Label("选择服务").Changed() {
		for i, label := range labels {
			if label == chosen {
				s.SelectLLM(presets[i].ID)
			}
		}
	}
}

// testStatus is the connection test's state: a dot and a word, the error when it failed.
func (s screen) testStatus(c *ui.Context, t *ui.Theme) {
	dot, text := t.TextMuted, "尚未测试"
	switch result := s.LLMResult(); {
	case s.TestingLLM():
		text = "正在发送测试请求…"
	case result == "连接成功":
		dot, text = t.Success, result
	case result != "":
		dot, text = t.Danger, result
	}
	statusDot(c, dot)
	breeze.Text(c, text, breeze.TextSecondary).MaxWidth(220).Wrap().Selectable()
}

// fieldWidth gives a text field in a row its room: wide enough for a URL, narrow enough to leave the title its line.
func fieldWidth(e ui.Element) ui.Element {
	return e.Width(280)
}
