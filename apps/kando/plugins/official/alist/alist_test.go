// Package alist is the source of Kando's official Alist plugin (kando-plugin.json, plugin.tsx, nsgo.yaml), a kind of media source (docs/media-sources.md 11.5) which is not built into the app: .github/scripts/official-plugins.sh signs it with nsgo publish and official.Wrap makes the .kandoplugin of it. Its tests sign this folder as it is with a key they make, standing in for Kando's official root, and run it in Kando's sandbox, its sources read by Kando's own driver and its requests sent by Kando's own fetcher (sourcetest), against a fake Alist.
package alist

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
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
)

// password is the user kando's password: a quote, a backslash, an ampersand and a non-ASCII letter, which a login body must carry as they are.
const password = `pa"ss\wörd&1`

// file is a file of the fake Alist: its bytes, and whether a cloud drive keeps it, whose raw_url is then the drive's own link, elsewhere than the server; else a local storage's, whose raw_url is the server's /p/ link.
type file struct {
	data  string
	cloud bool
}

// alist is a fake Alist: a tree of folders and files under the user kando's base path, /home, which a guest sees too when guest is set; the login, fs/list and fs/get it is asked through, which answer as Alist does, in their body's code; /d/ and /p/, which take a file's whole path and its signature; and a cloud drive (cdn), which /d/ redirects a cloud file to.
type alist struct {
	*httptest.Server
	cdn *httptest.Server

	mu     sync.Mutex
	dirs   map[string]bool
	files  map[string]file
	guest  bool
	tokens map[string]bool
	seq    int
	// logins counts the logins, pages the fs/list pages asked; heard are the requests, by method, path and Authorization; drive the requests the drive heard, by path and Authorization.
	logins int
	pages  int
	heard  []string
	drive  []string
}

func newAlist(t *testing.T) *alist {
	t.Helper()
	a := &alist{dirs: map[string]bool{"/home": true, "/home/films": true, "/home/剧集 S1": true, "/home/many": true}, files: map[string]file{
		"/home/films/talk.mp4":   {data: "0123456789abcdefghij"},
		"/home/films/cloud.mkv":  {data: "from the drive", cloud: true},
		"/home/剧集 S1/第1集 #1.mp4": {data: "ep1"},
	}, tokens: map[string]bool{}}
	for i := range 1203 {
		a.files[fmt.Sprintf("/home/many/%04d.mp4", i)] = file{data: "x"}
	}
	a.cdn = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.drive = append(a.drive, r.URL.Path+" "+r.Header.Get("Authorization"))
		f, ok := a.files[strings.TrimPrefix(r.URL.Path, "/drive")]
		a.mu.Unlock()
		if !ok || !f.cloud {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "f", time.Time{}, strings.NewReader(f.data))
	}))
	t.Cleanup(a.cdn.Close)
	a.Server = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.Close)
	return a
}

// sign is the signature of a file's whole path.
func sign(p string) string { return "sig:" + p }

// reply writes Alist's answer: HTTP 200 with the code and the data in the body.
func reply(w http.ResponseWriter, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message, "data": data})
}

