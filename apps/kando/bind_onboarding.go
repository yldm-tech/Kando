package main

import (
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// The first-run guide's reads and clicks (onboarding.App).

func (a *app) OnboardingStep() int { return a.onb.step }

func (a *app) LanguageChoices() []string { return settings.LanguageChoices(a.prefs.language) }

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

func (a *app) OnboardingTools() []onboarding.Tool { return a.onboardingTools() }

func (a *app) OnboardingDownloadTool(name string) { a.kitsInstall(name) }

func (a *app) OnboardingModels() []onboarding.Model { return a.onboardingModels() }

func (a *app) OnboardingDownload(id string) { a.onboardingDownload(id) }

func (a *app) OnboardingCancelDownload() { a.cancelDownload() }

// OnboardingUseModel makes the downloaded model id the one speech recognition uses, as 使用 in 设置 › 语音识别 does.
func (a *app) OnboardingUseModel(id string) {
	if path := a.prefs.speechModels[id]; path != "" {
		a.useSpeechModel(path)
	}
}

func (a *app) OnboardingDownloading() bool { return a.prefs.downloading != "" }

func (a *app) OnboardingDownloadsOnNext() bool { return a.onboardingDownloadsOnNext() }

// OnboardingAI says which model translates, or what the translation still needs, as the sidebar's status line words it: a model to choose, or the EveryAPI sign-in or a key.
func (a *app) OnboardingAI() onboarding.AI {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return onboarding.AI{Text: locales.OnboardingAIConfigured(), Ready: true}
	}
	model := strings.TrimSpace(tools.Translator.Model)
	switch {
	case model == "":
		return onboarding.AI{Text: noLLMModel(), Missing: true}
	case tools.Translator.Credential == nil && tools.Translator.BaseURL == media.EveryAPIBaseURL && tools.Translator.APIKey == "":
		return onboarding.AI{Text: notSignedIn(), Missing: true}
	case tools.Translator.Credential != nil && a.side.ready.signInErr != nil && a.side.ready.signInDone:
		return onboarding.AI{Text: notSignedIn(), Missing: true}
	case tools.Translator.Credential != nil && !a.side.ready.signInDone:
		// The sidebar checks the sign-in once the tools and a speech model are there, so before that it is not known.
		return onboarding.AI{Text: locales.OnboardingAIModelUnchecked(model)}
	}
	return onboarding.AI{Text: locales.OnboardingAIModel(model), Ready: true}
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
