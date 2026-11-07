package media

import (
	"os"
	"path/filepath"
	"runtime"
)

// Browser is a browser whose login yt-dlp can borrow.
type Browser struct {
	// ID is its name for --cookies-from-browser; Name is how the library names it.
	ID, Name string
	// app is its bundle on macOS, and command its executable elsewhere.
	app, command string
}

// Browsers are the browsers yt-dlp can read logins from, in the order Kando offers them: Safari last, since macOS lets an app read its cookies only with Full Disk Access.
var Browsers = []Browser{
	{ID: "chrome", Name: "Chrome", app: "Google Chrome.app", command: "google-chrome"},
	{ID: "firefox", Name: "Firefox", app: "Firefox.app", command: "firefox"},
	{ID: "edge", Name: "Edge", app: "Microsoft Edge.app", command: "microsoft-edge"},
	{ID: "brave", Name: "Brave", app: "Brave Browser.app", command: "brave-browser"},
	{ID: "safari", Name: "Safari", app: "Safari.app"},
}

// InstalledBrowsers are the Browsers this machine has, in their order.
func InstalledBrowsers() []Browser {
	var out []Browser
	home, _ := os.UserHomeDir()
	for _, b := range Browsers {
		if browserInstalled(b, home) {
			out = append(out, b)
		}
	}
	return out
}

func browserInstalled(b Browser, home string) bool {
	if runtime.GOOS != "darwin" {
		return b.command != "" && lookTool(b.command) != ""
	}
	for _, dir := range []string{"/Applications", "/System/Applications", filepath.Join(home, "Applications")} {
		if info, err := os.Stat(filepath.Join(dir, b.app)); err == nil && info.IsDir() {
			return true
		}
	}
	// Safari lives in /System/Volumes/Preboot on recent macOS but is always linked from /Applications.
	return b.ID == "safari"
}

// BrowserName is how the library names the browser whose --cookies-from-browser name is id, or id itself.
func BrowserName(id string) string {
	for _, b := range Browsers {
		if b.ID == id {
			return b.Name
		}
	}
	return id
}
