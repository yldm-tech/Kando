package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// The page offers WebMCP tools to AI agents in the browser (static/webmcp.js): whether Kando downloads from a website, the download for the visitor's system, and the getting-started steps and features in any of the page's languages. What the tools answer from is written here, as files the script fetches only when an agent calls a tool, so that the page itself grows only by the download table it embeds (agentData).

// agentDir is the folder of the built site the tools' files are in.
const agentDir = "webmcp"

// buildCommands build Kando from source, as the download panel's Windows tab shows them and get_started returns them. go tool mygo build is what make build runs, and works where make is not installed, as on Windows.
const buildCommands = `git clone https://github.com/yldm-tech/Kando.git
cd Kando
go tool mygo build apps/kando`

// installCommand installs the newest release for the current user with the install.sh mygo build makes, as the Linux tab shows it.
const installCommand = "curl -fsSL " + latestURL + "/download/install.sh | sh"

// readmeBuildURL is the README's Build from source section, where get_download sends a visitor whose system has no build.
const readmeBuildURL = repoURL + "#build-from-source"

// agentData is what webmcp.js reads from the page, in a <script type="application/json" id="webmcp-data">: the page's language, every language's content file, the sites file and the download table.
type agentData struct {
	Lang    string       `json:"lang"`
	Langs   []agentLang  `json:"langs"`
	Sites   string       `json:"sites"`
	Release agentRelease `json:"release"`
}

// agentLang is a language of the site as the tools name it, with the address of its content file.
type agentLang struct {
	Code    string `json:"code"`
	Tag     string `json:"tag"`
	Name    string `json:"name"`
	Page    string `json:"page"`
	Content string `json:"content"`
}

// agentRelease is the release the version in apps/kando/mygo.json was published as, and its packages.
type agentRelease struct {
	Version   string `json:"version"`
	Page      string `json:"page"`
	Checksums string `json:"checksums"`
	// Source is where to read how to build Kando, for a system with no package.
	Source    string     `json:"source"`
	Downloads []download `json:"downloads"`
	// Install is how to install a download, by its Kind.
	Install map[string]string `json:"install"`
}

// download is one package of a release: the system it is for, as get_download names systems, its file and how to install it.
type download struct {
	// Platform is "macos" or "linux".
	Platform string `json:"platform"`
	// Arch is "arm64" or "amd64".
	Arch string `json:"arch"`
	// Kind is the file's type: "dmg", "deb" or "tar.gz".
	Kind string `json:"kind"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// releaseDownloads are the packages the Release workflow publishes for version, under the names mygo build gives them (.github/workflows/package.yml): a disk image for each Mac, and for each Linux architecture a Debian package and an archive that the release's install.sh installs. Windows installers are built but not published, so Windows has none.
func releaseDownloads(version string) agentRelease {
	tag := "v" + version
	base := releasesURL + "/download/" + tag + "/"
	var downloads []download
	for _, arch := range []string{"arm64", "amd64"} {
		name := "kando-" + version + "-" + arch + ".dmg"
		downloads = append(downloads, download{Platform: "macos", Arch: arch, Kind: "dmg", Name: name, URL: base + name})
	}
	for _, arch := range []string{"amd64", "arm64"} {
		name := "kando_" + version + "_" + arch + ".deb"
		downloads = append(downloads, download{Platform: "linux", Arch: arch, Kind: "deb", Name: name, URL: base + name})
		name = "kando-" + version + "-linux-" + arch + ".tar.gz"
		downloads = append(downloads, download{Platform: "linux", Arch: arch, Kind: "tar.gz", Name: name, URL: base + name})
	}
	return agentRelease{
		Version:   version,
		Page:      releasesURL + "/tag/" + tag,
		Checksums: base + "checksums.txt",
		Source:    readmeBuildURL,
		Downloads: downloads,
		Install: map[string]string{
			"dmg":    "Open the disk image and drag Kando to Applications. Releases newer than 0.5.1 are signed with a Developer ID and notarized, so macOS opens them without a warning. 0.5.1 and earlier are not: on macOS 15 and later open it once, then choose System Settings › Privacy & Security › Open Anyway; on macOS 14 and earlier Control-click Kando in Applications and choose Open; or run xattr -dr com.apple.quarantine /Applications/Kando.app in Terminal.",
			"deb":    "On Debian, Ubuntu and their derivatives, run sudo apt install ./<file name> in the folder it was downloaded to.",
			"tar.gz": "On other distributions, download " + base + "install.sh into the same folder as the archive and run sh install.sh there. It installs Kando into ~/.local for the current user, without root, and adds it to the applications menu; sh install.sh --uninstall removes it.",
		},
	}
}

// appVersion is the version in apps/kando/mygo.json, which the Release workflow tags and publishes.
func appVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if manifest.Version == "" {
		return "", fmt.Errorf("%s has no version", path)
	}
	return manifest.Version, nil
}

// agentSite is a website of the catalog as search_supported_sites matches it: by its name, its domain and its aliases, as catalog.Site.Matches does in the app.
type agentSite struct {
	Name    string   `json:"name"`
	Domain  string   `json:"domain"`
	Aliases []string `json:"aliases,omitempty"`
}

// writeAgentSites writes the catalog's sites, without their logos, to sites.json in dir and returns the address the page fetches it by.
func writeAgentSites(catalogDir, dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(catalogDir, "sites.json"))
	if err != nil {
		return "", err
	}
	var c struct {
		YTDLP string      `json:"ytdlp"`
		Sites []agentSite `json:"sites"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("sites.json: %w", err)
	}
	return writeAgentJSON(dir, "sites.json", c)
}

