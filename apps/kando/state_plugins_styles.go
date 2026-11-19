package main

import (
	"slices"
	"strings"

	"kando/apps/kando/plugins"
	"kando/packages/media"
)

// Translate-style packs (docs/plugin-system.md E5): a pack's style instruction is offered as a preset of the translation prompt in Settings › 翻译 (settings.Styles). Choosing one puts its prompt into the prompt field, which is the user's to edit from there, and the translation appends its reply rule after it as after any prompt (media's subtitlePrompt), so a style cannot change how a translation is read. Nothing is kept of the choice but the prompt itself.

// TranslateStyles are the translate-style packs turned on, in the order the settings keep (settings.StylesApp).
func (a *app) TranslateStyles() []plugins.Pack { return a.pluginsOn(plugins.KindStyle) }

// TranslateStyle is the id of the translate-style pack turned on whose prompt the translation prompt is now, spaces around either aside; "" when it is none of theirs, as the default or the user's own.
func (a *app) TranslateStyle() string {
	prompt := strings.TrimSpace(a.prefs.translatePrompt)
	i := slices.IndexFunc(a.TranslateStyles(), func(p plugins.Pack) bool { return strings.TrimSpace(p.Style.Prompt) == prompt })
	if i < 0 {
		return ""
	}
	return a.TranslateStyles()[i].ID
}

// SetTranslateStyle puts the prompt of the translate-style pack id in place of the translation prompt, or the default prompt for "", and saves the settings; an id of no pack turned on changes nothing.
func (a *app) SetTranslateStyle(id string) {
	prompt := media.DefaultSubtitlePrompt
	if id != "" {
		i := slices.IndexFunc(a.TranslateStyles(), func(p plugins.Pack) bool { return p.ID == id })
		if i < 0 {
			return
		}
		prompt = a.TranslateStyles()[i].Style.Prompt
	}
	if strings.TrimSpace(a.prefs.translatePrompt) == strings.TrimSpace(prompt) {
		return
	}
	a.prefs.translatePrompt = prompt
	a.Save()
}
