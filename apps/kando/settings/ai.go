package settings

import (
	"slices"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ai is the AI 服务 page: the service that translates and how Kando reaches it, in one card as the prototype's, with where the key comes from in the footnote under it. The prototype's page starts 6 points lower than one that opens with a section label.
type ai struct{ s screen }

// Render implements core.Node.
func (p ai) Render(cx *core.Scope) core.Node {
	a := p.s.a
	return core.Show{When: live.Func(a, func() bool { _, known := a.Tools(); return known }),
		Then: func(cx *core.Scope) core.Node { return aiService(cx, a) },
		Else: func(cx *core.Scope) core.Node {
			card := form.Card(form.Row{Title: locales.SettingsAITranslationService(cx), Trailing: []core.Node{form.Value(static(locales.SettingsAINotConfigured(cx)), 13)}})
			card.Style.Margin = core.Edges{Top: 6}
			return card
		}}
}

// aiService is the page once there are tools to translate with.
func aiService(cx *core.Scope, a App) core.Node {
	// mark is the service's logo, built again only when the service changes.
	provider := core.NewMemo(cx, live.Func(a, func() string { preset, _ := a.CurrentPreset(); return preset.ID }))
	mark := core.Dynamic(func(*core.Scope) core.Node {
		if logo := providerMark(provider.Get()); logo != nil {
			return core.SVGView{SVG: logo, Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0}}
		}
		return core.Group(nil)
	})
	base := ui.Input{TestID: "settings-llm-base", Label: "Base URL",
		Value: live.Func(a, func() string { return *a.LLMBase() }),
		OnChange: live.On(a, func(text string) {
			*a.LLMBase() = text
			a.SetLLMProvider(a.ProviderID(text))
			a.ApplyLLM()
		}),
		Style: core.Style{Width: core.Pt(330), FontFamily: "monospace"}}
	key := ui.Input{TestID: "settings-llm-key", Label: "API Key", Masked: true,
		Value:           live.Func(a, func() string { return *a.LLMKey() }),
		PlaceholderFunc: keyPlaceholder(cx, a),
		OnChange: live.On(a, func(text string) {
			*a.LLMKey() = text
			a.ApplyLLM()
		}),
		Style: core.Style{Width: core.Pct(100)}}
	// keyPage is whether the service has a page where its key is made, which 获取 API Key opens; the field gives the button room, so the row ends where the others do.
	keyPage := live.Func(a, func() bool { preset, _ := a.CurrentPreset(); return preset.KeyURL != "" })
	keyField := core.View{Children: []core.Node{key}, StyleFunc: func() core.Style {
		if keyPage() {
			return core.Style{Width: core.Pt(230)}
		}
		return core.Style{Width: core.Pt(330)}
	}}
	getKey := core.Show{When: keyPage, Then: func(cx *core.Scope) core.Node {
		return form.Button{Text: locales.SettingsAIGetKey(cx), TestID: "settings-llm-get-key", OnPress: live.Do(a, a.OpenKeyPage)}
	}}
	everyAPI := core.Show{When: live.Func(a, func() bool {
		cli := a.EveryAPI()
		return cli.Missing || cli.Checking || cli.SignedOut || cli.Failed != ""
	}), Then: func(cx *core.Scope) core.Node {
		return core.Group([]core.Node{form.Separator(true), everyAPIRow(cx, a)})
	}}
	testing := live.Func(a, a.TestingLLM)
	test := core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{testStatus(cx, a)}},
			form.Button{Text: locales.SettingsAITest(cx), TestID: "settings-llm-test", Disabled: testing, OnPress: live.Do(a, func() {
				if !a.TestingLLM() {
					a.TestLLM()
				}
			})},
		}}
	card := form.Card(
		form.Row{Title: locales.SettingsAIService(cx), NoGroup: true, Trailing: []core.Node{mark, providerSelect(cx, a)}},
		form.Separator(true),
		form.Row{Title: "Base URL", NoGroup: true, Trailing: []core.Node{base}},
		form.Separator(true),
		form.Row{Title: "API Key", NoGroup: true, Trailing: []core.Node{keyField, getKey}},
		everyAPI,
		form.Separator(true),
		form.Row{Title: locales.SettingsAIModel(cx), NoGroup: true, SubtitleFunc: modelsFailure(a), Trailing: []core.Node{modelSelect(cx, a)}},
		test,
	)
	card.Style.Margin = core.Edges{Top: 6}
	return form.Page(card, form.Note(aiNote(cx, a), 0))
}

