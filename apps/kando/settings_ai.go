package main

import (
	"context"
	"errors"
	"log"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// Settings › AI 服务: the translation model's provider, endpoint and key.

// savedLLM is the AI service's part of settings.json: the service in use, and LLMServices, the others used before, by their preset id (settings.Presets), so choosing one again brings back its key and model.
type savedLLM struct {
	LLMProvider string                     `json:"llmProvider,omitempty"`
	LLMBaseURL  string                     `json:"llmBaseURL,omitempty"`
	LLMModel    string                     `json:"llmModel,omitempty"`
	LLMAPIKey   string                     `json:"llmAPIKey,omitempty"`
	LLMServices map[string]savedLLMService `json:"llmServices,omitempty"`
}

// savedLLMService is a service used before: its endpoint, which only 自定义 takes back, as the others have their preset's, the model chosen on it and its key, which goes back only to the host it was used with.
type savedLLMService struct {
	BaseURL string `json:"baseURL,omitempty"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
}

func (s *savedLLM) clean() {
	s.LLMBaseURL = strings.TrimSpace(s.LLMBaseURL)
	s.LLMModel = strings.TrimSpace(s.LLMModel)
	s.LLMAPIKey = strings.TrimSpace(s.LLMAPIKey)
	s.LLMProvider = strings.TrimSpace(s.LLMProvider)
	for id, service := range s.LLMServices {
		service = savedLLMService{BaseURL: strings.TrimSpace(service.BaseURL), Model: strings.TrimSpace(service.Model), APIKey: strings.TrimSpace(service.APIKey)}
		if _, known := settings.PresetByID(locales.In(locales.Source), id); !known || service.BaseURL == "" {
			delete(s.LLMServices, id)
			continue
		}
		s.LLMServices[id] = service
	}
	if len(s.LLMServices) == 0 {
		s.LLMServices = nil
	}
}

func (a *app) saveLLM(saved *savedSettings) {
	saved.LLMProvider = a.prefs.llmProvider
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.LLMBaseURL = tools.Translator.BaseURL
		saved.LLMModel = tools.Translator.Model
		saved.LLMAPIKey = tools.Translator.APIKey
	}
	saved.LLMServices = maps.Clone(a.prefs.llmServices)
}

// restoreLLM uses the AI service saved last time for what the environment does not name. The provider is read from the endpoint, not from the saved llmProvider.
func (a *app) restoreLLM(saved savedSettings) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if os.Getenv("KANDO_LLM_BASE_URL") == "" && saved.LLMBaseURL != "" {
		tools.Translator.BaseURL = saved.LLMBaseURL
	}
	if os.Getenv("KANDO_LLM_MODEL") == "" && saved.LLMModel != "" {
		tools.Translator.Model = saved.LLMModel
	}
	a.prefs.llmServices = maps.Clone(saved.LLMServices)
	// A key goes only to the endpoint it belongs to: KANDO_LLM_API_KEY to whichever endpoint is in use, EVERYAPI_API_KEY to EveryAPI only, the saved key to the host it was saved with. ToolsFromEnv may have put EVERYAPI_API_KEY in for its EveryAPI default, which the saved endpoint can replace.
	if os.Getenv("KANDO_LLM_API_KEY") == "" {
		everyAPIKey := os.Getenv("EVERYAPI_API_KEY")
		switch base := tools.Translator.BaseURL; {
		case base == media.EveryAPIBaseURL && everyAPIKey != "":
			tools.Translator.APIKey = everyAPIKey
		case settings.SameHost(base, saved.LLMBaseURL):
			tools.Translator.APIKey = saved.LLMAPIKey
		default:
			tools.Translator.APIKey = ""
		}
	}
	a.syncLLM(tools.UseEveryAPICLI())
}

// syncLLM shows the tools' AI service in the settings, its key as the key of its endpoint (llmKeyOwner).
func (a *app) syncLLM(tools media.Tools) {
	a.pipeline = tools
	a.prefs.llmBase = tools.Translator.BaseURL
	a.prefs.llmModel = tools.Translator.Model
	a.prefs.llmKey = tools.Translator.APIKey
	a.prefs.llmKeyOwner = llmKeyOwner{key: tools.Translator.APIKey, base: tools.Translator.BaseURL}
	a.prefs.llmProvider = settings.ProviderID(tools.Translator.BaseURL)
}

// llmKeyOwner is the key in the API Key field and the endpoint it belongs to: the one it came from settings.json, the environment or a service used before with, or the one that listed its models or passed the connection test with it. A key typed since belongs to no endpoint yet, so it goes with the Base URL as that is typed, in whichever order the two are filled in.
type llmKeyOwner struct{ key, base string }

// llmKeyUsed records that the key of translator worked on its endpoint, as when the endpoint listed its models or passed the connection test with it, so the key is not sent elsewhere when the Base URL moves to another host.
func (a *app) llmKeyUsed(translator media.ChatTranslator) {
	if translator.APIKey != "" && translator.APIKey == strings.TrimSpace(a.prefs.llmKey) {
		a.prefs.llmKeyOwner = llmKeyOwner{key: translator.APIKey, base: translator.BaseURL}
	}
}

// selectLLM switches to the service id. The service in use is remembered first (llmRemember), and the one chosen comes back as it was left: its model and the key it had, which goes back only to the host it was used with. A service not used before gets its preset's endpoint and model, which adoptLLMModel fills in once the service lists its models when the preset names none, and no key. 自定义 has no endpoint of its own: it takes back the last custom endpoint, or starts with none, so neither the endpoint nor the key of the service before is kept for it.
func (a *app) selectLLM(id string) {
	if a.prefs.llmProvider == id {
		return
	}
	preset, ok := settings.PresetByID(a.messages(), id)
	if !ok {
		return
	}
	a.llmRemember()
	a.prefs.llmProvider = id
	base, model, key := preset.BaseURL, preset.Model, ""
	if before, used := a.prefs.llmServices[id]; used {
		if base == "" {
			base = before.BaseURL
		}
		if before.Model != "" {
			model = before.Model
		}
		if settings.SameHost(before.BaseURL, base) {
			key = before.APIKey
		}
	}
	a.prefs.llmBase, a.prefs.llmModel, a.prefs.llmKey = base, model, key
	a.prefs.llmKeyOwner = llmKeyOwner{key: key, base: base}
	a.applyLLM()
}

// llmRemember keeps the AI service in use, its endpoint, model and key, under its preset id for when it is chosen again (selectLLM); settings.json keeps them too (savedLLM.LLMServices).
func (a *app) llmRemember() {
	tools, ok := a.pipeline.(media.Tools)
	base := strings.TrimSpace(tools.Translator.BaseURL)
	if !ok || base == "" {
		return
	}
	if a.prefs.llmServices == nil {
		a.prefs.llmServices = map[string]savedLLMService{}
	}
	a.prefs.llmServices[settings.ProviderID(base)] = savedLLMService{BaseURL: base, Model: strings.TrimSpace(tools.Translator.Model), APIKey: tools.Translator.APIKey}
}

func (a *app) applyLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	base := strings.TrimSpace(a.prefs.llmBase)
	key := strings.TrimSpace(a.prefs.llmKey)
	// A key that belongs to an endpoint (llmKeyOwner) is dropped rather than sent elsewhere when the Base URL moves to another host. A key typed since belongs to none yet and goes with the Base URL, so a custom endpoint can be filled in key first.
	if owner := a.prefs.llmKeyOwner; key != "" && key == owner.key && !settings.SameHost(base, owner.base) {
		key = ""
		a.prefs.llmKey = ""
	}
	// Without a key EveryAPI uses EVERYAPI_API_KEY, as at startup (media.ToolsFromEnv), and otherwise the signed-in everyapi CLI, as the tools last found it (重新检测 looks again).
	if base == media.EveryAPIBaseURL && key == "" {
		key = os.Getenv("EVERYAPI_API_KEY")
		a.prefs.llmKey = key
		a.prefs.llmKeyOwner = llmKeyOwner{key: key, base: base}
	}
	if base != tools.Translator.BaseURL || key != tools.Translator.APIKey {
		a.prefs.llmEdited = time.Now()
	}
	model := strings.TrimSpace(a.prefs.llmModel)
	// The connection test's verdict is about the service, key and model it tested.
	if base != tools.Translator.BaseURL || key != tools.Translator.APIKey || model != tools.Translator.Model {
		a.llmForgetTest()
	}
	tools.Translator.BaseURL = base
	tools.Translator.Model = model
	tools.Translator.APIKey = key
	a.pipeline = tools.UseEveryAPICLI()
	a.areasLLMChanged()
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsAISaveFailed(err.Error())
	}
}

// openLLMKeyPage opens the page of the service in use where an API Key is made, as 获取 API Key on the AI 服务 page and in the first-run guide does, and says in the library's message bar when the system could not (menuOpenLink).
func (a *app) openLLMKeyPage() {
	if preset, ok := settings.PresetByID(a.messages(), a.prefs.llmProvider); ok && preset.KeyURL != "" {
		a.menuOpenLink(preset.KeyURL)
	}
}

// everyAPISite is EveryAPI's website, where the everyapi command-line program is downloaded. Kando does not download it.
const everyAPISite = "https://everyapi.ai"

// everyAPIOpenURL opens a page in the user's browser. Tests replace it.
var everyAPIOpenURL = openBrowser

// everyAPIStatus is what the AI 服务 page says about the everyapi command-line program: nothing unless the service is EveryAPI without an API Key; then whether the program was found, and, once the sidebar's check asked it, whether someone is signed in to it.
func (a *app) everyAPIStatus() settings.EveryAPI {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.Translator.BaseURL != media.EveryAPIBaseURL || tools.Translator.APIKey != "" {
		return settings.EveryAPI{}
	}
	status := settings.EveryAPI{OpenErr: a.prefs.everyAPIOpenErr}
	switch {
	case tools.Translator.Credential == nil:
		status.Missing = true
	case a.side.ready.asking:
		status.Checking = true
	case a.side.ready.signInDone && a.side.ready.signInErr != nil:
		switch err := a.side.ready.signInErr; {
		case errors.Is(err, media.ErrEveryAPIMissing):
			status.Missing = true
		case errors.Is(err, media.ErrEveryAPISignedOut):
			status.SignedOut = true
		default:
			status.Failed = films.Shorten(err.Error(), 160)
		}
	}
	return status
}

// getEveryAPI opens EveryAPI's website in the browser off the UI thread, and keeps why it could not for the page to say.
func (a *app) getEveryAPI() {
	a.everyAPIForgetOpen()
	gen := a.prefs.everyAPIOpenGen
	update := a.update
	go func() {
		err := everyAPIOpenURL(everyAPISite)
		update(func() {
			if err != nil && gen == a.prefs.everyAPIOpenGen {
				a.prefs.everyAPIOpenErr = err.Error()
			}
		})
	}()
}

// everyAPIForgetOpen drops why 获取 EveryAPI last could not open the website, and the answer of any press still waiting, when it is pressed again or the tools are detected again, so the row says what the program needs now.
func (a *app) everyAPIForgetOpen() {
	a.prefs.everyAPIOpenGen++
	a.prefs.everyAPIOpenErr = ""
}

func (a *app) testLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.prefs.testingLLM {
		return
	}
	a.llmForgetTest()
	a.prefs.testingLLM = true
	gen := a.prefs.llmTestGen
	translator, ping := tools.Translator, a.prefs.pingLLM
	credential := translator.Credential != nil
	// Offline mode turned on meanwhile stops the test (privacyContext).
	update, net := a.update, a.privacyContext()
	go func() {
		ctx, cancel := context.WithTimeout(net, 30*time.Second)
		defer cancel()
		err := ping(ctx, translator)
		update(func() {
			if gen != a.prefs.llmTestGen {
				return
			}
			a.prefs.testingLLM = false
			a.prefs.llmTestOK = err == nil
			a.prefs.llmTestErr = err
			if err != nil {
				log.Printf("settings: the AI service's connection test failed: %v", err)
			} else {
				a.llmKeyUsed(translator)
			}
			a.areasLLMTested(err, credential)
		})
	}()
}

// llmForgetTest drops the connection test's verdict, and the answer of a test still running, when the service, key or model it was about changes or the test starts again.
func (a *app) llmForgetTest() {
	a.prefs.llmTestGen++
	a.prefs.testingLLM = false
	a.prefs.llmTestOK, a.prefs.llmTestErr = false, nil
}

// llmTypingPause is how long the Base URL and the key must rest before the models are fetched for them, so typing an address asks the service once, not at every key.
const llmTypingPause = 700 * time.Millisecond

// llmModelsKey names the service the model list was fetched for: the Base URL, the key, and whether the EveryAPI CLI's login stands in for the key.
func llmModelsKey(t media.ChatTranslator) string {
	login := ""
	if t.Credential != nil {
		login = "login"
	}
	return strings.TrimSpace(t.BaseURL) + "\n" + t.APIKey + "\n" + login
}

// loadLLMModels fetches the AI service's model list while Settings › AI 服务 shows, once per service, Base URL and key, and again on retry. It waits for typing in the Base URL or the key to pause. It reports whether it changed the settings' state, as when it started a fetch.
func (a *app) loadLLMModels() bool {
	if !a.prefs.open || a.SettingsSection() != settings.SectionAI {
		return false
	}
	return a.fetchLLMModels(false)
}

// fetchLLMModels starts fetching the model list, or waits for typing to pause first, and reports whether it changed the settings' state so.
func (a *app) fetchLLMModels(force bool) bool {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.prefs.listModels == nil || strings.TrimSpace(tools.Translator.BaseURL) == "" {
		return false
	}
	key := llmModelsKey(tools.Translator)
	if !force && key == a.prefs.llmModelsFor {
		return false
	}
	if a.prefs.llmModelsLoading && key == a.prefs.llmModelsFor {
		return false
	}
	if wait := llmTypingPause - time.Since(a.prefs.llmEdited); wait > 0 {
		if a.prefs.llmWaiting {
			return false
		}
		a.prefs.llmWaiting = true
		update := a.update
		time.AfterFunc(wait, func() {
			update(func() {
				a.prefs.llmWaiting = false
				a.loadLLMModels()
			})
		})
		return true
	}
	a.prefs.llmModelsFor = key
	a.prefs.llmModelsLoading = true
	a.prefs.llmModels = nil
	a.prefs.llmModelsErr = nil
	// Offline mode turned on meanwhile stops the fetch (privacyContext).
	translator, list, update, net := tools.Translator, a.prefs.listModels, a.update, a.privacyContext()
	go func() {
		ctx, cancel := context.WithTimeout(net, 20*time.Second)
		defer cancel()
		models, err := list(ctx, translator)
		update(func() {
			if a.prefs.llmModelsFor != key {
				return
			}
			a.prefs.llmModelsLoading = false
			if err != nil {
				a.prefs.llmModels = nil
				a.prefs.llmModelsErr = err
				return
			}
			a.prefs.llmModels = models
			a.llmKeyUsed(translator)
			a.adoptLLMModel()
		})
	}()
	return true
}

// adoptLLMModel picks a model when none is chosen yet, or when the one in use is the service's own default and the service no longer lists it: the service's own default when it offers it, else the first chat model it lists (settings.DefaultModel). A model the user chose is kept even when the list lacks it; the drop-down shows it on top. The model picked is tested at once, where the service can answer without a key or has one and EveryAPI's sign-in is not missing, so the page says whether translation works.
func (a *app) adoptLLMModel() {
	if len(a.prefs.llmModels) == 0 {
		return
	}
	preset, _ := settings.PresetByID(a.messages(), a.prefs.llmProvider)
	model := strings.TrimSpace(a.prefs.llmModel)
	if model != "" && (model != preset.Model || slices.Contains(a.prefs.llmModels, model)) {
		return
	}
	a.prefs.llmModel = settings.DefaultModel(preset, a.prefs.llmModels)
	a.applyLLM()
	if tools, ok := a.pipeline.(media.Tools); ok && (!preset.NeedsKey() || tools.Translator.APIKey != "") && !everyAPICLIMissing(tools.Translator) {
		a.testLLM()
	}
}
