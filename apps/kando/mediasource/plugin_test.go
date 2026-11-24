package mediasource

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/packages/media"

	"golang.org/x/net/webdav"
)

// davSource is a WebDAV source as the official plugin declares it: its password sent as HTTP Basic with its user, and a login secret for the tests of login bodies.
var davSource = plugins.MediaSource{ID: "webdav", Title: plugins.Text{"en": "WebDAV"}, Icon: "server", PostPaths: []string{"/login"}, Fields: []plugins.MediaSourceField{
	{Key: "endpoint", Type: plugins.MediaFieldURL, Title: plugins.Text{"en": "Address"}},
	{Key: "user", Type: plugins.MediaFieldText, Title: plugins.Text{"en": "User"}},
	{Key: "password", Type: plugins.MediaFieldSecret, Title: plugins.Text{"en": "Password"}, Auth: &plugins.MediaSourceAuth{Basic: []string{"user", "password"}}},
	{Key: "pin", Type: plugins.MediaFieldSecret, Title: plugins.Text{"en": "PIN"}, Auth: &plugins.MediaSourceAuth{Login: true}},
}}

// davServer is a WebDAV server of a folder holding films/talk.mp4, under /dav/, that takes the user kando with the password secret alone, and what it heard of each request.
type davServer struct {
	*httptest.Server
	mu    sync.Mutex
	heard []*http.Request
	// ignoreRange answers a ranged GET with the whole file, as a server without Range support.
	ignoreRange bool
}

// newDAV starts a davServer, over TLS when tls is set, with the video's bytes as video.
func newDAV(t *testing.T, tls bool, video string) *davServer {
	t.Helper()
	fs := webdav.NewMemFS()
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/films", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.OpenFile(ctx, "/films/talk.mp4", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, video); err != nil {
		t.Fatal(err)
	}
	f.Close()
	s := &davServer{}
	dav := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.heard = append(s.heard, r.Clone(context.Background()))
		ignore := s.ignoreRange
		s.mu.Unlock()
		switch {
		case r.URL.Path == "/dav/elsewhere":
			http.Redirect(w, r, "/dav/films/talk.mp4", http.StatusFound)
			return
		case r.URL.Path == "/dav/login":
			body, _ := io.ReadAll(r.Body)
			fmt.Fprint(w, string(body))
			return
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "kando" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Set-Cookie", "session=1")
		if ignore {
			r.Header.Del("Range")
		}
		dav.ServeHTTP(w, r)
	})
	if tls {
		s.Server = httptest.NewTLSServer(handler)
	} else {
		s.Server = httptest.NewServer(handler)
	}
	t.Cleanup(s.Close)
	return s
}

// last is the last request the server heard.
func (s *davServer) last() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.heard[len(s.heard)-1]
}

// fetcher is a Fetcher of the WebDAV source at the server's /dav/, the user kando with password.
func (s *davServer) fetcher(t *testing.T, password string) *Fetcher {
	t.Helper()
	u, err := ParseEndpoint(s.URL + "/dav")
	if err != nil {
		t.Fatal(err)
	}
	f := &Fetcher{Endpoint: u, Source: davSource, Config: map[string]string{"user": "kando"}, Secrets: map[string]string{"password": password, "pin": "1234"}, Log: func(string, ...any) {}}
	if s.Certificate() != nil {
		f.Roots = x509.NewCertPool()
		f.Roots.AddCert(s.Certificate())
	}
	return f
}

