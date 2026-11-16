package browse

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/browserext"
	"kando/apps/kando/sites/catalog"
)

// extensionDir is the extension's folder among the app's resources, from this package's folder.
var extensionDir = filepath.Join("..", "resources", browserext.FolderName)

// The extension's generated files in the repository are what the buttons' code and data make now, for the app's version, as the browsers load the folder as it is.
func TestExtensionFilesAreCurrent(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "nsgo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	version, err := AppVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range ExtensionFiles(version) {
		got, err := os.ReadFile(filepath.Join(extensionDir, filepath.FromSlash(name)))
		// Git may check the files out with CRLF line endings on Windows.
		if err != nil || !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is not what the buttons make now (%v): run go generate in apps/kando/browse", name, err)
		}
	}
}

// The content script is the buttons' own script speaking to the extension's background, and runs on the pages of every site the buttons know, which the catalog lists, and on none other.
func TestExtensionRunsOnTheButtonsSites(t *testing.T) {
	script := ExtensionScript()
	for _, want := range []string{"const sites = (" + videoSitesJSON() + ")", "api.runtime.sendMessage(data)", "hear((data) =>", AskMessage, AddMessage, StateMessage, "ev.isTrusted"} {
		if !strings.Contains(script, want) {
			t.Errorf("the content script has no %q", want)
		}
	}
	if strings.Contains(script, "window.nsgo") || !strings.Contains(Script(), "window.nsgo") {
		t.Error("each script speaks through the other's bridge")
	}
	var manifest struct {
		Key            string `json:"key"`
		ContentScripts []struct {
			Matches []string `json:"matches"`
			JS      []string `json:"js"`
		} `json:"content_scripts"`
		Specific struct {
			Gecko struct {
				ID string `json:"id"`
			} `json:"gecko"`
		} `json:"browser_specific_settings"`
	}
	if err := json.Unmarshal(extensionManifest("1.0.0"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Key != browserext.PublicKey || manifest.Specific.Gecko.ID != browserext.GeckoID || len(manifest.ContentScripts) != 1 || !slices.Equal(manifest.ContentScripts[0].JS, []string{"content.js"}) {
		t.Fatalf("manifest %+v", manifest)
	}
	matches := manifest.ContentScripts[0].Matches
	var domains []string
	for _, s := range videoSites {
		domains = append(domains, s.Domain)
	}
	if len(matches) != len(domains) {
		t.Fatalf("matches %q, sites %q", matches, domains)
	}
	for i, m := range matches {
		host := strings.TrimSuffix(strings.TrimPrefix(m, "*://*."), "/*")
		if host != domains[i] || !catalog.Load().On("https://"+host+"/") {
			t.Errorf("match %q is not the site %q of the catalog", m, domains[i])
		}
	}
}

// matchPattern reports whether u matches a match pattern as the browsers read it: any scheme of http and https, the host or any of its subdomains, and the path with its query, where * is any characters (https://developer.chrome.com/docs/extensions/develop/concepts/match-patterns).
func matchPattern(pattern, raw string) bool {
	rest, ok := strings.CutPrefix(pattern, "*://*.")
	if !ok {
		return false
	}
	host, path, _ := strings.Cut(rest, "/")
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Hostname() != host && !strings.HasSuffix(u.Hostname(), "."+host) {
		return false
	}
	glob := "^" + strings.ReplaceAll(regexp.QuoteMeta("/"+path), `\*`, ".*") + "$"
	target := u.EscapedPath()
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return regexp.MustCompile(glob).MatchString(target)
}

// The menu item on links shows on every link of a video the buttons know, and its patterns are each of a shape's, on a site of the catalog; links of the same sites that are not one video it shows on where a pattern cannot tell them apart, and adds nothing for them, as it adds only what VideoLink takes.
func TestExtensionMenuShowsOnVideosLinks(t *testing.T) {
	patterns := menuPatterns()
	for _, s := range videoSites {
		for _, v := range s.Videos {
			if len(v.Menu) == 0 {
				t.Errorf("%s: a shape without the menu's paths: %+v", s.Domain, v)
			}
			for _, path := range v.Menu {
				if !strings.HasPrefix(path, "/") {
					t.Errorf("%s: menu path %q", s.Domain, path)
				}
			}
		}
	}
	videos := []string{
		"https://www.youtube.com/watch?v=jNQXAC9IVRw",
		"https://www.youtube.com/watch?v=jNQXAC9IVRw&list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf",
		"https://m.youtube.com/watch?v=jNQXAC9IVRw",
		"https://www.youtube.com/shorts/TRWgBxVAb1M",
		"https://www.youtube.com/live/TRWgBxVAb1M",
		"https://youtu.be/jNQXAC9IVRw",
		"https://www.bilibili.com/video/BV1xx411c7mD/?spm_id_from=333.1007",
		"https://m.bilibili.com/video/av170001",
		"https://www.bilibili.com/zh-Hans/video/BV1nx4y1x7t1/",
		"https://vimeo.com/76979871",
		"https://www.ted.com/talks/ken_robinson_do_schools_kill_creativity",
		"https://www.nicovideo.jp/watch/sm9",
		"https://www.dailymotion.com/video/x8abcd1",
		"https://www.tiktok.com/@scout2015/video/6718335390845095173",
	}
	for _, u := range videos {
		if !VideoLink(u) {
			t.Fatalf("%s is no video", u)
		}
		if !slices.ContainsFunc(patterns, func(p string) bool { return matchPattern(p, u) }) {
			t.Errorf("the menu does not show on %s", u)
		}
	}
	for _, u := range []string{"https://www.youtube.com/", "https://www.youtube.com/@veritasium", "https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "https://www.bilibili.com/", "https://space.bilibili.com/123456", "https://vimeo.com/channels/staffpicks", "https://www.ted.com/", "https://example.com/watch?v=jNQXAC9IVRw"} {
		if slices.ContainsFunc(patterns, func(p string) bool { return matchPattern(p, u) }) {
			t.Errorf("the menu shows on %s", u)
		}
	}
}

// What the extension adds is read as the built-in browser's pages' AddMessage is: one video's link, and a title on one line, kept short.
func TestAddVideoReadsAsAddMessage(t *testing.T) {
	link, title, ok := AddVideo("https://youtu.be/jNQXAC9IVRw", "Me at\nthe zoo\t"+strings.Repeat("x", 300))
	if !ok || link != "https://youtu.be/jNQXAC9IVRw" || strings.ContainsAny(title, "\n\t") || len([]rune(title)) != maxTitle+1 {
		t.Fatalf("AddVideo = %q, %q, %v", link, title, ok)
	}
	if _, _, ok := AddVideo("https://www.youtube.com/@veritasium", ""); ok {
		t.Fatal("a channel was read as a video")
	}
	if got := AskLinks([]string{"https://youtu.be/jNQXAC9IVRw", "https://youtu.be/jNQXAC9IVRw", "https://www.youtube.com/"}); !slices.Equal(got, []string{"https://youtu.be/jNQXAC9IVRw"}) {
		t.Fatalf("AskLinks = %q", got)
	}
}