// agentContent is a language's content file: the Get started section and the features, in that language's words.
type agentContent struct {
	Lang     string        `json:"lang"`
	Name     string        `json:"name"`
	Page     string        `json:"page"`
	Start    agentStart    `json:"start"`
	Features agentFeatures `json:"features"`
}

type agentStart struct {
	Title  string      `json:"title"`
	Lead   string      `json:"lead"`
	Steps  []agentStep `json:"steps"`
	Readme agentLink   `json:"readme"`
}

// agentStep is a card of the Get started section, in the order the page shows its parts: Text, then Tools or Command, then Notes and Link.
type agentStep struct {
	// Number is the card's number on the page, 0 for the alternative of building from source.
	Number  int         `json:"number,omitempty"`
	Title   string      `json:"title"`
	Text    []string    `json:"text"`
	Tools   []agentTool `json:"tools,omitempty"`
	Command string      `json:"command,omitempty"`
	Notes   []string    `json:"notes,omitempty"`
	Link    *agentLink  `json:"link,omitempty"`
}

type agentTool struct {
	Name string `json:"name"`
	Does string `json:"does"`
}

type agentLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type agentFeatures struct {
	Title string         `json:"title"`
	Lead  string         `json:"lead"`
	Items []agentFeature `json:"items"`
}

type agentFeature struct {
	Title string     `json:"title"`
	Text  string     `json:"text"`
	Link  *agentLink `json:"link,omitempty"`
}

// detailKeys are the cards of the details section of page.html in its order, by the prefix of their Title and Body keys.
var detailKeys = []string{"detailReplay", "detailModes", "detailSearch", "detailWhisper", "detailAI", "detailBrowser", "detailUpload", "detailShortcuts", "detailLanguages"}

