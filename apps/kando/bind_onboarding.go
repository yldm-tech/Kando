package main

import (
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The first-run guide's reads and clicks (onboarding.App).

func (a *app) OnboardingStep() int { return a.onb.step }

func (a *app) LanguageChoices() []string { return settings.LanguageChoices(a.prefs.language) }

// OnboardingTargetName names the target language as every screen does (subtitle.TargetLanguageName).
func (a *app) OnboardingTargetName(cx *ns.Scope, value string) string {
	return subtitle.TargetLanguageName(cx, value)
}

func (a *app) OnboardingSave() { a.onboardingSave() }

func (a *app) OnboardingLearning() []string { return a.onb.learning }

// OnboardingToggle adds code to the languages chosen, or takes it out, and saves them.
func (a *app) OnboardingToggle(code string) {
	if !onboarding.Known(code) {
		return
	}
	kept := onboarding.Effective(a.onb.learning, a.prefs.language)
	at := -1
	for i, c := range kept {
		if c == code {
			at = i
		}
	}
	if at >= 0 {
		kept = append(kept[:at], kept[at+1:]...)
	} else {
		kept = append(kept, code)
	}
	// The language subtitles are translated into stays chosen when it was, for when another target language is picked.
	own := onboarding.LanguageCode(a.prefs.language)
	for _, c := range a.onb.learning {
		if c == own && c != code {
			kept = append(kept, c)
		}
	}
	a.onb.learning = kept
	a.onboardingSave()
}

func (a *app) OnboardingTools(cx *ns.Scope) []onboarding.Tool { return a.onboardingTools(cx) }

func (a *app) OnboardingDownloadTool(name string) { a.kitsInstall(name) }

func (a *app) OnboardingModels(cx *ns.Scope) []onboarding.Model { return a.onboardingModels(cx) }

func (a *app) OnboardingDownload(id string) { a.onboardingDownload(id) }

// OnboardingDownloadFromMirror downloads model id again from hf-mirror.com, which becomes the download source, as the button by a failed download from Hugging Face does.
func (a *app) OnboardingDownloadFromMirror(id string) {
	a.onb.lastDownload = id
	a.speechDownloadFromMirror(id)
}

func (a *app) OnboardingCancelDownload() { a.cancelDownload() }

// OnboardingUseModel makes the downloaded model id the one speech recognition uses, as 使用 in 设置 › 语音识别 does.
func (a *app) OnboardingUseModel(id string) {
	if path := a.prefs.speechModels[id]; path != "" {
		a.useSpeechModel(path)
	}
}

func (a *app) OnboardingDownloading() bool { return a.prefs.downloading != "" }

func (a *app) OnboardingDownloadsOnNext() bool { return a.onboardingDownloadsOnNext() }

// OnboardingAI says which model translates, or what the translation still needs, as the sidebar's status line words it, in the language shown under cx: the Base URL of a custom endpoint, which is typed in Settings › AI 服务, the API Key of a service that cannot do without one, a model to choose, which the service's list fills in while it loads (onboardingLoadModels), or the EveryAPI sign-in.
func (a *app) OnboardingAI(cx *ns.Scope) onboarding.AI {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return onboarding.AI{Text: locales.OnboardingAIConfigured(cx), Ready: true}
	}
	model := strings.TrimSpace(tools.Translator.Model)
	preset, _ := settings.PresetByID(locales.For(cx), a.prefs.llmProvider)
	switch {
	case strings.TrimSpace(tools.Translator.BaseURL) == "":
		return onboarding.AI{Text: locales.OnboardingAINeedsBaseURL(cx), Missing: true}
	case preset.NeedsKey() && tools.Translator.APIKey == "":
		return onboarding.AI{Text: locales.OnboardingAINeedsKey(cx), Missing: true}
	case model == "" && (a.prefs.llmModelsLoading || a.prefs.llmWaiting):
		return onboarding.AI{Text: locales.SettingsAILoadingModels(cx)}
	case model == "" && a.prefs.llmModelsErr != nil:
		return onboarding.AI{Text: settings.LLMModelsError(locales.For(cx), a.prefs.llmModelsErr), Missing: true}
	case model == "":
		return onboarding.AI{Text: noLLMModel(cx), Missing: true}
	case everyAPICLIMissing(tools.Translator) && a.prefs.everyAPIOpenErr != "":
		return onboarding.AI{Text: locales.SettingsAIEveryAPIOpenFailed(cx, a.prefs.everyAPIOpenErr), Missing: true}
	case everyAPICLIMissing(tools.Translator):
		return onboarding.AI{Text: everyAPIMissing(cx), Missing: true}
	case tools.Translator.Credential != nil && a.side.ready.signInErr != nil && a.side.ready.signInDone:
		return onboarding.AI{Text: everyAPISignInFailed(a.side.ready.signInErr)(cx), Missing: true}
	case tools.Translator.Credential != nil && !a.side.ready.signInDone:
		// The sidebar checks the sign-in once the tools and a speech model are there, so before that it is not known; 测试连接 checks it too.
		return onboarding.AI{Text: locales.OnboardingAIModelUnchecked(cx, model)}
	}
	return onboarding.AI{Text: locales.OnboardingAIModel(cx, model), Ready: true}
}

// OnboardingServices are the services of Settings › AI 服务 (settings.Presets), named in the language shown under cx, the ones on this computer after their group as the settings name them. The guide asks for the key of the services in the cloud but EveryAPI, which signs in with the everyapi command-line program; those on this computer need none, and a custom endpoint is set up in the settings, where its Base URL is typed.
func (a *app) OnboardingServices(cx *ns.Scope) []onboarding.Service {
	presets := settings.Presets(locales.For(cx))
	out := make([]onboarding.Service, len(presets))
	for i, preset := range presets {
		name := preset.Name
		if preset.Group == settings.GroupLocal {
			name = locales.SettingsAILocalService(cx, preset.Name)
		}
		out[i] = onboarding.Service{ID: preset.ID, Name: name, Key: preset.NeedsKey(), KeyPage: preset.KeyURL != "", InSettings: preset.Group == settings.GroupOther}
	}
	return out
}

func (a *app) OnboardingEveryAPIMissing() bool { return a.everyAPIStatus().Missing }

func (a *app) OnboardingLink() *string { return &a.onb.link }

func (a *app) OnboardingLinkError() string {
	if a.onb.errLink != a.onb.link {
		return ""
	}
	return a.onb.linkErr
}

func (a *app) OnboardingBack() {
	if a.onb.step > 1 {
		a.onb.step--
	}
	a.onb.linkErr = ""
}

func (a *app) OnboardingNext() { a.onboardingNext() }

func (a *app) OnboardingSkip() { a.closeOnboarding() }
