package media

import (
	"os"
	"path/filepath"
	"testing"
)

// browserIDs is the --cookies-from-browser names of browsers, in their order.
func browserIDs(browsers []Browser) []string {
	var ids []string
	for _, b := range browsers {
		ids = append(ids, b.ID)
	}
	return ids
}

// mkdirs makes each folder under root.
func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// A browser counts as installed when the folder yt-dlp reads its login from (yt_dlp/cookies.py of 2026.08.19) is there, on each system in its own layout. Safari, whose cookies only macOS has, is offered on macOS alone, and always there.
func TestInstalledBrowsersFindsTheLoginFoldersYTDLPReads(t *testing.T) {
	for _, tc := range []struct {
		name, goos string
		dirs       []string
		env        map[string]string
		want       []string
	}{
		{name: "macOS with nothing but Safari", goos: "darwin", want: []string{"safari"}},
		{name: "macOS", goos: "darwin", dirs: []string{"Library/Application Support/Google/Chrome", "Library/Application Support/Firefox/Profiles", "Library/Application Support/Microsoft Edge", "Library/Application Support/BraveSoftware/Brave-Browser", "Library/Application Support/Chromium"}, want: []string{"chrome", "firefox", "edge", "brave", "chromium", "safari"}},
		{name: "Windows with Chrome and Edge", goos: "windows", dirs: []string{"AppData/Local/Google/Chrome/User Data", "AppData/Local/Microsoft/Edge/User Data"}, env: map[string]string{"LOCALAPPDATA": "AppData/Local", "APPDATA": "AppData/Roaming"}, want: []string{"chrome", "edge"}},
		{name: "Windows with Firefox, Brave and Chromium", goos: "windows", dirs: []string{"AppData/Roaming/Mozilla/Firefox/Profiles", "AppData/Local/BraveSoftware/Brave-Browser/User Data", "AppData/Local/Chromium/User Data"}, env: map[string]string{"LOCALAPPDATA": "AppData/Local", "APPDATA": "AppData/Roaming"}, want: []string{"firefox", "brave", "chromium"}},
		{name: "Windows with the Store's Firefox", goos: "windows", dirs: []string{"AppData/Local/Packages/Mozilla.Firefox_n80bbvh6b1yt2/LocalCache/Roaming/Mozilla/Firefox/Profiles"}, env: map[string]string{"LOCALAPPDATA": "AppData/Local", "APPDATA": "AppData/Roaming"}, want: []string{"firefox"}},
		{name: "Windows with no browser yt-dlp reads", goos: "windows", env: map[string]string{"LOCALAPPDATA": "AppData/Local", "APPDATA": "AppData/Roaming"}},
		{name: "Linux with Chromium and Firefox", goos: "linux", dirs: []string{".config/chromium", ".mozilla/firefox"}, want: []string{"firefox", "chromium"}},
		{name: "Linux with Chrome, Edge and Brave", goos: "linux", dirs: []string{".config/google-chrome", ".config/microsoft-edge", ".config/BraveSoftware/Brave-Browser"}, want: []string{"chrome", "edge", "brave"}},
		{name: "Linux with Firefox from Flatpak", goos: "linux", dirs: []string{".var/app/org.mozilla.firefox/.mozilla/firefox"}, want: []string{"firefox"}},
		{name: "Linux with Firefox from Snap", goos: "linux", dirs: []string{"snap/firefox/common/.mozilla/firefox"}, want: []string{"firefox"}},
		{name: "Linux with its config elsewhere", goos: "linux", dirs: []string{"xdg/google-chrome", "xdg/mozilla/firefox"}, env: map[string]string{"XDG_CONFIG_HOME": "xdg"}, want: []string{"chrome", "firefox"}},
		{name: "Linux with no browser profile", goos: "linux", dirs: []string{".config/other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			mkdirs(t, home, tc.dirs...)
			getenv := func(key string) string {
				if v, ok := tc.env[key]; ok {
					return filepath.Join(home, filepath.FromSlash(v))
				}
				return ""
			}
			got := browserIDs(installedBrowsers(tc.goos, home, getenv))
			if len(got) != len(tc.want) || (len(got) > 0 && filepath.Join(got...) != filepath.Join(tc.want...)) {
				t.Fatalf("installed %v, want %v", got, tc.want)
			}
		})
	}
}

// A file where a browser's folder would be is not the browser.
func TestInstalledBrowsersNeedsAFolder(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".config")
	if err := os.WriteFile(filepath.Join(home, ".config", "google-chrome"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := installedBrowsers("linux", home, func(string) string { return "" }); len(got) != 0 {
		t.Fatalf("installed %v", browserIDs(got))
	}
}
