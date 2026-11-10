package settings

import (
	"slices"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// ai is the AI 服务 page: the service that translates and how Kando reaches it, in one card as the prototype's, with what the fields mean in the footnote under it. The prototype's page starts 6 points lower than one that opens with a section label.
func (s screen) ai(c *ui.Context, t *ui.Theme) {
	if _, known := s.Tools(); !known {
		form.Card(c, func() {
			form.Row(c, locales.SettingsAITranslationService(), "", func() { form.Value(c, locales.SettingsAINotConfigured(), 13) })
		}).Margin(6, 0, 0, 0)
		return
	}
	preset, _ := s.CurrentPreset()
	form.Card(c, func() {
		form.Row(c, locales.SettingsAIService(), "", func() {
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
		form.Row(c, "API Key", "", func() {
			c.Key("llm-key")
			if field(breeze.PasswordField(c, s.LLMKey()).Label("API Key")).Changed() {
				s.ApplyLLM()
			}
		})
		if cli := s.EveryAPI(); cli.Missing || cli.Checking || cli.SignedOut {
			form.Separator(c, true)
			s.everyAPIRow(c, cli)
		}
		form.Separator(c, true)
		form.Row(c, locales.SettingsAIModel(), "", func() { s.modelSelect(c, t) })
		ui.Row(c).Shrink(0).MinHeight(40).Padding(6, 14).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { s.testStatus(c, t) })
			c.Key("llm-test")
			if form.Button(c, locales.SettingsAITest(), false).Disabled(s.TestingLLM()).Clicked() && !s.TestingLLM() {
				s.TestLLM()
			}
		})
	}).Margin(6, 0, 0, 0)
	key := locales.SettingsAIKeyStoredLocally()
	if s.LLMProvider() == "everyapi" && strings.TrimSpace(*s.LLMKey()) == "" {
		key = locales.SettingsAIKeyEveryAPILogin()
	}
	note := locales.SettingsAINote(key, locales.SettingsAICompatible())
	if preset.Blurb != "" {
		note = locales.SettingsAINoteWithBlurb(preset.Blurb, key, locales.SettingsAICompatible())
	}
	form.Note(c, note, 0)
}

// everyAPIRow says what the everyapi command-line program, whose sign-in EveryAPI uses without an API Key, still needs: to be installed, with 获取 EveryAPI to open the website it is downloaded from, or to be signed in to in Terminal; 重新检测 looks for it and asks it again afterwards. While Kando asks it whether someone is signed in, the row says so and 重新检测 waits.
func (s screen) everyAPIRow(c *ui.Context, cli EveryAPI) {
	note := locales.ReadinessCheckingSignIn()
	switch {
	case cli.Missing && cli.OpenErr != "":
		note = locales.SettingsAIEveryAPIOpenFailed(cli.OpenErr)
	case cli.Missing:
		note = locales.SettingsAIEveryAPIMissing()
	case cli.SignedOut:
		note = locales.SettingsAIEveryAPISignedOut()
	}
	form.Row(c, locales.SettingsAIEveryAPICLI(), note, func() {
		if cli.Missing {
			c.Key("everyapi-get")
			if form.Button(c, locales.SettingsAIGetEveryAPI(), false).Clicked() {
				s.GetEveryAPI()
			}
		}
		c.Key("everyapi-redetect")
		if form.Button(c, locales.SettingsAIRedetect(), false).Disabled(cli.Checking).Clicked() && !cli.Checking {
			s.Redetect()
		}
	})
}

// providerSelect chooses the service among the presets. The drop-down shows their names, the ones on this Mac after their group, as 本机 · Ollama; the setting keeps the id. Grouped drop-downs (mygo-breeze-ui B10) are not planned, so the group stays in the name.
func (s screen) providerSelect(c *ui.Context) {
	presets := s.Presets()
	labels := make([]string, len(presets))
	chosen := ""
	for i, preset := range presets {
		labels[i] = preset.Name
		if preset.Group == GroupLocal {
			labels[i] = locales.SettingsAILocalService(preset.Name)
		}
		if preset.ID == s.LLMProvider() {
			chosen = labels[i]
		}
	}
	c.Key("llm-provider")
	if form.Select(c, &chosen, labels).Label(locales.SettingsAIChooseService()).Changed() {
		for i, label := range labels {
			if label == chosen {
				s.SelectLLM(presets[i].ID)
			}
		}
	}
}

// modelSelect chooses the model among those the service lists, which the app fetches once the service, its Base URL and its key are known (the endpoint's GET /models), rather than having it typed. A model chosen before stays on top of the list when the service no longer lists it, so it is not changed behind the user's back. While the list loads, or when it could not be fetched, the row says so, with 重试.
func (s screen) modelSelect(c *ui.Context, t *ui.Theme) {
	model := s.LLMModel()
	options := s.LLMModels()
	if current := strings.TrimSpace(*model); current != "" && !slices.Contains(options, current) {
		options = append([]string{current}, options...)
	}
	if len(options) > 0 {
		c.Key("llm-model")
		if field(form.Select(c, model, options).Label(locales.SettingsAIModel())).Changed() {
			s.ApplyLLM()
		}
	}
	switch {
	case s.LoadingLLMModels():
		if len(options) == 0 {
			form.Value(c, locales.SettingsAILoadingModels(), 13)
		}
	case s.LLMModelsErr() != "":
		if len(options) == 0 {
			ui.Text(c, s.LLMModelsErr()).FontSize(12).TextColor(t.Danger).Shrink(1).Wrap()
		}
		c.Key("llm-models-retry")
		if form.Button(c, locales.SettingsAIRetry(), false).Tooltip(s.LLMModelsErr()).Clicked() {
			s.RetryLLMModels()
		}
	}
}

// testStatus is the connection test's state: a dot and a word, the error when it failed.
func (s screen) testStatus(c *ui.Context, t *ui.Theme) {
	dot, color, text := form.Idle, form.Secondary, locales.SettingsAIUntested()
	switch result := s.LLMResult(); {
	case s.TestingLLM():
		text = locales.SettingsAITesting()
	case result == locales.SettingsAIConnected():
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
