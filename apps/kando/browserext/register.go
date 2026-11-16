package browserext

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"kando/apps/kando/clitool"
)

// Browser is a browser the host can be registered with: Name is how Settings names it, Firefox whether it is Firefox, whose manifest allows the extension by GeckoID (allowed_extensions) rather than by its origin (allowed_origins).
type Browser struct {
	Name    string
	Firefox bool
}

// place is where a browser keeps its things on one system: Data is the folder of its user data, whose presence tells it is installed, Hosts the folder it reads its user's host manifests from (macOS and Linux), and Key the registry key under HKEY_CURRENT_USER whose default value is the path of the host's manifest (Windows).
type place struct {
	Browser
	Data  []string
	Hosts string
	Key   string
}

// Env is what Look, Connect and Disconnect see of the computer. EnvHere reads this one; tests give folders and a registry of their own.
type Env struct {
	GOOS string
	// Home is the user's home folder, and Getenv reads the environment: XDG_CONFIG_HOME and APPIMAGE on Linux, LOCALAPPDATA and APPDATA on Windows.
	Home   string
	Getenv func(string) string
	// Exe is this app's executable, as os.Executable names it.
	Exe string
	// Dir is Kando's own folder beside its settings, where Windows keeps the host's manifests, as the registry only points to them.
	Dir string
	// Registry is HKEY_CURRENT_USER on Windows, nil elsewhere.
	Registry Registry
}

// Registry is the part of the Windows registry the host is registered in: the default values of keys under HKEY_CURRENT_USER.
type Registry interface {
	// Value is the default value of key, "" with no error when the key does not exist.
	Value(key string) (string, error)
	// Set makes key, with its parents, and sets its default value.
	Set(key, value string) error
	// Delete removes key, which has no subkeys; a key that does not exist is no error.
	Delete(key string) error
}

// EnvHere is this computer, with Kando's own folder dir.
func EnvHere(dir string) Env {
	exe, _ := os.Executable()
	home, _ := os.UserHomeDir()
	return Env{GOOS: runtime.GOOS, Home: home, Getenv: os.Getenv, Exe: exe, Dir: dir, Registry: userRegistry()}
}

