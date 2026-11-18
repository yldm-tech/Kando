package settings

import (
	"fmt"
	"net/url"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// CacheClear is where 清除缓存 stands, as the 隐私 page shows it: Busy while the files are being removed; once done, Done with Freed, the bytes removed, and Err, why some files stayed, "" when none did.
type CacheClear struct {
	Busy, Done bool
	Freed      int64
	Err        string
}

// privacy is the 隐私 page: the offline switch, what each step sends over the network, and the caches on this computer.
type privacy struct{ s screen }

// tagKind is how a step of the table stands: on this computer only, over the network, over the network only when the user asks, or stopped by offline mode.
type tagKind int

const (
	tagLocal tagKind = iota
	tagNetwork
	tagManual
	tagOff
)

// step is a row of the table: what the step is, how it uses the network, and its tag.
type step struct {
	id            string
	title, detail string
	tag           string
	kind          tagKind
}

// Render implements core.Node.
func (p privacy) Render(cx *core.Scope) core.Node {
	a := p.s.a
	toggle := form.SwitchRow{Title: locales.SettingsPrivacyOffline(cx), Subtitle: locales.SettingsPrivacyOfflineHint(cx), TestID: "settings-privacy-offline",
		On: live.Func(a, a.Offline), OnChange: live.On(a, a.SetOffline)}
	// The table is built again when what its rows say changes: offline mode turned, or another AI service chosen.
	shape := core.NewMemo(cx, live.Func(a, func() string {
		return fmt.Sprint(a.Offline(), a.AILocal(), aiName(a))
	}))
	table := core.Dynamic(func(cx *core.Scope) core.Node {
		_ = shape.Get()
		var rows []core.Node
		for i, s := range steps(cx, a) {
			if i > 0 {
				rows = append(rows, form.Separator(true))
			}
			rows = append(rows, stepRow(cx, s))
		}
		return form.Card(rows...)
	})
	clear := form.Row{Title: locales.SettingsPrivacyCache(cx), SubtitleFunc: live.Func(a, func() string { return cacheSubtitle(cx, a.CacheClear()) }), TestID: "settings-privacy-cache",
		Trailing: []core.Node{form.Button{TextFunc: live.Func(a, func() string {
			if a.CacheClear().Busy {
				return locales.SettingsPrivacyClearing(cx)
			}
			return locales.SettingsPrivacyClearCache(cx)
		}), Disabled: live.Func(a, func() bool { return a.CacheClear().Busy }), OnPress: live.Do(a, a.ClearCache), TestID: "settings-privacy-clear-cache"}}}
	return form.Page(
		form.Card(toggle),
		form.Label(locales.SettingsPrivacySteps(cx), 12),
		table,
		form.Label(locales.SettingsPrivacyData(cx), 12),
		form.Card(clear),
	)
}

// cacheSubtitle is what the cache's row says under its title: what the cache holds, and once 清除缓存 is done how much it freed or why some files stayed.
func cacheSubtitle(cx *core.Scope, c CacheClear) string {
	hint := locales.SettingsPrivacyCacheHint(cx)
	switch {
	case !c.Done:
		return hint
	case c.Err != "":
		return locales.SettingsPrivacyClearFailed(cx, c.Err)
	case c.Freed == 0:
		return locales.SettingsPrivacyClearedNothing(cx)
	}
	return locales.SettingsPrivacyCleared(cx, byteSize(c.Freed))
}

// byteSize writes n bytes as the size a person reads, in KB, MB or GB from 1 KB on.
func byteSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.0f KB", max(1, float64(n)/(1<<10)))
}

// aiName names the AI service in use as the 翻译 row says it: its preset's name, or the host of a service of the user's own.
func aiName(a App) string {
	if preset, ok := a.CurrentPreset(); ok {
		return preset.Name
	}
	base := strings.TrimSpace(*a.LLMBase())
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}

