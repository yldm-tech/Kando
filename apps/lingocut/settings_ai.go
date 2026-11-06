package main

import (
	"context"
	"strings"
	"time"

	"lingocut/packages/media"
)

// Settings › AI 服务: the translation model's provider, endpoint and key.

type llmPreset struct {
	ID, Name, Blurb, BaseURL, Model, Group string
}

func llmPresets() []llmPreset {
	return []llmPreset{
		{ID: "everyapi", Name: "EveryAPI", Blurb: "本机已登录的 EveryAPI", BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Group: "云服务"},
		{ID: "openai", Name: "OpenAI", Blurb: "OpenAI 官方接口", BaseURL: "https://api.openai.com/v1", Group: "云服务"},
		{ID: "openrouter", Name: "OpenRouter", Blurb: "一个接口调用多家模型", BaseURL: "https://openrouter.ai/api/v1", Group: "云服务"},
		{ID: "groq", Name: "Groq", Blurb: "Groq 的 OpenAI 兼容接口", BaseURL: "https://api.groq.com/openai/v1", Group: "云服务"},
		{ID: "deepseek", Name: "DeepSeek", Blurb: "DeepSeek 的 OpenAI 兼容接口", BaseURL: "https://api.deepseek.com/v1", Group: "云服务"},
		{ID: "xai", Name: "xAI", Blurb: "Grok 的 OpenAI 兼容接口", BaseURL: "https://api.x.ai/v1", Group: "云服务"},
		{ID: "mistral", Name: "Mistral", Blurb: "Mistral 的 OpenAI 兼容接口", BaseURL: "https://api.mistral.ai/v1", Group: "云服务"},
		{ID: "dashscope", Name: "通义千问", Blurb: "阿里云 DashScope 兼容接口", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Group: "云服务"},
		{ID: "moonshot", Name: "Kimi", Blurb: "Moonshot 的 OpenAI 兼容接口", BaseURL: "https://api.moonshot.cn/v1", Group: "云服务"},
		{ID: "zhipu", Name: "智谱 GLM", Blurb: "智谱的 OpenAI 兼容接口", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Group: "云服务"},
		{ID: "siliconflow", Name: "硅基流动", Blurb: "硅基流动的 OpenAI 兼容接口", BaseURL: "https://api.siliconflow.cn/v1", Group: "云服务"},
		{ID: "ollama", Name: "Ollama", Blurb: "本机 Ollama", BaseURL: "http://127.0.0.1:11434/v1", Group: "本机"},
		{ID: "lmstudio", Name: "LM Studio", Blurb: "本机 LM Studio", BaseURL: "http://127.0.0.1:1234/v1", Group: "本机"},
		{ID: "custom", Name: "OpenAI 兼容", Blurb: "任何兼容 OpenAI Chat Completions 的接口", Group: "其他"},
	}
}

func llmPresetByID(id string) (llmPreset, bool) {
	for _, preset := range llmPresets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return llmPreset{}, false
}

func llmProviderID(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, preset := range llmPresets() {
		if preset.ID == "custom" || preset.BaseURL == "" {
			continue
		}
		if strings.TrimRight(preset.BaseURL, "/") == base {
			return preset.ID
		}
	}
	return "custom"
}

func (a *app) syncLLM(tools media.Tools) {
	a.pipeline = tools
	a.prefs.llmBase = tools.Translator.BaseURL
	a.prefs.llmModel = tools.Translator.Model
	a.prefs.llmKey = tools.Translator.APIKey
	a.prefs.llmCredential = tools.Translator.Credential
	a.prefs.llmProvider = llmProviderID(tools.Translator.BaseURL)
}

func (a *app) selectLLM(id string) {
	if a.prefs.llmProvider == id {
		return
	}
	preset, ok := llmPresetByID(id)
	if !ok {
		return
	}
	a.prefs.llmProvider = id
	if preset.BaseURL != "" {
		a.prefs.llmBase = preset.BaseURL
	}
	if preset.Model != "" {
		a.prefs.llmModel = preset.Model
	}
	a.applyLLM()
}

func (a *app) applyLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.Translator.BaseURL = strings.TrimSpace(a.prefs.llmBase)
	tools.Translator.Model = strings.TrimSpace(a.prefs.llmModel)
	tools.Translator.APIKey = strings.TrimSpace(a.prefs.llmKey)
	tools.Translator.Credential = nil
	if tools.Translator.BaseURL == media.EveryAPIBaseURL && tools.Translator.APIKey == "" {
		tools.Translator.Credential = a.prefs.llmCredential
	}
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = "翻译服务无法保存：" + err.Error()
	}
}

func (a *app) testLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.prefs.testingLLM {
		return
	}
	a.prefs.testingLLM = true
	a.prefs.llmTestResult = ""
	translator := tools.Translator
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := translator.Ping(ctx)
		update(func() {
			a.prefs.testingLLM = false
			if err != nil {
				a.prefs.llmTestResult = err.Error()
				return
			}
			a.prefs.llmTestResult = "连接成功"
		})
	}()
}
