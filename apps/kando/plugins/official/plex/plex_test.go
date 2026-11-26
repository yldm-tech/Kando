// Package plex is the source of Kando's official Plex plugin (kando-plugin.json, plugin.tsx, nsgo.yaml), a kind of media source (docs/media-sources.md 11.5) which is not built into the app: .github/scripts/official-plugins.sh signs it with nsgo publish and official.Wrap makes the .kandoplugin of it. Its tests sign this folder as it is with a key they make, standing in for Kando's official root, and run it in Kando's sandbox, its sources read by Kando's own driver and its requests sent by Kando's own fetcher, against a fake Plex Media Server (sourcetest.PlexServer) that takes the token only in the X-Plex-Token header.
package plex

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

// key is the token.
const key = "AbCdEf0123456789xyzW"

// authorized reports whether a request carries the token in the X-Plex-Token header, and not in the query, where Plex takes it too but where it would be written in logs.
func authorized(r *http.Request) bool {
	return r.Header.Get("X-Plex-Token") == key && r.URL.Query().Get("X-Plex-Token") == ""
}

// open opens a Plex source of the plugin at the server with token.
func open(t *testing.T, p *sourcetest.Plugin, s *sourcetest.PlexServer, token string) *mediasource.PluginDriver {
	t.Helper()
	return p.Open(t, "plex", map[string]string{"endpoint": s.URL}, map[string]string{"token": token}, sourcetest.Options{})
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

// TestPluginPlexBrowses checks the source end to end in Kando's sandbox: the film and TV libraries and an other-video library as folders, music and photos left out; a film library's films named by title, year and extension, two of the same name told apart, a film of two files as two parts, an extension from the part's container when its file has none; a TV library's shows, seasons and episodes (S01E02 Title), and a show that hides its one season listing its episodes; a video's size, time and version, in the listing already; finding a video, a folder, the root and nothing, again from a plugin that listed nothing yet; and reading a video's part, its original file, from an offset. Every request carries the token Kando put in, in the X-Plex-Token header, and the plugin's own requests are GETs with no token in them.
func TestPluginPlexBrowses(t *testing.T) {
	sections, items := sourcetest.PlexLibrary()
	s := sourcetest.NewPlexServer(t, authorized, sections, items)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.plex")
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
	if got := list("Films"); !slices.Equal(got, []string{"Long Film (2019) - pt1.avi", "Long Film (2019) - pt2.avi", "Raw (2001).mp4", "Talk (2024).mkv", "Twin (2020) (2).mp4", "Twin (2020).mp4"}) {
		t.Errorf("Films lists %v", got)
	}
	if got := list("Shows"); !slices.Equal(got, []string{"Night Day/", "Solo/"}) {
		t.Errorf("Shows lists %v", got)
	}
	if got := list("Shows/Night Day"); !slices.Equal(got, []string{"Season 1/", "Season 2/"}) {
		t.Errorf("the show lists %v", got)
	}
	if got := list("Shows/Night Day/Season 1"); !slices.Equal(got, []string{"S01E01 Pilot.mp4", "S01E02 Second.mkv"}) {
		t.Errorf("season 1 lists %v", got)
	}
	if got := list("Shows/Solo"); !slices.Equal(got, []string{"S01E01 Only.mkv"}) {
		t.Errorf("a show of one hidden season lists %v", got)
	}
	if got := list("Home videos"); !slices.Equal(got, []string{"beach.mov"}) {
		t.Errorf("Home videos lists %v", got)
	}
	if _, err := d.List(ctx, "Music"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("listing music: %v", err)
	}
	films, err := d.List(ctx, "Films")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range films {
		if e.Size == 0 || e.Version == "" || e.Modified.IsZero() {
			t.Errorf("Films lists %+v", e)
		}
	}

	if e, err := d.Stat(ctx, "Films/Talk (2024).mkv"); err != nil || e.Dir || e.Size != 20 || !strings.HasPrefix(e.Version, "/library/parts/1010/") || e.Modified.Unix() != 1762165230 {
		t.Errorf("stat of a film %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, "Films/Twin (2020) (2).mp4"); err != nil || e.Size != int64(len("twin b")) {
		t.Errorf("stat of the second twin %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, "Shows/Night Day"); err != nil || !e.Dir {
		t.Errorf("stat of a show %+v, %v", e, err)
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
	if got := read(t, d, "Films/Long Film (2019) - pt2.avi", 0, -1); got != "second" {
		t.Errorf("read the second part %q", got)
	}

	// Another source of the plugin has listed nothing: it finds its way down the paths.
	fresh := open(t, p, s, key)
	if e, err := fresh.Stat(ctx, "Shows/Night Day/Season 2/S02E01 Back.avi"); err != nil || e.Size != 4 {
		t.Errorf("stat from nothing listed %+v, %v", e, err)
	}
	if got := read(t, fresh, "Shows/Solo/S01E01 Only.mkv", 0, -1); got != "only" {
		t.Errorf("read %q", got)
	}
	if got := read(t, fresh, "Home videos/beach.mov", 0, -1); got != "waves" {
		t.Errorf("read %q", got)
	}

	for _, r := range p.Sent() {
		if r.Method != http.MethodGet || r.Login || r.Body != "" {
			t.Errorf("the plugin sent %s %s", r.Method, r.Path)
		}
		for k, v := range r.Header {
			if strings.EqualFold(k, "X-Plex-Token") || strings.Contains(v, key) {
				t.Errorf("the plugin set %s itself", k)
			}
		}
		for k, v := range r.Query {
			if strings.Contains(v, key) || strings.EqualFold(k, "X-Plex-Token") {
				t.Errorf("the plugin put the token in the query")
			}
		}
	}
	for _, h := range s.Heard() {
		if strings.Contains(h, "transcode") {
			t.Errorf("Kando asked for %s, which the server transcodes", h)
		}
	}
}

// TestPluginPlexPages checks a film library of more films than a page: every one listed, by pages of 300.
func TestPluginPlexPages(t *testing.T) {
	sections, items := sourcetest.PlexMany(1203)
	s := sourcetest.NewPlexServer(t, authorized, sections, items)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.plex")
	got, err := open(t, p, s, key).List(context.Background(), "Many")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1203 || got[0].Name != "Film 0000.mkv" || got[1202].Name != "Film 1202.mkv" {
		t.Errorf("listed %d", len(got))
	}
	pages := 0
	for _, h := range s.Heard() {
		if strings.HasPrefix(h, "GET /library/sections/9/all?") {
			pages++
		}
	}
	if pages != 5 {
		t.Errorf("asked %d pages", pages)
	}
}

// TestPluginPlexFailures checks the failures the user acts on: a token the server refuses is auth, for a listing, a file found and a file read; an address that is not a Plex server, a page or JSON of another server, is unavailable.
func TestPluginPlexFailures(t *testing.T) {
	sections, items := sourcetest.PlexLibrary()
	s := sourcetest.NewPlexServer(t, authorized, sections, items)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.plex")
	ctx := context.Background()
	wrong := open(t, p, s, "wrong")
	if _, err := wrong.List(ctx, ""); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong token lists: %v", err)
	}
	if _, err := wrong.Stat(ctx, "Films/Talk (2024).mkv"); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong token stats: %v", err)
	}
	if _, err := wrong.Open(ctx, "Films/Talk (2024).mkv", 0, -1); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong token reads: %v", err)
	}
	for _, body := range []string{"<html>a router's page</html>", `{"Items": []}`} {
		page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, body)
		}))
		t.Cleanup(page.Close)
		if _, err := p.Open(t, "plex", map[string]string{"endpoint": page.URL}, map[string]string{"token": key}, sourcetest.Options{}).List(ctx, ""); code(err) != mediasource.PluginUnavailable {
			t.Errorf("a server that answers %s: %v", body, err)
		}
	}
	if p.Closed() {
		t.Error("the runner stopped the plugin for failures it reported")
	}
}

// TestPluginPlexPublishes checks that nsgo.yaml, what nsgo publish signs, is the plugin kando-plugin.json says: its app ID and version, and exactly the permissions it declares, kando.source.fetch alone, with no network rule of its own; that its manifest is valid; and that its one source sends the token only in Plex's X-Plex-Token header, which Kando fills by signing in to Plex, and POSTs nowhere.
func TestPluginPlexPublishes(t *testing.T) {
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
	if src.ID != "plex" || len(src.PostPaths) != 0 || !slices.Equal(src.SecretKeys(), []string{"token"}) {
		t.Errorf("the source %+v", src)
	}
	for _, f := range src.Fields {
		if f.Type == plugins.MediaFieldSecret && (f.Auth == nil || f.Auth.Header != "X-Plex-Token" || f.Auth.Format != "{secret}" || f.Auth.SignIn != "plex") {
			t.Errorf("the key goes as %+v", f.Auth)
		}
		for _, lang := range []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"} {
			if f.Title[lang] == "" {
				t.Errorf("field %s has no %s title", f.Key, lang)
			}
		}
	}
}
