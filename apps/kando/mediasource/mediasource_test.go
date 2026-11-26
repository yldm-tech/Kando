package mediasource

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDriver is a Driver over nothing: a kind's Open in the tests gives it.
type fakeDriver struct{}

func (fakeDriver) List(context.Context, string) ([]Entry, error) { return nil, nil }
func (fakeDriver) Stat(context.Context, string) (Entry, error)   { return Entry{}, ErrNotExist }
func (fakeDriver) Close() error                                  { return nil }
func (fakeDriver) Open(context.Context, string, int64, int64) (io.ReadCloser, error) {
	return nil, ErrNotExist
}

func title(s string) func(string) string { return func(string) string { return s } }

func open(context.Context, Source, map[string]string) (Driver, error) { return fakeDriver{}, nil }

// smbKind is a built-in kind as P1 will register SMB.
func smbKind() Kind {
	return Kind{ID: KindSMB, Title: title("SMB"), Icon: "server", Phone: true, Open: open, Fields: []Field{
		{Key: "host", Type: FieldText, Title: title("Host")},
		{Key: "port", Type: FieldNumber, Title: title("Port"), Default: "445"},
		{Key: "share", Type: FieldText, Title: title("Share")},
		{Key: "user", Type: FieldText, Title: title("User")},
		{Key: "password", Type: FieldSecret, Title: title("Password")},
	}}
}

// webdavKind is a plugin's kind as the official WebDAV plugin will contribute it.
func webdavKind() Kind {
	return Kind{ID: PluginKindID("kando.webdav", "webdav"), Title: title("WebDAV"), Icon: "cloud", Origin: Origin{Plugin: "kando.webdav"}, Open: open, Fields: []Field{
		{Key: EndpointKey, Type: FieldURL, Title: title("Server address")},
		{Key: "user", Type: FieldText, Title: title("User")},
		{Key: "password", Type: FieldSecret, Title: title("Password"), Auth: &Auth{Basic: []string{"user", "password"}}},
	}}
}

func TestNewIDHasTheShapeOfAnID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := NewID()
		if !ValidID(id) || seen[id] {
			t.Fatalf("NewID gave %q", id)
		}
		seen[id] = true
	}
	for _, id := range []string{"", "0123456789abcde", "0123456789abcdef0", "0123456789ABCDEF", "0123456789abcdeg", "0123456789abcd/f"} {
		if ValidID(id) {
			t.Errorf("ValidID(%q)", id)
		}
	}
}

// TestRegistryListsBuiltinKindsFirst registers kinds, refuses one of an ID it holds and an invalid one, lists the built-in kinds before the plugins' in the order each came, finds a kind by ID, and takes a kind or a plugin's kinds off.
func TestRegistryListsBuiltinKindsFirst(t *testing.T) {
	var r Registry
	sftp := smbKind()
	sftp.ID = KindSFTP
	alist := webdavKind()
	alist.ID, alist.Origin.Plugin = PluginKindID("kando.alist", "alist"), "kando.alist"
	for _, k := range []Kind{webdavKind(), smbKind(), alist, sftp} {
		if err := r.Register(k); err != nil {
			t.Fatalf("Register(%s): %v", k.ID, err)
		}
	}
	if err := r.Register(smbKind()); !errors.Is(err, ErrKindExists) {
		t.Fatalf("registering smb again = %v", err)
	}
	bad := smbKind()
	bad.ID = "Bad"
	if err := r.Register(bad); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("registering an invalid kind = %v", err)
	}
	ids := func() string {
		var out []string
		for _, k := range r.Kinds() {
			out = append(out, k.ID)
		}
		return strings.Join(out, " ")
	}
	if got := ids(); got != "smb sftp plugin:kando.webdav:webdav plugin:kando.alist:alist" {
		t.Fatalf("Kinds() = %s", got)
	}
	if k, ok := r.Kind(KindSFTP); !ok || k.ID != KindSFTP || k.Title("en") != "SMB" {
		t.Fatalf("Kind(sftp) = %+v, %v", k, ok)
	}
	if _, ok := r.Kind("nfs"); ok {
		t.Fatal("Kind(nfs) found one")
	}
	if !r.Unregister(KindSFTP) || r.Unregister(KindSFTP) {
		t.Fatal("Unregister(sftp) twice")
	}
	if n := r.UnregisterPlugin("kando.webdav"); n != 1 || r.UnregisterPlugin("") != 0 {
		t.Fatalf("UnregisterPlugin took %d", n)
	}
	if got := ids(); got != "smb plugin:kando.alist:alist" {
		t.Fatalf("after taking kinds off: %s", got)
	}
	// What a caller does to the fields it registered leaves the registered kind as it was.
	k := webdavKind()
	k.ID, k.Origin.Plugin = PluginKindID("p", "s"), "p"
	_ = r.Register(k)
	k.Fields[0].Key = "changed"
	if got, _ := r.Kind(k.ID); got.Fields[0].Key != EndpointKey {
		t.Fatalf("the registered kind's fields changed: %+v", got.Fields)
	}
}

