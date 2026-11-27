package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// PhoneRelay is the storage relay as the 手机同步 page shows it: phones away from this computer's network reach it through the storage of 账号与上传 (yldm-tech/Kando#219).
type PhoneRelay struct {
	// On is the relay's switch, which settings.json keeps.
	On bool
	// Store names the storage the relay goes through, as "R2 · films", "" while none is set up in 账号与上传.
	Store string
	// Offline is 隐私's 离线模式, under which the relay does not reach the storage.
	Offline bool
	// Working is a turn of the relay that went through since it started, and Err why its last turn failed, in the interface's language, "" for none.
	Working bool
	Err     string
	// SubtitlesOnly is 只同步字幕, which settings.json keeps: the films go to the storage as their lines and posters, without their videos.
	SubtitlesOnly bool
	// Storage is what the films and the messages take in the storage (phone_storage.go).
	Storage PhoneStorage
	// Hosted is 看懂托管 chosen, which settings.json keeps: the relay goes through Kando Plus's hosted storage rather than the one of 账号与上传, and Store names it. HostedLapsed is it chosen while its Kando Plus and the grace after it ended, so the relay went back to the storage of 账号与上传, or to the local network alone.
	Hosted, HostedLapsed bool
	// Plus is Kando Plus active, as EveryAPI last said: 看懂托管 can be turned on only with it. HostedUsed and HostedQuota are what the account uses of the hosted storage and its quota in bytes, 0 for not known; HostedFull is it full while the relay runs there, so the films go as subtitles only.
	Plus                    bool
	HostedUsed, HostedQuota int64
	HostedFull              bool
}

// hostedInUse reports whether the relay goes through 看懂托管.
func (r PhoneRelay) hostedInUse() bool { return r.Hosted && !r.HostedLapsed }

// relayState is what the relay's status row says.
type relayState uint8

const (
	relayOff relayState = iota
	relayOffline
	relayNoStore
	relayFailed
	relayWorking
	relayChecking
	relayHostedFull
)

func (r PhoneRelay) state() relayState {
	switch {
	case !r.On:
		return relayOff
	case r.Offline:
		return relayOffline
	case r.Store == "":
		return relayNoStore
	case r.Err != "":
		return relayFailed
	case r.HostedFull && r.hostedInUse():
		return relayHostedFull
	case r.Working:
		return relayWorking
	}
	return relayChecking
}

// relayCard is the card of 不在同一网络时: the relay's switch, 看懂托管 while it is on, the storage of 账号与上传 it goes through otherwise with 去设置…, and while it is on, whether it works.
func relayCard(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	relay := func() PhoneRelay { return a.PhoneSync().Relay }
	toggle := form.SwitchRow{Title: locales.SettingsPhoneRelay(cx), Subtitle: locales.SettingsPhoneRelayHint(cx), TestID: "settings-phone-relay",
		On:       live.Func(a, func() bool { return relay().On }),
		OnChange: live.On(a, a.SetPhoneRelay)}
	store := form.Row{Title: locales.SettingsPhoneRelayStore(cx), TestID: "settings-phone-relay-store", Trailing: []core.Node{
		core.Text{TestID: "settings-phone-relay-store-name", ContentFunc: live.Func(a, func() string {
			if s := relay().Store; s != "" {
				return s
			}
			return locales.SettingsPhoneRelayNoStore(cx)
		}), MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FontSize: 12, LineHeight: 17, MaxWidth: core.Pt(220), FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)},
		form.Button{Text: locales.SettingsPhoneRelaySetUp(cx), TestID: "settings-phone-relay-set-up", OnPress: live.Do(a, func() { a.SetSettingsSection(SectionAccount) })},
	}}
	state := core.NewMemo(cx, live.Func(a, func() relayState { return relay().state() }))
	dot := func() core.Color {
		switch state.Get() {
		case relayWorking:
			return form.OK
		case relayFailed, relayNoStore:
			return colors.Danger()
		}
		return k.Idle()
	}
	color := func() core.Color {
		switch state.Get() {
		case relayWorking:
			return k.SuccessInk()
		case relayFailed, relayNoStore:
			return colors.Danger()
		}
		return colors.Muted()
	}
	text := live.Func(a, func() string {
		switch state.Get() {
		case relayOffline:
			return locales.SettingsPhoneRelayOffline(cx)
		case relayNoStore:
			return locales.SettingsPhoneRelayNeedsStore(cx)
		case relayFailed:
			return relay().Err
		case relayWorking:
			return locales.SettingsPhoneRelayWorking(cx)
		case relayHostedFull:
			return locales.SettingsPhoneRelayHostedFull(cx)
		}
		return locales.SettingsPhoneRelayChecking(cx)
	})
	status := core.Show{When: func() bool { return state.Get() != relayOff }, Then: func(cx *core.Scope) core.Node {
		return core.Group{form.Separator(true), form.Row{Title: locales.SettingsPhoneRelayStatus(cx), TestID: "settings-phone-relay-status", Trailing: []core.Node{
			form.Status(dot, color, text),
		}}}
	}}
	// 只同步字幕 shows while the relay is on, as it is about what goes through it.
	subtitles := core.Show{When: live.Func(a, func() bool { return relay().On }), Then: func(cx *core.Scope) core.Node {
		return core.Group{form.Separator(true), form.SwitchRow{Title: locales.SettingsPhoneRelaySubtitlesOnly(cx), Subtitle: locales.SettingsPhoneRelaySubtitlesOnlyHint(cx), TestID: "settings-phone-relay-subtitles-only",
			On:       live.Func(a, func() bool { return relay().SubtitlesOnly }),
			OnChange: live.On(a, a.SetPhoneRelaySubtitlesOnly)}}
	}}
	// 看懂托管 shows while the relay is on, as a choice of where it goes; the storage of 账号与上传 shows while the relay goes there.
	hosted := core.Show{When: live.Func(a, func() bool { return relay().On }), Then: func(cx *core.Scope) core.Node {
		return core.Group{form.Separator(true), hostedRow(cx, a)}
	}}
	own := core.Show{When: live.Func(a, func() bool { return !relay().hostedInUse() }), Then: func(*core.Scope) core.Node {
		return core.Group{form.Separator(true), store}
	}}
	return form.Card(toggle, hosted, own, status, subtitles)
}

