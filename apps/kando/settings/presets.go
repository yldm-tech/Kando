package settings

import (
	"net/url"
	"slices"
	"strings"

	"kando/apps/kando/locales"
	"kando/packages/media"
)

// Preset is one translation service the AI page can select: its id, its name and what it is in the language of the messages Presets is given, its group, one of the Group values, and the endpoint and default model it fills in. Custom has neither. Every service in the cloud names a chat model of its own, as the first id a service lists is often an embedding, image or guard model; the services on this computer, whose models are the user's, leave the choice to DefaultModel once they list their models, which also replaces a preset's model the service no longer lists. KeyURL is the service's page where an API Key is made, which 获取 API Key opens; EveryAPI has none, as it signs in with the everyapi command-line program, nor have the services on this computer and custom.
type Preset struct {
	ID, Name, Blurb, Group string
	BaseURL, Model         string
	KeyURL                 string
}

// The groups of the presets: services in the cloud, services running on this Mac, which the drop-down names after their group, and any other endpoint.
const (
	GroupCloud = "cloud"
	GroupLocal = "local"
	GroupOther = "other"
)

// Presets are the services the AI 服务 page offers, their names and blurbs in the language of m.
func Presets(m locales.Messages) []Preset {
	return []Preset{
		{ID: "everyapi", Name: "EveryAPI", Blurb: m.SettingsAIBlurbEveryAPI(), BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Group: GroupCloud},
		{ID: "openai", Name: "OpenAI", Blurb: m.SettingsAIBlurbOpenAI(), BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini", KeyURL: "https://platform.openai.com/api-keys", Group: GroupCloud},
		{ID: "openrouter", Name: "OpenRouter", Blurb: m.SettingsAIBlurbOpenRouter(), BaseURL: "https://openrouter.ai/api/v1", Model: "openai/gpt-4.1-mini", KeyURL: "https://openrouter.ai/settings/keys", Group: GroupCloud},
		{ID: "groq", Name: "Groq", Blurb: m.SettingsAIBlurbGroq(), BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", KeyURL: "https://console.groq.com/keys", Group: GroupCloud},
		{ID: "deepseek", Name: "DeepSeek", Blurb: m.SettingsAIBlurbDeepSeek(), BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", KeyURL: "https://platform.deepseek.com/api_keys", Group: GroupCloud},
		{ID: "xai", Name: "xAI", Blurb: m.SettingsAIBlurbXAI(), BaseURL: "https://api.x.ai/v1", Model: "grok-4.20-0309-non-reasoning", KeyURL: "https://console.x.ai", Group: GroupCloud},
		{ID: "mistral", Name: "Mistral", Blurb: m.SettingsAIBlurbMistral(), BaseURL: "https://api.mistral.ai/v1", Model: "mistral-small-latest", KeyURL: "https://console.mistral.ai/api-keys", Group: GroupCloud},
		{ID: "dashscope", Name: m.SettingsAINameDashScope(), Blurb: m.SettingsAIBlurbDashScope(), BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus", KeyURL: "https://bailian.console.aliyun.com/?tab=model#/api-key", Group: GroupCloud},
		{ID: "moonshot", Name: "Kimi", Blurb: m.SettingsAIBlurbMoonshot(), BaseURL: "https://api.moonshot.cn/v1", Model: "kimi-k3", KeyURL: "https://platform.moonshot.cn/console/api-keys", Group: GroupCloud},
		{ID: "zhipu", Name: m.SettingsAINameZhipu(), Blurb: m.SettingsAIBlurbZhipu(), BaseURL: "https://open.bigmodel.cn/api/paas/v4", Model: "glm-4.7-flash", KeyURL: "https://bigmodel.cn/usercenter/proj-mgmt/apikeys", Group: GroupCloud},
		{ID: "siliconflow", Name: m.SettingsAINameSiliconFlow(), Blurb: m.SettingsAIBlurbSiliconFlow(), BaseURL: "https://api.siliconflow.cn/v1", Model: "deepseek-ai/DeepSeek-V3.2", KeyURL: "https://cloud.siliconflow.cn/account/ak", Group: GroupCloud},
		{ID: "ollama", Name: "Ollama", Blurb: m.SettingsAIBlurbOllama(), BaseURL: "http://127.0.0.1:11434/v1", Group: GroupLocal},
		{ID: "lmstudio", Name: "LM Studio", Blurb: m.SettingsAIBlurbLMStudio(), BaseURL: "http://127.0.0.1:1234/v1", Group: GroupLocal},
		{ID: "custom", Name: m.SettingsAINameCustom(), Blurb: m.SettingsAIBlurbCustom(), Group: GroupOther},
	}
}

// NeedsKey reports whether the service cannot be reached without an API Key: the services in the cloud but EveryAPI, which signs in with the everyapi command-line program instead. The services on this computer need none, and a custom endpoint may not.
func (p Preset) NeedsKey() bool {
	return p.Group == GroupCloud && p.ID != "everyapi"
}

// nonChatSigns are what the ids of models that do not chat carry: embeddings (BAAI's bge and MiniLM too), speech and transcription (FunAudioLLM's SenseVoice and CosyVoice too), images (FLUX, Kolors and Stable Diffusion too), moderation and guard models, reranking and OpenAI's completion-only models. DefaultModel skips them.
var nonChatSigns = []string{"embed", "bge", "minilm", "tts", "whisper", "transcribe", "sensevoice", "cosyvoice", "dall-e", "image", "flux", "kolors", "stable-diffusion", "moderation", "guard", "rerank", "babbage", "davinci", "realtime", "audio"}

// DefaultModel is the model to use on a service that lists models when none is chosen yet: the preset's own when the service lists it, else the first that looks like a chat model (nonChatSigns), else the first listed; "" when models is empty.
func DefaultModel(preset Preset, models []string) string {
	if len(models) == 0 {
		return ""
	}
	if preset.Model != "" && slices.Contains(models, preset.Model) {
		return preset.Model
	}
	for _, model := range models {
		id := strings.ToLower(model)
		if !slices.ContainsFunc(nonChatSigns, func(sign string) bool { return strings.Contains(id, sign) }) {
			return model
		}
	}
	return models[0]
}

// PresetByID is the preset id among presets, the list the app offers (the app's llmPresets), with its name and blurb in whatever language that list was made in.
func PresetByID(presets []Preset, id string) (Preset, bool) {
	for _, preset := range presets {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

// ProviderID is the preset among presets whose endpoint base is, "custom" for an endpoint none of them has. Only the presets' endpoints are read, so the language of their names does not matter.
func ProviderID(presets []Preset, base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, preset := range presets {
		if preset.ID == "custom" || preset.BaseURL == "" {
			continue
		}
		if strings.TrimRight(preset.BaseURL, "/") == base {
			return preset.ID
		}
	}
	return "custom"
}

// SameHost reports whether two endpoints are on the same scheme and host, so that a key for one may go to the other.
func SameHost(a, b string) bool {
	return host(a) == host(b)
}

func host(base string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}