// TestKindValidateRefuses refuses the kinds docs/media-sources.md 3.2 and 11.1 rule out.
func TestKindValidateRefuses(t *testing.T) {
	cases := map[string]func(k *Kind){
		"a built-in kind with a plugin's ID":   func(k *Kind) { k.ID = "plugin:a:b" },
		"a built-in kind with an endpoint":     func(k *Kind) { k.Fields = append(k.Fields, Field{Key: EndpointKey, Type: FieldURL, Title: title("x")}) },
		"a built-in secret with an Auth":       func(k *Kind) { k.Fields[4].Auth = &Auth{Login: true} },
		"no title":                             func(k *Kind) { k.Title = nil },
		"no Open":                              func(k *Kind) { k.Open = nil },
		"an icon Kando does not draw":          func(k *Kind) { k.Icon = "https://example.com/i.png" },
		"a field key with a dot":               func(k *Kind) { k.Fields[0].Key = "a.b" },
		"a field key in capitals":              func(k *Kind) { k.Fields[0].Key = "Host" },
		"a field twice":                        func(k *Kind) { k.Fields[1].Key = "host" },
		"a field without a title":              func(k *Kind) { k.Fields[0].Title = nil },
		"a field of an unknown type":           func(k *Kind) { k.Fields[0].Type = "file" },
		"a url field not keyed endpoint":       func(k *Kind) { k.Fields[0].Type = FieldURL },
		"an Auth on a field that is no secret": func(k *Kind) { k.Fields[0].Auth = &Auth{Login: true} },
		"a number of several lines":            func(k *Kind) { k.Fields[1].Multiline = true },
		"two host keys": func(k *Kind) {
			k.Fields = append(k.Fields, Field{Key: "a", Type: FieldHostKey, Title: title("x")}, Field{Key: "b", Type: FieldHostKey, Title: title("x")})
		},
		"13 fields": func(k *Kind) {
			for i := range 8 {
				k.Fields = append(k.Fields, Field{Key: "f" + string(rune('a'+i)), Type: FieldText, Title: title("x")})
			}
		},
		"4 secrets": func(k *Kind) {
			for _, key := range []string{"a", "b", "c"} {
				k.Fields = append(k.Fields, Field{Key: key, Type: FieldSecret, Title: title("x")})
			}
		},
	}
	for name, change := range cases {
		k := smbKind()
		change(&k)
		if err := k.Validate(); !errors.Is(err, ErrInvalidKind) {
			t.Errorf("%s: Validate() = %v", name, err)
		}
	}
	plugin := map[string]func(k *Kind){
		"an ID of another plugin":               func(k *Kind) { k.ID = PluginKindID("kando.other", "webdav") },
		"a plugin's ID that is no plugin's":     func(k *Kind) { k.Origin.Plugin = "Kando WebDAV"; k.ID = PluginKindID("Kando WebDAV", "webdav") },
		"a source ID in capitals":               func(k *Kind) { k.ID = PluginKindID("kando.webdav", "WebDAV") },
		"a plugin's kind a phone reaches":       func(k *Kind) { k.Phone = true },
		"no endpoint":                           func(k *Kind) { k.Fields = k.Fields[1:] },
		"a secret that says not how it is sent": func(k *Kind) { k.Fields[2].Auth = nil },
		"basic with a user field that is no text": func(k *Kind) {
			k.Fields[2].Auth = &Auth{Basic: []string{"password", "password"}}
		},
		"basic naming another secret": func(k *Kind) { k.Fields[2].Auth = &Auth{Basic: []string{"user", "token"}} },
		"two ways":                    func(k *Kind) { k.Fields[2].Auth = &Auth{Basic: []string{"user", "password"}, Login: true} },
		"a header Kando decides":      func(k *Kind) { k.Fields[2].Auth = &Auth{Header: "Cookie"} },
		"a header that is no header":  func(k *Kind) { k.Fields[2].Auth = &Auth{Header: "X Token"} },
		"a format without {secret}":   func(k *Kind) { k.Fields[2].Auth = &Auth{Header: "X-Emby-Token", Format: "token"} },
		"a format without a header":   func(k *Kind) { k.Fields[2].Auth = &Auth{Format: "{secret}", Login: true} },
		"a sign-in Kando has not":     func(k *Kind) { k.Fields[2].Auth = &Auth{Header: "X-Token", SignIn: "jellyfin"} },
		"a sign-in not in a header":   func(k *Kind) { k.Fields[2].Auth = &Auth{Login: true, SignIn: SignInPlex} },
		"a host key": func(k *Kind) {
			k.Fields = append(k.Fields, Field{Key: "hostKey", Type: FieldHostKey, Title: title("x")})
		},
	}
	for name, change := range plugin {
		k := webdavKind()
		change(&k)
		if err := k.Validate(); !errors.Is(err, ErrInvalidKind) {
			t.Errorf("%s: Validate() = %v", name, err)
		}
	}
	for _, auth := range []*Auth{{Header: "X-Emby-Token"}, {Header: "Authorization", Format: "MediaBrowser Token=\"{secret}\""}, {Login: true}, {Header: "X-Plex-Token", SignIn: SignInPlex}} {
		k := webdavKind()
		k.Fields[2].Auth = auth
		if err := k.Validate(); err != nil {
			t.Errorf("Auth %+v: %v", auth, err)
		}
	}
	k := webdavKind()
	if account, key := k.SignIn(); account != "" || key != "" {
		t.Errorf("a kind without a sign-in: SignIn() = %q %q", account, key)
	}
	k.Fields[2].Auth = &Auth{Header: "X-Plex-Token", SignIn: SignInPlex}
	if account, key := k.SignIn(); account != SignInPlex || key != k.Fields[2].Key {
		t.Errorf("SignIn() = %q %q", account, key)
	}
	if keys := smbKind().SecretKeys(); len(keys) != 1 || keys[0] != "password" {
		t.Fatalf("SecretKeys() = %v", keys)
	}
}

