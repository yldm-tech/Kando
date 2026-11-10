package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	texttemplate "text/template"
	"unicode"
	"unicode/utf8"
)

// siteURL is where the site is served, the address of canonical links, Open Graph tags and the sitemap. CNAME binds the GitHub Pages site to it.
const siteURL = "https://kando.yldm.ai"

const (
	repoURL     = "https://github.com/yldm-tech/Kando"
	releasesURL = repoURL + "/releases"
	// latestURL redirects to the newest published release, so the Download buttons reach the current files even when the site was built before a release came out. Drafts and prereleases are not "latest", so it never lands on a release that 404s for visitors.
	latestURL   = releasesURL + "/latest"
	licenseURL  = repoURL + "/blob/main/LICENSE"
	issuesURL   = repoURL + "/issues"
	everyAPIURL = "https://everyapi.ai"
)

// language is one page of the site: the interface languages Kando ships, in the order the switcher lists them.
type language struct {
	// Code names the content file, the screenshot folder under docs/assets and the page's folder.
	Code string
	// Tag is the BCP 47 tag of the page, for <html lang> and hreflang: a script rather than a region for Chinese, so that the Simplified page also serves Singapore and the Traditional one Hong Kong and Macau.
	Tag string
	// Locale is the Open Graph locale.
	Locale string
	// Name is the language named in itself, as the switcher shows it.
	Name string
	// Readme is the README file written in this language.
	Readme string
}

var languages = []language{
	{Code: "en", Tag: "en", Locale: "en_US", Name: "English", Readme: "README.md"},
	{Code: "zh-CN", Tag: "zh-Hans", Locale: "zh_CN", Name: "简体中文", Readme: "README.zh-CN.md"},
	{Code: "zh-TW", Tag: "zh-Hant", Locale: "zh_TW", Name: "繁體中文", Readme: "README.zh-TW.md"},
	{Code: "ja", Tag: "ja", Locale: "ja_JP", Name: "日本語", Readme: "README.ja.md"},
	{Code: "ko", Tag: "ko", Locale: "ko_KR", Name: "한국어", Readme: "README.ko.md"},
}

// sourceLang is the language the other content files are translated from, and the page at the root.
const sourceLang = "en"

// Path is the page's path on the site: the root for English, a folder named after the language for the others.
func (l language) Path() string {
	if l.Code == sourceLang {
		return "/"
	}
	return "/" + l.Code + "/"
}

// URL is the page's absolute address.
func (l language) URL() string { return siteURL + l.Path() }

// ReadmeURL is the README in this language on GitHub.
func (l language) ReadmeURL() string { return repoURL + "/blob/main/" + l.Readme }

// screenshots are the pictures of the app each page shows, as docs/assets/<language>/<name>.png: the player in the hero and the library beside the Library section.
var screenshots = []string{"library", "player"}

// surface is what the Command line and MCP section counts and lists, read from the app's source so that the page follows a command or tool added there.
type surface struct {
	// Commands is the number of the kando command's commands, apps/kando/cli's command table.
	Commands int
	// Tools are the names of the MCP server's tools, in apps/kando/mcpserver's order.
	Tools []string
}

var (
	commandRe = regexp.MustCompile(`(?m)^\s*\{name: "[a-z]+"`)
	toolRe    = regexp.MustCompile(`(?m)^\s*Name:\s+"(kando_[a-z_]+)"`)
)

// readSurface reads the commands of apps/kando/cli/cli.go and the tools of apps/kando/mcpserver/tools.go under root.
func readSurface(root string) (surface, error) {
	cli, err := os.ReadFile(filepath.Join(root, "apps", "kando", "cli", "cli.go"))
	if err != nil {
		return surface{}, err
	}
	tools, err := os.ReadFile(filepath.Join(root, "apps", "kando", "mcpserver", "tools.go"))
	if err != nil {
		return surface{}, err
	}
	var s surface
	s.Commands = len(commandRe.FindAll(cli, -1))
	for _, m := range toolRe.FindAllSubmatch(tools, -1) {
		s.Tools = append(s.Tools, string(m[1]))
	}
	if s.Commands == 0 || len(s.Tools) == 0 {
		return surface{}, fmt.Errorf("found %d commands in apps/kando/cli/cli.go and %d tools in apps/kando/mcpserver/tools.go; the patterns no longer match the code", s.Commands, len(s.Tools))
	}
	return s, nil
}

// shot is a screenshot as the page links it, with its size for the width and height attributes.
type shot struct {
	Src  string
	W, H int
}

