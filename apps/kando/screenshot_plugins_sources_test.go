package main

import (
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
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

// shootPluginSources renders the details of a script plugin that adds a kind of media source (docs/media-sources.md 11.1, 11.4): its permission to reach the servers the user fills in, under the network's heading, and the source among what it adds (plugins-script-source).
func shootPluginSources(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "plugins-script-source", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		root := filepath.Join(f.home, "sources")
		pluginsScriptInstalled(t, root, pluginScriptFolderAbout(t, key, "acme.nas", "1.0.0", pluginNASAbout, pluginNASFields, pluginNASCode))
		a.plugins.root = root
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		onEngine(a, a.pluginsScan)
		scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
		scriptsGrant(t, a, tt, updates, "acme.nas", string(plugins.PermSourceFetch))
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