// aiNote is the footnote under the card: what the service is for one on this computer, where the key comes from otherwise, and for a custom endpoint which ones work.
func aiNote(cx *core.Scope, a App) func() string {
	return live.Func(a, func() string {
		preset, _ := a.CurrentPreset()
		switch {
		case preset.Group == GroupLocal:
			return locales.SettingsAINoteWithBlurb(cx, preset.Blurb)
		case preset.Group == GroupOther:
			return locales.SettingsAINote(cx, locales.SettingsAIKeyStoredLocally(cx), locales.SettingsAICompatible(cx))
		case a.LLMProvider() == "everyapi" && strings.TrimSpace(*a.LLMKey()) == "":
			return locales.SettingsAIKeyEveryAPILogin(cx)
		}
		return locales.SettingsAIKeyStoredLocally(cx)
	})
}

// keyPlaceholder is what the empty API Key field says: that it may stay empty for EveryAPI, which signs in with the everyapi command-line program, and for a service on this computer.
func keyPlaceholder(cx *core.Scope, a App) func() string {
	return live.Func(a, func() string {
		if preset, _ := a.CurrentPreset(); preset.ID == "everyapi" || preset.Group == GroupLocal {
			return locales.SettingsAIKeyOptional(cx)
		}
		return ""
	})
}

// everyAPIRow says what the everyapi command-line program, whose sign-in EveryAPI uses without an API Key, still needs: to be installed, with 获取 EveryAPI to open the website it is downloaded from, or to be signed in to in Terminal, or what it said when asking it failed otherwise; 重新检测 looks for it and asks it again afterwards. While Kando asks it whether someone is signed in, the row says so and 重新检测 waits.
func everyAPIRow(cx *core.Scope, a App) core.Node {
	note := live.Func(a, func() string {
		switch cli := a.EveryAPI(); {
		case cli.Missing && cli.OpenErr != "":
			return locales.SettingsAIEveryAPIOpenFailed(cx, cli.OpenErr)
		case cli.Missing:
			return locales.SettingsAIEveryAPIMissing(cx)
		case cli.SignedOut:
			return locales.SettingsAIEveryAPISignedOut(cx)
		case cli.Failed != "":
			return locales.SettingsAIEveryAPIFailed(cx, cli.Failed)
		}
		return locales.ReadinessCheckingSignIn(cx)
	})
	get := core.Show{When: live.Func(a, func() bool { return a.EveryAPI().Missing }), Then: func(cx *core.Scope) core.Node {
		return form.Button{Text: locales.SettingsAIGetEveryAPI(cx), OnPress: live.Do(a, a.GetEveryAPI), TestID: "settings-everyapi-get"}
	}}
	checking := live.Func(a, func() bool { return a.EveryAPI().Checking })
	redetect := form.Button{Text: locales.SettingsAIRedetect(cx), TestID: "settings-everyapi-redetect", Disabled: checking, OnPress: live.Do(a, func() {
		if !a.EveryAPI().Checking {
			a.Redetect()
		}
	})}
	return form.Row{Title: locales.SettingsAIEveryAPICLI(cx), SubtitleFunc: note, Trailing: []core.Node{get, redetect}}
}

// providerSelect chooses the service among the presets. The drop-down shows their names, the ones on this Mac after their group, as 本机 · Ollama; the setting keeps the id.
func providerSelect(cx *core.Scope, a App) core.Node {
	presets := a.Presets()
	options := make([]ui.Option[string], len(presets))
	for i, preset := range presets {
		label := preset.Name
		if preset.Group == GroupLocal {
			label = locales.SettingsAILocalService(cx, preset.Name)
		}
		options[i] = ui.Option[string]{Value: preset.ID, Label: label}
	}
	return ui.Select[string]{Options: options, Label: locales.SettingsAIChooseService(cx), TestID: "settings-llm-provider",
		Value: live.Func(a, a.LLMProvider), OnChange: live.On(a, a.SelectLLM), Style: core.Style{Width: core.Pt(220)}}
}