// steps are the table's rows, in the order a film goes through them and then the rest, as they stand now.
func steps(cx *core.Scope, a App) []step {
	offline := a.Offline()
	network := func(id, title, detail string, kind tagKind, tag string) step {
		if offline {
			return step{id: id, title: title, detail: detail, tag: locales.SettingsPrivacyTagOff(cx), kind: tagOff}
		}
		return step{id: id, title: title, detail: detail, tag: tag, kind: kind}
	}
	out := []step{
		network("download", locales.SettingsPrivacyStepDownload(cx), locales.SettingsPrivacyStepDownloadDetail(cx), tagNetwork, locales.SettingsPrivacyTagNetwork(cx)),
		{id: "speech", title: locales.SettingsPrivacyStepSpeech(cx), detail: locales.SettingsPrivacyStepSpeechDetail(cx), tag: locales.SettingsPrivacyTagLocal(cx), kind: tagLocal},
	}
	name := aiName(a)
	if a.AILocal() {
		out = append(out, step{id: "ai", title: locales.SettingsPrivacyStepAI(cx), detail: locales.SettingsPrivacyStepAILocal(cx, name), tag: locales.SettingsPrivacyTagLocal(cx), kind: tagLocal})
	} else {
		out = append(out, network("ai", locales.SettingsPrivacyStepAI(cx), locales.SettingsPrivacyStepAICloud(cx, name), tagNetwork, locales.SettingsPrivacyTagCloud(cx)))
	}
	out = append(out,
		network("upload", locales.SettingsPrivacyStepUpload(cx), locales.SettingsPrivacyStepUploadDetail(cx), tagManual, locales.SettingsPrivacyTagManual(cx)),
		network("stream", locales.SettingsPrivacyStepStream(cx), locales.SettingsPrivacyStepStreamDetail(cx), tagNetwork, locales.SettingsPrivacyTagNetwork(cx)),
		network("tools", locales.SettingsPrivacyStepTools(cx), locales.SettingsPrivacyStepToolsDetail(cx), tagManual, locales.SettingsPrivacyTagManual(cx)),
		network("browse", locales.SettingsPrivacyStepBrowse(cx), locales.SettingsPrivacyStepBrowseDetail(cx), tagManual, locales.SettingsPrivacyTagManual(cx)),
	)
	// Offline mode stops only the check the user starts: the updater's daily one is NSGo's (CheckForUpdates in the main package says why), so the row says it goes on.
	updates := step{id: "updates", title: locales.SettingsPrivacyStepUpdates(cx), detail: locales.SettingsPrivacyStepUpdatesDetail(cx), tag: locales.SettingsPrivacyTagNetwork(cx), kind: tagNetwork}
	if offline {
		updates.detail = locales.SettingsPrivacyStepUpdatesOffline(cx)
	}
	out = append(out, updates,
		step{id: "phone", title: locales.SettingsPrivacyStepPhone(cx), detail: locales.SettingsPrivacyStepPhoneDetail(cx), tag: locales.SettingsPrivacyTagLAN(cx), kind: tagManual})
	return out
}

// stepRow is a row of the table: the step over how it uses the network, and its tag at the end, green on this computer, orange over the network, gray when the user starts it or offline mode stopped it.
func stepRow(cx *core.Scope, s step) core.Node {
	k := theme.Of(cx)
	colors := ui.ColorsOf(cx)
	fill, ink := k.WarnFill, k.WarnInk
	switch s.kind {
	case tagLocal:
		fill, ink = k.OKFill, k.SuccessInk
	case tagManual, tagOff:
		fill, ink = func() core.Color { return k.Shade(0.06) }, colors.Muted
	}
	tag := core.Text{Content: s.tag, MaxLines: 1, TestID: "settings-privacy-tag-" + s.id,
		Style:     core.Style{FontSize: 11, LineHeight: 18, FontWeight: ui.Bold, CornerRadius: 9, Padding: core.Symmetric(0, 8), FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Background: fill(), Color: ink()} }}
	return form.Row{Title: s.title, Subtitle: s.detail, Trailing: []core.Node{tag}, TestID: "settings-privacy-step-" + s.id}
}