// TestRefRoundTrips writes a video's address on a source and reads it back, whatever its names hold: spaces, Chinese, %, #, ? and the like.
func TestRefRoundTrips(t *testing.T) {
	for _, p := range []string{"a.mp4", "Movies/Heat (1995)/Heat.mkv", "电影/热天午后 1975.mp4", "100% real/#1?.mp4", "a+b;c,d=e&f/@~!$'.avi", "  spaced  /x.mp4", "%2F.mp4"} {
		r := Ref{Source: NewID(), Path: p}
		s := r.String()
		if !strings.HasPrefix(s, "kando-source://"+r.Source+"/") || strings.ContainsAny(strings.TrimPrefix(s, "kando-source://"), " #?") {
			t.Errorf("String() = %q", s)
		}
		got, err := ParseRef(s)
		if err != nil || got != r {
			t.Errorf("ParseRef(%q) = %+v, %v; want %+v", s, got, err, r)
		}
	}
	if got, err := ParseRef("KANDO-SOURCE://0123456789abcdef/a.mp4"); err != nil || got.Path != "a.mp4" {
		t.Fatalf("a scheme in capitals: %+v, %v", got, err)
	}
}

// TestParseRefRefuses refuses what is not a file's address under a source's root.
func TestParseRefRefuses(t *testing.T) {
	const id = "0123456789abcdef"
	invalidPath := []string{
		"kando-source://" + id + "/",
		"kando-source://" + id + "/../a.mp4",
		"kando-source://" + id + "/a/../../b.mp4",
		"kando-source://" + id + "/a/./b.mp4",
		"kando-source://" + id + "//a.mp4",
		"kando-source://" + id + "/a%2Fb.mp4",
		"kando-source://" + id + "/a%5Cb.mp4",
		"kando-source://" + id + "/a%00.mp4",
		"kando-source://" + id + "/%FF.mp4",
		"kando-source://" + id + "/a/",
	}
	for _, raw := range invalidPath {
		if r, err := ParseRef(raw); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("ParseRef(%q) = %+v, %v; want ErrInvalidPath", raw, r, err)
		}
	}
	for _, raw := range []string{
		"", "a.mp4", "file:///a.mp4", "https://" + id + "/a.mp4", "kando-source:" + id + "/a.mp4",
		"kando-source:///a.mp4", "kando-source://" + id, "kando-source://0123456789ABCDEF/a.mp4", "kando-source://nas.local/a.mp4",
		"kando-source://" + id + ":445/a.mp4", "kando-source://me@" + id + "/a.mp4",
		"kando-source://" + id + "/a.mp4?x=1", "kando-source://" + id + "/a.mp4?", "kando-source://" + id + "/a.mp4#t=1", "kando-source://" + id + "/a.mp4#",
	} {
		if r, err := ParseRef(raw); err == nil {
			t.Errorf("ParseRef(%q) = %+v", raw, r)
		}
	}
}

