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

// MarksApp is what the 逐词标注 card of 字幕外观 reads and changes: whether readings are drawn over the words of the original line, the word list whose levels draw a line in their color under them, and whether the words in the vocabulary have a dashed one. The plugins compute the marks (the app's state_plugins_marks.go); like PluginsApp it is an App of its own, which the window is given (Window.Marks), so App does not grow.
type MarksApp interface {
	live.State

	// MarkReadings is whether readings are drawn; SetMarkReadings turns them on or off and saves that.
	MarkReadings() bool
	SetMarkReadings(on bool)
	// MarkWordlists are the word list packs turned on, which the levels can be colored by; MarkWordlist is the id of the one chosen, "" for none, as while the one chosen is turned off; SetMarkWordlist chooses one, or none with "", and saves that.
	MarkWordlists() []plugins.Pack
	MarkWordlist() string
	SetMarkWordlist(id string)
	// MarkKnown is whether the words in the vocabulary are underlined; SetMarkKnown turns that on or off and saves it.
	MarkKnown() bool
	SetMarkKnown(on bool)
}

// Marks is the 逐词标注 card at the end of 字幕外观: 注音或拼音, 按等级画线 with the word lists turned on, and 标出已学词. Every change shows on the picture once the app has marked the open film's lines again, and is saved.
type Marks struct{ App MarksApp }

// Render implements core.Node.
func (m Marks) Render(cx *core.Scope) core.Node {
	a := m.App
	readings := form.SwitchRow{Title: locales.SettingsLookReadings(cx), Subtitle: locales.SettingsLookReadingsHint(cx), TestID: "settings-look-readings",
		On: live.Func(a, a.MarkReadings), OnChange: live.On(a, a.SetMarkReadings)}
	known := form.SwitchRow{Title: locales.SettingsLookKnown(cx), Subtitle: locales.SettingsLookKnownHint(cx), TestID: "settings-look-known",
		On: live.Func(a, a.MarkKnown), OnChange: live.On(a, a.SetMarkKnown)}
	// lists is the word lists' ids and names, joined, so the drop-down is built again only when they change; the names are in the language shown.
	lists := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, p := range a.MarkWordlists() {
			name := p.Name.In(i18n.Language(cx))
			if name == "" {
				name = p.ID
			}
			out = append(out, p.ID+"\x00"+name)
		}
		return strings.Join(out, "\x01")
	}))
	choose := core.Dynamic(func(cx *core.Scope) core.Node {
		options := []ui.Option[string]{{Value: "", Label: locales.SettingsLookTiersOff(cx)}}
		if joined := lists.Get(); joined != "" {
			for _, item := range strings.Split(joined, "\x01") {
				id, name, _ := strings.Cut(item, "\x00")
				options = append(options, ui.Option[string]{Value: id, Label: name})
			}
		}
		return ui.Select[string]{Options: options, Label: locales.SettingsLookTiers(cx), TestID: "settings-look-tiers",
			Value: live.Func(a, a.MarkWordlist), OnChange: live.On(a, a.SetMarkWordlist), Style: core.Style{Width: core.Pt(180)}}
	})
	return core.Group([]core.Node{
		form.Label(locales.SettingsLookMarks(cx), 12),
		form.Card(
			readings,
			form.Separator(true),
			form.Row{Title: locales.SettingsLookTiers(cx), Subtitle: locales.SettingsLookTiersHint(cx), Trailing: []core.Node{choose}},
			form.Separator(true),
			known,
		),
	})
}