// siteTile is a featured website of the catalog in the works-with strip: its name after its mark, which is AI Logo's where it has one (siteMarks), the logo of the app's catalog otherwise, or its initial on a colored plate when the catalog has none either, as the app's Supported Websites sheet draws it.
type siteTile struct {
	Name, Domain string
	// Lang is the language of Name when it is not the page's: a name written in Chinese characters on a page in another language is marked zh-Hans, the script of the catalog, so that a screen reader reads it as Chinese.
	Lang    string
	Logo    string
	Initial string
	// Plate is the index of the plate color under the initial, a class plate-<n> in site.css.
	Plate int
}

// page is what a page template is executed with.
type page struct {
	Lang   language
	Langs  []language
	Others []language
	Root   bool
	// NotFound is set on 404.html, which has no section links: GitHub Pages serves it at any missing address.
	NotFound bool
	// Missing is what 404.html says, in every language there is: one 404 page serves every address, so it cannot know the reader's language.
	Missing  []missingText
	CSS      string
	JS       string
	Shots    map[string]shot
	Sites    []siteTile
	Total    int
	Repo     string
	Rel      string
	Latest   string
	License  string
	Issues   string
	EveryAPI string
	// Version is the app's version in apps/kando/mygo.json, the release the Download buttons lead to.
	Version string
	// Surface is the commands and tools of the Command line and MCP section.
	Surface surface
	// AIServices are the marks of the AI card, and ClaudeCode the MCP tab's.
	AIServices []brand
	ClaudeCode brand
	// WebMCPJS is the script that offers the page's WebMCP tools, and WebMCP the data it reads from the page (webmcp.go).
	WebMCPJS string
	WebMCP   agentData
	// BuildCommands are the commands the Windows tab of the download panel shows.
	BuildCommands string
	// InstallCommand is the command the Linux tab shows.
	InstallCommand string
}

// missingText is 404.html's text in one language.
type missingText struct {
	Lang              language
	Title, Body, Home string
}

// OGImage is the absolute address of the picture link previews show: the library in the page's language.
func (p page) OGImage() string { return siteURL + p.Shots["library"].Src }

// report is what a build wrote, for main and the tests.
type report struct {
	// Langs are the languages that have a content file, and so a page.
	Langs []language
	// Used are the content keys the templates looked up.
	Used map[string]bool
	// AgentUsed are the content keys the WebMCP tools' content files were made from.
	AgentUsed map[string]bool
}

