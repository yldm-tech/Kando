package main

import (
	"slices"
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

func (a *app) OnboardingDownloadTool(name string) { a.toolsUseOwn(name) }

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
	preset, _ := settings.PresetByID(a.llmPresets(locales.For(cx)), a.prefs.llmProvider)
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
	case tools.Translator.EveryAPIUnsigned() && a.everyapi.signingIn:
		return onboarding.AI{Text: locales.EveryAPIWaitingBrowser(cx)}
	case tools.Translator.EveryAPIUnsigned():
		return onboarding.AI{Text: everyAPISignIn(cx), Missing: true}
	case a.signInFailed(tools.Translator) != nil:
		return onboarding.AI{Text: everyAPISignInFailed(a.signInFailed(tools.Translator))(cx), Missing: true}
	case a.signInUnchecked(tools.Translator):
		// The sidebar checks the sign-in once the tools and a speech model are there, so before that it is not known; 测试连接 checks it too.
		return onboarding.AI{Text: locales.OnboardingAIModelUnchecked(cx, model)}
	}
	return onboarding.AI{Text: locales.OnboardingAIModel(cx, model), Ready: true}
}

// OnboardingServices are the services of Settings › AI 服务 (settings.Presets), named in the language shown under cx, the ones on this computer after their group as the settings name them, under three headings offered alike: an EveryAPI account to sign in to, the user's own API Key for a service in the cloud or a custom endpoint, and a model on this computer. Where the interface is in simplified Chinese, the services reachable from mainland China come first among those that take a key (onboardingDomestic). The guide asks for the key of the services in the cloud but EveryAPI, which signs in to an account; those on this computer need none, and a custom endpoint is set up in the settings, where its Base URL is typed.
func (a *app) OnboardingServices(cx *ns.Scope) []onboarding.Service {
	m := locales.For(cx)
	presets := settings.Presets(m)
	if m.Language() == "zh-Hans" {
		presets = onboardingDomesticFirst(presets)
	}
	var account, key, local []onboarding.Service
	for _, preset := range presets {
		s := onboarding.Service{ID: preset.ID, Name: preset.Name, Key: preset.NeedsKey(), KeyPage: preset.KeyURL != "", InSettings: preset.Group == settings.GroupOther}
		switch {
		case preset.ID == "everyapi":
			s.Group = locales.OnboardingServicesAccount(cx)
			account = append(account, s)
		case preset.Group == settings.GroupLocal:
			s.Name, s.Group = locales.SettingsAILocalService(cx, preset.Name), locales.OnboardingServicesLocal(cx)
			local = append(local, s)
		default:
			s.Group = locales.OnboardingServicesKey(cx)
			key = append(key, s)
		}
	}
	return append(append(account, key...), local...)
}

// onboardingDomestic are the services that take a key reachable from mainland China, in the order the guide lists them first there.
var onboardingDomestic = []string{"deepseek", "dashscope", "moonshot", "zhipu", "siliconflow"}

// onboardingDomesticFirst is presets with onboardingDomestic first, in that order, and the others as they were.
func onboardingDomesticFirst(presets []settings.Preset) []settings.Preset {
	out := make([]settings.Preset, 0, len(presets))
	for _, id := range onboardingDomestic {
		if p, ok := settings.PresetByID(presets, id); ok {
			out = append(out, p)
		}
	}
	for _, p := range presets {
		if !slices.Contains(onboardingDomestic, p.ID) {
			out = append(out, p)
		}
	}
	return out
}

// OnboardingEveryAPISignIn reports whether EveryAPI is in use without an API Key or a sign-in, which the guide's 登录 gives it.
func (a *app) OnboardingEveryAPISignIn() bool {
	tools, ok := a.pipeline.(media.Tools)
	return ok && tools.Translator.EveryAPIUnsigned()
}

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
