package main

import (
	"context"
	"log"

	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	psync "kando/packages/sync"
)

// phoneSettings is the settings a paired phone reads and changes over protocol v2 (psync.SettingsService): the language subtitles are translated into, which the phone's Settings › Translate into shows and sets, so the phone and this computer name one language. The AI service's key is not among them: it never leaves the computer.
type phoneSettings struct{ a *app }

func (s phoneSettings) Settings(ctx context.Context, _ psync.Caller) (psync.Settings, error) {
	a := s.a
	return remote.OnUI(ctx, a.update, func() (psync.Settings, error) { return a.phoneSettings(), nil })
}

// PutSettings changes the language translated into as Settings › Translate does, and keeps it in settings.json.
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
		if value != "" && value != a.prefs.language {
			a.prefs.language = value
			if err := a.saveSettings(); err != nil {
				log.Printf("phonesync: keep the language %s chose: %v", caller.Name, err)
				return psync.Settings{}, err
			}
		}
		return a.phoneSettings(), nil
	})
}

// phoneSettings are the settings as a phone sees them. UI thread only.
func (a *app) phoneSettings() psync.Settings {
	return psync.Settings{Target: phonesync.TargetTag(a.prefs.language), TargetName: a.prefs.language}
}
