package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The appearances 通用 › 外观 offers besides following the system's (""), as App.Appearance and settings.json keep them.
const (
	AppearanceLight = "light"
	AppearanceDark  = "dark"
)

// appearanceRow is 通用's 外观 row: follow the system, which is the default, or always light or dark. The choice shows at once in every window and is saved.
func appearanceRow(cx *core.Scope, a App) core.Node {
	options := []ui.Option[string]{
		{Value: "", Label: locales.SettingsGeneralAppearanceSystem(cx)},
		{Value: AppearanceLight, Label: locales.SettingsGeneralAppearanceLight(cx)},
		{Value: AppearanceDark, Label: locales.SettingsGeneralAppearanceDark(cx)},
	}
	choose := ui.Select[string]{Options: options, Label: locales.SettingsGeneralAppearance(cx), TestID: "settings-appearance",
		Value: live.Func(a, a.Appearance), OnChange: live.On(a, a.SetAppearance), Style: core.Style{Width: core.Pt(160)}}
	return form.Row{Title: locales.SettingsGeneralAppearance(cx), NoGroup: true, Trailing: []core.Node{choose}}
}