// TestParseEndpoint checks the addresses a plugin's source may reach (docs/media-sources.md 11.3): https anywhere, http only on the local network, and nothing with a user, query or fragment.
func TestParseEndpoint(t *testing.T) {
	for raw, want := range map[string]string{
		"https://dav.example.com":          "https://dav.example.com/",
		"https://dav.example.com/remote":   "https://dav.example.com/remote/",
		"http://192.168.1.20:5005/dav":     "http://192.168.1.20:5005/dav/",
		"http://nas.local/":                "http://nas.local/",
		"http://nas.lan:8080":              "http://nas.lan:8080/",
		"http://[fd00::1]/":                "http://[fd00::1]/",
		"http://127.0.0.1:9/x":             "http://127.0.0.1:9/x/",
		"http://dav.example.com":           "",
		"http://100.64.1.2/":               "",
		"ftp://nas.local/":                 "",
		"https://user:pw@dav.example.com/": "",
		"https://dav.example.com/?a=1":     "",
		"https://dav.example.com/#a":       "",
		"dav.example.com":                  "",
		"":                                 "",
	} {
		u, err := ParseEndpoint(raw)
		got := ""
		if err == nil {
			got = u.String()
		}
		if got != want {
			t.Errorf("ParseEndpoint(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

// TestFetchListsAWebDAVFolder checks that a plugin's PROPFIND reaches the server under the endpoint, with the password put in as HTTP Basic over the plugin's own Authorization, and that the plugin is given the reply's status, a few of its headers and its body, never its cookie.
func TestFetchListsAWebDAVFolder(t *testing.T) {
	s := newDAV(t, false, "video")
	f := s.fetcher(t, "secret")
	got, err := f.Fetch(context.Background(), FetchRequest{Method: "PROPFIND", Path: "films", Header: map[string]string{"Depth": "1", "Authorization": "Basic forged"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != http.StatusMultiStatus || !strings.Contains(string(got.Body), "talk.mp4") {
		t.Fatalf("reply %d %s", got.Status, got.Body)
	}
	if _, ok := got.Header["Set-Cookie"]; ok || got.Header["Content-Type"] == "" {
		t.Errorf("headers %v", got.Header)
	}
	r := s.last()
	if user, pass, _ := r.BasicAuth(); r.URL.Path != "/dav/films" || user != "kando" || pass != "secret" || r.Header.Get("Depth") != "1" {
		t.Errorf("the server heard %s %s as %s:%s", r.Method, r.URL.Path, user, pass)
	}
	if got := PluginReply(got); got["status"] != http.StatusMultiStatus {
		t.Errorf("the plugin's reply %v", got)
	}
}

// TestFetchRefuses checks what kando.source.fetch never sends (docs/media-sources.md 11.3, 11.4): a method that writes, a POST outside the source's postPaths, a path that leaves the endpoint, a header Kando or the request decides, too many headers, too large a body.
func TestFetchRefuses(t *testing.T) {
	s := newDAV(t, false, "video")
	f := s.fetcher(t, "secret")
	many := map[string]string{}
	for i := range FetchHeadersMax + 1 {
		many[fmt.Sprintf("X-H%d", i)] = "1"
	}
	for _, tt := range []struct {
		name string
		r    FetchRequest
		want error
	}{
		{"PUT", FetchRequest{Method: "PUT", Path: "films/x"}, ErrFetchDenied},
		{"DELETE", FetchRequest{Method: "DELETE", Path: "films/talk.mp4"}, ErrFetchDenied},
		{"MOVE", FetchRequest{Method: "MOVE", Path: "films/talk.mp4"}, ErrFetchDenied},
		{"MKCOL", FetchRequest{Method: "MKCOL", Path: "new"}, ErrFetchDenied},
		{"a POST outside postPaths", FetchRequest{Method: "POST", Path: "films"}, ErrFetchDenied},
		{"a POST beside a postPath", FetchRequest{Method: "POST", Path: "logins"}, ErrFetchDenied},
		{"a path that climbs", FetchRequest{Method: "GET", Path: "../secret"}, ErrFetchDenied},
		{"a path that climbs escaped", FetchRequest{Method: "GET", Path: "films/%2e%2e/%2e%2e/x"}, ErrFetchDenied},
		{"an escaped slash", FetchRequest{Method: "GET", Path: "films%2f..%2fx"}, ErrFetchDenied},
		{"a backslash", FetchRequest{Method: "GET", Path: `films\..\x`}, ErrFetchDenied},
		{"a query in the path", FetchRequest{Method: "GET", Path: "films?x=1"}, ErrFetchDenied},
		{"a Cookie", FetchRequest{Method: "GET", Path: "films", Header: map[string]string{"Cookie": "a=1"}}, ErrFetchDenied},
		{"a Host", FetchRequest{Method: "GET", Path: "films", Header: map[string]string{"Host": "evil.example"}}, ErrFetchDenied},
		{"a Range", FetchRequest{Method: "GET", Path: "films", Header: map[string]string{"Range": "bytes=0-"}}, ErrFetchDenied},
		{"a line break", FetchRequest{Method: "GET", Path: "films", Header: map[string]string{"X-A": "1\r\nCookie: a"}}, ErrFetchDenied},
		{"17 headers", FetchRequest{Method: "GET", Path: "films", Header: many}, ErrFetchDenied},
		{"a method override", FetchRequest{Method: "POST", Path: "login", Header: map[string]string{"X-HTTP-Method-Override": "DELETE"}}, ErrFetchDenied},
		{"a method override of another name", FetchRequest{Method: "GET", Path: "films", Header: map[string]string{"x-http-method": "DELETE"}}, ErrFetchDenied},
		{"a login that is not a POST", FetchRequest{Method: "PROPFIND", Path: "films", Body: "{{secret:pin}}", Login: true}, ErrFetchDenied},
		{"a login by GET", FetchRequest{Method: "GET", Path: "login", Login: true}, ErrFetchDenied},
		{"a body over 1 MiB", FetchRequest{Method: "POST", Path: "login", Body: strings.Repeat("x", FetchBodyMax+1)}, ErrFetchTooLarge},
	} {
		n := len(s.heard)
		if _, err := f.Fetch(context.Background(), tt.r); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
		if len(s.heard) != n {
			t.Errorf("%s reached the server", tt.name)
		}
	}
}

// TestFetchLogin checks that a login request has the source's login secret put in its body, and only a login request: another sends the marker as it is.
func TestFetchLogin(t *testing.T) {
	s := newDAV(t, false, "video")
	f := s.fetcher(t, "secret")
	body := `{"user":"kando","pin":"{{secret:pin}}","password":"{{secret:password}}"}`
	got, err := f.Fetch(context.Background(), FetchRequest{Method: "POST", Path: "login", Body: body, Login: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Body) != `{"user":"kando","pin":"1234","password":"{{secret:password}}"}` {
		t.Errorf("a login sent %s", got.Body)
	}
	if got, _ = f.Fetch(context.Background(), FetchRequest{Method: "POST", Path: "login", Body: body}); string(got.Body) != body {
		t.Errorf("a request that is no login sent %s", got.Body)
	}
}

// TestFetcherOpens checks that a file is read from where open said, from an offset, with the password and a Range; that a server that ignores the Range has the bytes before the offset skipped; and that nothing there is ErrNotExist.
func TestFetcherOpens(t *testing.T) {
	s := newDAV(t, false, "0123456789")
	f := s.fetcher(t, "secret")
	read := func(t OpenTarget, offset, length int64) (string, error) {
		r, err := f.Open(context.Background(), t, offset, length)
		if err != nil {
			return "", err
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		return string(data), err
	}
	if got, err := read(OpenTarget{Path: "films/talk.mp4"}, 3, -1); err != nil || got != "3456789" {
		t.Errorf("from 3: %q, %v", got, err)
	}
	if r := s.last(); r.Header.Get("Range") != "bytes=3-" {
		t.Errorf("the Range %q", r.Header.Get("Range"))
	}
	if got, err := read(OpenTarget{URL: s.URL + "/dav/films/talk.mp4"}, 2, 4); err != nil || got != "2345" {
		t.Errorf("4 bytes from 2 by its url: %q, %v", got, err)
	}
	s.mu.Lock()
	s.ignoreRange = true
	s.mu.Unlock()
	if got, err := read(OpenTarget{Path: "films/talk.mp4"}, 6, 2); err != nil || got != "67" {
		t.Errorf("a server without ranges: %q, %v", got, err)
	}
	if _, err := read(OpenTarget{Path: "films/gone.mp4"}, 0, -1); !errors.Is(err, ErrNotExist) {
		t.Errorf("nothing there: %v", err)
	}
	var status ErrFetchStatus
	if _, err := s.fetcher(t, "wrong").Open(context.Background(), OpenTarget{Path: "films/talk.mp4"}, 0, -1); !errors.As(err, &status) || !status.Auth() {
		t.Errorf("a wrong password: %v", err)
	}
	for _, u := range []string{"http://evil.example/dav/films/talk.mp4", s.URL + "/other/talk.mp4", s.URL + "/dav/films/%2e%2e/x"} {
		if _, err := read(OpenTarget{URL: u}, 0, -1); !errors.Is(err, ErrFetchDenied) {
			t.Errorf("%s: %v", u, err)
		}
	}
}

// TestFetcherRedirects checks docs/media-sources.md 11.3's redirects as a file is read: one on the endpoint's origin keeps the password; one to another origin by https goes on without it, the plugin's headers or anything but the Range; one to http is refused.
func TestFetcherRedirects(t *testing.T) {
	s := newDAV(t, false, "0123456789")
	f := s.fetcher(t, "secret")
	r, err := f.Open(context.Background(), OpenTarget{URL: s.URL + "/dav/elsewhere"}, 1, -1)
	if err != nil {
		t.Fatalf("a redirect on the origin: %v", err)
	}
	data, _ := io.ReadAll(r)
	r.Close()
	if string(data) != "123456789" {
		t.Errorf("read %q", data)
	}
	var cdn struct {
		sync.Mutex
		header http.Header
	}
	far := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdn.Lock()
		cdn.header = r.Header.Clone()
		cdn.Unlock()
		http.ServeContent(w, r, "talk.mp4", time.Time{}, strings.NewReader("abcdef"))
	}))
	defer far.Close()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "x") }))
	defer plain.Close()
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		to := far.URL + "/talk.mp4"
		if r.URL.Query().Get("to") == "http" {
			to = plain.URL + "/talk.mp4"
		}
		http.Redirect(w, r, to, http.StatusFound)
	}))
	defer away.Close()
	u, _ := ParseEndpoint(away.URL)
	g := &Fetcher{Endpoint: u, Source: davSource, Config: map[string]string{"user": "kando"}, Secrets: map[string]string{"password": "secret"}, Roots: x509.NewCertPool()}
	g.Roots.AddCert(far.Certificate())
	r, err = g.Open(context.Background(), OpenTarget{Path: "x", Header: map[string]string{"X-Token": "plugin's"}}, 2, -1)
	if err != nil {
		t.Fatalf("a redirect to a CDN: %v", err)
	}
	data, _ = io.ReadAll(r)
	r.Close()
	cdn.Lock()
	h := cdn.header
	cdn.Unlock()
	if string(data) != "cdef" || h.Get("Authorization") != "" || h.Get("X-Token") != "" || h.Get("Range") != "bytes=2-" {
		t.Errorf("the CDN gave %q and heard %v", data, h)
	}
	if _, err := g.Open(context.Background(), OpenTarget{Path: "x", Query: map[string]string{"to": "http"}}, 0, -1); !errors.Is(err, ErrFetchDenied) {
		t.Errorf("a redirect to http: %v", err)
	}
}