// modelSelect chooses the model among those the service lists, which the app fetches once the service, its Base URL and its key are known (the endpoint's GET /models). A model chosen before stays on top of the list when the service no longer lists it, so it is not changed behind the user's back. The drop-down is searched by typing part of a model's ID, as a gateway such as OpenRouter lists hundreds. While the list loads the row says so; when it could not be fetched or the service lists none, as a proxy without GET /models, the model's ID is typed into a field instead, with 重试 to fetch the list again.
func modelSelect(cx *core.Scope, a App) core.Node {
	// typed is whether the model's ID is typed rather than chosen. It does not follow the model itself, so the field stays while it is typed into.
	typed := live.Func(a, func() bool { return !a.LoadingLLMModels() && len(a.LLMModels()) == 0 })
	field := func(cx *core.Scope) core.Node {
		return ui.Input{TestID: "settings-llm-model-id", Label: locales.SettingsAIModel(cx),
			Value: live.Func(a, func() string { return *a.LLMModel() }),
			OnChange: live.On(a, func(text string) {
				*a.LLMModel() = text
				a.ApplyLLM()
			}),
			Style: core.Style{Width: core.Pt(330), FontFamily: "monospace"}}
	}
	// options is the list, joined, so the drop-down is built again only when the list changes.
	options := core.NewMemo(cx, live.Func(a, func() string {
		list := a.LLMModels()
		if current := strings.TrimSpace(*a.LLMModel()); current != "" && !slices.Contains(list, current) {
			list = append([]string{current}, list...)
		}
		return strings.Join(list, "\n")
	}))
	list := func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			joined := options.Get()
			if joined == "" {
				return form.Value(static(locales.SettingsAILoadingModels(cx)), 13)
			}
			models := strings.Split(joined, "\n")
			opts := make([]ui.Option[string], len(models))
			for i, model := range models {
				opts[i] = ui.Option[string]{Value: model, Label: model}
			}
			return ui.Select[string]{Options: opts, Label: locales.SettingsAIModel(cx), TestID: "settings-llm-model",
				Searchable: true, SearchPlaceholder: locales.SettingsAISearchModels(cx),
				Value: live.Func(a, func() string { return strings.TrimSpace(*a.LLMModel()) }),
				OnChange: live.On(a, func(model string) {
					*a.LLMModel() = model
					a.ApplyLLM()
				}),
				Style: core.Style{Width: core.Pt(330)}}
		})
	}
	choose := core.Show{When: typed, Then: field, Else: list}
	retry := core.Show{When: live.Func(a, func() bool { return !a.LoadingLLMModels() && a.LLMModelsErr() != "" }), Then: func(cx *core.Scope) core.Node {
		// 重试 says why the list could not be fetched as the pointer rests on it, as does the line under the row's title.
		return ui.Tooltip{TextFunc: live.Func(a, a.LLMModelsErr),
			Trigger: form.Button{Text: locales.SettingsAIRetry(cx), OnPress: live.Do(a, a.RetryLLMModels), TestID: "settings-llm-models-retry"}}
	}}
	return core.Group([]core.Node{choose, retry})
}

// modelsFailure is why the service's model list could not be fetched, under the Model row's title, or "" while it could be or is being fetched.
func modelsFailure(a App) func() string {
	return live.Func(a, func() string {
		if a.LoadingLLMModels() {
			return ""
		}
		return a.LLMModelsErr()
	})
}

// testStatus is the connection test's state: a dot and a word, why it failed when it did, with what the service answered as the pointer rests on it.
func testStatus(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	failed := func() bool { return !a.TestingLLM() && !a.LLMConnected() && a.LLMResult() != "" }
	dot := live.Func(a, func() core.Color {
		switch {
		case a.TestingLLM():
			return form.Idle
		case a.LLMConnected():
			return form.OK
		case failed():
			return colors.Danger()
		}
		return form.Idle
	})
	color := live.Func(a, func() core.Color {
		switch {
		case a.TestingLLM():
			return form.Secondary
		case a.LLMConnected():
			return form.SuccessText
		case failed():
			return colors.Danger()
		}
		return form.Secondary
	})
	text := live.Func(a, func() string {
		switch {
		case a.TestingLLM():
			return locales.SettingsAITesting(cx)
		case a.LLMConnected():
			return locales.SettingsAIConnected(cx)
		case failed():
			return a.LLMResult()
		}
		return locales.SettingsAIUntested(cx)
	})
	status := form.Status(dot, color, text)
	return core.Show{When: live.Func(a, func() bool { return failed() && a.LLMResultDetail() != "" }),
		Then: func(*core.Scope) core.Node {
			return ui.Tooltip{TextFunc: live.Func(a, a.LLMResultDetail), Trigger: status, TestID: "settings-llm-test-detail",
				Style: core.Style{Direction: core.Row, FlexShrink: 1, MinWidth: core.Pt(0)}}
		},
		Else: func(*core.Scope) core.Node { return status }}
}
