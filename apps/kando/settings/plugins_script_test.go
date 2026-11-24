package settings

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
)

// TestPluginAddsMediaSources checks that a plugin's details and install confirmation list each kind of media source it adds, by its title in the interface's language.
func TestPluginAddsMediaSources(t *testing.T) {
	m := plugins.Manifest{Contributes: &plugins.Contributes{Sources: []plugins.MediaSource{{ID: "webdav", Title: plugins.Text{"en": "WebDAV", "zh-Hans": "WebDAV 服务器"}}}}}
	if adds := PluginAdds(testlocale.Messages(), m); !slices.Equal(adds, []string{"媒体来源：WebDAV 服务器"}) {
		t.Errorf("adds %q", adds)
	}
	testlocale.Use(t, "en")
	if adds := PluginAdds(testlocale.Messages(), m); !slices.Equal(adds, []string{"Media source: WebDAV"}) {
		t.Errorf("in English, adds %q", adds)
	}
}
