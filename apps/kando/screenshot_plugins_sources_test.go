package main

import (
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginNASFields are the fields of acme.nas, the script plugin of the media sources' scenes and tests (docs/media-sources.md 11.1): a WebDAV source whose password Kando sends as HTTP Basic, reaching its server through kando.source.fetch alone.
const pluginNASFields = `"capabilities": {"host": ["kando.source.fetch"]},
  "contributes": {
    "sources": [{"id": "webdav", "icon": "server", "title": {"en": "WebDAV", "zh-Hans": "WebDAV", "zh-Hant": "WebDAV", "ja": "WebDAV", "ko": "WebDAV"}, "fields": [
      {"key": "endpoint", "type": "url", "title": {"en": "Server address", "zh-Hans": "服务器地址", "zh-Hant": "伺服器位址", "ja": "サーバーのアドレス", "ko": "서버 주소"}},
      {"key": "user", "type": "text", "title": {"en": "User", "zh-Hans": "用户名", "zh-Hant": "使用者名稱", "ja": "ユーザー名", "ko": "사용자 이름"}},
      {"key": "password", "type": "secret", "auth": {"basic": ["user", "password"]}, "title": {"en": "Password", "zh-Hans": "密码", "zh-Hant": "密碼", "ja": "パスワード", "ko": "비밀번호"}}
    ]}]
  }`

// pluginNASAbout is acme.nas' author, name and description.
const pluginNASAbout = `"author": "Acme", "name": {"en": "NAS Browser", "zh-Hans": "NAS 浏览器", "zh-Hant": "NAS 瀏覽器", "ja": "NAS ブラウザ", "ko": "NAS 브라우저"},
  "description": {"en": "Browse the films on a WebDAV server.", "zh-Hans": "浏览 WebDAV 服务器上的影片。", "zh-Hant": "瀏覽 WebDAV 伺服器上的影片。", "ja": "WebDAV サーバーの動画を見る。", "ko": "WebDAV 서버의 영상을 둘러봅니다."}`

// pluginNASCode is acme.nas' code in these scenes, which only show what it asks for: a source that lists nothing.
const pluginNASCode = `export async function list(source, dir) { return [] }
export async function stat(source, path) { return null }
export async function open(source, path) { return { path } }
`

// screenshotNAS installs acme.nas in a plugins folder of the fixture's home named folder, a key standing in for Kando's official root, and turns it on, granted kando.source.fetch.
func screenshotNAS(t *testing.T, f *screenshotFixture, a *app, tt *nsgotest.App, updates chan func(), key pluginsKey, folder string) {
	t.Helper()
	root := filepath.Join(f.home, folder)
	pluginsScriptInstalled(t, root, pluginScriptFolderAbout(t, key, "acme.nas", "1.0.0", pluginNASAbout, pluginNASFields, pluginNASCode))
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	onEngine(a, a.pluginsScan)
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
	scriptsGrant(t, a, tt, updates, "acme.nas", string(plugins.PermSourceFetch))
	refresh(tt, a)
}

// screenshotNASDraft opens 设置 › 媒体来源's dialog for a new source of acme.nas' kind at https://nas.local:5006/dav, the user kando with a password typed.
func screenshotNASDraft(t *testing.T, a *app, tt *nsgotest.App) *nsgotest.App {
	t.Helper()
	a.prefs.overMain = true
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	onEngine(a, func() {
		a.AddMediaSource()
		a.SetMediaSourceKind("plugin:acme.nas:webdav")
		d := a.MediaSourceDraft()
		d.Name = "NAS"
		d.Values[mediasource.EndpointKey], d.Values["user"], d.Values["password"] = "https://nas.local:5006/dav", "kando", "password"
	})
	refresh(tt, a)
	return tt
}

// shootPluginSources renders a script plugin that adds a kind of media source (docs/media-sources.md 11): its details, with its permission to reach the servers the user fills in under the network's heading and the source among what it adds (plugins-script-source); 设置 › 媒体来源's dialog adding a source of its kind, named with the plugin, its fields the plugin's (media-sources-plugin-form); and the question whether to trust the certificate of its server, which the system does not, by its fingerprint (media-sources-plugin-trust).
func shootPluginSources(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "media-sources-plugin-form", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotNAS(t, f, a, tt, updates, key, "sources-form")
		tt = screenshotNASDraft(t, a, tt)
		if !tt.Exists("id:media-source-field-endpoint") || !hasText(tt, testlocale.Messages().SettingsSourcesKindPlugin("WebDAV", "NAS 浏览器")) {
			t.Fatalf("no plugin's form; texts %q", tt.Texts())
		}
	})
	f.shoot(t, "media-sources-plugin-trust", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotNAS(t, f, a, tt, updates, key, "sources-trust")
		tt = screenshotNASDraft(t, a, tt)
		onEngine(a, func() {
			s, _, _, _ := a.mediaSourceDraftSource()
			a.mediaSourceAskTrust("", s, &mediasource.CertError{Fingerprint: "SHA256 46:81:74:FD:18:AE:99:0A:0A:1E:10:56:8E:30:F9:81:9A:8A:CD:23:22:4C:31:9F:4E:C3:EB:4F:6F:29:80:D9"})
		})
		refresh(tt, a)
		if !tt.Exists("id:media-source-trust") {
			t.Fatalf("no question; texts %q", tt.Texts())
		}
	})
	f.shoot(t, "plugins-script-source", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotNAS(t, f, a, tt, updates, key, "sources")
		a.prefs.overMain = true
		tt = settingsOpenAt(a, tt, settings.SectionPlugins)
		tt.Press("id:settings-plugins-details-acme.nas")
		m := testlocale.Messages()
		for _, want := range []string{m.SettingsPluginsPermSourceFetch(), m.SettingsPluginsAddsSource("WebDAV")} {
			if !hasText(tt, want) && !hasText(tt, "· "+want) {
				t.Fatalf("no %q; texts %q", want, tt.Texts())
			}
		}
	})
}
