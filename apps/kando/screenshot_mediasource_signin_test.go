package main

import (
	_ "embed"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/mediasource/plexlink"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// plexSceneAbout is the author, name and description of acme.plex, which stands in for the official Plex plugin in the scenes: its source is the official plugin's, as kando-plugin.json declares it.
const plexSceneAbout = `"author": "Kando", "name": {"en": "Plex", "zh-Hans": "Plex", "zh-Hant": "Plex", "ja": "Plex", "ko": "Plex"},
  "description": {"en": "Browses and imports films and episodes from a Plex Media Server.", "zh-Hans": "浏览并导入 Plex 媒体服务器上的电影和剧集。", "zh-Hant": "瀏覽並匯入 Plex 媒體伺服器上的電影和影集。", "ja": "Plex Media Server の映画とエピソードを閲覧して読み込みます。", "ko": "Plex 미디어 서버의 영화와 에피소드를 둘러보고 가져옵니다."}`

// plexSceneManifest is the official Plex plugin's manifest, built into the test so `nsgo test -target`, which runs it away from the package's folder, has it too.
//
//go:embed plugins/official/plex/kando-plugin.json
var plexSceneManifest []byte

// plexSceneFields are the capabilities and the source of the official Plex plugin's manifest (plexSceneManifest), so the scenes show its fields, titles and hints as they are.
func plexSceneFields(t *testing.T) string {
	t.Helper()
	var m struct {
		Capabilities json.RawMessage `json:"capabilities"`
		Contributes  json.RawMessage `json:"contributes"`
	}
	if err := json.Unmarshal(plexSceneManifest, &m); err != nil {
		t.Fatal(err)
	}
	return `"capabilities": ` + string(m.Capabilities) + `, "contributes": ` + string(m.Contributes)
}

// screenshotPlexDraft installs acme.plex, turned on and granted kando.source.fetch, and opens 设置 › 媒体来源's dialog for a new source of its kind.
func screenshotPlexDraft(t *testing.T, f *screenshotFixture, a *app, tt *nsgotest.App, updates chan func(), key pluginsKey, folder string) *nsgotest.App {
	t.Helper()
	root := filepath.Join(f.home, folder)
	pluginsScriptInstalled(t, root, pluginScriptFolderAbout(t, key, "acme.plex", "1.1.0", plexSceneAbout, plexSceneFields(t), pluginNASCode))
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	onEngine(a, a.pluginsScan)
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
	scriptsGrant(t, a, tt, updates, "acme.plex", string(plugins.PermSourceFetch))
	a.prefs.overMain = true
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	onEngine(a, func() {
		a.AddMediaSource()
		a.SetMediaSourceKind(mediasource.PluginKindID("acme.plex", "plex"))
	})
	refresh(tt, a)
	if !tt.Exists("id:media-source-sign-in") {
		t.Fatalf("no sign-in; texts %q", tt.Texts())
	}
	return tt
}

// shootPlexSignIn renders the dialog of a Plex source (docs/media-sources.md 11.5): before signing in, Sign In with Plex over the address and token it fills (media-sources-plex-form); and once signed in to an account of two servers, the draft filled with the one that answered, the account's servers to choose among, and the Test's result (media-sources-plex-signed-in).
func shootPlexSignIn(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "media-sources-plex-form", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotPlexDraft(t, f, a, tt, updates, key, "plex-form")
	})
	f.shoot(t, "media-sources-plex-signed-in", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		tt = screenshotPlexDraft(t, f, a, tt, updates, key, "plex-signed-in")
		onEngine(a, func() {
			l := &a.sources.signIn
			l.servers = []mediaSourceSignInServer{
				{Server: plexlink.Server{Name: "Living Room", ID: "nas", Owned: true, Token: "nas-token"}, address: "https://192-168-1-20.0123456789abcdef.plex.direct:32400", reached: true},
				{Server: plexlink.Server{Name: "Family", ID: "family", Token: "family-token"}, address: "https://203-0-113-9.fedcba9876543210.plex.direct:32400", reached: true},
			}
			l.chosen = "nas"
			d := a.MediaSourceDraft()
			d.Name = "Living Room"
			d.Values[mediasource.EndpointKey], d.Values["token"] = l.servers[0].address, l.servers[0].Token
			a.sources.tests[""] = mediaSourceTest{ok: true}
		})
		refresh(tt, a)
		if !tt.Exists("id:media-source-sign-in-server") {
			t.Fatalf("no servers to choose; texts %q", tt.Texts())
		}
	})
}