func (a *alist) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.heard = append(a.heard, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
	if p, ok := strings.CutPrefix(r.URL.Path, "/d"); ok && r.Method == http.MethodGet {
		f, ok := a.files[p]
		if !ok || r.URL.Query().Get("sign") != sign(p) {
			reply(w, 401, "sign mismatch", nil)
			return
		}
		if f.cloud {
			http.Redirect(w, r, a.cdn.URL+"/drive"+(&url.URL{Path: p}).EscapedPath(), http.StatusFound)
			return
		}
		http.ServeContent(w, r, "f", time.Time{}, strings.NewReader(f.data))
		return
	}
	if p, ok := strings.CutPrefix(r.URL.Path, "/p"); ok && r.Method == http.MethodGet {
		f, ok := a.files[p]
		if !ok || f.cloud || r.URL.Query().Get("sign") != sign(p) {
			reply(w, 401, "sign mismatch", nil)
			return
		}
		http.ServeContent(w, r, "f", time.Time{}, strings.NewReader(f.data))
		return
	}
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	if r.URL.Path == "/api/auth/login" {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			reply(w, 400, err.Error(), nil)
			return
		}
		if req.Username != "kando" || req.Password != password {
			reply(w, 400, "username or password is incorrect", nil)
			return
		}
		a.logins++
		a.seq++
		token := fmt.Sprintf("token-%d", a.seq)
		a.tokens[token] = true
		reply(w, 200, "success", map[string]string{"token": token})
		return
	}
	token := r.Header.Get("Authorization")
	switch {
	case token == "" && !a.guest:
		reply(w, 401, "Guest user is disabled, login please", nil)
		return
	case token != "" && !a.tokens[token]:
		reply(w, 401, "token is expired", nil)
		return
	}
	var req struct {
		Path    string `json:"path"`
		Page    int    `json:"page"`
		PerPage int    `json:"per_page"`
	}
	json.Unmarshal(body, &req)
	whole := path.Join("/home", req.Path)
	switch r.URL.Path {
	case "/api/fs/list":
		if !a.dirs[whole] {
			reply(w, 500, "failed get objs: failed to list objs: object not found", nil)
			return
		}
		a.pages++
		var content []map[string]any
		for _, name := range a.children(whole) {
			content = append(content, a.obj(path.Join(whole, name)))
		}
		total := len(content)
		per := min(max(req.PerPage, 1), 500)
		page := max(req.Page, 1)
		start, end := min((page-1)*per, total), min(page*per, total)
		reply(w, 200, "success", map[string]any{"content": content[start:end], "total": total, "provider": "Local"})
	case "/api/fs/get":
		if !a.dirs[whole] && a.files[whole] == (file{}) {
			reply(w, 500, "failed get obj: object not found", nil)
			return
		}
		o := a.obj(whole)
		if f, ok := a.files[whole]; ok {
			escaped := (&url.URL{Path: whole}).EscapedPath()
			if f.cloud {
				o["raw_url"] = "https://drive.example.com/link?id=" + url.QueryEscape(whole)
			} else {
				o["raw_url"] = a.URL + "/p" + escaped + "?sign=" + url.QueryEscape(sign(whole))
			}
			o["sign"] = sign(whole)
		}
		o["virtual_path"] = whole
		reply(w, 200, "success", o)
	default:
		http.NotFound(w, r)
	}
}

// children are the names in the folder dir, in order.
func (a *alist) children(dir string) []string {
	var out []string
	for d := range a.dirs {
		if path.Dir(d) == dir && d != dir {
			out = append(out, path.Base(d))
		}
	}
	for f := range a.files {
		if path.Dir(f) == dir {
			out = append(out, path.Base(f))
		}
	}
	slices.Sort(out)
	return out
}

// obj is the object Alist answers of p.
func (a *alist) obj(p string) map[string]any {
	o := map[string]any{"name": path.Base(p), "is_dir": a.dirs[p], "size": 0, "modified": "2026-11-03T10:20:30.123456789+08:00", "sign": "", "thumb": "", "type": 0}
	if f, ok := a.files[p]; ok {
		o["size"] = len(f.data)
	}
	return o
}

// expire forgets every token, as Alist does once one is older than its expiry.
func (a *alist) expire() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens = map[string]bool{}
}

// counts are how many logins and fs/list pages Alist answered.
func (a *alist) counts() (logins, pages int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.logins, a.pages
}

// requests are the requests Alist heard, and those the drive heard.
func (a *alist) requests() (server, drive []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.heard), slices.Clone(a.drive)
}