// TestFetcherTrustsACertificateOnce checks docs/media-sources.md 17.5: a server whose certificate the system does not verify, as a NAS's own, is refused with its fingerprint, reached once that fingerprint is trusted, and refused again, naming both, once it offers another.
func TestFetcherTrustsACertificateOnce(t *testing.T) {
	s := newDAV(t, true, "video")
	f := s.fetcher(t, "secret")
	f.Roots = x509.NewCertPool()
	_, err := f.Fetch(context.Background(), FetchRequest{Method: "PROPFIND", Path: "", Header: map[string]string{"Depth": "0"}})
	var cert *CertError
	want := Fingerprint(s.Certificate().Raw)
	if !errors.As(err, &cert) || cert.Fingerprint != want || cert.Pinned != "" {
		t.Fatalf("a certificate not verified: %v", err)
	}
	f.Pin = cert.Fingerprint
	if got, err := f.Fetch(context.Background(), FetchRequest{Method: "PROPFIND", Path: "", Header: map[string]string{"Depth": "0"}}); err != nil || got.Status != http.StatusMultiStatus {
		t.Fatalf("once trusted: %v %v", got.Status, err)
	}
	f.Pin = "SHA256 00"
	if _, err := f.Fetch(context.Background(), FetchRequest{Method: "GET", Path: ""}); !errors.As(err, &cert) || cert.Pinned != "SHA256 00" || cert.Fingerprint != want {
		t.Errorf("another certificate: %v", err)
	}
	if !strings.HasPrefix(want, "SHA256 ") || len(strings.Split(want, ":")) != 32 {
		t.Errorf("fingerprint %q", want)
	}
}