// places are the browsers whose host folders or registry keys are known on env's system, in the order Settings names them. Chrome, Chromium and Edge are as their makers document them (Chrome: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location and https://chromium.googlesource.com/chromium/src/+/HEAD/docs/user_data_dir.md; Edge: https://learn.microsoft.com/microsoft-edge/extensions/developer-guide/native-messaging); Brave, Vivaldi and Arc read the NativeMessagingHosts folder of their user data, as every browser built on Chromium does, and on Windows the key of their own product; Firefox is as MDN documents it (https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_manifests#manifest_location).
func places(env Env) []place {
	chromium := func(name string, data, key string) place {
		return place{Browser: Browser{Name: name}, Data: []string{data}, Hosts: filepath.Join(data, "NativeMessagingHosts"), Key: key}
	}
	switch env.GOOS {
	case "darwin":
		support := filepath.Join(env.Home, "Library", "Application Support")
		return []place{
			chromium("Chrome", filepath.Join(support, "Google", "Chrome"), ""),
			chromium("Edge", filepath.Join(support, "Microsoft Edge"), ""),
			chromium("Brave", filepath.Join(support, "BraveSoftware", "Brave-Browser"), ""),
			chromium("Vivaldi", filepath.Join(support, "Vivaldi"), ""),
			chromium("Arc", filepath.Join(support, "Arc", "User Data"), ""),
			chromium("Chromium", filepath.Join(support, "Chromium"), ""),
			{Browser: Browser{Name: "Firefox", Firefox: true}, Data: []string{filepath.Join(support, "Firefox")}, Hosts: filepath.Join(support, "Mozilla", "NativeMessagingHosts")},
		}
	case "windows":
		local, roaming := env.Getenv("LOCALAPPDATA"), env.Getenv("APPDATA")
		if local == "" || roaming == "" {
			return nil
		}
		hosts := `Software\%s\NativeMessagingHosts\` + HostName
		win := func(name, data, vendor string) place {
			return place{Browser: Browser{Name: name}, Data: []string{filepath.Join(local, data, "User Data")}, Key: fmt.Sprintf(hosts, vendor)}
		}
		return []place{
			win("Chrome", filepath.Join("Google", "Chrome"), `Google\Chrome`),
			win("Edge", filepath.Join("Microsoft", "Edge"), `Microsoft\Edge`),
			win("Brave", filepath.Join("BraveSoftware", "Brave-Browser"), `BraveSoftware\Brave-Browser`),
			win("Vivaldi", "Vivaldi", "Vivaldi"),
			win("Chromium", "Chromium", "Chromium"),
			{Browser: Browser{Name: "Firefox", Firefox: true}, Data: []string{filepath.Join(roaming, "Mozilla", "Firefox")}, Key: fmt.Sprintf(hosts, "Mozilla")},
		}
	}
	config := env.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(env.Home, ".config")
	}
	return []place{
		chromium("Chrome", filepath.Join(config, "google-chrome"), ""),
		chromium("Edge", filepath.Join(config, "microsoft-edge"), ""),
		chromium("Brave", filepath.Join(config, "BraveSoftware", "Brave-Browser"), ""),
		chromium("Vivaldi", filepath.Join(config, "vivaldi"), ""),
		chromium("Chromium", filepath.Join(config, "chromium"), ""),
		{Browser: Browser{Name: "Firefox", Firefox: true}, Data: []string{filepath.Join(env.Home, ".mozilla", "firefox"), filepath.Join(env.Home, "snap", "firefox", "common", ".mozilla", "firefox")}, Hosts: filepath.Join(env.Home, ".mozilla", "native-messaging-hosts")},
	}
}

// installed reports whether p's user data is on the computer.
func (p place) installed() bool {
	for _, dir := range p.Data {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// manifestPath is the file p reads the host's manifest from: in its host folder, or on Windows Kando's own, one for the browsers built on Chromium and one for Firefox, as they allow the extension differently.
func (p place) manifestPath(env Env) string {
	if p.Key == "" {
		return filepath.Join(p.Hosts, HostName+".json")
	}
	family := "chromium"
	if p.Firefox {
		family = "firefox"
	}
	return filepath.Join(env.Dir, "NativeMessagingHosts", family, HostName+".json")
}

// Status is where the host stands, as Settings › 通用 › 浏览器扩展 shows it.
type Status struct {
	// Exe is the executable the browsers start as the host: this app's with its links resolved, or on Linux the AppImage it runs from.
	Exe string
	// Temporary reports whether Exe is in a place that goes away, a disk image or a copy macOS made (clitool.Status.Temporary), where Connect refuses to register it.
	Temporary bool
	// Found are the browsers on the computer, Connected those of them whose host is this app.
	Found     []string
	Connected []string
}

// Ready reports whether every browser found is connected, and at least one is.
func (s Status) Ready() bool { return len(s.Found) > 0 && len(s.Connected) == len(s.Found) }

// hostExe is the executable the browsers should start: on Linux the AppImage Kando runs from, which $APPIMAGE names, as its own executable is in the AppImage's mount, which goes when Kando quits (as clitool's command runs it), else the app's own as clitool resolves it.
func hostExe(env Env) clitool.Status {
	if env.GOOS == "linux" {
		if image := env.Getenv("APPIMAGE"); filepath.IsAbs(image) {
			if info, err := os.Stat(image); err == nil && info.Mode().IsRegular() {
				return clitool.Status{Exe: image}
			}
		}
	}
	return clitool.Look(clitool.Env{Exe: env.Exe})
}

// Look finds the browsers on the computer and those whose host is this app. It reads the disk and the registry, so it runs off the UI thread.
func Look(env Env) Status {
	exe := hostExe(env)
	st := Status{Exe: exe.Exe, Temporary: exe.Temporary}
	for _, p := range places(env) {
		if !p.installed() {
			continue
		}
		st.Found = append(st.Found, p.Name)
		if registered(env, p, st.Exe) {
			st.Connected = append(st.Connected, p.Name)
		}
	}
	return st
}

// registered reports whether p starts exe as the host: its manifest names exe and allows the extension, and on Windows its key points to that manifest.
func registered(env Env, p place, exe string) bool {
	path := p.manifestPath(env)
	if p.Key != "" {
		if env.Registry == nil {
			return false
		}
		if value, err := env.Registry.Value(p.Key); err != nil || !samePath(value, path) {
			return false
		}
	}
	data, err := os.ReadFile(path)
	return err == nil && bytes.Equal(data, HostManifest(p.Firefox, exe))
}

func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

// hostManifest is a native messaging host's manifest, as Chrome and Firefox read it.
type hostManifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedOrigins    []string `json:"allowed_origins,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
}

// HostManifest is the manifest that has a browser start exe as the host for Kando's extension: by its origin (ChromeOrigin) for the browsers built on Chromium, by GeckoID for Firefox.
func HostManifest(firefox bool, exe string) []byte {
	m := hostManifest{Name: HostName, Description: "Kando", Path: exe, Type: "stdio"}
	if firefox {
		m.AllowedExtensions = []string{GeckoID}
	} else {
		m.AllowedOrigins = []string{ChromeOrigin()}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		panic("browserext: host manifest: " + err.Error())
	}
	return append(data, '\n')
}

// ErrTemporary is why Connect refuses an executable in a place that goes away (Status.Temporary).
var ErrTemporary = clitool.ErrTemporary

// ErrNoBrowser is why Connect connected nothing: none of the browsers it knows is on the computer.
var ErrNoBrowser = errors.New("no browser Kando's extension runs in was found")

// Connect registers this app as the host with every browser found: it writes the host's manifest into each one's host folder, or on Windows into Kando's own folder with the browser's registry key pointing to it, and returns the browsers' names. It refuses an executable in a place that goes away (ErrTemporary). A browser it could not register is left out, and the error says why, with those it could registered all the same.
func Connect(env Env) ([]string, error) {
	exe := hostExe(env)
	if exe.Temporary {
		return nil, ErrTemporary
	}
	var names []string
	var errs []error
	for _, p := range places(env) {
		if !p.installed() {
			continue
		}
		if err := register(env, p, exe.Exe); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
			continue
		}
		names = append(names, p.Name)
	}
	if len(names) == 0 && len(errs) == 0 {
		return nil, ErrNoBrowser
	}
	return names, errors.Join(errs...)
}

