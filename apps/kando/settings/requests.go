package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// requests is the AI 服务 page's 请求 section: how many lines a translation request carries and how many times one that failed with a rate limit, a server error or no usable answer is sent again, with 恢复默认 bringing both back to media's defaults.
func requests(cx *core.Scope, a App) core.Node {
	batch := ui.NumberInput{TestID: "settings-llm-batch", Label: locales.SettingsAIBatchSize(cx), Min: media.MinBatchSize, Max: media.MaxBatchSize, Step: 5,
		Value:    live.Func(a, func() float64 { return float64(a.LLMBatchSize()) }),
		OnChange: live.On(a, func(v float64) { a.SetLLMRequests(int(v), a.LLMRetries()) }),
		Style:    core.Style{Width: core.Pt(96)}}
	retries := ui.NumberInput{TestID: "settings-llm-retries", Label: locales.SettingsAIRetries(cx), Min: 0, Max: media.MaxRetries, Step: 1,
		Value:    live.Func(a, func() float64 { return float64(a.LLMRetries()) }),
		OnChange: live.On(a, func(v float64) { a.SetLLMRequests(a.LLMBatchSize(), int(v)) }),
		Style:    core.Style{Width: core.Pt(96)}}
	isDefault := live.Func(a, func() bool {
		return a.LLMBatchSize() == media.DefaultBatchSize && a.LLMRetries() == media.DefaultRetries
	})
	restore := form.Button{Text: locales.SettingsAIRequestsRestore(cx), TestID: "settings-llm-requests-reset", Disabled: isDefault,
		OnPress: live.Do(a, func() { a.SetLLMRequests(media.DefaultBatchSize, media.DefaultRetries) })}
	return core.Group([]core.Node{
		form.Label(locales.SettingsAIRequests(cx), 14, restore),
		form.Card(
			form.Row{Title: locales.SettingsAIBatchSize(cx), Subtitle: locales.SettingsAIBatchSizeHint(cx), Trailing: []core.Node{batch}},
			form.Separator(true),
			form.Row{Title: locales.SettingsAIRetries(cx), Subtitle: locales.SettingsAIRetriesHint(cx), Trailing: []core.Node{retries}},
		),
		form.Note(static(locales.SettingsAIRequestsNote(cx, media.DefaultBatchSize, media.DefaultRetries)), 0),
	})
}
