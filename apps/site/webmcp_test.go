package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// webmcpBlock finds the data block webmcp.js reads.
var webmcpBlock = regexp.MustCompile(`<script type="application/json" id="webmcp-data">([^<]*)</script>`)

// pageAgentData is the WebMCP data a built page embeds.
func pageAgentData(t *testing.T, l language) agentData {
	t.Helper()
	m := webmcpBlock.FindStringSubmatch(readOut(t, pageFile(l)))
	if m == nil {
		t.Fatalf("%s has no webmcp-data block", l.Code)
	}
	var data agentData
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		t.Fatalf("%s: webmcp-data: %v", l.Code, err)
	}
	return data
}

// outFile is the built file a path the page fetches names, without its query.
func outFile(link string) string {
	path, _, _ := strings.Cut(link, "?")
	return filepath.Join(built.out, filepath.FromSlash(strings.TrimPrefix(path, "/")))
}

// Each page loads webmcp.js, deferred, and embeds its data: the page's language, every language's content file, the sites file and the downloads of the version apps/kando/mygo.json names.
func TestWebMCPPageData(t *testing.T) {
	version, err := appVersion(filepath.Join(repoRoot, "apps", "kando", "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	script := regexp.MustCompile(`<script src="/static/webmcp\.js\?v=[0-9a-f]+" defer></script>`)
	for _, l := range built.rep.Langs {
		t.Run(l.Code, func(t *testing.T) {
			if !script.MatchString(readOut(t, pageFile(l))) {
				t.Error("the page does not load /static/webmcp.js with defer")
			}
			data := pageAgentData(t, l)
			if data.Lang != l.Code {
				t.Errorf("lang %q, want %q", data.Lang, l.Code)
			}
			if len(data.Langs) != len(built.rep.Langs) {
				t.Errorf("%d languages, want %d", len(data.Langs), len(built.rep.Langs))
			}
			for _, al := range data.Langs {
				raw, err := os.ReadFile(outFile(al.Content))
				if err != nil {
					t.Errorf("%s's content file %s is not in the site: %v", al.Code, al.Content, err)
					continue
				}
				// get_started sends people to the newest published release, as the page's Download button does.
				var c agentContent
				if err := json.Unmarshal(raw, &c); err != nil {
					t.Errorf("%s's content file: %v", al.Code, err)
				} else if len(c.Start.Steps) == 0 || c.Start.Steps[0].Link == nil || c.Start.Steps[0].Link.URL != latestURL {
					t.Errorf("%s: the download step does not link %s", al.Code, latestURL)
				}
			}
			if _, err := os.Stat(outFile(data.Sites)); err != nil {
				t.Errorf("the sites file %s is not in the site: %v", data.Sites, err)
			}
			rel := data.Release
			if rel.Version != version {
				t.Errorf("release %q, want mygo.json's %q", rel.Version, version)
			}
			if rel.Page != releasesURL+"/tag/v"+version || rel.Checksums != releasesURL+"/download/v"+version+"/checksums.txt" {
				t.Errorf("release page %q, checksums %q", rel.Page, rel.Checksums)
			}
			kinds := map[string]bool{}
			for _, d := range rel.Downloads {
				if d.URL != releasesURL+"/download/v"+version+"/"+d.Name {
					t.Errorf("%s: URL %q", d.Name, d.URL)
				}
				kinds[d.Kind] = true
			}
			for kind := range kinds {
				if rel.Install[kind] == "" {
					t.Errorf("no install instructions for .%s", kind)
				}
			}
		})
	}
}

// The downloads are the files the Release workflow publishes, by the names v0.1.0's assets have on GitHub: a disk image per Mac, a Debian package and an archive per Linux architecture, and nothing for Windows.
func TestReleaseDownloads(t *testing.T) {
	rel := releaseDownloads("0.1.0")
	var got []string
	for _, d := range rel.Downloads {
		got = append(got, d.Platform+"/"+d.Arch+" "+d.Name)
	}
	want := []string{
		"macos/arm64 kando-0.1.0-arm64.dmg",
		"macos/amd64 kando-0.1.0-amd64.dmg",
		"linux/amd64 kando_0.1.0_amd64.deb",
		"linux/amd64 kando-0.1.0-linux-amd64.tar.gz",
		"linux/arm64 kando_0.1.0_arm64.deb",
		"linux/arm64 kando-0.1.0-linux-arm64.tar.gz",
	}
	if !slices.Equal(got, want) {
		t.Errorf("downloads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(rel.Install["tar.gz"], releasesURL+"/download/v0.1.0/install.sh") {
		t.Errorf("the archive's instructions do not link the release's install.sh: %s", rel.Install["tar.gz"])
	}
}

// webmcp/sites.json is the app's catalog, every site with its name, domain and aliases, for search_supported_sites.
func TestWebMCPSites(t *testing.T) {
	read := func(path string) (c struct {
		YTDLP string      `json:"ytdlp"`
		Sites []agentSite `json:"sites"`
	}) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return c
	}
	want := read(filepath.Join(repoRoot, "apps", "kando", "sites", "catalog", "sites.json"))
	got := read(outFile(pageAgentData(t, languages[0]).Sites))
	if got.YTDLP != want.YTDLP || len(got.Sites) != len(want.Sites) {
		t.Fatalf("sites.json has %d sites of yt-dlp %s, want the catalog's %d of %s", len(got.Sites), got.YTDLP, len(want.Sites), want.YTDLP)
	}
	for i, s := range want.Sites {
		if g := got.Sites[i]; g.Name != s.Name || g.Domain != s.Domain || !slices.Equal(g.Aliases, s.Aliases) {
			t.Errorf("site %d is %+v, want %+v", i, g, s)
		}
	}
	if !strings.Contains(readOut(t, agentDir+"/sites.json"), `"domain":"youtube.com","aliases":["youtu.be"`) {
		t.Error("sites.json does not list YouTube with youtu.be")
	}
}

// The tools' content files say everything the page's Get started and features sections say, so that get_started and list_features do not fall behind a card added to page.html.
func TestWebMCPContentCoversPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "apps", "site", "templates", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := string(data)
	key := regexp.MustCompile(`\btf? "(\w+)"`)
	// Labels of controls, the small lines above headings, a screenshot's description and the comments of the terminal example, which the tools have no use for.
	skip := map[string]bool{"copyLabel": true, "copiedLabel": true, "dlTabsLabel": true, "cliTabsLabel": true, "cliTabCommands": true, "libraryEyebrow": true, "cliEyebrow": true, "detailsEyebrow": true, "shotLibraryAlt": true,
		"cliCommentAdd": true, "cliCommentList": true, "cliCommentSubtitles": true, "cliCommentStatus": true, "mcpCommentAdd": true, "mcpCommentTools": true, "detailModesMark": true, "detailLanguagesMark": true}
	for _, id := range []string{"library", "cli", "features", "start"} {
		start := strings.Index(tmpl, `id="`+id+`"`)
		if start < 0 {
			t.Fatalf("page.html has no section %s", id)
		}
		section := tmpl[start:]
		section = section[:strings.Index(section, "</section>")]
		for _, m := range key.FindAllStringSubmatch(section, -1) {
			if !skip[m[1]] && !built.rep.AgentUsed[m[1]] {
				t.Errorf("#%s shows %s, which the WebMCP content files leave out (newAgentContent)", id, m[1])
			}
		}
	}
}

// webmcp.js registers its tools with a fake WebMCP and answers each of them, run by node on the built site through testdata/webmcp_harness.js. It is skipped where node is not installed.
func TestWebMCPScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	for _, l := range built.rep.Langs {
		t.Run(l.Code, func(t *testing.T) {
			cmd := exec.Command(node, filepath.Join("testdata", "webmcp_harness.js"), filepath.Join(built.out, "static", "webmcp.js"), built.out, pageFile(l), filepath.Join("content"))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
