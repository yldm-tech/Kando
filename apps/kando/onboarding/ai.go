package onboarding

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Service is a translation service the second step offers, as Settings › AI 服务 lists it: its id, its name as the drop-down shows it, the heading it is listed under (Group), whether the step asks for its API Key (Key), whether it has a page where the key is made, which 获取 API Key opens (KeyPage), and whether it is set up in Settings › AI 服务 instead, as a custom endpoint, whose Base URL the step has no field for (InSettings).
type Service struct {
	ID, Name, Group          string
	Key, KeyPage, InSettings bool
}

// aiCard is the second step's card of the translation service, so the guide ends with translation working: the service, chosen in place among an EveryAPI account, the user's own API Key and a model on this computer, with which model translates or what it still needs; its API Key where it takes one, with 获取 API Key; and 测试连接 with its result. EveryAPI without a key signs in to an account instead, so until it is signed in the card offers 登录 in place of the test. 设置… opens Settings › AI 服务 for the rest, as another model or a custom endpoint.
type aiCard struct {
	a  App
	ai func() AI
}

// Render implements core.Node.
func (c aiCard) Render(cx *core.Scope) core.Node {
	a, ai := c.a, c.ai
	k, colors := theme.Of(cx), ui.ColorsOf(cx)
	current := live.Memo(cx, a, func() Service {
		for _, s := range a.OnboardingServices(cx) {
			if s.ID == a.LLMProvider() {
				return s
			}
		}
		return Service{ID: a.LLMProvider()}
	})
	signIn := live.Memo(cx, a, a.OnboardingEveryAPISignIn)
	head := core.View{Style: rowStyle.Merge(core.Style{MinHeight: core.Pt(46)}), Children: []core.Node{
		rowText(func() string { return locales.OnboardingTranslationService(cx) }, func() (string, core.Color) {
			if ai().Missing {
				return ai().Text, k.WarnInk()
			}
			return ai().Text, colors.Muted()
		}),
		core.Show{When: func() bool { return ai().Ready }, Then: func(*core.Scope) core.Node { return mark(readyMark) }},
		serviceSelect(cx, a),
	}}
	key := core.Show{When: func() bool { return current().Key }, Then: func(cx *core.Scope) core.Node {
		return core.Group{rule(), keyRow(cx, a, func() bool { return current().KeyPage })}
	}}
	// The test's result, once there is one; before that the head says what the service needs.
	result := core.Show{When: live.Func(a, func() bool { return a.TestingLLM() || a.LLMResult() != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "onboarding-ai-result", ContentFunc: live.Func(a, func() string {
			switch {
			case a.TestingLLM():
				return locales.SettingsAITesting(cx)
			case a.LLMConnected():
				return locales.SettingsAIConnected(cx)
			}
			return a.LLMResult()
		}), Style: core.Style{FontSize: 11, LineHeight: 15, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: live.Func(a, func() core.Style {
			switch {
			case a.TestingLLM():
				return core.Style{Color: k.AccentInk()}
			case a.LLMConnected():
				return core.Style{Color: k.SuccessInk()}
			}
			return core.Style{Color: colors.Danger()}
		})}
	}}
	settings := smallButton(cx, "onboarding-ai", func() string { return locales.OnboardingSettings(cx) },
		ui.Button{OnPress: live.Do(a, func() { a.OpenSettings(sectionid.AI) })})
	actions := core.Show{When: signIn, Then: func(cx *core.Scope) core.Node {
		return smallButton(cx, "onboarding-everyapi-signin", func() string { return locales.EveryAPISignIn(cx) }, ui.Button{OnPress: live.Do(a, a.EveryAPISignIn)})
	}, Else: func(cx *core.Scope) core.Node {
		return smallButton(cx, "onboarding-ai-test", func() string { return locales.SettingsAITest(cx) },
			ui.Button{Disabled: live.Func(a, a.TestingLLM), OnPress: live.Do(a, func() {
				if !a.TestingLLM() {
					a.TestLLM()
				}
			})})
	}}
	foot := core.View{Style: rowStyle.Merge(core.Style{MinHeight: core.Pt(40)}), Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{result}},
		settings,
		actions,
	}}
	return card(head, key, rule(), foot)
}

// serviceSelect is the drop-down of the services, as Settings › AI 服务 has it, under the headings of the three ways to translate (Service.Group); one set up there, as a custom endpoint, opens it. ui.Select takes its name and options as it is built, so it is built again when the language shown changes the names, which happens while the settings, not this field, have the keyboard.
func serviceSelect(cx *core.Scope, a App) core.Node {
	face := live.Memo(cx, a, func() selectFace {
		var names []string
		for _, s := range a.OnboardingServices(cx) {
			names = append(names, s.ID+"\x01"+s.Name+"\x01"+s.Group)
		}
		return selectFace{label: locales.SettingsAIChooseService(cx), choices: strings.Join(names, "\x00")}
	})
	return core.Dynamic(func(*core.Scope) core.Node {
		f := face()
		var options []ui.Option[string]
		for _, choice := range strings.Split(f.choices, "\x00") {
			fields := strings.SplitN(choice, "\x01", 3)
			options = append(options, ui.Option[string]{Value: fields[0], Label: fields[1], Group: fields[2]})
		}
		return ui.Select[string]{TestID: "onboarding-ai-service", Label: f.label, Options: options, Style: core.Style{Width: core.Pt(150), FlexShrink: 0, AlignSelf: core.AlignCenter},
			Value: live.Func(a, a.LLMProvider), OnChange: live.On(a, func(id string) {
				a.SelectLLM(id)
				for _, s := range a.OnboardingServices(cx) {
					if s.ID == id && s.InSettings {
						a.OpenSettings(sectionid.AI)
					}
				}
			})}
	})
}

// keyRow is the service's API Key, typed into a masked field, with 获取 API Key after it when the service has a page where the key is made. The field is built once, so it keeps the keyboard and what was typed as the app's state changes.
func keyRow(cx *core.Scope, a App, page func() bool) core.Node {
	field := ui.Input{TestID: "onboarding-ai-key", Label: "API Key", Placeholder: "API Key", Masked: true,
		Value: live.Func(a, func() string { return *a.LLMKey() }),
		OnChange: live.On(a, func(text string) {
			*a.LLMKey() = text
			a.ApplyLLM()
		}),
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}}
	get := core.Show{When: page, Then: func(cx *core.Scope) core.Node {
		return smallButton(cx, "onboarding-ai-get-key", func() string { return locales.SettingsAIGetKey(cx) }, ui.Button{OnPress: live.Do(a, a.OpenKeyPage)})
	}}
	return core.View{Style: rowStyle.Merge(core.Style{MinHeight: core.Pt(42)}), Children: []core.Node{field, get}}
}
