package settings

import (
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// PhoneSync is where Phone Sync stands, as its page shows it: whether it is on, why it could not start, and while it runs, the pairing code on offer with where the phone finds the desktop, and the phones paired with it.
type PhoneSync struct {
	On bool
	// Receive is 接收手机发来的链接, and AskFirst 收到后 set to 先问我: a link a phone sends waits in the link field for the user to start it, rather than being queued at once.
	Receive, AskFirst bool
	// Running is a server listening; Err is why the last start failed.
	Running bool
	Err     string
	// Addrs are where the desktop listens, as ip:port; none while the computer is on no local network.
	Addrs []string
	// Code is the pairing code on offer, "" for none; Used and Expired say it no longer pairs.
	Code          string
	Used, Expired bool
	// Fingerprint is the short form of the certificate's fingerprint, which a phone paired by hand shows to compare.
	Fingerprint string
	// QR is the pairing URL's QR code as a PNG, QRModules how many modules it is across; nil while no code pairs. The app keeps one slice per code, so the picture is not loaded again at each read.
	QR        []byte
	QRModules int
	Devices   []PhoneDevice
	// RevokeErr is why the last revocation failed.
	RevokeErr string
}

// PhoneDevice is a paired phone: its ID, the name it gave itself, its platform (ios, android or ""), when it paired and when it was last seen.
type PhoneDevice struct {
	ID, Name, Platform string
	Paired, LastSeen   time.Time
}

// live reports whether the code on offer pairs.
func (p PhoneSync) live() bool { return p.Code != "" && !p.Used && !p.Expired }

// qrPoints is how many points of the page each module of the QR code takes: half of the pixels the app draws it with (phonesync.QRScale), so it is sharp on a 2x screen.
const qrPoints = 3

// phone is the 手机同步 page: the switch, the pairing code while it is on, and the paired phones.
type phone struct{ s screen }

// Render implements core.Node.
func (p phone) Render(cx *core.Scope) core.Node {
	a := p.s.a
	colors := ui.ColorsOf(cx)
	toggle := form.SwitchRow{Title: locales.SettingsPhoneSwitch(cx), Subtitle: locales.SettingsPhoneSwitchHint(cx), TestID: "settings-phone-sync",
		On:       live.Func(a, func() bool { return a.PhoneSync().On }),
		OnChange: live.On(a, a.SetPhoneSync)}
	children := []core.Node{form.Card(toggle)}
	// Where the system asks before an app accepts connections from the network, the phone cannot connect until the user allows it.
	if platform := desktop.Platform(cx); platform == "macOS" || platform == "Windows" {
		children = append(children, form.Note(static(locales.SettingsPhoneFirewall(cx, platform)), 0))
	}
	failed := core.Show{When: live.Func(a, func() bool { return a.PhoneSync().Err != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, func() string { return locales.SettingsPhoneStartFailed(cx, a.PhoneSync().Err) }), Selectable: core.SelectableOn,
			Style: core.Style{FontSize: 11, Padding: core.Edges{Top: 4, Right: 12, Left: 12}}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	running := core.Show{When: live.Func(a, func() bool { return a.PhoneSync().Running }), Then: func(cx *core.Scope) core.Node {
		return core.Group([]core.Node{
			form.Label(locales.SettingsPhonePair(cx), 14, form.Button{Text: locales.SettingsPhoneNewCode(cx), OnPress: live.Do(a, a.NewPhoneCode), TestID: "settings-phone-new-code"}),
			form.Card(pairing{a: a}),
			form.Label(locales.SettingsPhoneDevices(cx), 14),
			form.Card(devices{a: a}),
			form.Label(locales.SettingsPhoneLinks(cx), 14),
			form.Card(receiveLinks(cx, a)...),
			core.Show{When: live.Func(a, func() bool { return a.PhoneSync().RevokeErr != "" }), Then: func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: live.Func(a, func() string { return locales.SettingsPhoneRevokeFailed(cx, a.PhoneSync().RevokeErr) }), Selectable: core.SelectableOn,
					Style: core.Style{FontSize: 11, Padding: core.Edges{Top: 4, Right: 12, Left: 12}}, StyleFunc: ui.Colored(colors.Danger)}
			}},
		})
	}}
	children = append(children, failed, running)
	return form.Page(children...)
}

// pairing is the card of the code on offer: its QR code beside where the desktop is, the code and the fingerprint's short form, which a phone that cannot scan types and compares. A code that was used or expired says so in place of the QR code, and 新代码 above the card offers another.
type pairing struct{ a App }

// pairingState is what the QR code's place shows.
type pairingState uint8

const (
	pairingQR pairingState = iota
	pairingUsed
	pairingExpired
	pairingNoNetwork
	pairingNone
)

// Render implements core.Node.
func (p pairing) Render(cx *core.Scope) core.Node {
	a := p.a
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	state := core.NewMemo(cx, live.Func(a, func() pairingState {
		switch st := a.PhoneSync(); {
		case len(st.Addrs) == 0:
			return pairingNoNetwork
		case st.Code == "":
			return pairingNone
		case st.Used:
			return pairingUsed
		case st.Expired:
			return pairingExpired
		case st.QR == nil:
			return pairingNone
		}
		return pairingQR
	}))
	modules := core.NewMemo(cx, live.Func(a, func() int { return a.PhoneSync().QRModules }))
	side := func() core.Points { return core.Points(max(modules.Get(), 29) * qrPoints) }
	code := core.Dynamic(func(cx *core.Scope) core.Node {
		if state.Get() == pairingQR {
			return core.Image{Label: locales.SettingsPhoneQRLabel(cx), TestID: "settings-phone-qr", ResizeMode: core.Contain,
				SourceFunc: live.Func(a, func() core.ImageSource { return core.ImageSource{Data: a.PhoneSync().QR} }),
				StyleFunc: func() core.Style {
					return core.Style{Width: core.Pt(side()), Height: core.Pt(side()), FlexShrink: 0, CornerRadius: 6}
				}}
		}
		var text string
		switch state.Get() {
		case pairingUsed:
			text = locales.SettingsPhoneCodeUsed(cx)
		case pairingExpired:
			text = locales.SettingsPhoneCodeExpired(cx)
		case pairingNoNetwork:
			text = locales.SettingsPhoneNoNetwork(cx)
		}
		return core.View{TestID: "settings-phone-no-qr", StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(side()), Height: core.Pt(side()), FlexShrink: 0, CornerRadius: 6, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 12),
				Background: k.Tile()}
		}, Children: []core.Node{core.Text{Content: text, Style: core.Style{FontSize: 12, LineHeight: 17, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)}}}
	})
	field := func(title func() string, value func() string, id string, size core.Points) core.Node {
		return core.View{Style: core.Style{Gap: 2, FlexShrink: 0}, Children: []core.Node{
			core.Text{ContentFunc: title, Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)},
			core.Text{ContentFunc: value, TestID: id, Selectable: core.SelectableOn, Style: core.Style{FontFamily: "monospace", FontSize: size, LineHeight: size + 5}, StyleFunc: ui.Colored(colors.Text)},
		}}
	}
	addrs := live.Func(a, func() []string { return a.PhoneSync().Addrs })
	texts := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 12}, Children: []core.Node{
		core.Text{Content: locales.SettingsPhonePairHint(cx), Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)},
		field(func() string { return locales.SettingsPhoneAddress(cx, len(addrs())) }, func() string { return strings.Join(addrs(), "\n") }, "settings-phone-address", 13),
		field(func() string { return locales.SettingsPhoneCode(cx) }, live.Func(a, func() string {
			if st := a.PhoneSync(); st.live() && len(st.Addrs) > 0 {
				return st.Code[:3] + " " + st.Code[3:]
			}
			return "—"
		}), "settings-phone-code", 20),
		field(func() string { return locales.SettingsPhoneFingerprint(cx) }, live.Func(a, func() string { return a.PhoneSync().Fingerprint }), "settings-phone-fingerprint", 13),
	}}
	return core.View{Role: core.RoleGroup, Label: locales.SettingsPhonePair(cx), TestID: "settings-phone-pairing",
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, Padding: core.Symmetric(14, 14), Gap: 18, AlignItems: core.AlignStart},
		Children: []core.Node{code, texts}}
}

