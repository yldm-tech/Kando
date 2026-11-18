package synctest

import (
	"context"
	"slices"

	"kando/packages/sync"
)

// targets are the languages the stand-in translates into, by their tags.
var targets = []string{"zh-Hans", "zh-Hant", "en", "ja", "ko"}

// Target is the language the desktop translates into, a BCP 47 tag; it starts as zh-Hans.
func (d *Desktop) Target() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.target
}

// SetTarget changes the language the desktop translates into, as its settings would.
func (d *Desktop) SetTarget(tag string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.target = tag
}

// Context reports whether the desktop's translation sends the lines before a batch with it; it starts on.
func (d *Desktop) Context() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.context
}

// SetService sets the AI service the desktop translates with, by its name and model, as its settings would; "" for none.
func (d *Desktop) SetService(name, model string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.service, d.model = name, model
}

// settings are the desktop's settings as v2's calls answer them.
func (d *Desktop) settings() sync.Settings {
	d.mu.Lock()
	defer d.mu.Unlock()
	return sync.Settings{Target: d.target, Service: d.service, Model: d.model, Context: new(d.context)}
}

// settingsService is the stand-in's settings as v2's calls read and change them.
type settingsService struct{ d *Desktop }

func (s settingsService) Settings(context.Context, sync.Caller) (sync.Settings, error) {
	return s.d.settings(), nil
}

func (s settingsService) PutSettings(_ context.Context, _ sync.Caller, req sync.SettingsPut) (sync.Settings, error) {
	if req.Target != "" {
		if !slices.Contains(targets, req.Target) {
			return sync.Settings{}, sync.Errorf(sync.CodeInvalid, "no language %q to translate into", req.Target)
		}
		s.d.SetTarget(req.Target)
	}
	if req.Context != nil {
		s.d.mu.Lock()
		s.d.context = *req.Context
		s.d.mu.Unlock()
	}
	return s.d.settings(), nil
}