// build renders the site from the repository at root into out.
func build(root, out string) (*report, error) {
	siteDir := filepath.Join(root, "apps", "site")
	content, err := loadContent(filepath.Join(siteDir, "content"))
	if err != nil {
		return nil, err
	}
	var langs []language
	for _, l := range languages {
		if _, ok := content[l.Code]; ok {
			langs = append(langs, l)
		}
	}
	if err := resetOut(out); err != nil {
		return nil, err
	}

	static, err := copyStatic(filepath.Join(siteDir, "static"), filepath.Join(out, "static"))
	if err != nil {
		return nil, err
	}
	if err := writeIcons(filepath.Join(root, "apps", "kando", "resources", "icon.png"), out); err != nil {
		return nil, err
	}
	shots := map[string]map[string]shot{}
	for _, l := range langs {
		if shots[l.Code], err = copyShots(filepath.Join(root, "docs", "assets", l.Code), filepath.Join(out, "shots", l.Code), "/shots/"+l.Code+"/"); err != nil {
			return nil, err
		}
	}
	marks := newMarks(out)
	tiles, total, err := copySites(filepath.Join(root, "apps", "kando", "sites", "catalog"), filepath.Join(out, "sites"), marks)
	if err != nil {
		return nil, err
	}
	services, err := marks.brands(aiServices...)
	if err != nil {
		return nil, err
	}
	claude, err := marks.brands(claudeCode)
	if err != nil {
		return nil, err
	}
	siteNames, err := loadSiteNames(filepath.Join(siteDir, "content"), langs, tiles)
	if err != nil {
		return nil, err
	}

	version, err := appVersion(filepath.Join(root, "apps", "kando", "mygo.json"))
	if err != nil {
		return nil, err
	}
	surf, err := readSurface(root)
	if err != nil {
		return nil, err
	}

	agentUsed := map[string]bool{}
	agent, err := writeAgentFiles(root, out, langs, content, total, version, surf, agentUsed)
	if err != nil {
		return nil, err
	}

	used := map[string]bool{}
	tmpl, err := template.New("").Funcs(contentFuncs(nil, used)).ParseGlob(filepath.Join(siteDir, "templates", "*.html"))
	if err != nil {
		return nil, err
	}
	for _, l := range langs {
		p := page{
			Lang: l, Langs: langs, Root: l.Code == sourceLang,
			CSS: static["site.css"], JS: static["site.js"],
			Shots: shots[l.Code], Sites: localTiles(tiles, l, siteNames[l.Code]), Total: total,
			Repo: repoURL, Rel: releasesURL, Latest: latestURL, License: licenseURL, Issues: issuesURL, EveryAPI: everyAPIURL,
			Version: version, Surface: surf, AIServices: services, ClaudeCode: claude[0],
			WebMCPJS: static["webmcp.js"], WebMCP: agent[l.Code], BuildCommands: buildCommands, InstallCommand: installCommand,
		}
		for _, o := range langs {
			if o.Code != l.Code {
				p.Others = append(p.Others, o)
			}
		}
		if err := execute(tmpl, "page.html", content[l.Code], used, p, filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(l.Path(), "/")), "index.html")); err != nil {
			return nil, fmt.Errorf("%s: %w", l.Code, err)
		}
	}
	notFound := page{NotFound: true, Lang: languages[0], Langs: langs, CSS: static["site.css"], JS: static["site.js"], Shots: shots[sourceLang], Repo: repoURL, Rel: releasesURL, Latest: latestURL, License: licenseURL, Issues: issuesURL, EveryAPI: everyAPIURL, Version: version}
	for _, l := range langs {
		strs := content[l.Code]
		notFound.Missing = append(notFound.Missing, missingText{Lang: l, Title: strs["notFoundTitle"], Body: strs["notFoundBody"], Home: strs["notFoundHome"]})
	}
	for _, key := range []string{"notFoundTitle", "notFoundBody", "notFoundHome"} {
		used[key] = true
	}
	if err := execute(tmpl, "404.html", content[sourceLang], used, notFound, filepath.Join(out, "404.html")); err != nil {
		return nil, fmt.Errorf("404: %w", err)
	}
	if err := writeSitemap(filepath.Join(out, "sitemap.xml"), langs); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(out, "robots.txt"), []byte("User-agent: *\nAllow: /\n\nSitemap: "+siteURL+"/sitemap.xml\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(out, "CNAME"), []byte(strings.TrimPrefix(siteURL, "https://")+"\n"), 0o644); err != nil {
		return nil, err
	}
	return &report{Langs: langs, Used: used, AgentUsed: agentUsed}, nil
}

// placeholder is a value a content string names, such as {count}, filled in when the page is rendered.
var placeholder = regexp.MustCompile(`\{[a-zA-Z]+\}`)

// loadContent reads every content file there is, keyed by language code. en.json must exist; every other file must have exactly its keys, none empty, with the same placeholders in each string.
func loadContent(dir string) (map[string]map[string]string, error) {
	content := map[string]map[string]string{}
	for _, l := range languages {
		data, err := os.ReadFile(filepath.Join(dir, l.Code+".json"))
		if errors.Is(err, os.ErrNotExist) && l.Code != sourceLang {
			continue
		}
		if err != nil {
			return nil, err
		}
		var strs map[string]string
		if err := json.Unmarshal(data, &strs); err != nil {
			return nil, fmt.Errorf("content/%s.json: %w", l.Code, err)
		}
		content[l.Code] = strs
	}
	source := content[sourceLang]
	for code, strs := range content {
		if err := sameKeys(source, strs); err != nil {
			return nil, fmt.Errorf("content/%s.json: %w", code, err)
		}
		for key, s := range strs {
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("content/%s.json: %s is empty", code, key)
			}
			if want, got := placeholders(source[key]), placeholders(s); !slices.Equal(want, got) {
				return nil, fmt.Errorf("content/%s.json: %s has placeholders %v, en.json has %v", code, key, got, want)
			}
			if strings.Count(s, "`")%2 != 0 {
				return nil, fmt.Errorf("content/%s.json: %s has a ` that opens code it never closes", code, key)
			}
		}
		if !strings.Contains(strs["heroTitle"], strs["heroTitleMark"]) {
			return nil, fmt.Errorf("content/%s.json: heroTitleMark %q is not in heroTitle %q", code, strs["heroTitleMark"], strs["heroTitle"])
		}
	}
	return content, nil
}

