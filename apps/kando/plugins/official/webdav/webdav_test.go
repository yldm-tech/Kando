// Package webdav is the source of Kando's official WebDAV plugin (kando-plugin.json, plugin.tsx, nsgo.yaml), a kind of media source (docs/media-sources.md 11.5) which is not built into the app: .github/scripts/official-plugins.sh signs it with nsgo publish and official.Wrap makes the .kandoplugin of it. Its tests sign this folder as it is with a key they make, standing in for Kando's official root, and run it in Kando's sandbox, its sources read by Kando's own driver and its requests sent by Kando's own fetcher (sourcetest), against a fake WebDAV server.
package webdav

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/official/sourcetest"
	"kando/apps/kando/plugins/script"

	"go.yaml.in/yaml/v3"
	"golang.org/x/net/webdav"
)

// davServer is a WebDAV server under /dav/ that takes the user kando with the password secret alone: films/talk.mp4, a folder of a name that needs escaping, 我的 电影, holding a #1.mkv, and an empty folder; and the passwords it heard.
type davServer struct {
	*httptest.Server
	mu    sync.Mutex
	heard []string
}

const video = "0123456789abcdefghij"

func newDAV(t *testing.T) *davServer {
	t.Helper()
	fs := webdav.NewMemFS()
	ctx := context.Background()
	for _, dir := range []string{"/films", "/我的 电影", "/empty"} {
		if err := fs.Mkdir(ctx, dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{"/films/talk.mp4": video, "/我的 电影/a #1.mkv": "mkv"} {
		f, err := fs.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(f, data)
		f.Close()
	}
	s := &davServer{}
	dav := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		s.mu.Lock()
		s.heard = append(s.heard, r.Method+" "+r.URL.EscapedPath()+" "+user+":"+pass)
		s.mu.Unlock()
		if user != "kando" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		dav.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// requests are the requests the server heard, by method, path and credentials.
func (s *davServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.heard)
}

// open opens a WebDAV source of the plugin at endpoint with password.
func open(t *testing.T, p *sourcetest.Plugin, endpoint, password string) *mediasource.PluginDriver {
	t.Helper()
	return p.Open(t, "webdav", map[string]string{"endpoint": endpoint, "user": "kando"}, map[string]string{"password": password}, sourcetest.Options{})
}

// names are the entries' names, a folder's with a slash after it.
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

// wantOnlyPropfind fails t unless every request the plugin sent was a PROPFIND, with no body but the properties it asks for and no Authorization of its own.
func wantOnlyPropfind(t *testing.T, p *sourcetest.Plugin) {
	t.Helper()
	for _, r := range p.Sent() {
		if r.Method != "PROPFIND" || r.Login || !strings.Contains(r.Body, "propfind") {
			t.Errorf("the plugin sent %s %s login=%v", r.Method, r.Path, r.Login)
		}
		for k := range r.Header {
			if strings.EqualFold(k, "Authorization") {
				t.Errorf("the plugin set %s itself", k)
			}
		}
	}
}

// TestPluginWebDAVBrowses checks the source end to end in Kando's sandbox: listing the root and folders under it, their names decoded (spaces, Chinese, #), the folder's own entry left out; a file's size, time and version (its ETag); finding a file, a folder, the root and nothing; and reading a file's bytes from an offset, which Kando GETs itself with the password, while the plugin's own requests are PROPFINDs with no password in them.
func TestPluginWebDAVBrowses(t *testing.T) {
	s := newDAV(t)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.webdav")
	d := open(t, p, s.URL+"/dav/", "secret")
	ctx := context.Background()

	root, err := d.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(root); !slices.Equal(got, []string{"empty/", "films/", "我的 电影/"}) {
		t.Errorf("the root lists %v", got)
	}
	films, err := d.List(ctx, "films")
	if err != nil {
		t.Fatal(err)
	}
	if len(films) != 1 || films[0].Name != "talk.mp4" || films[0].Dir || films[0].Size != int64(len(video)) || films[0].Version == "" || time.Since(films[0].Modified) > time.Hour {
		t.Errorf("films lists %+v", films)
	}
	mine, err := d.List(ctx, "我的 电影")
	if err != nil || !slices.Equal(names(mine), []string{"a #1.mkv"}) {
		t.Errorf("我的 电影 lists %v, %v", names(mine), err)
	}
	if empty, err := d.List(ctx, "empty"); err != nil || len(empty) != 0 {
		t.Errorf("an empty folder lists %v, %v", empty, err)
	}
	if _, err := d.List(ctx, "nothing"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("listing nothing: %v", err)
	}

	e, err := d.Stat(ctx, "films/talk.mp4")
	if err != nil || e.Name != "talk.mp4" || e.Dir || e.Size != int64(len(video)) || e.Version != films[0].Version {
		t.Errorf("stat of the film %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, "我的 电影"); err != nil || e.Name != "我的 电影" || !e.Dir {
		t.Errorf("stat of a folder %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, ""); err != nil || e.Name != "dav" || !e.Dir {
		t.Errorf("stat of the root %+v, %v", e, err)
	}
	if _, err := d.Stat(ctx, "films/none.mp4"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("stat of nothing: %v", err)
	}
	wantOnlyPropfind(t, p)

	r, err := d.Open(ctx, "films/talk.mp4", 5, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != video[5:9] {
		t.Errorf("read %q", got)
	}
	r, err = d.Open(ctx, "我的 电影/a #1.mkv", 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(r)
	r.Close()
	if string(got) != "mkv" {
		t.Errorf("read %q", got)
	}
	if len(p.Sent()) != 0 {
		t.Error("the plugin sent a request to read a file: Kando reads it itself")
	}
	for _, h := range s.requests() {
		if !strings.HasSuffix(h, " kando:secret") {
			t.Errorf("the server heard %s", h)
		}
		if strings.HasPrefix(h, "GET ") && h != "GET /dav/films/talk.mp4 kando:secret" && h != "GET /dav/%E6%88%91%E7%9A%84%20%E7%94%B5%E5%BD%B1/a%20%231.mkv kando:secret" {
			t.Errorf("Kando read %s", h)
		}
	}
}

// TestPluginWebDAVServerRoot checks a source whose address is the server's root, and one under a folder named without its trailing slash: the same listing, relative to it.
func TestPluginWebDAVServerRoot(t *testing.T) {
	s := newDAV(t)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.webdav")
	ctx := context.Background()
	d := open(t, p, s.URL+"/dav/films", "secret")
	if got, err := d.List(ctx, ""); err != nil || !slices.Equal(names(got), []string{"talk.mp4"}) {
		t.Errorf("films/ lists %v, %v", names(got), err)
	}
	if e, err := d.Stat(ctx, "talk.mp4"); err != nil || e.Size != int64(len(video)) {
		t.Errorf("stat %+v, %v", e, err)
	}
}

// TestPluginWebDAVFailures checks the failures the user acts on: a password the server refuses is auth, for a listing and for a file; an address that is not a WebDAV folder, or where nothing listens, is unavailable.
func TestPluginWebDAVFailures(t *testing.T) {
	s := newDAV(t)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.webdav")
	ctx := context.Background()
	code := func(err error) string {
		var pe *mediasource.PluginError
		if errors.As(err, &pe) {
			return pe.Code
		}
		return "not coded: " + errString(err)
	}
	wrong := open(t, p, s.URL+"/dav/", "wrong")
	if _, err := wrong.List(ctx, ""); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong password lists: %v", err)
	}
	if _, err := wrong.Stat(ctx, "films/talk.mp4"); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong password stats: %v", err)
	}
	var status mediasource.ErrFetchStatus
	if _, err := wrong.Open(ctx, "films/talk.mp4", 0, -1); !errors.As(err, &status) || !status.Auth() {
		t.Errorf("a wrong password reads: %v", err)
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		io.WriteString(w, "<html>a router's page</html>")
	}))
	t.Cleanup(plain.Close)
	if _, err := open(t, p, plain.URL, "secret").List(ctx, ""); code(err) != mediasource.PluginUnavailable {
		t.Errorf("a server that is not WebDAV: %v", err)
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	addr := gone.URL
	gone.Close()
	if _, err := open(t, p, addr, "secret").List(ctx, ""); code(err) != mediasource.PluginUnavailable {
		t.Errorf("nothing listening: %v", err)
	}
	if p.Closed() {
		t.Error("the runner stopped the plugin for failures it reported")
	}
}

func errString(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

// TestPluginWebDAVMultistatus checks the multistatus of servers other than Go's: a lowercase prefix, the DAV: namespace as the default one with no prefix, hrefs as absolute URLs, an href escaped otherwise than the plugin escapes it, a propstat of 404 for what the server lacks, entities and CDATA in a name, and the folder's own entry not first; a getlastmodified that is not a date leaves the time out.
func TestPluginWebDAVMultistatus(t *testing.T) {
	answers := map[string]string{
		"/lower/": `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:s="http://sabredav.org/ns">
 <d:response><d:href>/lower/a%20film.mp4</d:href>
  <d:propstat><d:prop><d:resourcetype/><d:getcontentlength>42</d:getcontentlength><d:getlastmodified>Tue, 03 Nov 2026 10:20:30 GMT</d:getlastmodified><d:getetag>"e1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
 </d:response>
 <d:response><d:href>/lower/</d:href>
  <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  <d:propstat><d:prop><d:getcontentlength/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>
 </d:response>
 <d:response><d:href>/lower/Tom%20&amp;%20Jerry/</d:href>
  <d:propstat><d:prop><d:resourcetype><d:collection /></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
 </d:response>
 <d:response><d:href><![CDATA[/lower/x<y>.mkv]]></d:href>
  <d:propstat><d:prop><d:getcontentlength>7</d:getcontentlength><d:getlastmodified>soon</d:getlastmodified></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  <d:propstat><d:prop><d:getetag/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>
 </d:response>
</d:multistatus>`,
		"/plain/": `<?xml version="1.0"?>
<multistatus xmlns="DAV:">
 <response><href>http://nas.example:8080/plain/</href><propstat><prop><resourcetype><collection/></resourcetype></prop><status>HTTP/1.1 200 OK</status></propstat></response>
 <response><href>http://nas.example:8080/plain/%E7%89%87.mp4</href><propstat><prop><resourcetype/><getcontentlength>3</getcontentlength><getetag>W/"x"</getetag></prop><status>HTTP/1.1 200 OK</status></propstat></response>
 <response><href>/plain/(1).mp4</href><propstat><prop><getcontentlength>1</getcontentlength></prop><status>HTTP/1.1 200 OK</status></propstat></response>
</multistatus>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := answers[r.URL.Path]
		if r.Method != "PROPFIND" || !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.webdav")
	ctx := context.Background()
	lower, err := open(t, p, srv.URL+"/lower/", "").List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(lower); !slices.Equal(got, []string{"Tom & Jerry/", "a film.mp4", "x<y>.mkv"}) {
		t.Errorf("lists %v", got)
	}
	for _, e := range lower {
		switch e.Name {
		case "a film.mp4":
			if e.Size != 42 || e.Version != `"e1"` || !e.Modified.Equal(time.Date(2026, 11, 3, 10, 20, 30, 0, time.UTC)) {
				t.Errorf("a film.mp4 %+v", e)
			}
		case "x<y>.mkv":
			if e.Size != 7 || !e.Modified.IsZero() || e.Version == "" {
				t.Errorf("x<y>.mkv %+v", e)
			}
		}
	}
	plain, err := open(t, p, srv.URL+"/plain", "").List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(plain); !slices.Equal(got, []string{"(1).mp4", "片.mp4"}) {
		t.Errorf("lists %v", got)
	}
}

// TestPluginWebDAVPublishes checks that nsgo.yaml, what nsgo publish signs, is the plugin kando-plugin.json says: its app ID and version, and exactly the permissions it declares, kando.source.fetch alone, with no network rule of its own; that its manifest is valid; and that its one source sends the password only as HTTP Basic with the user and POSTs nowhere.
func TestPluginWebDAVPublishes(t *testing.T) {
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
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
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
	if src.ID != "webdav" || len(src.PostPaths) != 0 || !slices.Equal(src.SecretKeys(), []string{"password"}) {
		t.Errorf("the source %+v", src)
	}
	for _, f := range src.Fields {
		if f.Type == plugins.MediaFieldSecret && (f.Auth == nil || !slices.Equal(f.Auth.Basic, []string{"user", "password"})) {
			t.Errorf("the password goes as %+v", f.Auth)
		}
		for _, lang := range []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"} {
			if f.Title[lang] == "" {
				t.Errorf("field %s has no %s title", f.Key, lang)
			}
		}
	}
}
