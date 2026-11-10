package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// repoRoot is the repository root as seen from this package's directory, where go test runs.
const repoRoot = "../.."

// built is the site rendered once for the whole test binary, into a temporary folder.
var built struct {
	out string
	rep *report
}

func TestMain(m *testing.M) {
	out, err := os.MkdirTemp("", "kando-site-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rep, err := build(repoRoot, out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.RemoveAll(out)
		os.Exit(1)
	}
	built.out, built.rep = out, rep
	code := m.Run()
	os.RemoveAll(out)
	os.Exit(code)
}

// readOut is a file of the built site.
func readOut(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(built.out, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// pageFile is the file a language's page is written to.
func pageFile(l language) string {
	return strings.TrimPrefix(l.Path(), "/") + "index.html"
}

func readJSON(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

// The site renders a page for each language that has a content file, English always among them, and each content file has exactly en.json's keys, none empty.
func TestContentFiles(t *testing.T) {
	dir := filepath.Join(repoRoot, "apps", "site", "content")
	en := readJSON(t, filepath.Join(dir, "en.json"))
	var want []string
	for _, l := range languages {
		path := filepath.Join(dir, l.Code+".json")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		want = append(want, l.Code)
		strs := readJSON(t, path)
		if err := sameKeys(en, strs); err != nil {
			t.Errorf("%s.json: %v", l.Code, err)
		}
		for key, s := range strs {
			if strings.TrimSpace(s) == "" {
				t.Errorf("%s.json: %s is empty", l.Code, key)
			}
		}
	}
	var got []string
	for _, l := range built.rep.Langs {
		got = append(got, l.Code)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rendered %v, want a page for each content file %v", got, want)
	}
	if len(got) == 0 || got[0] != sourceLang {
		t.Errorf("rendered %v, want %s first", got, sourceLang)
	}
}

// notes.json has a translator's note for every key of en.json, and none for a key that is gone.
func TestNotes(t *testing.T) {
	dir := filepath.Join(repoRoot, "apps", "site", "content")
	en := readJSON(t, filepath.Join(dir, "en.json"))
	notes := readJSON(t, filepath.Join(dir, "notes.json"))
	if err := sameKeys(en, notes); err != nil {
		t.Errorf("notes.json: %v", err)
	}
	for key, s := range notes {
		if strings.TrimSpace(s) == "" {
			t.Errorf("notes.json: %s is empty", key)
		}
	}
}

// Every key of en.json is shown somewhere, so that no string is translated for nothing.
func TestEveryKeyUsed(t *testing.T) {
	en := readJSON(t, filepath.Join(repoRoot, "apps", "site", "content", "en.json"))
	for key := range en {
		if !built.rep.Used[key] {
			t.Errorf("en.json: %s is not used by any template", key)
		}
	}
}

// A content file whose keys or placeholders differ from en.json's fails the build, rather than rendering a page with a hole.
func TestContentValidation(t *testing.T) {
	en := readJSON(t, filepath.Join(repoRoot, "apps", "site", "content", "en.json"))
	cases := map[string]func(map[string]string){
		"missing key":         func(m map[string]string) { delete(m, "heroTitle") },
		"extra key":           func(m map[string]string) { m["heroTitleTypo"] = "x" },
		"empty string":        func(m map[string]string) { m["heroTitle"] = " " },
		"missing placeholder": func(m map[string]string) { m["sitesMore"] = "and more" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(code string, m map[string]string) {
				data, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, code+".json"), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("en", en)
			ja := map[string]string{}
			for k, v := range en {
				ja[k] = v
			}
			change(ja)
			write("ja", ja)
			if _, err := loadContent(dir); err == nil {
				t.Error("loadContent accepted the changed ja.json")
			}
		})
	}
}

var (
	attrRe = regexp.MustCompile(`\s(href|src|content|action|srcset|poster)="([^"]*)"`)
	idRe   = regexp.MustCompile(`\sid="([^"]+)"`)
	// tagRe finds a start tag of the given element, to read its attributes.
	tagRe = func(name string) *regexp.Regexp { return regexp.MustCompile(`<` + name + `\s[^>]*>`) }
)

// attr is the value of attribute name in the start tag tag, or "" when it has none.
func attr(tag, name string) string {
	m := regexp.MustCompile(`\s` + regexp.QuoteMeta(name) + `="([^"]*)"`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1]
}

// Each page says its language, names its canonical address, lists every page as an alternate with x-default for the English one, has the social preview tags, and links each other page from its switcher.
func TestPageHead(t *testing.T) {
	for _, l := range built.rep.Langs {
		t.Run(l.Code, func(t *testing.T) {
			html := readOut(t, pageFile(l))
			if !strings.Contains(html, `<html lang="`+l.Tag+`"`) {
				t.Errorf("no <html lang=%q>", l.Tag)
			}
			links := tagRe("link").FindAllString(html, -1)
			canonical := ""
			alternates := map[string]string{}
			for _, tag := range links {
				switch attr(tag, "rel") {
				case "canonical":
					canonical = attr(tag, "href")
				case "alternate":
					alternates[attr(tag, "hreflang")] = attr(tag, "href")
				}
			}
			if canonical != l.URL() {
				t.Errorf("canonical %q, want %q", canonical, l.URL())
			}
			for _, o := range built.rep.Langs {
				if alternates[o.Tag] != o.URL() {
					t.Errorf("hreflang %s: %q, want %q", o.Tag, alternates[o.Tag], o.URL())
				}
			}
			if alternates["x-default"] != siteURL+"/" {
				t.Errorf("hreflang x-default: %q, want %q", alternates["x-default"], siteURL+"/")
			}
			if len(alternates) != len(built.rep.Langs)+1 {
				t.Errorf("%d alternates, want %d", len(alternates), len(built.rep.Langs)+1)
			}

			metas := map[string]string{}
			for _, tag := range tagRe("meta").FindAllString(html, -1) {
				key := attr(tag, "property")
				if key == "" {
					key = attr(tag, "name")
				}
				metas[key] = attr(tag, "content")
			}
			for _, key := range []string{"description", "og:title", "og:description", "og:image", "og:image:alt", "og:url", "og:locale", "twitter:card", "twitter:image"} {
				if metas[key] == "" {
					t.Errorf("no %s", key)
				}
			}
			if metas["og:url"] != l.URL() {
				t.Errorf("og:url %q, want %q", metas["og:url"], l.URL())
			}
			wantImage := siteURL + "/shots/" + l.Code + "/library.png"
			if metas["og:image"] != wantImage || metas["twitter:image"] != wantImage {
				t.Errorf("og:image %q, twitter:image %q, want %q", metas["og:image"], metas["twitter:image"], wantImage)
			}

			menu := html[strings.Index(html, `class="lang-menu"`):]
			menu = menu[:strings.Index(menu, "</ul>")]
			for _, o := range built.rep.Langs {
				link := fmt.Sprintf(`href="%s" hreflang="%s" lang="%s" data-lang="%s"`, o.Path(), o.Tag, o.Tag, o.Code)
				if !strings.Contains(menu, link) || !strings.Contains(menu, ">"+o.Name+"</a>") {
					t.Errorf("switcher has no link %s named %s", link, o.Name)
				}
			}
			if (l.Code == sourceLang) != strings.Contains(html, "data-root") {
				t.Errorf("data-root, which turns on the redirect by browser language, should be on the root page only")
			}
		})
	}
}

// Each page shows the screenshots in its own language, and the works-with strip names the catalog's count and its featured sites.
func TestPageBody(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "apps", "kando", "sites", "catalog", "sites.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c catalog
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	featured := 0
	for _, s := range c.Sites {
		if s.Featured {
			featured++
		}
	}
	for _, l := range built.rep.Langs {
		t.Run(l.Code, func(t *testing.T) {
			html := readOut(t, pageFile(l))
			for _, name := range screenshots {
				if !strings.Contains(html, `src="/shots/`+l.Code+`/`+name+`.png"`) {
					t.Errorf("no %s screenshot in %s", name, l.Code)
				}
			}
			for _, tag := range tagRe("img").FindAllString(html, -1) {
				if attr(tag, "width") == "" || attr(tag, "height") == "" {
					t.Errorf("image without width and height, which shifts the layout as it loads: %s", tag)
				}
				if !strings.Contains(tag, " alt=") {
					t.Errorf("image without alt: %s", tag)
				}
			}
			strip := html[strings.Index(html, `class="logos"`):]
			strip = strip[:strings.Index(strip, "</ul>")]
			if n := strings.Count(strip, "<li title="); n != featured {
				t.Errorf("%d sites in the strip, want the catalog's %d featured", n, featured)
			}
			for _, n := range []int{len(c.Sites), len(c.Sites) - featured} {
				if !strings.Contains(html, fmt.Sprint(n)) {
					t.Errorf("page does not show %d, from the catalog's %d sites", n, len(c.Sites))
				}
			}
			if !strings.Contains(html, `href="`+l.ReadmeURL()+`"`) {
				t.Errorf("no link to %s", l.ReadmeURL())
			}
			// The Download buttons open the newest published release, which stays current when a release comes out after the site was built; the footer lists every release.
			downloads := 0
			for _, tag := range tagRe("a").FindAllString(html, -1) {
				if attr(tag, "class") == "btn primary" {
					downloads++
					if href := attr(tag, "href"); href != latestURL {
						t.Errorf("Download button points to %q, want %q", href, latestURL)
					}
				}
			}
			if downloads != 2 {
				t.Errorf("%d Download buttons, want the hero's and the Get started card's", downloads)
			}
			if i := strings.Index(html, `class="foot-links"`); i < 0 {
				t.Errorf("no footer links nav")
			} else if end := strings.Index(html[i:], "</nav>"); end < 0 {
				t.Errorf("no footer links nav")
			} else if !strings.Contains(html[i:i+end], `href="`+releasesURL+`"`) {
				t.Errorf("the footer does not link the releases list %s", releasesURL)
			}
		})
	}
}

// logoStrip is the works-with strip of a built page.
func logoStrip(t *testing.T, html string) string {
	t.Helper()
	i := strings.Index(html, `class="logos"`)
	if i < 0 {
		t.Fatal("no logo strip")
	}
	strip := html[i:]
	return strip[:strings.Index(strip, "</ul>")]
}

// The names of the logo strip are written in the page's script: the Traditional Chinese page shows none of the catalog's Simplified forms, and a page in another language marks a name in Chinese characters as Simplified Chinese, so that a screen reader reads it in Chinese.
func TestLogoStripNames(t *testing.T) {
	nameAttr := regexp.MustCompile(`\s(?:title|alt|aria-label)="([^"]*)"`)
	for _, l := range built.rep.Langs {
		t.Run(l.Code, func(t *testing.T) {
			strip := logoStrip(t, readOut(t, pageFile(l)))
			if l.Code == "zh-TW" {
				for _, m := range nameAttr.FindAllStringSubmatch(strip, -1) {
					if strings.ContainsAny(m[1], "哔红视爱国优腾讯鱼") {
						t.Errorf("Simplified Chinese site name %q on the Traditional Chinese page", m[1])
					}
				}
				if !strings.Contains(strip, `title="嗶哩嗶哩"`) {
					t.Error("no 嗶哩嗶哩 in the strip")
				}
			}
			for _, tag := range tagRe("li").FindAllString(strip, -1) {
				name := attr(tag, "title")
				if name == "" {
					continue
				}
				han := strings.ContainsFunc(name, func(r rune) bool { return unicode.Is(unicode.Han, r) })
				want := ""
				if han && !strings.HasPrefix(l.Tag, "zh") {
					want = "zh-Hans"
				}
				if got := attr(tag, "lang"); got != want {
					t.Errorf("%s: lang %q, want %q", name, got, want)
				}
			}
		})
	}
}

// A site-name override for a domain that is not a featured site fails the build, rather than leaving the catalog's name in place unnoticed.
func TestSiteNamesRejectUnknownDomain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sites.zh-TW.json"), []byte(`{"bilibil.com": "嗶哩嗶哩"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tiles := []siteTile{{Name: "哔哩哔哩", Domain: "bilibili.com"}}
	if _, err := loadSiteNames(dir, languages, tiles); err == nil {
		t.Error("loadSiteNames accepted a domain that is not a featured site")
	}
}

// 404.html is no language's home page, so its switchers mark no link as the current page.
func TestNotFoundHasNoCurrentPage(t *testing.T) {
	if html := readOut(t, "404.html"); strings.Contains(html, "aria-current") {
		t.Error("404.html marks a language's home page as the current page")
	}
	for _, l := range built.rep.Langs {
		html := readOut(t, pageFile(l))
		if n := strings.Count(html, `data-lang="`+l.Code+`" aria-current="page"`); n != 2 {
			t.Errorf("%s: %d switcher links marked as the current page, want 2 (header and footer)", l.Code, n)
		}
	}
}

// htmlFiles are the built site's HTML files, relative to its root.
func htmlFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(built.out, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".html") {
			rel, _ := filepath.Rel(built.out, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// isExternal reports whether ref leaves the site.
func isExternal(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "//")
}

// Every local href and src names a file of the built site, and every #fragment an element of the page it points into.
func TestLocalLinksResolve(t *testing.T) {
	ids := map[string]map[string]bool{}
	for _, file := range htmlFiles(t) {
		ids[file] = map[string]bool{}
		for _, m := range idRe.FindAllStringSubmatch(readOut(t, file), -1) {
			ids[file][m[1]] = true
		}
	}
	for _, file := range htmlFiles(t) {
		for _, m := range attrRe.FindAllStringSubmatch(readOut(t, file), -1) {
			name, ref := m[1], m[2]
			if name == "content" || isExternal(ref) || strings.HasPrefix(ref, "mailto:") {
				continue
			}
			ref, fragment, _ := strings.Cut(ref, "#")
			ref, _, _ = strings.Cut(ref, "?")
			target := file
			if ref != "" {
				if !strings.HasPrefix(ref, "/") {
					t.Errorf("%s: %s=%q is relative; links are absolute paths so that 404.html works at any address", file, name, m[2])
					continue
				}
				target = strings.TrimPrefix(ref, "/")
				if target == "" || strings.HasSuffix(target, "/") {
					target += "index.html"
				}
				if _, err := os.Stat(filepath.Join(built.out, filepath.FromSlash(target))); err != nil {
					t.Errorf("%s: %s=%q names no file of the site", file, name, m[2])
					continue
				}
			}
			if fragment != "" && !ids[target][fragment] {
				t.Errorf("%s: %s=%q: %s has no element with id %q", file, name, m[2], target, fragment)
			}
		}
	}
}

// allowedExternal are the only places outside the site a page may point to: the repository (with its releases, license and READMEs), EveryAPI, and the site's own absolute address in its meta tags.
var allowedExternal = []string{repoURL, everyAPIURL, siteURL}

func allowedRef(ref string) bool {
	for _, base := range allowedExternal {
		if ref == base {
			return true
		}
		if rest, ok := strings.CutPrefix(ref, base); ok && (strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "?") || strings.HasPrefix(rest, "#")) {
			return true
		}
	}
	return false
}

// The pages load nothing from elsewhere and link nowhere but the repository and EveryAPI: no fonts, scripts, analytics or CDNs.
func TestNoExternalHosts(t *testing.T) {
	for _, file := range htmlFiles(t) {
		for _, m := range attrRe.FindAllStringSubmatch(readOut(t, file), -1) {
			if ref := m[2]; isExternal(ref) && !allowedRef(ref) {
				t.Errorf("%s: %s=%q points outside the allowed hosts", file, m[1], ref)
			}
		}
	}
	urlRe := regexp.MustCompile(`(?:https?:)?//[A-Za-z0-9.-]+\.[A-Za-z]{2,}[^\s"')]*`)
	for _, file := range []string{"static/site.css", "static/site.js", "static/webmcp.js"} {
		for _, ref := range urlRe.FindAllString(readOut(t, file), -1) {
			if !allowedRef(ref) {
				t.Errorf("%s refers to %s", file, ref)
			}
		}
	}
	for _, re := range []string{`<script>`, `<style`, ` style="`, ` on[a-z]+="`} {
		for _, file := range htmlFiles(t) {
			if regexp.MustCompile(re).MatchString(readOut(t, file)) {
				t.Errorf("%s has inline script or style (%s)", file, re)
			}
		}
	}
}

// The sitemap lists each page with all of its alternates, and robots.txt and CNAME point at the site's address.
func TestSitemapRobotsCNAME(t *testing.T) {
	sitemap := readOut(t, "sitemap.xml")
	if n := strings.Count(sitemap, "<loc>"); n != len(built.rep.Langs) {
		t.Errorf("sitemap lists %d pages, want %d", n, len(built.rep.Langs))
	}
	for _, l := range built.rep.Langs {
		if !strings.Contains(sitemap, "<loc>"+l.URL()+"</loc>") {
			t.Errorf("sitemap has no <loc> for %s", l.URL())
		}
		alt := fmt.Sprintf(`<xhtml:link rel="alternate" hreflang="%s" href="%s"/>`, l.Tag, l.URL())
		if n := strings.Count(sitemap, alt); n != len(built.rep.Langs) {
			t.Errorf("sitemap names %s as an alternate %d times, want once per page", l.URL(), n)
		}
	}
	if n := strings.Count(sitemap, `hreflang="x-default" href="`+siteURL+`/"`); n != len(built.rep.Langs) {
		t.Errorf("sitemap has %d x-default alternates, want one per page", n)
	}
	if got := readOut(t, "CNAME"); got != "kando.yldm.ai\n" {
		t.Errorf("CNAME %q", got)
	}
	if robots := readOut(t, "robots.txt"); !strings.Contains(robots, "Sitemap: "+siteURL+"/sitemap.xml") {
		t.Errorf("robots.txt does not name the sitemap:\n%s", robots)
	}
	notFound := readOut(t, "404.html")
	if !strings.Contains(notFound, `<meta name="robots" content="noindex">`) {
		t.Error("404.html is not marked noindex")
	}
}

// The icons made from the app icon have their sizes.
func TestIcons(t *testing.T) {
	for file, size := range map[string]int{"logo.png": logoSize, "apple-touch-icon.png": 180, "favicon-32.png": 32, "favicon-16.png": 16} {
		f, err := os.Open(filepath.Join(built.out, file))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if cfg.Width != size || cfg.Height != size {
			t.Errorf("%s is %d×%d, want %d×%d", file, cfg.Width, cfg.Height, size, size)
		}
	}
	ico := readOut(t, "favicon.ico")
	if len(ico) < 6 || ico[2] != 1 || ico[4] != 2 {
		t.Errorf("favicon.ico has no header for two images")
	}
}

// build will not empty a folder that holds something other than a previous build, so a mistyped -out cannot wipe the repository.
func TestBuildRefusesForeignFolder(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := build(repoRoot, out); err == nil {
		t.Fatal("build emptied a folder that held no previous build")
	}
	if _, err := os.Stat(filepath.Join(out, "keep.txt")); err != nil {
		t.Errorf("keep.txt is gone: %v", err)
	}
}

// Under its headline each page sets the headline of another page in that page's language: the Simplified Chinese one on the English page, the English one on every other.
func TestHeadlineEcho(t *testing.T) {
	content, err := loadContent(filepath.Join(repoRoot, "apps", "site", "content"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range built.rep.Langs {
		t.Run(l.Code, func(t *testing.T) {
			want := echoLang(l, built.rep.Langs)
			if want.Code == l.Code {
				t.Fatalf("the %s page echoes itself", l.Code)
			}
			line := fmt.Sprintf(`<p class="echo" lang="%s" aria-hidden="true">%s</p>`, want.Tag, template.HTMLEscapeString(content[want.Code]["heroTitle"]))
			if !strings.Contains(readOut(t, pageFile(l)), line) {
				t.Errorf("no %s", line)
			}
		})
	}
}