// devices is the card of the paired phones, each with when it was last seen and 撤销, or a line saying none is paired.
type devices struct{ a App }

// Render implements core.Node.
func (d devices) Render(cx *core.Scope) core.Node {
	a := d.a
	// ids is the devices' IDs joined, so the rows are built again only when a phone pairs or is revoked; a row reads its device's times in its bindings.
	ids := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, dev := range a.PhoneSync().Devices {
			out = append(out, dev.ID)
		}
		return strings.Join(out, ",")
	}))
	return core.Dynamic(func(cx *core.Scope) core.Node {
		joined := ids.Get()
		if joined == "" {
			return form.Row{Title: locales.SettingsPhoneNoDevices(cx), NoGroup: true, TestID: "settings-phone-no-devices"}
		}
		var rows []core.Node
		for i, id := range strings.Split(joined, ",") {
			if i > 0 {
				rows = append(rows, form.Separator(true))
			}
			rows = append(rows, deviceRow(cx, a, id))
		}
		return core.Group(rows)
	})
}

// deviceRow is the row of paired phone id: its name over its platform and when it was last seen, and 撤销.
func deviceRow(cx *core.Scope, a App, id string) core.Node {
	device := func() PhoneDevice {
		for _, dev := range a.PhoneSync().Devices {
			if dev.ID == id {
				return dev
			}
		}
		return PhoneDevice{}
	}
	name := device().Name
	return form.Row{Title: name, TestID: "settings-phone-device-" + id,
		SubtitleFunc: live.Func(a, func() string {
			dev := device()
			return locales.SettingsPhoneDeviceSeen(cx, dev.Platform, dev.LastSeen)
		}),
		Trailing: []core.Node{form.Button{Text: locales.SettingsPhoneRevoke(cx), Label: locales.SettingsPhoneRevokeLabel(cx, name), TestID: "settings-phone-revoke-" + id,
			OnPress: live.Do(a, func() { a.RevokePhone(id) })}}}
}