// sameKeys reports the keys one of a and b has and the other does not.
func sameKeys[V any](a, b map[string]V) error {
	var missing, extra []string
	for k := range a {
		if _, ok := b[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			extra = append(extra, k)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return fmt.Errorf("keys differ from en.json: missing %v, extra %v", missing, extra)
}

func placeholders(s string) []string {
	found := placeholder.FindAllString(s, -1)
	sort.Strings(found)
	return found
}

// contentFuncs are the template functions that look up content strings: t "key", and tf "key" "name" value … that fills in {name}. A key that is not in strs, or a placeholder left unfilled, fails the page.
func contentFuncs(strs map[string]string, used map[string]bool) template.FuncMap {
	lookup := func(key string) (string, error) {
		s, ok := strs[key]
		if !ok {
			return "", fmt.Errorf("no content key %q", key)
		}
		used[key] = true
		return s, nil
	}
	return template.FuncMap{
		"rich":     rich,
		"headline": headline,
		"code": func(text, copyLabel, copiedLabel string) codeBlock {
			return codeBlock{Text: text, Copy: copyLabel, Copied: copiedLabel}
		},
		"t": lookup,
		"tf": func(key string, pairs ...any) (string, error) {
			s, err := lookup(key)
			if err != nil {
				return "", err
			}
			if len(pairs)%2 != 0 {
				return "", fmt.Errorf("tf %q: odd number of arguments", key)
			}
			for i := 0; i < len(pairs); i += 2 {
				s = strings.ReplaceAll(s, "{"+fmt.Sprint(pairs[i])+"}", fmt.Sprint(pairs[i+1]))
			}
			if left := placeholder.FindString(s); left != "" {
				return "", fmt.Errorf("tf %q: %s is not filled in", key, left)
			}
			return s, nil
		},
	}
}

// rich is s as HTML, with each pair of ` around a name, a file or a command set in <code>, as Kando's content strings mark them, and each line break a <br>, where a heading breaks in the design.
func rich(s string) template.HTML {
	parts := strings.Split(s, "`")
	var b strings.Builder
	for i, part := range parts {
		text := strings.ReplaceAll(template.HTMLEscapeString(part), "\n", "<br>")
		if i%2 == 1 {
			b.WriteString("<code>" + text + "</code>")
		} else {
			b.WriteString(text)
		}
	}
	return template.HTML(b.String())
}

// headline is the hero's title as HTML: its first mark set apart, and each line break of the content string a <br>.
func headline(title, mark string) template.HTML {
	before, after, _ := strings.Cut(title, mark)
	line := func(s string) string {
		return strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>")
	}
	return template.HTML(line(before) + `<em class="mark">` + line(mark) + "</em>" + line(after))
}

// codeBlock is a command the page shows with a button that copies it, the "code" template.
type codeBlock struct {
	Text, Copy, Copied string
}

// execute renders the template name with strs into the file at path.
func execute(tmpl *template.Template, name string, strs map[string]string, used map[string]bool, data page, path string) error {
	t, err := tmpl.Clone()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Funcs(contentFuncs(strs, used)).ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// resetOut empties out, refusing a folder that holds anything but a previous build, so that a wrong -out cannot wipe the repository.
func resetOut(out string) error {
	entries, err := os.ReadDir(out)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(out, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(out, "CNAME")); err != nil {
			return fmt.Errorf("%s is not empty and holds no previous build (no CNAME); choose another -out", out)
		}
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	return os.MkdirAll(out, 0o755)
}

// copyStatic copies the static files and returns, for each, the path pages link it by, with a hash of its content in the query so that a changed file is fetched again.
func copyStatic(src, dst string) (map[string]string, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, err
	}
	links := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return nil, err
		}
		if err := writeFile(filepath.Join(dst, e.Name()), data); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		links[e.Name()] = "/static/" + e.Name() + "?v=" + hex.EncodeToString(sum[:5])
	}
	return links, nil
}

// copyShots copies a language's screenshots and returns them with their sizes.
func copyShots(src, dst, prefix string) (map[string]shot, error) {
	shots := map[string]shot{}
	for _, name := range screenshots {
		file := name + ".png"
		data, err := os.ReadFile(filepath.Join(src, file))
		if err != nil {
			return nil, err
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(src, file), err)
		}
		if err := writeFile(filepath.Join(dst, file), data); err != nil {
			return nil, err
		}
		shots[name] = shot{Src: prefix + file, W: cfg.Width, H: cfg.Height}
	}
	return shots, nil
}

// catalog is the part of apps/kando/sites/catalog/sites.json the site reads.
type catalog struct {
	Sites []struct {
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		Logo     string `json:"logo"`
		Featured bool   `json:"featured"`
	} `json:"sites"`
}

