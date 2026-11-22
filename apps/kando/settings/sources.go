package settings

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

// SourcesApp is what the 翻译来源 row of 翻译 reads and changes: the machine translation sources of the script plugins turned on, which one translates the films, and choosing one. Like StylesApp it is an App of its own, which the window is given (Window.Sources), so App does not grow.
type SourcesApp interface {
	live.State

	// TranslateSources are the translation sources of the script plugins turned on, in the order the plugins are listed, each named in lang. TranslateSource is the value of the one chosen, "" for the AI service, and "" too while the one chosen is gone; SetTranslateSource chooses the source of value, "" for the AI service, and saves it. TranslateSourceGone is the name of the source chosen whose plugin was turned off or removed, so the films are translated with the AI service, "" while there is none.
	TranslateSources(lang string) []TranslateSource
	TranslateSource() string
	SetTranslateSource(value string)
	TranslateSourceGone() string
}

// TranslateSource is a translation source a script plugin adds, as 翻译来源 offers it: the value it is chosen by and its name.
type TranslateSource struct {
	Value, Name string
}

// Sources is the 翻译来源 row at the top of 翻译's first card, with its separator under it, while a script plugin turned on adds a translation source or the one chosen is gone, and nothing otherwise: a drop-down of the AI service and the plugins' sources. While a plugin's source is chosen the row says that proofreading and term suggestions still use the AI service; while the one chosen is gone it says the films are translated with the AI service.
type Sources struct{ App SourcesApp }

// Render implements core.Node.
func (s Sources) Render(cx *core.Scope) core.Node {
	a := s.App
	// sources is the sources' values and names, joined, so the drop-down's options are read again only when they change; the names are in the language shown.
	sources := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, src := range a.TranslateSources(i18n.Language(cx)) {
			out = append(out, src.Value+"\x00"+src.Name)
		}
		return strings.Join(out, "\x01")
	}))
	shown := live.Func(a, func() bool { return sources.Get() != "" || a.TranslateSourceGone() != "" })
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		// The options and the name are bound, so a plugin's source added or gone, or another language, updates the drop-down in place and keeps its focus.
		options := func() []ui.Option[string] {
			options := []ui.Option[string]{{Value: "", Label: locales.SettingsTranslateSourceAI(cx)}}
			if all := sources.Get(); all != "" {
				for _, item := range strings.Split(all, "\x01") {
					value, name, _ := strings.Cut(item, "\x00")
					options = append(options, ui.Option[string]{Value: value, Label: name})
				}
			}
			return options
		}
		choose := ui.Select[string]{OptionsFunc: options, LabelFunc: func() string { return locales.SettingsTranslateSource(cx) }, TestID: "settings-translate-source",
			Value: live.Func(a, a.TranslateSource), OnChange: live.On(a, a.SetTranslateSource), Style: core.Style{Width: core.Pt(180)}}
		subtitle := live.Func(a, func() string {
			if gone := a.TranslateSourceGone(); gone != "" {
				return locales.SettingsTranslateSourceGone(cx, gone)
			}
			if a.TranslateSource() != "" {
				return locales.SettingsTranslateSourceHint(cx)
			}
			return ""
		})
		return core.Group{
			form.Row{Title: locales.SettingsTranslateSource(cx), SubtitleFunc: subtitle, NoGroup: true, Trailing: []core.Node{choose}, TestID: "settings-translate-source-row"},
			form.Separator(true),
		}
	}}
}
