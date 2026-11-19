package main

import (
	"context"
	"log"
	"strings"

	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// phoneSettings is the settings a paired phone reads and changes over protocol v2 (psync.SettingsService): the language subtitles are translated into, which the phone's Settings › Translate into shows and sets, so the phone and this computer name one language; whether translating sends the lines before a batch (Settings › Translate's context switch), which the phone's page of this computer sets; and the AI service's name and model, which it only shows. The AI service's address and key are not among them: they never leave the computer.
type phoneSettings struct{ a *app }

func (s phoneSettings) Settings(ctx context.Context, _ psync.Caller) (psync.Settings, error) {
	a := s.a
	return remote.OnUI(ctx, a.update, func() (psync.Settings, error) { return a.phoneSettings(), nil })
}

// PutSettings changes the language translated into and the context switch as Settings › Translate does, and keeps them in settings.json.
func (s phoneSettings) PutSettings(ctx context.Context, caller psync.Caller, req psync.SettingsPut) (psync.Settings, error) {
	a := s.a
	value, ok := "", true
	if req.Target != "" {
		value, ok = phonesync.TargetValue(req.Target)
	}
	if !ok {
		return psync.Settings{}, psync.Errorf(psync.CodeInvalid, "this desktop does not translate into %q", req.Target)
	}
	return remote.OnUI(ctx, a.update, func() (psync.Settings, error) {
		language, context := a.prefs.language, a.prefs.translateContext
		if value != "" {
			a.prefs.language = value
		}
		if req.Context != nil {
			a.prefs.translateContext = *req.Context
		}
		if a.prefs.language != language || a.prefs.translateContext != context {
			if err := a.saveSettings(); err != nil {
				// Not kept, the change is not made: the phone, which is answered the error, puts it again, and this computer keeps translating as settings.json says.
				a.prefs.language, a.prefs.translateContext = language, context
				log.Printf("phonesync: keep the settings %s chose: %v", caller.Name, err)
				return psync.Settings{}, err
			}
		}
		return a.phoneSettings(), nil
	})
}

// phoneSettings are the settings as a phone sees them: the AI service named as the AI 服务 page names it in this computer's language, without its address. UI thread only.
func (a *app) phoneSettings() psync.Settings {
	out := psync.Settings{Target: phonesync.TargetTag(a.prefs.language), TargetName: a.prefs.language, Context: new(a.prefs.translateContext)}
	if tools, ok := a.pipeline.(media.Tools); ok && strings.TrimSpace(tools.Translator.BaseURL) != "" {
		presets := a.llmPresets(a.messages())
		preset, _ := settings.PresetByID(presets, settings.ProviderID(presets, tools.Translator.BaseURL))
		out.Service, out.Model = preset.Name, strings.TrimSpace(tools.Translator.Model)
	}
	return out
}
