package settings

import (
	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
)

// Background is how Kando runs with its window closed, as the 后台运行 card of 通用 shows it.
type Background struct {
	// LaunchAtLogin is whether the system opens Kando at login, which CanLaunchAtLogin says Kando can set: on macOS, Windows and Linux.
	LaunchAtLogin, CanLaunchAtLogin bool
	// KeepRunning is whether closing the main window keeps Kando running on Windows and Linux, as it always does on macOS.
	KeepRunning bool
	// TrayIcon is whether Kando's status icon shows in the menu bar, the notification area or the system tray.
	TrayIcon bool
}

// background is the rows of the 后台运行 card: 登录电脑时打开看懂 where Kando can set it, 关掉窗口后继续在后台运行 off macOS, where closing the window never quits, and the status icon. Each switch is saved at once.
type background struct{ s screen }

// Render implements core.Node.
func (b background) Render(cx *core.Scope) core.Node {
	a := b.s.a
	platform := desktop.Platform(cx)
	var rows []core.Node
	add := func(row core.Node) {
		if len(rows) > 0 {
			rows = append(rows, form.Separator(true))
		}
		rows = append(rows, row)
	}
	if a.Background().CanLaunchAtLogin {
		add(form.SwitchRow{Title: locales.SettingsGeneralLaunchAtLogin(cx), Subtitle: locales.SettingsGeneralLaunchAtLoginHint(cx, platform), TestID: "settings-launch-at-login",
			On:       live.Func(a, func() bool { return a.Background().LaunchAtLogin }),
			OnChange: live.On(a, a.SetLaunchAtLogin)})
	}
	if platform != "macOS" {
		add(form.SwitchRow{Title: locales.SettingsGeneralKeepRunning(cx), Subtitle: locales.SettingsGeneralKeepRunningHint(cx), TestID: "settings-keep-running",
			On: live.Func(a, func() bool { return a.Background().KeepRunning }),
			OnChange: live.On(a, func(on bool) {
				a.SetKeepRunning(on)
				a.Save()
			})})
	}
	add(form.SwitchRow{Title: locales.SettingsGeneralTrayIcon(cx, platform), Subtitle: locales.SettingsGeneralTrayIconHint(cx), TestID: "settings-tray-icon",
		On: live.Func(a, func() bool { return a.Background().TrayIcon }),
		OnChange: live.On(a, func(on bool) {
			a.SetTrayIcon(on)
			a.Save()
		})})
	return core.View{Style: core.Style{AlignItems: core.AlignStretch}, Children: rows}
}