// hostedRow is 看懂托管: the relay through Kando Plus's hosted storage in place of the one of 账号与上传. Without Plus its switch cannot be turned on, and 订阅… opens EveryAPI's page to subscribe; with it, the row says how much of the storage is used.
func hostedRow(cx *core.Scope, a App) core.Node {
	relay := func() PhoneRelay { return a.PhoneSync().Relay }
	subtitle := live.Func(a, func() string {
		r := relay()
		switch {
		case r.HostedLapsed:
			return locales.SettingsPhoneRelayHostedLapsed(cx)
		case !r.Plus:
			return locales.SettingsPhoneRelayHostedNeedsPlus(cx)
		case r.HostedQuota > 0:
			return locales.SettingsPhoneRelayHostedUsage(cx, byteSize(r.HostedUsed), byteSize(r.HostedQuota))
		}
		return ""
	})
	subscribe := core.Show{When: live.Func(a, func() bool { return !relay().Plus }), Then: func(cx *core.Scope) core.Node {
		return form.Button{Text: locales.SettingsPhoneRelayHostedSubscribe(cx), TestID: "settings-phone-relay-hosted-subscribe", OnPress: live.Do(a, a.SubscribePhoneRelay)}
	}}
	// Turning it off is always allowed; on, only with Plus.
	toggle := ui.Switch{Label: locales.SettingsPhoneRelayHosted(cx), TestID: "settings-phone-relay-hosted",
		Checked:  live.Func(a, func() bool { return relay().Hosted }),
		Disabled: live.Func(a, func() bool { r := relay(); return !r.Plus && !r.Hosted }),
		OnChange: live.On(a, a.SetPhoneRelayHosted)}
	return form.Row{Title: locales.SettingsPhoneRelayHosted(cx), SubtitleFunc: subtitle, NoGroup: true, TestID: "settings-phone-relay-hosted-row", Trailing: []core.Node{subscribe, toggle}}
}

// storageSection is the 存储用量 label and card, while the relay is on with a storage set up and not paused by 离线模式.
func storageSection(cx *core.Scope, a App) core.Node {
	shown := live.Func(a, func() bool {
		r := a.PhoneSync().Relay
		return r.state() == relayWorking || r.state() == relayChecking || r.state() == relayFailed || r.state() == relayHostedFull
	})
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		where := core.Text{TestID: "settings-phone-storage-where", ContentFunc: live.Func(a, func() string { return a.PhoneSync().Relay.Store }), MaxLines: 1,
			Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
		return core.Group{form.Label(locales.SettingsPhoneStorage(cx), 14, where), storageCard(cx, a)}
	}}
}
