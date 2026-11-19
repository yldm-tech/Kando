package settings

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

// PluginExport is the sheet that exports the user's glossary (Glossary) or vocabulary as a pack, while Open: the pack's Name and ID; for a glossary the Scope its terms come from, one of Scopes, the language they are in (From, whisper's code) and the one they translate into (To, as the settings name it); for a word list the Language of its words, one of Languages (whisper's codes); Count, the terms or words the pack would hold; Busy while it is built and saved, and Err why the last try failed, in the interface's language.
type PluginExport struct {
	Open, Glossary bool
	Name, ID       string
	Scope          string
	Scopes         []PluginExportScope
	From, To       string
	Language       string
	Languages      []string
	Count          int
	Busy           bool
	Err            string
}

// PluginExportScope is a scope a glossary export can take its terms from: its id and how the sheet names it, in the interface's language.
type PluginExportScope struct {
	ID, Label string
}

// PluginExportDialog is the export sheet over the main window while Open says it is: the pack's name and id, what goes in it, how many terms or words that is, with 取消 and 导出…, which builds the pack, checks it as an install would and asks where to save it. The fields read the app in bindings, so typing keeps the focus; the sheet is built again only for another kind of pack or language of the interface.
type PluginExportDialog struct {
	App  PluginsApp
	Open func() bool
}

// Render implements core.Node.
func (d PluginExportDialog) Render(cx *core.Scope) core.Node {
	a := d.App
	kind := core.NewMemo(cx, live.Func(a, func() int {
		e := a.PluginExport()
		switch {
		case !e.Open:
			return 0
		case e.Glossary:
			return 1
		}
		return 2
	}))
	cancel := live.Do(a, a.CancelPluginExport)
	return core.Show{When: func() bool { return d.Open() && kind.Get() != 0 }, Then: func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			glossary := kind.Get() == 1
			_ = i18n.Language(cx)
			return core.Untracked(cx, func() core.Node { return pluginExportDialog(cx, a, glossary, cancel) })
		})
	}}
}

// pluginExportWidth is the export sheet's width; its rows fill it, as the feedback dialog's fill its own.
const pluginExportWidth core.Points = 460

// pluginExportDialog is the sheet of a glossary pack, or of a word list.
func pluginExportDialog(cx *core.Scope, a PluginsApp, glossary bool, cancel func()) core.Node {
	colors := ui.ColorsOf(cx)
	e := live.Func(a, a.PluginExport)
	title, ok := locales.PluginsExportWordsTitle(cx), locales.PluginsExportSave(cx)
	if glossary {
		title = locales.PluginsExportGlossaryTitle(cx)
	}
	name := ui.Input{TestID: "plugins-export-name", Label: locales.PluginsExportName(cx), Value: func() string { return e().Name }, OnChange: live.On(a, a.SetPluginExportName),
		Style: core.Style{MinWidth: core.Pt(0)}}
	id := ui.Input{TestID: "plugins-export-id", Label: locales.PluginsExportID(cx), Value: func() string { return e().ID }, OnChange: live.On(a, a.SetPluginExportID),
		Style: core.Style{FontFamily: ui.Mono, MinWidth: core.Pt(0)}}
	// Each field fills the sheet's width, as the feedback dialog's do: its label over its control, stretched, never wider than the sheet.
	field := func(label string, control core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 4, AlignItems: core.AlignStretch, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Text{Content: label, Style: core.Style{FontSize: 12, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)},
			control,
		}}
	}
	// fill makes a drop-down as wide as the field it is in, rather than ui.Select's 200 points; half is each of the two language fields side by side, sharing the row equally.
	fill := core.Style{Width: core.Pct(100), MinWidth: core.Pt(0)}
	half := core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignStretch}
	children := []core.Node{field(locales.PluginsExportName(cx), name), field(locales.PluginsExportID(cx), id)}
	if glossary {
		var scopes []ui.Option[string]
		for _, s := range core.Untracked(cx, e).Scopes {
			scopes = append(scopes, ui.Option[string]{Value: s.ID, Label: s.Label})
		}
		from := make([]ui.Option[string], 0, len(media.SpeechLanguages()))
		for _, code := range media.SpeechLanguages() {
			from = append(from, ui.Option[string]{Value: code, Label: subtitle.SpokenLanguageName(cx, code)})
		}
		var to []ui.Option[string]
		for _, language := range subtitle.TargetLanguages() {
			to = append(to, ui.Option[string]{Value: language, Label: subtitle.TargetLanguageName(cx, language)})
		}
		children = append(children,
			field(locales.PluginsExportScope(cx), ui.Select[string]{Options: scopes, Label: locales.PluginsExportScope(cx), TestID: "plugins-export-scope",
				Value: func() string { return e().Scope }, OnChange: live.On(a, a.SetPluginExportScope), Style: fill}),
			core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignStart}, Children: []core.Node{
				core.View{Style: half, Children: []core.Node{field(locales.PluginsExportFrom(cx), ui.Select[string]{Options: from, Label: locales.PluginsExportFrom(cx), TestID: "plugins-export-from", Searchable: true,
					Value: func() string { return e().From }, OnChange: live.On(a, a.SetPluginExportFrom), Style: fill})}},
				core.View{Style: half, Children: []core.Node{field(locales.PluginsExportTo(cx), ui.Select[string]{Options: to, Label: locales.PluginsExportTo(cx), TestID: "plugins-export-to", Searchable: true,
					Value: func() string { return e().To }, OnChange: live.On(a, a.SetPluginExportTo), Style: fill})}},
			}})
	} else {
		var languages []ui.Option[string]
		for _, code := range core.Untracked(cx, e).Languages {
			languages = append(languages, ui.Option[string]{Value: code, Label: subtitle.SpokenLanguageName(cx, code)})
		}
		children = append(children, field(locales.PluginsExportLanguage(cx), ui.Select[string]{Options: languages, Label: locales.PluginsExportLanguage(cx), TestID: "plugins-export-language",
			Value: func() string { return e().Language }, OnChange: live.On(a, a.SetPluginExportLanguage), Style: fill}))
	}
	children = append(children,
		core.Text{TestID: "plugins-export-count", ContentFunc: func() string {
			if glossary {
				return locales.PluginsExportTermCount(cx, e().Count)
			}
			return locales.PluginsExportWordCount(cx, e().Count)
		}, Style: core.Style{FontSize: 12, LineHeight: 17, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)},
		core.Text{Content: locales.PluginsExportNote(cx), Style: core.Style{FontSize: 12, LineHeight: 17, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Muted)},
		core.Show{When: func() bool { return e().Err != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "plugins-export-error", ContentFunc: func() string { return e().Err }, Selectable: core.SelectableOn, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Danger)}
		}},
	)
	return ui.Dialog{TestID: "plugins-export", NoCloseOnBackdropPress: true, Open: func() bool { return true }, Title: title,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				cancel()
			}
		},
		Style:    core.Style{Width: core.Pt(pluginExportWidth)},
		Children: []core.Node{core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignStretch, MinWidth: core.Pt(0)}, Children: children}},
		Footer: []core.Node{
			ui.Button{Text: locales.SettingsPluginsCancel(cx), Variant: ui.ButtonSecondary, TestID: "plugins-export-cancel", Disabled: func() bool { return e().Busy }, OnPress: cancel},
			ui.Button{Text: ok, TestID: "plugins-export-save", Disabled: func() bool { x := e(); return x.Busy || x.Count == 0 }, OnPress: live.Do(a, a.ConfirmPluginExport)},
		}}
}
