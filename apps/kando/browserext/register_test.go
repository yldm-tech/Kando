package browserext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// fakeRegistry is HKEY_CURRENT_USER for the tests, by key.
type fakeRegistry map[string]string

func (r fakeRegistry) Value(key string) (string, error) { return r[key], nil }
func (r fakeRegistry) Set(key, value string) error      { r[key] = value; return nil }
func (r fakeRegistry) Delete(key string) error          { delete(r, key); return nil }

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fakeEnv is a computer of goos with a home of its own and an executable in it, with its links resolved, as Look resolves the executable's.
func fakeEnv(t *testing.T, goos string, vars map[string]string) Env {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(mkdir(t, home, "Applications", "Kando.app", "Contents", "MacOS"), "Kando")
	if err := os.WriteFile(exe, []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	getenv := func(name string) string {
		if v, ok := vars[name]; ok {
			return filepath.Join(home, v)
		}
		return ""
	}
	env := Env{GOOS: goos, Home: home, Getenv: getenv, Exe: exe, Dir: mkdir(t, home, "Kando")}
	if goos == "windows" {
		env.Registry = fakeRegistry{}
	}
	return env
}

func readManifest(t *testing.T, path string) hostManifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m hostManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Connect writes the host's manifest where each browser on the computer reads it, for the browsers built on Chromium allowing the extension's origin and for Firefox its ID, and only for the browsers there; Look then finds them connected, and Disconnect takes them away.
func TestConnectWritesTheManifestsOfTheBrowsersThere(t *testing.T) {
	for _, c := range []struct {
		goos   string
		vars   map[string]string
		data   [][]string
		hosts  map[string][]string
		absent []string
	}{
		{
			goos: "darwin",
			data: [][]string{{"Library", "Application Support", "Google", "Chrome"}, {"Library", "Application Support", "Arc", "User Data"}, {"Library", "Application Support", "Firefox"}},
			hosts: map[string][]string{
				"Chrome":  {"Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"},
				"Arc":     {"Library", "Application Support", "Arc", "User Data", "NativeMessagingHosts"},
				"Firefox": {"Library", "Application Support", "Mozilla", "NativeMessagingHosts"},
			},
			absent: []string{"Library/Application Support/Microsoft Edge/NativeMessagingHosts"},
		},
		{
			goos: "linux",
			vars: map[string]string{"XDG_CONFIG_HOME": "xdg"},
			data: [][]string{{"xdg", "chromium"}, {"xdg", "BraveSoftware", "Brave-Browser"}, {".mozilla", "firefox"}},
			hosts: map[string][]string{
				"Brave":    {"xdg", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"},
				"Chromium": {"xdg", "chromium", "NativeMessagingHosts"},
				"Firefox":  {".mozilla", "native-messaging-hosts"},
			},
			absent: []string{"xdg/google-chrome/NativeMessagingHosts"},
		},
	} {
		t.Run(c.goos, func(t *testing.T) {
			env := fakeEnv(t, c.goos, c.vars)
			for _, d := range c.data {
				mkdir(t, append([]string{env.Home}, d...)...)
			}
			if st := Look(env); len(st.Found) != len(c.hosts) || len(st.Connected) != 0 || st.Ready() {
				t.Fatalf("before connecting: %+v", st)
			}
			names, err := Connect(env)
			if err != nil || len(names) != len(c.hosts) {
				t.Fatalf("Connect = %q, %v", names, err)
			}
			for name, dir := range c.hosts {
				m := readManifest(t, filepath.Join(append(append([]string{env.Home}, dir...), HostName+".json")...))
				if m.Name != HostName || m.Path != env.Exe || m.Type != "stdio" {
					t.Fatalf("%s: manifest %+v", name, m)
				}
				if name == "Firefox" {
					if !slices.Equal(m.AllowedExtensions, []string{GeckoID}) || m.AllowedOrigins != nil {
						t.Fatalf("Firefox allows %+v", m)
					}
				} else if !slices.Equal(m.AllowedOrigins, []string{ChromeOrigin()}) || m.AllowedExtensions != nil {
					t.Fatalf("%s allows %+v", name, m)
				}
			}
			for _, dir := range c.absent {
				if _, err := os.Stat(filepath.Join(env.Home, filepath.FromSlash(dir))); !os.IsNotExist(err) {
					t.Fatalf("%s was written for a browser that is not there", dir)
				}
			}
			st := Look(env)
			if !st.Ready() || len(st.Connected) != len(c.hosts) || st.Exe != env.Exe {
				t.Fatalf("after connecting: %+v", st)
			}

			// A manifest of the same name another program wrote stays when Kando disconnects; Kando's go.
			other := filepath.Join(append(append([]string{env.Home}, c.hosts["Firefox"]...), HostName+".json")...)
			if err := os.WriteFile(other, []byte(`{"name":"ai.yldm.kando","path":"/other","allowed_extensions":["other@example.com"]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Disconnect(env); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(other); err != nil {
				t.Fatalf("another program's manifest went: %v", err)
			}
			if st := Look(env); len(st.Connected) != 0 {
				t.Fatalf("after disconnecting: %+v", st)
			}
		})
	}
}

// On Windows the manifests go into Kando's own folder, one for the browsers built on Chromium and one for Firefox, and each browser's key under HKEY_CURRENT_USER points to its family's.
func TestConnectOnWindowsSetsTheRegistry(t *testing.T) {
	env := fakeEnv(t, "windows", map[string]string{"LOCALAPPDATA": "Local", "APPDATA": "Roaming"})
	mkdir(t, env.Home, "Local", "Google", "Chrome", "User Data")
	mkdir(t, env.Home, "Local", "Microsoft", "Edge", "User Data")
	mkdir(t, env.Home, "Roaming", "Mozilla", "Firefox")
	reg := env.Registry.(fakeRegistry)
	reg[`Software\Vivaldi\NativeMessagingHosts\other`] = "untouched"
	names, err := Connect(env)
	if err != nil || !slices.Equal(names, []string{"Chrome", "Edge", "Firefox"}) {
		t.Fatalf("Connect = %q, %v", names, err)
	}
	chromium := filepath.Join(env.Dir, "NativeMessagingHosts", "chromium", HostName+".json")
	firefox := filepath.Join(env.Dir, "NativeMessagingHosts", "firefox", HostName+".json")
	want := fakeRegistry{
		`Software\Google\Chrome\NativeMessagingHosts\ai.yldm.kando`:  chromium,
		`Software\Microsoft\Edge\NativeMessagingHosts\ai.yldm.kando`: chromium,
		`Software\Mozilla\NativeMessagingHosts\ai.yldm.kando`:        firefox,
		`Software\Vivaldi\NativeMessagingHosts\other`:                "untouched",
	}
	if len(reg) != len(want) {
		t.Fatalf("registry %q", reg)
	}
	for k, v := range want {
		if reg[k] != v {
			t.Fatalf("registry %q, want %q", reg, want)
		}
	}
	if m := readManifest(t, chromium); m.Path != env.Exe || !slices.Equal(m.AllowedOrigins, []string{ChromeOrigin()}) {
		t.Fatalf("Chromium's manifest %+v", m)
	}
	if m := readManifest(t, firefox); m.Path != env.Exe || !slices.Equal(m.AllowedExtensions, []string{GeckoID}) {
		t.Fatalf("Firefox's manifest %+v", m)
	}
	if st := Look(env); !st.Ready() {
		t.Fatalf("after connecting: %+v", st)
	}
	// A key that points elsewhere is not Kando's connection.
	reg[`Software\Microsoft\Edge\NativeMessagingHosts\ai.yldm.kando`] = `C:\Other\host.json`
	if st := Look(env); st.Ready() || slices.Contains(st.Connected, "Edge") {
		t.Fatalf("with Edge's key pointing elsewhere: %+v", st)
	}
	if err := Disconnect(env); err != nil {
		t.Fatal(err)
	}
	if len(reg) != 2 || reg[`Software\Microsoft\Edge\NativeMessagingHosts\ai.yldm.kando`] != `C:\Other\host.json` || reg[`Software\Vivaldi\NativeMessagingHosts\other`] != "untouched" {
		t.Fatalf("after disconnecting, registry %q", reg)
	}
	if _, err := os.Stat(chromium); !os.IsNotExist(err) {
		t.Fatalf("Chromium's manifest stays: %v", err)
	}
}

// The host is the AppImage Kando runs from on Linux, not its executable inside the AppImage's folder, which goes away with it; once Kando runs from another place, Refresh points the browsers it is connected to there, and leaves alone those it is not.
func TestHostIsTheAppImageAndFollowsTheApp(t *testing.T) {
	env := fakeEnv(t, "linux", map[string]string{"APPIMAGE": "Kando.AppImage"})
	for _, image := range []string{filepath.Join(env.Home, "Kando.AppImage"), filepath.Join(mkdir(t, env.Home, "Apps"), "Kando.AppImage")} {
		if err := os.WriteFile(image, []byte("appimage"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkdir(t, env.Home, ".config", "google-chrome")
	mkdir(t, env.Home, ".config", "chromium")
	if _, err := Connect(env); err != nil {
		t.Fatal(err)
	}
	chrome := filepath.Join(env.Home, ".config", "google-chrome", "NativeMessagingHosts", HostName+".json")
	if m := readManifest(t, chrome); m.Path != filepath.Join(env.Home, "Kando.AppImage") {
		t.Fatalf("manifest %+v", m)
	}
	if err := os.Remove(filepath.Join(env.Home, ".config", "chromium", "NativeMessagingHosts", HostName+".json")); err != nil {
		t.Fatal(err)
	}
	moved := env
	moved.Getenv = func(name string) string {
		if name == "APPIMAGE" {
			return filepath.Join(env.Home, "Apps", "Kando.AppImage")
		}
		return ""
	}
	if err := Refresh(moved); err != nil {
		t.Fatal(err)
	}
	if m := readManifest(t, chrome); m.Path != filepath.Join(env.Home, "Apps", "Kando.AppImage") {
		t.Fatalf("after moving: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "chromium", "NativeMessagingHosts", HostName+".json")); !os.IsNotExist(err) {
		t.Fatalf("Refresh connected Chromium, which was not: %v", err)
	}
}

// Kando running from a copy macOS made where it was downloaded is not registered, as the path soon leads nowhere.
func TestConnectRefusesATemporaryCopy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("App Translocation is macOS's, and its path is written with slashes")
	}
	env := fakeEnv(t, "darwin", nil)
	mkdir(t, env.Home, "Library", "Application Support", "Google", "Chrome")
	exe := filepath.Join(mkdir(t, env.Home, "AppTranslocation", "X", "Kando.app", "Contents", "MacOS"), "Kando")
	if err := os.WriteFile(exe, []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.Exe = exe
	if _, err := Connect(env); err != ErrTemporary {
		t.Fatalf("Connect: %v, want ErrTemporary", err)
	}
	if st := Look(env); !st.Temporary || len(st.Connected) != 0 {
		t.Fatalf("status %+v", st)
	}
}