// newAgentContent is l's content file, made from the same content strings as the page's download panel (start) and its Library, Command line and details sections (features). used records the keys it read, which the tests compare with the keys those sections of page.html show.
func newAgentContent(l language, strs map[string]string, total int, surf surface, used map[string]bool) agentContent {
	t := func(key string) string {
		used[key] = true
		s := strs[key]
		for name, n := range map[string]int{"count": total, "commands": surf.Commands, "tools": len(surf.Tools)} {
			s = strings.ReplaceAll(s, "{"+name+"}", fmt.Sprint(n))
		}
		// The page sets the names between ` in code and breaks some headings in two; the tools return plain text on one line.
		return plainLine(strings.ReplaceAll(s, "`", ""))
	}
	join := func(keys ...string) string {
		parts := make([]string, len(keys))
		for i, key := range keys {
			parts[i] = t(key)
		}
		return strings.Join(parts, " ")
	}
	c := agentContent{
		Lang: l.Code,
		Name: l.Name,
		Page: l.URL(),
		Start: agentStart{
			Title: t("openTitle"),
			Lead:  t("openBody"),
			Steps: []agentStep{
				{
					Number: 1,
					Title:  "macOS",
					Text:   []string{t("dlMacBody")},
					Notes:  []string{t("dlFirstRun")},
					Link:   &agentLink{Text: t("dlMacButton"), URL: latestURL},
				},
				{
					Number:  2,
					Title:   "Linux",
					Text:    []string{t("dlLinuxBody")},
					Command: installCommand,
					Notes:   []string{t("dlLinuxNote"), t("dlFirstRun")},
					Link:    &agentLink{Text: t("dlLinuxButton"), URL: latestURL},
				},
				{
					Number:  3,
					Title:   "Windows",
					Text:    []string{t("dlWinBody")},
					Command: buildCommands,
					Notes:   []string{t("dlWinNote")},
				},
			},
			Readme: agentLink{Text: t("dlReadme"), URL: l.ReadmeURL()},
		},
		Features: agentFeatures{
			Title: t("detailsTitle"),
			Lead:  t("heroLead"),
			Items: []agentFeature{
				{Title: t("libraryTitle"), Text: join("libraryBody", "libraryPoint1", "libraryPoint2", "libraryPoint3")},
				{Title: t("cliTitle"), Text: join("cliBody", "cliPoint1", "cliPoint2", "cliPoint3")},
			},
		},
	}
	for _, key := range detailKeys {
		f := agentFeature{Title: t(key + "Title"), Text: t(key + "Body")}
		if key == "detailAI" {
			f.Link = &agentLink{Text: t("detailAILink"), URL: everyAPIURL}
		}
		c.Features.Items = append(c.Features.Items, f)
	}
	return c
}

// plainLine is s with its line breaks taken out: nothing after a Chinese character, kana or CJK punctuation mark, as Chinese and Japanese put no space between words, and a space after anything else, Korean among it.
func plainLine(s string) string {
	var b strings.Builder
	var prev rune
	for _, r := range s {
		if r == '\n' {
			unspaced := unicode.In(prev, unicode.Han, unicode.Hiragana, unicode.Katakana) || (prev >= 0x3000 && prev <= 0x303F) || (prev >= 0xFF00 && prev <= 0xFFEF)
			if !unspaced {
				b.WriteByte(' ')
			}
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// writeAgentFiles writes the tools' files into out's webmcp folder, the sites file and one content file per language, and returns the data each page embeds, by language code.
func writeAgentFiles(root, out string, langs []language, content map[string]map[string]string, total int, version string, surf surface, used map[string]bool) (map[string]agentData, error) {
	dir := filepath.Join(out, agentDir)
	sites, err := writeAgentSites(filepath.Join(root, "apps", "kando", "sites", "catalog"), dir)
	if err != nil {
		return nil, err
	}
	var all []agentLang
	for _, l := range langs {
		link, err := writeAgentJSON(dir, l.Code+".json", newAgentContent(l, content[l.Code], total, surf, used))
		if err != nil {
			return nil, err
		}
		all = append(all, agentLang{Code: l.Code, Tag: l.Tag, Name: l.Name, Page: l.Path(), Content: link})
	}
	release := releaseDownloads(version)
	data := map[string]agentData{}
	for _, l := range langs {
		data[l.Code] = agentData{Lang: l.Code, Langs: all, Sites: sites, Release: release}
	}
	return data, nil
}

// writeAgentJSON writes v as name in dir and returns the path the page fetches it by, with a hash of its content in the query, as copyStatic links the static files.
func writeAgentJSON(dir, name string, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if err := writeFile(filepath.Join(dir, name), data); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "/" + agentDir + "/" + name + "?v=" + hex.EncodeToString(sum[:5]), nil
}