func TestCleanPath(t *testing.T) {
	for p, want := range map[string]string{"": "", ".": "", "a": "a", "a/./b/../c": "a/c", "a//b/": "a/b", "a/..": ""} {
		if got, err := CleanPath(p); err != nil || got != want {
			t.Errorf("CleanPath(%q) = %q, %v; want %q", p, got, err, want)
		}
	}
	for _, p := range []string{"/a", "..", "../a", "a/../..", `a\b`, "a\x00", "\xff"} {
		if got, err := CleanPath(p); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("CleanPath(%q) = %q, %v", p, got, err)
		}
	}
}

// TestLocalByAddress judges a host by its address: loopback, RFC 1918, ULA and link-local are local; public addresses and 100.64.0.0/10 are not; a name by every address it resolves to, not by how it reads.
func TestLocalByAddress(t *testing.T) {
	names := map[string][]string{
		"nas.local":       {"192.168.1.10"},
		"nas.lan":         {"10.0.0.5", "fd12:3456:789a::5"},
		"evil.local":      {"203.0.113.7"},
		"split.lan":       {"192.168.1.10", "8.8.8.8"},
		"localhost":       {"127.0.0.1", "::1"},
		"tailnet.ts.net":  {"100.101.102.103"},
		"nowhere.example": nil,
	}
	resolve := func(_ context.Context, host string) ([]netip.Addr, error) {
		addrs, ok := names[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		var out []netip.Addr
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out, nil
	}
	cases := map[string]bool{
		"127.0.0.1": true, "::1": true, "[::1]": true, "10.1.2.3": true, "172.16.0.1": true, "172.31.255.255": true, "192.168.0.1": true,
		"fc00::1": true, "fd00::1": true, "169.254.1.1": true, "fe80::1": true, "fe80::1%en0": true, "::ffff:192.168.1.1": true,
		"172.32.0.1": false, "8.8.8.8": false, "2001:db8::1": false, "100.64.0.1": false, "100.127.255.254": false, "0.0.0.0": false, "::": false,
		"nas.local": true, "nas.lan": true, "nas.local.": true, "localhost": true,
		"evil.local": false, "split.lan": false, "tailnet.ts.net": false,
	}
	for host, want := range cases {
		got, err := LocalBy(t.Context(), host, resolve)
		if err != nil || got != want {
			t.Errorf("LocalBy(%q) = %v, %v; want %v", host, got, err, want)
		}
	}
	for _, host := range []string{"", "[]", "unknown.example", "nowhere.example"} {
		if got, err := LocalBy(t.Context(), host, resolve); got || err == nil {
			t.Errorf("LocalBy(%q) = %v, %v; want an error", host, got, err)
		}
	}
	// An address is never looked up.
	if got, err := LocalBy(t.Context(), "192.168.1.1", func(context.Context, string) ([]netip.Addr, error) {
		t.Fatal("looked up an address")
		return nil, nil
	}); !got || err != nil {
		t.Fatalf("an address: %v, %v", got, err)
	}
	if got, err := Local(t.Context(), "127.0.0.1"); !got || err != nil {
		t.Fatalf("Local(127.0.0.1) = %v, %v", got, err)
	}
}

// TestPlaceholderStaysInTheLibrary puts an indexed film's placeholder in the library under the source's name, every name made one a file can have on any system, a number before the extension when the path is taken, and never outside the library.
func TestPlaceholderStaysInTheLibrary(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "Library")
	cases := []struct{ name, path, want string }{
		{"客厅 NAS", "Movies/Heat (1995)/Heat.mkv", "客厅 NAS/Movies/Heat (1995)/Heat.mkv"},
		{"nas: home/films", `a/b?c*"d".mp4`, "nas_ home_films/a/b_c__d_.mp4"},
		{"..", "x.mp4", "_/x.mp4"},
		{"NAS.", "CON/aux.mkv", "NAS/_CON/_aux.mkv"},
		{"nas", "com1.txt/LPT9/COM10.mp4", "nas/_com1.txt/_LPT9/COM10.mp4"},
		{"", "a/trailing dot. /b.mp4", "_/a/trailing dot/b.mp4"},
		{"nas", "tab\there/bell\a.mp4", "nas/tab_here/bell_.mp4"},
		{"nas", "a/./b/../c.mp4", "nas/a/c.mp4"},
	}
	for _, c := range cases {
		got, err := Placeholder(lib, c.name, c.path, nil)
		want := filepath.Join(lib, filepath.FromSlash(c.want))
		if err != nil || got != want {
			t.Errorf("Placeholder(%q, %q) = %q, %v; want %q", c.name, c.path, got, err, want)
			continue
		}
		if rel, err := filepath.Rel(lib, got); err != nil || !filepath.IsLocal(rel) {
			t.Errorf("%q is outside the library", got)
		}
	}
	long := strings.Repeat("长", 200)
	if got, err := Placeholder(lib, long, "a.mp4", nil); err != nil || got != filepath.Join(lib, strings.Repeat("长", sourceNameMax), "a.mp4") {
		t.Errorf("a long name: %q, %v", got, err)
	}

	taken := map[string]bool{
		filepath.Join(lib, "nas", "Heat.mkv"):     true,
		filepath.Join(lib, "nas", "Heat (2).mkv"): true,
		filepath.Join(lib, "nas", "README"):       true,
	}
	isTaken := func(p string) bool { return taken[p] }
	if got, err := Placeholder(lib, "nas", "Heat.mkv", isTaken); err != nil || got != filepath.Join(lib, "nas", "Heat (3).mkv") {
		t.Errorf("a taken path: %q, %v", got, err)
	}
	if got, err := Placeholder(lib, "nas", "README", isTaken); err != nil || got != filepath.Join(lib, "nas", "README (2)") {
		t.Errorf("a taken name without an extension: %q, %v", got, err)
	}
	if _, err := Placeholder(lib, "nas", "a.mp4", func(string) bool { return true }); err == nil {
		t.Error("every name taken")
	}
	for _, p := range []string{"", ".", "..", "../a.mp4", "/etc/passwd", `..\a.mp4`, "a/../../b.mp4"} {
		if got, err := Placeholder(lib, "nas", p, nil); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Placeholder(%q) = %q, %v; want ErrInvalidPath", p, got, err)
		}
	}
}

