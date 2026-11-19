// Package deepl is the source of Kando's official DeepL plugin (kando-plugin.json, plugin.tsx, nsgo.yaml), a translation source (docs/plugin-system.md E14) which is not built into the app: .github/scripts/official-plugins.sh signs it with nsgo publish and official.Wrap makes the .kandoplugin of it. The app's own tests (apps/kando's TestOfficialDeepL) sign and wrap this folder as a release does and run it against a fake DeepL; this one checks what nsgo publish signs of it.
package deepl

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"

	"go.yaml.in/yaml/v3"
)

// source is the plugin's folder, the one this file is in, found from the file's own path rather than the working directory, which nsgo test -target runs the test binary elsewhere than.
func source(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no path of the test's source")
	}
	return filepath.Dir(file)
}

// TestPluginDeepLPublishes checks that nsgo.yaml, what nsgo publish signs, is the plugin kando-plugin.json says: its app ID and version, and exactly the permissions it declares, kando.net.credentialFetch alone, with no network rule of its own, as the key goes to DeepL only through Kando (its credential's hosts, methods and paths); and that its manifest is valid and its credential sends the key only to DeepL's two API hosts.
func TestPluginDeepLPublishes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(source(t), "nsgo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var y struct {
		Name         string `yaml:"name"`
		ID           string `yaml:"id"`
		Version      string `yaml:"version"`
		Capabilities struct {
			Host    []string `yaml:"host"`
			Network *struct {
				HTTP []map[string]any `yaml:"http"`
			} `yaml:"network"`
		} `yaml:"capabilities"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&y); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(source(t), plugins.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	m, err := plugins.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if y.Name == "" || y.ID != script.AppIDPrefix+m.ID || y.Version != m.Version {
		t.Errorf("nsgo.yaml is %q %s %s, the manifest %s %s", y.Name, y.ID, y.Version, m.ID, m.Version)
	}
	d := m.Declared()
	var perms []string
	for _, p := range d.Perms {
		perms = append(perms, string(p))
	}
	if !slices.Equal(y.Capabilities.Host, perms) || !slices.Equal(perms, []string{string(plugins.PermCredFetch)}) {
		t.Errorf("nsgo.yaml signs the permissions %v, the manifest declares %v", y.Capabilities.Host, perms)
	}
	if y.Capabilities.Network != nil || len(d.Network) > 0 {
		t.Errorf("the plugin reaches the network on its own: nsgo.yaml %v, the manifest %v", y.Capabilities.Network, d.Network)
	}
	if len(m.Credentials) != 1 || !slices.Equal(m.Credentials[0].Hosts, []string{"api-free.deepl.com", "api.deepl.com"}) || m.Credentials[0].Mismatched() != "" {
		t.Errorf("the key goes to %+v", m.Credentials)
	}
	if m.Contributes == nil || len(m.Contributes.Translators) != 1 || m.Contributes.Translators[0].ID != "deepl" {
		t.Errorf("the plugin adds %+v", m.Contributes)
	}
}
