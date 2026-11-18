package settings

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// devices is the card of the paired phones, each with whether it is online, its 只读 switch and 撤销…, which opens under it what revoking it does before it is revoked; or a line saying none is paired.
type devices struct{ a App }

// Render implements core.Node.
func (d devices) Render(cx *core.Scope) core.Node {
	a := d.a
	// ids is the devices' IDs joined, so the rows are built again only when a phone pairs or is revoked; a row reads its device's times and switch in its bindings.
	ids := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, dev := range a.PhoneSync().Devices {
			out = append(out, dev.ID)
		}
		return strings.Join(out, ",")
	}))
	// asking is the phone whose revocation is being confirmed, "" for none.
	asking := core.NewSignal(cx, "")
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
			rows = append(rows, deviceRow(cx, a, id, asking))
		}
		return core.Group(rows)
	})
}

// deviceRow is the row of paired phone id: a dot, green while it is online, its name over its platform and whether it is online or when it was last seen, its 只读 switch and 撤销…, and under it, while asking names it, what revoking it does with 撤销这台设备 and 取消.
func deviceRow(cx *core.Scope, a App, id string, asking *core.Signal[string]) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	device := func() PhoneDevice {
		for _, dev := range a.PhoneSync().Devices {
			if dev.ID == id {
				return dev
			}
		}
		return PhoneDevice{}
	}
	name := device().Name
	online := live.Func(a, func() bool { return device().Online })
	dot := form.Dot(func() core.Color {
		if online() {
			return form.OK
		}
		return k.Idle()
	})
	titles := form.Titles{Title: name, SubtitleFunc: live.Func(a, func() string {
		dev := device()
		if dev.Online {
			return locales.SettingsPhoneDeviceOnline(cx, dev.Platform)
		}
		return locales.SettingsPhoneDeviceSeen(cx, dev.Platform, dev.LastSeen)
	})}
	readOnly := core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
		core.Text{Content: locales.SettingsPhoneReadOnly(cx), Role: core.RoleNone, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)},
		ui.Switch{Label: locales.SettingsPhoneReadOnlyLabel(cx, name), Size: ui.SizeSmall, TestID: "settings-phone-read-only-" + id,
			Checked:  live.Func(a, func() bool { return device().ReadOnly }),
			OnChange: live.On(a, func(on bool) { a.SetPhoneReadOnly(id, on) })},
	}}
	revoke := form.Button{Text: locales.SettingsPhoneRevoke(cx), Label: locales.SettingsPhoneRevokeLabel(cx, name), TestID: "settings-phone-revoke-" + id,
		OnPress: func() {
			if asking.Get() == id {
				asking.Set("")
				return
			}
			asking.Set(id)
		}}
	row := core.View{Role: core.RoleGroup, Label: name, TestID: "settings-phone-device-" + id,
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(46), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
		Children: []core.Node{dot, titles, readOnly, revoke}}
	confirm := core.Show{When: func() bool { return asking.Get() == id }, Then: func(cx *core.Scope) core.Node {
		return revokeConfirm(cx, a, id, name, func() { asking.Set("") })
	}}
	return core.Group{row, confirm}
}

// revokeConfirm is what revoking phone id, named name, does, under its row: its key stops working at once, the jobs it sent go on, what it saved for offline stays on it, pairing it again takes a new code, and while the storage relay is on, its messages there are deleted but the links it holds last until they lapse; with 撤销这台设备, which revokes it, and 取消, which closes it (done).
func revokeConfirm(cx *core.Scope, a App, id, name string, done func()) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	text := func(s string, size core.Points, color func() core.Color) core.Node {
		return core.Text{Content: s, Style: core.Style{FontSize: size, LineHeight: size + 5}, StyleFunc: ui.Colored(color)}
	}
	consequences := []string{locales.SettingsPhoneRevokeKey(cx), locales.SettingsPhoneRevokeJobs(cx), locales.SettingsPhoneRevokeOffline(cx), locales.SettingsPhoneRevokePairAgain(cx)}
	// The relay's links the phone holds outlive its key until they lapse, which only a new key of the storage cuts short (packages/sync/relay.go, Revocation).
	if a.PhoneSync().Relay.On {
		consequences = append(consequences, locales.SettingsPhoneRevokeStorage(cx))
	}
	var points []core.Node
	for _, p := range consequences {
		points = append(points, core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{text("•", 12, colors.Muted), text(p, 12, colors.Text)}})
	}
	buttons := core.View{Style: core.Style{Direction: core.Row, Gap: 8, Margin: core.Edges{Top: 4}}, Children: []core.Node{
		ui.Button{Text: locales.SettingsPhoneRevokeConfirm(cx), Color: colors.Danger, Size: ui.SizeSmall, TestID: "settings-phone-revoke-confirm-" + id,
			OnPress: live.Do(a, func() {
				done()
				a.RevokePhone(id)
			})},
		ui.Button{Text: locales.SettingsPhoneRevokeCancel(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "settings-phone-revoke-cancel-" + id, OnPress: done},
	}}
	children := []core.Node{core.Text{Content: locales.SettingsPhoneRevokeTitle(cx, name), Role: core.RoleHeader, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)}}
	children = append(children, points...)
	children = append(children, buttons)
	return core.View{Role: core.RoleGroup, Label: locales.SettingsPhoneRevokeTitle(cx, name), TestID: "settings-phone-revoke-ask-" + id,
		Style:     core.Style{FlexShrink: 0, Gap: 4, Margin: core.Edges{Right: 14, Bottom: 10, Left: 14}, Padding: core.Symmetric(10, 12), CornerRadius: 10},
		StyleFunc: ui.Filled(k.Tile), Children: children}
}
