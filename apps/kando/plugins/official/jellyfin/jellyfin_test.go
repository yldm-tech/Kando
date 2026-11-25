// Package jellyfin is the source of Kando's official Jellyfin plugin (kando-plugin.json, plugin.tsx, nsgo.yaml), a kind of media source (docs/media-sources.md 11.5) which is not built into the app: .github/scripts/official-plugins.sh signs it with nsgo publish and official.Wrap makes the .kandoplugin of it. Its tests sign this folder as it is with a key they make, standing in for Kando's official root, and run it in Kando's sandbox, its sources read by Kando's own driver and its requests sent by Kando's own fetcher, against a fake Jellyfin (sourcetest.MediaServer) that takes the API key only as Jellyfin does, in an Authorization header of the MediaBrowser scheme.
package jellyfin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/official/sourcetest"
	"kando/apps/kando/plugins/script"

	"go.yaml.in/yaml/v3"
)

// key is the API key.
const key = "0123456789abcdef0123456789abcdef"

// authorized reports whether a request carries the API key as Jellyfin reads it since it turned legacy authorization off by default: Authorization: MediaBrowser Token="<key>", and neither X-Emby-Token nor an api_key in the query.
func authorized(r *http.Request) bool {
	return r.Header.Get("Authorization") == `MediaBrowser Token="`+key+`"` && r.Header.Get("X-Emby-Token") == "" && r.URL.Query().Get("api_key") == "" && r.URL.Query().Get("ApiKey") == ""
}

// open opens a Jellyfin source of the plugin at the server with apiKey.
func open(t *testing.T, p *sourcetest.Plugin, s *sourcetest.MediaServer, apiKey string) *mediasource.PluginDriver {
	t.Helper()
	return p.Open(t, "jellyfin", map[string]string{"endpoint": s.URL}, map[string]string{"apiKey": apiKey}, sourcetest.Options{})
}

func names(entries []mediasource.Entry) []string {
	var out []string
	for _, e := range entries {
		if e.Dir {
			out = append(out, e.Name+"/")
		} else {
			out = append(out, e.Name)
		}
	}
	slices.Sort(out)
	return out
}

