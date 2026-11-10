package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The page offers WebMCP tools to AI agents in the browser (static/webmcp.js): whether Kando downloads from a website, the download for the visitor's system, and the getting-started steps and features in any of the page's languages. What the tools answer from is written here, as files the script fetches only when an agent calls a tool, so that the page itself grows only by the download table it embeds (agentData).

// agentDir is the folder of the built site the tools' files are in.
const agentDir = "webmcp"

// buildCommands are the commands of the Build from source card, which the page shows and get_started returns.
const buildCommands = `git clone https://github.com/yldm-tech/Kando.git
cd Kando
make build`

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
			"dmg":    "Open the disk image and drag Kando to Applications. The app is not signed with a Developer ID, so macOS blocks it the first time: on macOS 15 and later open it once, then choose System Settings › Privacy & Security › Open Anyway; on macOS 14 and earlier Control-click Kando in Applications and choose Open; or run xattr -dr com.apple.quarantine /Applications/Kando.app in Terminal.",
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

// featureKeys are the feature cards of page.html in its order, by the prefix of their Title and Body keys.
var featureKeys = []string{"featureOnDevice", "featureModel", "featurePlayer", "featureFolders", "featureSites", "featureAgents", "featureLanguages", "featureUpload", "featureShortcuts"}

// newAgentContent is l's content file, made from the same content strings as the page's Get started and features sections. used records the keys it read, which the tests compare with the keys those sections of page.html show.
func newAgentContent(l language, strs map[string]string, total int, used map[string]bool) agentContent {
	t := func(key string) string {
		used[key] = true
		return strings.ReplaceAll(strs[key], "{count}", fmt.Sprint(total))
	}
	c := agentContent{
		Lang: l.Code,
		Name: l.Name,
		Page: l.URL(),
		Start: agentStart{
			Title: t("startTitle"),
			Lead:  t("startLead"),
			Steps: []agentStep{
				{
					Number: 1,
					Title:  t("startDownloadTitle"),
					Text:   []string{t("startDownloadBody")},
					Notes:  []string{t("startDownloadUnsigned")},
					Link:   &agentLink{Text: t("startDownloadButton"), URL: releasesURL},
				},
				{
					Number: 2,
					Title:  t("startToolsTitle"),
					Text:   []string{t("startToolsBody")},
					Tools:  []agentTool{{"yt-dlp", t("toolYtdlp")}, {"ffmpeg", t("toolFfmpeg")}, {"whisper-cli", t("toolWhisper")}},
					Notes:  []string{t("startToolsOther"), t("startSetup")},
				},
				{
					Title:   t("startBuildTitle"),
					Text:    []string{t("startBuildBody")},
					Command: buildCommands,
					Notes:   []string{t("startBuildNote")},
				},
			},
			Readme: agentLink{Text: t("startReadme"), URL: l.ReadmeURL()},
		},
		Features: agentFeatures{Title: t("featuresTitle"), Lead: t("featuresLead")},
	}
	for _, key := range featureKeys {
		f := agentFeature{Title: t(key + "Title"), Text: t(key + "Body")}
		if key == "featureModel" {
			f.Link = &agentLink{Text: t("featureModelLink"), URL: everyAPIURL}
		}
		c.Features.Items = append(c.Features.Items, f)
	}
	return c
}

// writeAgentFiles writes the tools' files into out's webmcp folder, the sites file and one content file per language, and returns the data each page embeds, by language code.
func writeAgentFiles(root, out string, langs []language, content map[string]map[string]string, total int, used map[string]bool) (map[string]agentData, error) {
	dir := filepath.Join(out, agentDir)
	sites, err := writeAgentSites(filepath.Join(root, "apps", "kando", "sites", "catalog"), dir)
	if err != nil {
		return nil, err
	}
	version, err := appVersion(filepath.Join(root, "apps", "kando", "mygo.json"))
	if err != nil {
		return nil, err
	}
	var all []agentLang
	for _, l := range langs {
		link, err := writeAgentJSON(dir, l.Code+".json", newAgentContent(l, content[l.Code], total, used))
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