// receiveLinks are the rows of 手机发来的链接: whether the desktop takes the links phones send, and what it does with one, which a link taken nowhere leaves disabled.
func receiveLinks(cx *core.Scope, a App) []core.Node {
	receive := form.SwitchRow{Title: locales.SettingsPhoneReceive(cx), Subtitle: locales.SettingsPhoneReceiveHint(cx), TestID: "settings-phone-receive",
		On:       live.Func(a, func() bool { return a.PhoneSync().Receive }),
		OnChange: live.On(a, a.SetPhoneReceive)}
	then := ui.Select[bool]{Label: locales.SettingsPhoneReceiveThen(cx), TestID: "settings-phone-receive-then",
		Options:  []ui.Option[bool]{{Value: false, Label: locales.SettingsPhoneReceiveQueue(cx)}, {Value: true, Label: locales.SettingsPhoneReceiveAsk(cx)}},
		Value:    live.Func(a, func() bool { return a.PhoneSync().AskFirst }),
		OnChange: live.On(a, a.SetPhoneAskFirst),
		Disabled: live.Func(a, func() bool { return !a.PhoneSync().Receive }),
		Style:    core.Style{Width: core.Pt(160)}}
	return []core.Node{receive, form.Separator(true), form.Row{Title: locales.SettingsPhoneReceiveThen(cx), NoGroup: true, Trailing: []core.Node{then}}}
}
