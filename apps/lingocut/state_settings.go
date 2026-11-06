package main

import (
	"context"

	"lingocut/packages/media"
)

// settingsUI is the state of the settings pages: the translation's language and prompt, the speech models and their download, the AI service and the tools' versions. The model and the AI service in use are the pipeline's (media.Tools); these fields are what the pages edit and show.
type settingsUI struct {
	language         string
	translatePrompt  string
	translateContext bool
	modelsDir        string
	extraModelDirs   []string
	downloading      string
	downloadWritten  int64
	downloadTotal    int64
	downloadErr      string
	downloadCancel   context.CancelFunc
	llmProvider      string
	llmBase          string
	llmModel         string
	llmKey           string
	showLLMKey       bool
	llmCredential    func(context.Context, bool) (media.Endpoint, error)
	testingLLM       bool
	llmTestResult    string
	toolVersions     map[string]string
	versionGen       int
}

func newSettingsUI() settingsUI {
	return settingsUI{language: "简体中文", translatePrompt: media.DefaultSubtitlePrompt, translateContext: true}
}