// register writes p's manifest for exe, and on Windows points p's key to it.
func register(env Env, p place, exe string) error {
	path := p.manifestPath(env)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, HostManifest(p.Firefox, exe), 0o644); err != nil {
		return err
	}
	if p.Key == "" {
		return nil
	}
	if env.Registry == nil {
		return errors.New("no registry")
	}
	return env.Registry.Set(p.Key, path)
}

// Disconnect unregisters the host from every browser it knows, found or not: it removes the manifests named HostName that are Kando's, and on Windows the keys that point to Kando's own. A manifest of that name another program wrote stays.
func Disconnect(env Env) error {
	var errs []error
	for _, p := range places(env) {
		path := p.manifestPath(env)
		if p.Key != "" && env.Registry != nil {
			if value, err := env.Registry.Value(p.Key); err == nil && value != "" && samePath(value, path) {
				if err := env.Registry.Delete(p.Key); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
				}
			}
		}
		if ours(path) {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// ours reports whether the file at path is a host manifest of Kando's extension.
func ours(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m hostManifest
	return json.Unmarshal(data, &m) == nil && m.Name == HostName && (slices.Contains(m.AllowedOrigins, ChromeOrigin()) || slices.Contains(m.AllowedExtensions, GeckoID))
}

// Refresh registers the host again with the browsers it is registered with, for exe as it is now, as after Kando moved or its AppImage was replaced; it does nothing where none is registered, or while Kando runs from a place that goes away.
func Refresh(env Env) error {
	exe := hostExe(env)
	if exe.Temporary {
		return nil
	}
	var errs []error
	for _, p := range places(env) {
		if !p.installed() || !ours(p.manifestPath(env)) || registered(env, p, exe.Exe) {
			continue
		}
		if err := register(env, p, exe.Exe); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
		}
	}
	return errors.Join(errs...)
}