// TestFetcherOffline checks that offline a source on the local network is reached and one on the internet is not, judged by the address dialed.
func TestFetcherOffline(t *testing.T) {
	s := newDAV(t, false, "video")
	f := s.fetcher(t, "secret")
	f.Offline = true
	if _, err := f.Fetch(context.Background(), FetchRequest{Method: "GET", Path: "films/talk.mp4"}); err != nil {
		t.Errorf("on this computer: %v", err)
	}
	u, _ := url.Parse("https://8.8.8.8:1/")
	f.Endpoint = u
	if _, err := f.Fetch(context.Background(), FetchRequest{Method: "GET", Path: ""}); !errors.Is(err, media.ErrOffline) {
		t.Errorf("on the internet: %v", err)
	}
	// A driver's fetcher outlives a change of offline mode, and reads it as each file is opened.
	offline := false
	f.Offline, f.OfflineNow = false, func() bool { return offline }
	offline = true
	if _, err := f.Open(context.Background(), OpenTarget{Path: "films/talk.mp4"}, 0, -1); !errors.Is(err, media.ErrOffline) {
		t.Errorf("a file on the internet once offline mode is on: %v", err)
	}
}

// TestPluginKindIsValid checks that a plugin's source Validate takes (plugins.checkMediaSources) is a kind the Registry takes too.
func TestPluginKindIsValid(t *testing.T) {
	k := PluginKind("acme.nas", davSource, func(context.Context, Source, map[string]string) (Driver, error) { return nil, nil })
	if err := k.Validate(); err != nil {
		t.Fatal(err)
	}
	if k.ID != "plugin:acme.nas:webdav" || k.Title("en") != "WebDAV" || k.Fields[2].Auth.Basic[0] != "user" || strings.Join(k.SecretKeys(), ",") != "password,pin" {
		t.Errorf("kind %+v", k)
	}
	arg := PluginSourceArg(k, "s_1", Source{Config: map[string]string{"endpoint": "https://x/", "user": "kando", "password": "leaked", PinKey: "SHA256 00"}})
	config := arg["config"].(map[string]any)
	if arg["id"] != "s_1" || len(config) != 2 || config["user"] != "kando" {
		t.Errorf("the plugin is given %v", arg)
	}
}