// SameAddress tells sources that reach the same files apart from others, whoever signs in, the server's name in any case and the folders with or without slashes.
func TestSameAddress(t *testing.T) {
	nas := Source{Kind: KindSMB, Config: map[string]string{SMBHost: "NAS.local", SMBShare: "media", SMBRoot: "/films/", SMBUser: "anna"}}
	same := Source{Kind: KindSMB, Config: map[string]string{SMBHost: "nas.local", SMBShare: "media/", SMBRoot: "films", SMBUser: "ben", SMBNoSigning: "true"}}
	if !SameAddress(nas, same) {
		t.Fatal("the same share read as another")
	}
	for name, other := range map[string]Source{
		"kind":  {Kind: "sftp", Config: same.Config},
		"host":  {Kind: KindSMB, Config: map[string]string{SMBHost: "nas2.local", SMBShare: "media", SMBRoot: "films"}},
		"share": {Kind: KindSMB, Config: map[string]string{SMBHost: "nas.local", SMBShare: "music", SMBRoot: "films"}},
		"root":  {Kind: KindSMB, Config: map[string]string{SMBHost: "nas.local", SMBShare: "media"}},
		"port":  {Kind: KindSMB, Config: map[string]string{SMBHost: "nas.local", SMBShare: "media", SMBRoot: "films", SMBPort: "4450"}},
	} {
		if SameAddress(nas, other) {
			t.Errorf("another %s read as the same", name)
		}
	}
}