func read(t *testing.T, d mediasource.Driver, p string, offset, length int64) string {
	t.Helper()
	r, err := d.Open(context.Background(), p, offset, length)
	if err != nil {
		t.Fatalf("open %s: %v", p, err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func code(err error) string {
	var pe *mediasource.PluginError
	if errors.As(err, &pe) {
		return pe.Code
	}
	if err == nil {
		return "nil"
	}
	return "not coded: " + err.Error()
}

// TestPluginJellyfinBrowses checks the source end to end in Kando's sandbox: the libraries that hold video as folders, music left out; a film library's films named by title, year and extension, two of the same name told apart; a TV library's series, seasons and episodes (S01E02-E03 Title); a home-video library's folders and videos, a photo left out; a video's size, time and version; finding a video, a folder, the root and nothing, again from a plugin that listed nothing yet; and reading a video's original file, static, from an offset. Every request carries the API key Kando put in, in Jellyfin's header, and the plugin's own requests are GETs with no key in them.
func TestPluginJellyfinBrowses(t *testing.T) {
	s := sourcetest.NewMediaServer(t, authorized, sourcetest.Library()...)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.jellyfin")
	d := open(t, p, s, key)
	ctx := context.Background()
	list := func(dir string) []string {
		t.Helper()
		got, err := d.List(ctx, dir)
		if err != nil {
			t.Fatalf("list %q: %v", dir, err)
		}
		return names(got)
	}
	if got := list(""); !slices.Equal(got, []string{"Films/", "Home videos/", "Shows/"}) {
		t.Errorf("the root lists %v", got)
	}
	if got := list("Films"); !slices.Equal(got, []string{"Talk (2024).mkv", "Twin (2020) (2).mp4", "Twin (2020).mp4"}) {
		t.Errorf("Films lists %v", got)
	}
	if got := list("Shows"); !slices.Equal(got, []string{"Night Day/"}) {
		t.Errorf("Shows lists %v", got)
	}
	if got := list("Shows/Night Day"); !slices.Equal(got, []string{"Season 1/", "Season 2/"}) {
		t.Errorf("the series lists %v", got)
	}
	if got := list("Shows/Night Day/Season 1"); !slices.Equal(got, []string{"S01E01 Pilot.mp4", "S01E02-E03 Two Parter.mkv"}) {
		t.Errorf("season 1 lists %v", got)
	}
	if got := list("Home videos/Trip"); !slices.Equal(got, []string{"beach.mov"}) {
		t.Errorf("Trip lists %v", got)
	}
	if _, err := d.List(ctx, "Music"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("listing music: %v", err)
	}

	if e, err := d.Stat(ctx, "Films/Talk (2024).mkv"); err != nil || e.Dir || e.Size != 20 || e.Version != "etag-m1" || e.Modified.IsZero() {
		t.Errorf("stat of a film %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, "Films/Twin (2020) (2).mp4"); err != nil || e.Size != int64(len("twin b")) {
		t.Errorf("stat of the second twin %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, "Shows/Night Day"); err != nil || !e.Dir {
		t.Errorf("stat of a series %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, ""); err != nil || !e.Dir {
		t.Errorf("stat of the root %+v, %v", e, err)
	}
	if _, err := d.Stat(ctx, "Films/None (1999).mkv"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("stat of nothing: %v", err)
	}
	if got := read(t, d, "Films/Talk (2024).mkv", 5, 4); got != "5678" {
		t.Errorf("read %q", got)
	}

	// Another source of the plugin has listed nothing: it finds its way down the paths.
	fresh := open(t, p, s, key)
	if e, err := fresh.Stat(ctx, "Shows/Night Day/Season 2/S02E01 Back.avi"); err != nil || e.Size != 4 {
		t.Errorf("stat from nothing listed %+v, %v", e, err)
	}
	if got := read(t, fresh, "Shows/Night Day/Season 1/S01E02-E03 Two Parter.mkv", 0, -1); got != "double" {
		t.Errorf("read %q", got)
	}
	if got := read(t, fresh, "Home videos/Trip/beach.mov", 0, -1); got != "waves" {
		t.Errorf("read %q", got)
	}

	for _, r := range p.Sent() {
		if r.Method != http.MethodGet || r.Login || r.Body != "" {
			t.Errorf("the plugin sent %s %s", r.Method, r.Path)
		}
		for k, v := range r.Header {
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "X-Emby-Token") || strings.Contains(v, key) {
				t.Errorf("the plugin set %s itself", k)
			}
		}
		for k, v := range r.Query {
			if strings.Contains(v, key) || strings.EqualFold(k, "api_key") {
				t.Errorf("the plugin put the key in the query")
			}
		}
	}
	for _, h := range s.Heard() {
		if strings.HasPrefix(h, "GET /Videos/") && !strings.HasSuffix(h, "?static=true") {
			t.Errorf("Kando read %s, which the server may transcode", h)
		}
	}
}

// TestPluginJellyfinPages checks a film library of more films than a page: every one listed, by pages of 500.
func TestPluginJellyfinPages(t *testing.T) {
	s := sourcetest.NewMediaServer(t, authorized, sourcetest.Many(1203)...)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.jellyfin")
	got, err := open(t, p, s, key).List(context.Background(), "Many")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1203 || got[0].Name != "Film 0000.mkv" || got[1202].Name != "Film 1202.mkv" {
		t.Errorf("listed %d", len(got))
	}
	pages := 0
	for _, h := range s.Heard() {
		if strings.HasPrefix(h, "GET /Items?") && strings.Contains(h, "ParentId=lib-many") {
			pages++
		}
	}
	if pages != 3 {
		t.Errorf("asked %d pages", pages)
	}
}

// TestPluginJellyfinFailures checks the failures the user acts on: a key the server refuses is auth, for a listing, a file found and a file read; an address that is not a media server is unavailable.
func TestPluginJellyfinFailures(t *testing.T) {
	s := sourcetest.NewMediaServer(t, authorized, sourcetest.Library()...)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.jellyfin")
	ctx := context.Background()
	wrong := open(t, p, s, "wrong")
	if _, err := wrong.List(ctx, ""); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong key lists: %v", err)
	}
	if _, err := wrong.Stat(ctx, "Films/Talk (2024).mkv"); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong key stats: %v", err)
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html>a router's page</html>")
	}))
	t.Cleanup(page.Close)
	if _, err := p.Open(t, "jellyfin", map[string]string{"endpoint": page.URL}, map[string]string{"apiKey": key}, sourcetest.Options{}).List(ctx, ""); code(err) != mediasource.PluginUnavailable {
		t.Errorf("a server that is not Jellyfin: %v", err)
	}
	if _, err := wrong.Open(ctx, "Films/Talk (2024).mkv", 0, -1); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong key reads: %v", err)
	}
	if p.Closed() {
		t.Error("the runner stopped the plugin for failures it reported")
	}
}

// TestPluginJellyfinPublishes checks that nsgo.yaml, what nsgo publish signs, is the plugin kando-plugin.json says: its app ID and version, and exactly the permissions it declares, kando.source.fetch alone, with no network rule of its own; that its manifest is valid; and that its one source sends the API key only in Jellyfin's Authorization header, and POSTs nowhere.
func TestPluginJellyfinPublishes(t *testing.T) {
	dir := sourcetest.Folder(t)
	data, err := os.ReadFile(dir + "/nsgo.yaml")
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
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&y); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dir + "/" + plugins.ManifestName)
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
	var perms []string
	for _, p := range m.Declared().Perms {
		perms = append(perms, string(p))
	}
	if !slices.Equal(y.Capabilities.Host, perms) || !slices.Equal(perms, []string{string(plugins.PermSourceFetch)}) {
		t.Errorf("nsgo.yaml signs the permissions %v, the manifest declares %v", y.Capabilities.Host, perms)
	}
	if y.Capabilities.Network != nil || len(m.Declared().Network) > 0 {
		t.Error("the plugin reaches the network on its own")
	}
	if m.Contributes == nil || len(m.Contributes.Sources) != 1 {
		t.Fatalf("the plugin adds %+v", m.Contributes)
	}
	src := m.Contributes.Sources[0]
	if src.ID != "jellyfin" || len(src.PostPaths) != 0 || !slices.Equal(src.SecretKeys(), []string{"apiKey"}) {
		t.Errorf("the source %+v", src)
	}
	for _, f := range src.Fields {
		if f.Type == plugins.MediaFieldSecret && (f.Auth == nil || f.Auth.Header != "Authorization" || f.Auth.Format != `MediaBrowser Token="{secret}"`) {
			t.Errorf("the key goes as %+v", f.Auth)
		}
		for _, lang := range []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"} {
			if f.Title[lang] == "" {
				t.Errorf("field %s has no %s title", f.Key, lang)
			}
		}
	}
}