// open opens an Alist source of the plugin at the fake, signed in as user with pass, Kando trusting the drive's certificate.
func (a *alist) open(t *testing.T, p *sourcetest.Plugin, user, pass string) *mediasource.PluginDriver {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(a.cdn.Certificate())
	return p.Open(t, "alist", map[string]string{"endpoint": a.URL, "user": user}, map[string]string{"password": pass}, sourcetest.Options{Roots: roots})
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

// TestPluginAlistBrowses checks the source end to end in Kando's sandbox: it logs in once, with a password of a quote, a backslash, an ampersand and a non-ASCII letter that Kando writes into the login's body and the plugin never sees, and lists folders by the token, relative to the user's base path; a file's size, time and version; finding a file, a folder, the root and nothing; and reading a local storage's file from its raw_url on the server and a cloud drive's from the server's /d/ link, which redirects to the drive, which Kando reaches without the token. The plugin POSTs only to its three paths.
func TestPluginAlistBrowses(t *testing.T) {
	a := newAlist(t)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.alist")
	d := a.open(t, p, "kando", password)
	ctx := context.Background()

	root, err := d.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(root); !slices.Equal(got, []string{"films/", "many/", "剧集 S1/"}) {
		t.Errorf("the root lists %v", got)
	}
	films, err := d.List(ctx, "films")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(films); !slices.Equal(got, []string{"cloud.mkv", "talk.mp4"}) {
		t.Errorf("films lists %v", got)
	}
	for _, e := range films {
		if e.Name == "talk.mp4" && (e.Size != 20 || e.Version == "" || !e.Modified.Equal(time.Date(2026, 11, 3, 2, 20, 30, 123456789, time.UTC))) {
			t.Errorf("talk.mp4 %+v", e)
		}
	}
	if got, err := d.List(ctx, "剧集 S1"); err != nil || !slices.Equal(names(got), []string{"第1集 #1.mp4"}) {
		t.Errorf("剧集 S1 lists %v, %v", names(got), err)
	}
	if _, err := d.List(ctx, "nothing"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("listing nothing: %v", err)
	}
	if e, err := d.Stat(ctx, "films/talk.mp4"); err != nil || e.Name != "talk.mp4" || e.Size != 20 || e.Dir {
		t.Errorf("stat %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, "剧集 S1"); err != nil || !e.Dir || e.Name != "剧集 S1" {
		t.Errorf("stat of a folder %+v, %v", e, err)
	}
	if e, err := d.Stat(ctx, ""); err != nil || !e.Dir {
		t.Errorf("stat of the root %+v, %v", e, err)
	}
	if _, err := d.Stat(ctx, "films/none.mp4"); !errors.Is(err, mediasource.ErrNotExist) {
		t.Errorf("stat of nothing: %v", err)
	}

	if got := read(t, d, "films/talk.mp4", 5, 4); got != "5678" {
		t.Errorf("read %q of the local file", got)
	}
	if got := read(t, d, "剧集 S1/第1集 #1.mp4", 0, -1); got != "ep1" {
		t.Errorf("read %q of the file of an escaped name", got)
	}
	if got := read(t, d, "films/cloud.mkv", 5, -1); got != "the drive" {
		t.Errorf("read %q of the cloud file", got)
	}
	if _, err := d.Open(ctx, "films", 0, -1); code(err) != mediasource.PluginNotFound {
		t.Errorf("reading a folder: %v", err)
	}

	if logins, _ := a.counts(); logins != 1 {
		t.Errorf("logged in %d times", logins)
	}
	for _, r := range p.Sent() {
		if r.Method != http.MethodPost || !slices.Contains([]string{"api/auth/login", "api/fs/list", "api/fs/get"}, r.Path) {
			t.Errorf("the plugin sent %s %s", r.Method, r.Path)
		}
		if r.Login != (r.Path == "api/auth/login") {
			t.Errorf("%s login=%v", r.Path, r.Login)
		}
		if strings.Contains(r.Body, "wörd") || strings.Contains(r.Header["Authorization"], "wörd") {
			t.Errorf("the plugin had the password: %s", r.Body)
		}
		if r.Login && !strings.Contains(r.Body, "{{secret:password}}") {
			t.Errorf("the login said %s", r.Body)
		}
	}
	server, drive := a.requests()
	for _, h := range server {
		if strings.HasPrefix(h, "POST /api/fs/") && !strings.HasSuffix(h, " token-1") {
			t.Errorf("Alist heard %s", h)
		}
	}
	if len(drive) == 0 {
		t.Error("the drive heard nothing")
	}
	for _, h := range drive {
		if h != "/drive/home/films/cloud.mkv " {
			t.Errorf("the drive heard %q, with the token", h)
		}
	}
}

// TestPluginAlistPages checks a folder of more entries than Alist answers a page: every one listed, a page of 500 at a time.
func TestPluginAlistPages(t *testing.T) {
	a := newAlist(t)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.alist")
	d := a.open(t, p, "kando", password)
	got, err := d.List(context.Background(), "many")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1203 || got[0].Name != "0000.mp4" || got[1202].Name != "1202.mp4" {
		t.Errorf("listed %d", len(got))
	}
	if _, pages := a.counts(); pages != 3 {
		t.Errorf("asked %d pages", pages)
	}
}

// TestPluginAlistLogins checks the sessions: a token Alist no longer takes is replaced by logging in again, once; a guest browses with no token when Alist lets it, and is refused as auth when it does not; a password Alist refuses is auth, after one login for one listing.
func TestPluginAlistLogins(t *testing.T) {
	a := newAlist(t)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.alist")
	ctx := context.Background()
	d := a.open(t, p, "kando", password)
	if _, err := d.List(ctx, ""); err != nil {
		t.Fatal(err)
	}
	a.expire()
	if got, err := d.List(ctx, "films"); err != nil || len(got) != 2 {
		t.Fatalf("after the token expired: %v, %v", got, err)
	}
	if logins, _ := a.counts(); logins != 2 {
		t.Errorf("logged in %d times", logins)
	}

	guest := a.open(t, p, "", "")
	if _, err := guest.List(ctx, ""); code(err) != mediasource.PluginAuth {
		t.Errorf("a guest Alist refuses: %v", err)
	}
	a.mu.Lock()
	a.guest = true
	a.mu.Unlock()
	if got, err := guest.List(ctx, "films"); err != nil || len(got) != 2 {
		t.Errorf("a guest lists %v, %v", got, err)
	}

	for _, r := range p.Sent() {
		if r.Path == "api/auth/login" && strings.Contains(r.Body, `"username":""`) {
			t.Error("the guest logged in")
		}
	}
	wrong := a.open(t, p, "kando", "wrong")
	if _, err := wrong.List(ctx, ""); code(err) != mediasource.PluginAuth {
		t.Errorf("a wrong password: %v", err)
	}
	if logins, _ := a.counts(); logins != 2 {
		t.Errorf("logged in %d times", logins)
	}
	tried := 0
	for _, r := range p.Sent() {
		if r.Login {
			tried++
		}
	}
	if tried != 1 {
		t.Errorf("a wrong password tried %d logins for one listing", tried)
	}
	if p.Closed() {
		t.Error("the runner stopped the plugin for failures it reported")
	}
}

// TestPluginAlistNotAlist checks an address that is not Alist: a page that is not JSON, or 404, is unavailable.
func TestPluginAlistNotAlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			io.WriteString(w, "<html>a router's page</html>")
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	p := sourcetest.Load(t, sourcetest.Folder(t), "kando.alist")
	ctx := context.Background()
	if _, err := p.Open(t, "alist", map[string]string{"endpoint": srv.URL, "user": "kando"}, map[string]string{"password": "x"}, sourcetest.Options{}).List(ctx, ""); code(err) != mediasource.PluginUnavailable {
		t.Errorf("not JSON: %v", err)
	}
	if _, err := p.Open(t, "alist", map[string]string{"endpoint": srv.URL}, nil, sourcetest.Options{}).List(ctx, ""); code(err) != mediasource.PluginUnavailable {
		t.Errorf("404: %v", err)
	}
}

// TestPluginAlistPublishes checks that nsgo.yaml, what nsgo publish signs, is the plugin kando-plugin.json says: its app ID and version, and exactly the permissions it declares, kando.source.fetch alone, with no network rule of its own; that its manifest is valid; and that its one source sends the password only into a login's body and POSTs only to Alist's login, fs/list and fs/get.
func TestPluginAlistPublishes(t *testing.T) {
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
	if src.ID != "alist" || !slices.Equal(src.PostPaths, []string{"/api/auth/login", "/api/fs/list", "/api/fs/get"}) || !slices.Equal(src.SecretKeys(), []string{"password"}) {
		t.Errorf("the source %+v", src)
	}
	for _, f := range src.Fields {
		if f.Type == plugins.MediaFieldSecret && (f.Auth == nil || !f.Auth.Login) {
			t.Errorf("the password goes as %+v", f.Auth)
		}
		for _, lang := range []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"} {
			if f.Title[lang] == "" {
				t.Errorf("field %s has no %s title", f.Key, lang)
			}
		}
	}
}
