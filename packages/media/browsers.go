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
}

// Browsers are the browsers yt-dlp can read logins from, in the order Kando offers them: Safari last, since macOS lets an app read its cookies only with Full Disk Access.
var Browsers = []Browser{
	{ID: "chrome", Name: "Chrome"},
	{ID: "firefox", Name: "Firefox"},
	{ID: "edge", Name: "Edge"},
	{ID: "brave", Name: "Brave"},
	{ID: "chromium", Name: "Chromium"},
	{ID: "safari", Name: "Safari"},
}

// InstalledBrowsers are the Browsers whose login data this machine has, in their order. It looks at the disk, so it is called off the UI thread.
func InstalledBrowsers() []Browser {
	home, _ := os.UserHomeDir()
	return installedBrowsers(runtime.GOOS, home, os.Getenv)
}

// installedBrowsers are the Browsers that have a folder yt-dlp reads their login from on goos, for the user whose home folder is home. Safari is offered on macOS alone, and always: its cookies are inside a container an app may not even look into without Full Disk Access.
func installedBrowsers(goos, home string, getenv func(string) string) []Browser {
	var out []Browser
	for _, b := range Browsers {
		if b.ID == "safari" {
			if goos == "darwin" {
				out = append(out, b)
			}
			continue
		}
		for _, dir := range browserDataDirs(goos, home, getenv, b.ID) {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				out = append(out, b)
				break
			}
		}
	}
	return out
}

// browserDataDirs are the folders yt-dlp looks for the login of the browser id in on goos, as yt_dlp/cookies.py of 2026.08.19 names them (_get_chromium_based_browser_settings and _firefox_browser_dirs).
func browserDataDirs(goos, home string, getenv func(string) string, id string) []string {
	switch goos {
	case "windows":
		local, roaming := getenv("LOCALAPPDATA"), getenv("APPDATA")
		if local == "" || roaming == "" {
			return nil
		}
		switch id {
		case "chrome":
			return []string{filepath.Join(local, "Google", "Chrome", "User Data")}
		case "edge":
			return []string{filepath.Join(local, "Microsoft", "Edge", "User Data")}
		case "brave":
			return []string{filepath.Join(local, "BraveSoftware", "Brave-Browser", "User Data")}
		case "chromium":
			return []string{filepath.Join(local, "Chromium", "User Data")}
		case "firefox":
			return []string{filepath.Join(roaming, "Mozilla", "Firefox", "Profiles"), filepath.Join(local, "Packages", "Mozilla.Firefox_n80bbvh6b1yt2", "LocalCache", "Roaming", "Mozilla", "Firefox", "Profiles")}
		}
	case "darwin":
		support := filepath.Join(home, "Library", "Application Support")
		switch id {
		case "chrome":
			return []string{filepath.Join(support, "Google", "Chrome")}
		case "edge":
			return []string{filepath.Join(support, "Microsoft Edge")}
		case "brave":
			return []string{filepath.Join(support, "BraveSoftware", "Brave-Browser")}
		case "chromium":
			return []string{filepath.Join(support, "Chromium")}
		case "firefox":
			return []string{filepath.Join(support, "Firefox", "Profiles")}
		}
	default:
		config := getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		switch id {
		case "chrome":
			return []string{filepath.Join(config, "google-chrome")}
		case "edge":
			return []string{filepath.Join(config, "microsoft-edge")}
		case "brave":
			return []string{filepath.Join(config, "BraveSoftware", "Brave-Browser")}
		case "chromium":
			return []string{filepath.Join(config, "chromium")}
		case "firefox":
			return []string{
				filepath.Join(config, "mozilla", "firefox"),
				filepath.Join(home, ".mozilla", "firefox"),
				filepath.Join(home, ".var", "app", "org.mozilla.firefox", "config", "mozilla", "firefox"),
				filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
				filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
			}
		}
	}
	return nil
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
