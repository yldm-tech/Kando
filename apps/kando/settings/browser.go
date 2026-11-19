package settings

import (
	"strings"

	"kando/apps/kando/browserext"
	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// BrowserExtension is where Kando's browser extension stands, as the 浏览器扩展 card of 通用 shows it: which browsers are on the computer, and which of them start Kando as the extension's native messaging host. The app looks it up when the page opens, never while the page is drawn.
type BrowserExtension struct {
	// Known reports whether the app has looked yet.
	Known bool
	// Found are the browsers on the computer the extension runs in, Connected those of them that start Kando as its host.
	Found     []string
	Connected []string
	// Temporary reports whether Kando runs from a place that goes away, where it cannot be connected: the card then asks to move Kando first.
	Temporary bool
	// Busy is a connection, a disconnection or a copy of the extension's folder under way; Err is why the last one failed.
	Busy bool
	Err  string
}

// connected reports whether every browser found starts Kando as the host.
func (b BrowserExtension) connected() bool {
	return b.Known && len(b.Found) > 0 && len(b.Connected) == len(b.Found)
}

// browserState is what the end of the 浏览器 row shows.
type browserState uint8

const (
	browserNothing browserState = iota
	browserConnected
	browserConnectable
)

// browserExt is the 浏览器扩展 card: the browsers that start Kando as the extension's host, with 连接 or 断开连接, and the extension's folder, with how each browser family loads it.
type browserExt struct{ s screen }

// Render implements core.Node.
func (b browserExt) Render(cx *core.Scope) core.Node {
	a := b.s.a
	colors := ui.ColorsOf(cx)
	state := core.NewMemo(cx, live.Func(a, func() browserState {
		switch st := a.BrowserExtension(); {
		case !st.Known, st.Temporary, len(st.Found) == 0:
			return browserNothing
		case st.connected():
			return browserConnected
		}
		return browserConnectable
	}))
	busy := live.Func(a, func() bool { return a.BrowserExtension().Busy })
	end := core.Dynamic(func(cx *core.Scope) core.Node {
		switch state.Get() {
		case browserConnected:
			return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, Gap: 10, AlignItems: core.AlignCenter}, Children: []core.Node{
				form.Status(ui.Fixed(form.OK), colors.Muted, static(locales.SettingsGeneralBrowserConnected(cx))),
				form.Button{Text: locales.SettingsGeneralBrowserDisconnect(cx), TestID: "settings-browser-disconnect", Disabled: busy, OnPress: live.Do(a, func() {
					if !a.BrowserExtension().Busy {
						a.DisconnectBrowsers()
					}
				})},
			}}
		case browserConnectable:
			return form.Button{Text: locales.SettingsGeneralBrowserConnect(cx), TestID: "settings-browser-connect", Disabled: busy, OnPress: live.Do(a, func() {
				if !a.BrowserExtension().Busy {
					a.ConnectBrowsers()
				}
			})}
		}
		return core.Group(nil)
	})
	browsers := form.Row{Title: locales.SettingsGeneralBrowserBrowsers(cx), SubtitleFunc: live.Func(a, func() string { return browserDetail(cx, a.BrowserExtension()) }), Trailing: []core.Node{end}}
	folder := form.Row{Title: locales.SettingsGeneralBrowserFolder(cx), Subtitle: locales.SettingsGeneralBrowserLoadHint(cx),
		Trailing: []core.Node{form.Button{Text: locales.SettingsGeneralBrowserShowFolder(cx), TestID: "settings-browser-folder", Disabled: busy, OnPress: live.Do(a, func() {
			if !a.BrowserExtension().Busy {
				a.ShowExtensionFolder()
			}
		})}}}
	startAtOnce := form.SwitchRow{Title: locales.SettingsGeneralBrowserStartAtOnce(cx), Subtitle: locales.SettingsGeneralBrowserStartAtOnceHint(cx), TestID: "settings-browser-start-at-once",
		On: live.Func(a, a.LinkStartAtOnce),
		OnChange: live.On(a, func(on bool) {
			a.SetLinkStartAtOnce(on)
			a.Save()
		})}
	rows := []core.Node{browsers, form.Separator(true), folder}
	// The bookmarklet opens a link of Kando's own scheme, which only the macOS package registers with the system (nsgo.yaml's url_schemes), so it is offered there alone.
	// TODO(nsgo#262): register the app's url_schemes with Windows (HKCU\Software\Classes) and the Linux desktop entry (x-scheme-handler), and hand the running app the links a second launch is started with to desktop.OnOpenLinks, then offer the bookmarklet there too https://github.com/nsgo-dev/nsgo/issues/262
	if desktop.Platform(cx) == "macOS" {
		rows = append(rows, form.Separator(true), bookmarklet(cx))
	}
	rows = append(rows, form.Separator(true), startAtOnce)
	failed := core.Show{When: live.Func(a, func() bool { return a.BrowserExtension().Err != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, func() string { return a.BrowserExtension().Err }),
			Style: core.Style{FontSize: 11, Padding: core.Edges{Top: 4, Right: 12, Left: 12}}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	return core.Group([]core.Node{
		form.Label(locales.SettingsGeneralBrowser(cx), 14),
		form.Card(rows...),
		failed,
	})
}

// bookmarklet is the 书签小工具 row: the bookmarklet works in any browser, without the extension, and 拷贝代码 puts its address on the clipboard, for a bookmark's address, reading 已拷贝 once it did.
func bookmarklet(cx *core.Scope) core.Node {
	copied := core.NewSignal(cx, false)
	label := locales.SettingsGeneralBookmarkletCopy(cx)
	return form.Row{Title: locales.SettingsGeneralBookmarklet(cx), Subtitle: locales.SettingsGeneralBookmarkletHint(cx), TestID: "settings-bookmarklet",
		Trailing: []core.Node{form.Button{TestID: "settings-bookmarklet-copy", Label: label,
			TextFunc: func() string {
				if copied.Get() {
					return locales.SettingsGeneralCLICopied(cx)
				}
				return label
			},
			OnPress: func() {
				cx.SetClipboard(browserext.Bookmarklet())
				copied.Set(true)
			}}}}
}

// browserDetail is the line under 浏览器, in the language the app shows under cx: what the connection is for until the app has looked or while no browser is connected, to move Kando first while it runs from a place that goes away, that no browser was found, or which browsers are connected.
func browserDetail(cx *core.Scope, st BrowserExtension) string {
	switch {
	case !st.Known:
		return locales.SettingsGeneralBrowserHint(cx)
	case st.Temporary:
		return locales.SettingsGeneralCLIMoveFirst(cx)
	case len(st.Found) == 0:
		return locales.SettingsGeneralBrowserNone(cx)
	case len(st.Connected) > 0:
		return locales.SettingsGeneralBrowserConnectedTo(cx, strings.Join(st.Connected, locales.SettingsGeneralBrowserSeparator(cx)))
	}
	return locales.SettingsGeneralBrowserHint(cx)
}