// copySites writes the marks of the catalog's featured sites into the site, AI Logo's (marks) or the catalog's own logos, and returns their tiles and the number of sites in the catalog.
func copySites(src, dst string, marks *marks) ([]siteTile, int, error) {
	data, err := os.ReadFile(filepath.Join(src, "sites.json"))
	if err != nil {
		return nil, 0, err
	}
	var c catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, 0, fmt.Errorf("sites.json: %w", err)
	}
	var tiles []siteTile
	for _, s := range c.Sites {
		if !s.Featured {
			continue
		}
		tile := siteTile{Name: s.Name, Domain: s.Domain}
		switch id, ok := siteMarks[s.Domain]; {
		case ok:
			if tile.Logo, err = marks.src(id); err != nil {
				return nil, 0, fmt.Errorf("%s: %w", s.Domain, err)
			}
		case s.Logo != "":
			if err := copyFile(filepath.Join(src, "logos", s.Logo), filepath.Join(dst, s.Logo)); err != nil {
				return nil, 0, err
			}
			tile.Logo = "/sites/" + s.Logo
		default:
			tile.Initial, tile.Plate = initial(s.Name), plate(s.Domain)
		}
		tiles = append(tiles, tile)
	}
	if len(tiles) == 0 {
		return nil, 0, errors.New("sites.json lists no featured site")
	}
	return tiles, len(c.Sites), nil
}

// loadSiteNames reads content/sites.<language>.json, where a language has one: the names of featured sites as that language writes them, by domain, where they differ from the catalog's, such as the Traditional Chinese forms of names the catalog writes in Simplified Chinese. The catalog is the app's and stays as it is. A domain that is not a featured site fails the build, so that a typo does not silently leave the catalog's name in place.
func loadSiteNames(dir string, langs []language, tiles []siteTile) (map[string]map[string]string, error) {
	featured := map[string]bool{}
	for _, tile := range tiles {
		featured[tile.Domain] = true
	}
	names := map[string]map[string]string{}
	for _, l := range langs {
		file := "sites." + l.Code + ".json"
		data, err := os.ReadFile(filepath.Join(dir, file))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("content/%s: %w", file, err)
		}
		for domain, name := range m {
			if !featured[domain] {
				return nil, fmt.Errorf("content/%s: %s is not a featured site of sites.json", file, domain)
			}
			if strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("content/%s: the name of %s is empty", file, domain)
			}
		}
		names[l.Code] = m
	}
	return names, nil
}

// localTiles are the tiles as the page in l shows them: with the names l writes differently, and a name in Chinese characters on a page that is not in Chinese marked as Simplified Chinese.
func localTiles(tiles []siteTile, l language, names map[string]string) []siteTile {
	out := make([]siteTile, len(tiles))
	for i, tile := range tiles {
		if name, ok := names[tile.Domain]; ok {
			tile.Name = name
			if tile.Logo == "" {
				tile.Initial = initial(name)
			}
		}
		if !strings.HasPrefix(l.Tag, "zh") && strings.ContainsFunc(tile.Name, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			tile.Lang = "zh-Hans"
		}
		out[i] = tile
	}
	return out
}

// initial is the letter a site without a logo shows, as apps/kando/sites.Initial picks it: the first letter or digit of its name, upper case.
func initial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	if r, _ := utf8.DecodeRuneInString(name); r != utf8.RuneError {
		return string(r)
	}
	return "?"
}

// plateCount is the number of plate colors under initials: the app's eight (apps/kando/sites.plates), as .plate-0 to .plate-7 in site.css.
const plateCount = 8

// plate is the index of domain's plate color, the one the app gives it.
func plate(domain string) int {
	h := fnv.New32a()
	h.Write([]byte(domain))
	return int(h.Sum32() % plateCount)
}

var sitemapTemplate = texttemplate.Must(texttemplate.New("sitemap").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">
{{- range $page := .}}
  <url>
    <loc>{{$page.URL}}</loc>
{{- range $alt := $}}
    <xhtml:link rel="alternate" hreflang="{{$alt.Tag}}" href="{{$alt.URL}}"/>
{{- end}}
    <xhtml:link rel="alternate" hreflang="x-default" href="{{(index $ 0).URL}}"/>
  </url>
{{- end}}
</urlset>
`))

// writeSitemap lists every page with its alternates in the other languages.
func writeSitemap(path string, langs []language) error {
	var buf bytes.Buffer
	if err := sitemapTemplate.Execute(&buf, langs); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
