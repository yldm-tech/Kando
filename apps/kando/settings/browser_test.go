package settings

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

// The 浏览器扩展 card offers 连接 while a browser found is not connected, says which are, and offers 断开连接 once all are; while Kando runs from its disk image it asks to move Kando first and offers neither. 显示扩展文件夹 is always there, with how each browser family loads the folder.
func TestBrowserCardConnects(t *testing.T) {
	a := newFakeApp()
	a.ext = BrowserExtension{Known: true, Found: []string{"Chrome", "Firefox"}, Connected: []string{"Chrome"}}
	app := start(t, a)
	app.Resize(880, 1800)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.SettingsGeneralBrowserConnectedTo("Chrome"))) || !app.Exists(whole(msgs.SettingsGeneralBrowserLoadHint())) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press("id:settings-browser-connect")
	app.Press("id:settings-browser-folder")
	if !slices.Contains(a.calls, "connect browsers") || !slices.Contains(a.calls, "show extension folder") {
		t.Fatalf("calls %q", a.calls)
	}

	change(app, a, func() { a.ext.Connected = []string{"Chrome", "Firefox"} })
	if app.Exists("id:settings-browser-connect") || !app.Exists(whole(msgs.SettingsGeneralBrowserConnected())) {
		t.Fatalf("connected: %q", app.Texts())
	}
	app.Press("id:settings-browser-disconnect")
	if !slices.Contains(a.calls, "disconnect browsers") {
		t.Fatalf("calls %q", a.calls)
	}

	change(app, a, func() { a.ext.Busy = true })
	if app.Enabled("id:settings-browser-disconnect") || app.Enabled("id:settings-browser-folder") {
		t.Fatal("the card's buttons are on while one of them works")
	}

	change(app, a, func() { a.ext = BrowserExtension{Known: true, Temporary: true, Found: []string{"Chrome"}} })
	if app.Exists("id:settings-browser-connect") || !app.Exists(whole(msgs.SettingsGeneralCLIMoveFirst())) {
		t.Fatalf("temporary: %q", app.Texts())
	}
	change(app, a, func() { a.ext = BrowserExtension{Known: true, Err: "permission denied"} })
	if app.Exists("id:settings-browser-connect") || !app.Exists(whole(msgs.SettingsGeneralBrowserNone())) || !app.Exists(whole("permission denied")) {
		t.Fatalf("no browser: %q", app.Texts())
	}
}