// fakeCalls stands in for a plugin's list, stat and open.
type fakeCalls struct {
	mu    sync.Mutex
	calls []string
	reply map[string]any
	err   map[string]error
}

func (c *fakeCalls) call(_ context.Context, export string, args ...any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, export+" "+fmt.Sprint(args[1:]...))
	return c.reply[export], c.err[export]
}

// TestPluginDriver checks a plugin's source as Kando reads it: the entries of its list that are of the shape asked for, a stat of nothing, and a file read from where open said, open asked again only once the server refuses it.
func TestPluginDriver(t *testing.T) {
	s := newDAV(t, false, "0123456789")
	calls := &fakeCalls{reply: map[string]any{
		plugins.MediaSourceList: []any{
			map[string]any{"name": "films", "dir": true},
			map[string]any{"name": "talk.mp4", "size": float64(10), "modified": "2026-11-20T10:00:00Z", "version": "e1"},
			map[string]any{"name": "talk.mp4", "size": float64(11)},
			map[string]any{"name": "../up"},
			map[string]any{"name": ".."},
			map[string]any{"name": "a\\b"},
			map[string]any{"name": "bad size", "size": float64(-1)},
			map[string]any{"name": "bad time", "modified": "yesterday"},
			"not an entry",
		},
		plugins.MediaSourceOpen: map[string]any{"path": "films/talk.mp4"},
	}, err: map[string]error{plugins.MediaSourceStat: &PluginError{Code: PluginNotFound}}}
	closed := 0
	d := &PluginDriver{Call: calls.call, Source: map[string]any{"id": "s_1"}, Fetcher: s.fetcher(t, "secret"), Done: func() { closed++ }}
	entries, err := d.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || !entries[0].Dir || entries[1].Size != 10 || entries[1].Version != "e1" || entries[1].Modified.Year() != 2026 {
		t.Errorf("entries %+v", entries)
	}
	if _, err := d.List(context.Background(), "../x"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("a folder above the root: %v", err)
	}
	if _, err := d.Stat(context.Background(), "gone.mp4"); !errors.Is(err, ErrNotExist) {
		t.Errorf("a stat of nothing: %v", err)
	}
	read := func() string {
		r, err := d.Open(context.Background(), "films/talk.mp4", 4, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		data, _ := io.ReadAll(r)
		return string(data)
	}
	if read() != "456789" || read() != "456789" {
		t.Error("the file read")
	}
	opens := func() int {
		calls.mu.Lock()
		defer calls.mu.Unlock()
		n := 0
		for _, c := range calls.calls {
			if strings.HasPrefix(c, plugins.MediaSourceOpen) {
				n++
			}
		}
		return n
	}
	if opens() != 1 {
		t.Errorf("open asked %d times while its answer lasts", opens())
	}
	d.Fetcher.Secrets["password"] = "changed"
	if _, err := d.Open(context.Background(), "films/talk.mp4", 0, -1); err == nil || opens() != 2 {
		t.Errorf("a refused read asks open again once: %v, %d", err, opens())
	}
	d.Close()
	d.Close()
	if closed != 1 {
		t.Errorf("Done ran %d times", closed)
	}
}

// TestPluginReplies checks the shapes of a plugin's answers Kando takes: open's, with its ttl kept to a day, and a fetch's request.
func TestPluginReplies(t *testing.T) {
	for _, tt := range []struct {
		v  any
		ok bool
	}{
		{map[string]any{"path": "a.mp4"}, true},
		{map[string]any{"url": "https://x/a.mp4", "header": map[string]any{"X": "1"}, "ttl": float64(60)}, true},
		{map[string]any{"path": "a", "url": "https://x/a"}, false},
		{map[string]any{}, false},
		{map[string]any{"path": "a", "header": map[string]any{"X": 1}}, false},
		{map[string]any{"path": "a", "ttl": float64(-1)}, false},
		{"a.mp4", false},
	} {
		if _, ok := pluginTarget(tt.v); ok != tt.ok {
			t.Errorf("open answered %v: %t", tt.v, ok)
		}
	}
	if got, _ := pluginTarget(map[string]any{"path": "a", "ttl": float64(1e9)}); got.TTL != openTTLMax {
		t.Errorf("ttl %v", got.TTL)
	}
	if got, _ := pluginTarget(map[string]any{"path": "a"}); got.TTL != openTTL {
		t.Errorf("default ttl %v", got.TTL)
	}
	source, r, ok := PluginRequest(map[string]any{"source": "s_1", "method": "PROPFIND", "path": "films", "header": map[string]any{"Depth": "1"}, "query": map[string]any{"a": "b"}, "body": "x", "login": true})
	if !ok || source != "s_1" || r.Method != "PROPFIND" || r.Header["Depth"] != "1" || r.Query["a"] != "b" || !r.Login {
		t.Errorf("request %v %+v %t", source, r, ok)
	}
	for _, bad := range []any{nil, "x", map[string]any{"method": "GET"}, map[string]any{"source": "s", "header": map[string]any{"a": 1}}} {
		if _, _, ok := PluginRequest(bad); ok {
			t.Errorf("request %v taken", bad)
		}
	}
}
