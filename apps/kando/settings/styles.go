package settings

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

// StylesApp is what the 风格预设 row of 翻译 reads and changes: the translate-style packs turned on, which of them the prompt is now, and putting one's prompt in place. Like PluginsApp it is an App of its own, which the window is given (Window.Styles), so App does not grow.
type StylesApp interface {
	live.State

	// TranslateStyles are the translate-style packs turned on; TranslateStyle is the id of the one whose prompt the translation prompt is now, "" for none; SetTranslateStyle puts the prompt of the pack id in place of the translation prompt, or the default prompt for "", and saves it.
	TranslateStyles() []plugins.Pack
	TranslateStyle() string
	SetTranslateStyle(id string)
}

// Styles is the 风格预设 row over the prompt of 翻译, with its separator under it, while a translate-style pack is turned on, and nothing otherwise: a drop-down of the packs' styles, which puts the one chosen into the prompt field, where it can be edited as any prompt; 不用预设 puts the default prompt back. The translation still appends the reply rule after whatever the prompt is.
type Styles struct{ App StylesApp }

// Render implements core.Node.
func (s Styles) Render(cx *core.Scope) core.Node {
	a := s.App
	// packs is the packs' ids and names, joined, so the drop-down is built again only when they change; the names are in the language shown.
	packs := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, p := range a.TranslateStyles() {
			name := p.Name.In(i18n.Language(cx))
			if name == "" {
				name = p.ID
			}
			out = append(out, p.ID+"\x00"+name)
		}
		return strings.Join(out, "\x01")
	}))
	return core.Show{When: func() bool { return packs.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		// TODO(nsgo#848): give ui.Select its options as a binding, so a list that changes does not build the drop-down again and take its focus; it is built again only when the style packs turned on change, which is done on another page https://github.com/nsgo-dev/nsgo/issues/848
		choose := core.Dynamic(func(cx *core.Scope) core.Node {
			options := []ui.Option[string]{{Value: "", Label: locales.SettingsTranslateStyleNone(cx)}}
			for _, item := range strings.Split(packs.Get(), "\x01") {
				id, name, _ := strings.Cut(item, "\x00")
				options = append(options, ui.Option[string]{Value: id, Label: name})
			}
			return ui.Select[string]{Options: options, Label: locales.SettingsTranslateStyle(cx), TestID: "settings-translate-style",
				Value: live.Func(a, a.TranslateStyle), OnChange: live.On(a, a.SetTranslateStyle), Style: core.Style{Width: core.Pt(180)}}
		})
		return core.Group{
			form.Row{Title: locales.SettingsTranslateStyle(cx), Subtitle: locales.SettingsTranslateStyleHint(cx), Trailing: []core.Node{choose}},
			form.Separator(true),
		}
	}}
}