// LinkedPath puts an indexed video where its copy would go: under the source's folder, a video at the source's root in a folder of its own as video.<ext>, numbered when taken.
func TestLinkedPath(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "lib")
	taken := map[string]bool{}
	is := func(p string) bool { return taken[p] }
	deep, err := LinkedPath(dir, "NAS", "Films/Heat.mkv", is)
	if err != nil || deep != filepath.Join(dir, "NAS", "Films", "Heat.mkv") {
		t.Fatalf("a deep video: %q %v", deep, err)
	}
	root, err := LinkedPath(dir, "NAS", "Heat.mkv", is)
	if err != nil || root != filepath.Join(dir, "NAS", "Heat", "video.mkv") {
		t.Fatalf("a video at the root: %q %v", root, err)
	}
	taken[filepath.Join(dir, "NAS", "Heat")] = true
	if again, err := LinkedPath(dir, "NAS", "Heat.mkv", is); err != nil || again != filepath.Join(dir, "NAS", "Heat (2)", "video.mkv") {
		t.Fatalf("a taken folder: %q %v", again, err)
	}
}

// TestSourceKeyOfASource checks that a file of a built-in source is named by its server's address (remotefs.SourceKey), the same as a phone that reaches the server with its own source computes, whatever the user or the source's ID, and that a source not among those given or a plugin's names none.
func TestSourceKeyOfASource(t *testing.T) {
	sources := []Source{
		{ID: "0123456789abcdef", Kind: KindSMB, Name: "NAS", Config: map[string]string{SMBHost: "NAS.local", SMBShare: "video", SMBRoot: "Films", SMBUser: "anna"}},
		{ID: "fedcba9876543210", Kind: "kando.webdav", Name: "Plugin", Config: map[string]string{"url": "https://nas.local/dav"}},
	}
	// smb ‖ nas.local ‖ 445 ‖ video ‖ Films/River.mp4 ‖ 1048576, the vector of remotefs's TestSourceKeyVectors.
	if got := SourceKey(sources, Ref{Source: "0123456789abcdef", Path: "River.mp4"}, 1<<20); got != "6aTKFGnkWxhnQ8vrl6Wj-G" {
		t.Errorf("SourceKey = %q, want the phone's key for the same file", got)
	}
	if got := SourceKey(sources, Ref{Source: "fedcba9876543210", Path: "River.mp4"}, 1<<20); got != "" {
		t.Errorf("a plugin's source has the key %q", got)
	}
	if got := SourceKey(sources, Ref{Source: "1111111111111111", Path: "River.mp4"}, 1<<20); got != "" {
		t.Errorf("a source not in the settings has the key %q", got)
	}
}
