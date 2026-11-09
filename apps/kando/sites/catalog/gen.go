//go:build ignore

// gen writes sites.json and logos/ from the yt-dlp on PATH: one site per registrable domain its working extractors download from, with the site's favicon. Run it with `go generate ./sites/catalog` from apps/kando after yt-dlp updates, and review the diff.
//
// It needs a yt-dlp installed as a Python package (Homebrew's or pip's), as it imports yt_dlp to list the extractors; the standalone binary cannot be imported. The favicons come from Google's favicon service, one request per domain.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

// dumpScript prints every working, listed extractor but the generic one as JSON: its name, description and the hosts of its test URLs, in the order yt-dlp lists them.
const dumpScript = `
import json, sys, urllib.parse
from yt_dlp.extractor import gen_extractor_classes
out = []
for ie in gen_extractor_classes():
    if ie.IE_NAME == 'generic' or ie.IE_DESC is False or not ie.working():
        continue
    hosts = []
    for t in ie.get_testcases(include_onlymatching=True):
        h = (urllib.parse.urlparse(t.get('url') or '').hostname or '').lower()
        if h and h not in hosts:
            hosts.append(h)
    out.append({'name': ie.IE_NAME, 'desc': ie.IE_DESC or '', 'hosts': hosts})
json.dump(out, sys.stdout, ensure_ascii=False)
`

type extractor struct {
	Name  string   `json:"name"`
	Desc  string   `json:"desc"`
	Hosts []string `json:"hosts"`
}

// site is an entry of sites.json; catalog.go reads the same fields.
type site struct {
	Name     string   `json:"name"`
	Domain   string   `json:"domain"`
	Aliases  []string `json:"aliases,omitempty"`
	Logo     string   `json:"logo,omitempty"`
	Featured bool     `json:"featured,omitempty"`
}

type snapshot struct {
	YTDLP string `json:"ytdlp"`
	Sites []site `json:"sites"`
}

// featured are the sites listed first, in this order, under the names people know them by. A domain here that yt-dlp no longer downloads from is left out.
var featured = []struct {
	domain, name string
	// aliases are more words the search finds the site by. An alias that is a domain yt-dlp downloads from too, as youtu.be of YouTube, folds that domain's extractors into the site.
	aliases []string
}{
	{"youtube.com", "YouTube", []string{"youtu.be"}},
	{"bilibili.com", "哔哩哔哩", nil},
	{"douyin.com", "抖音", nil},
	{"tiktok.com", "TikTok", nil},
	{"twitter.com", "X (Twitter)", []string{"x.com"}},
	{"instagram.com", "Instagram", nil},
	{"facebook.com", "Facebook", nil},
	{"vimeo.com", "Vimeo", nil},
	{"twitch.tv", "Twitch", nil},
	{"nicovideo.jp", "niconico", nil},
	{"weibo.com", "微博", nil},
	{"xiaohongshu.com", "小红书", nil},
	{"ixigua.com", "西瓜视频", nil},
	{"iqiyi.com", "爱奇艺", nil},
	{"iq.com", "iQIYI 国际版", nil},
	{"youku.com", "优酷", nil},
	{"qq.com", "腾讯视频", []string{"v.qq.com"}},
	{"wetv.vip", "WeTV", nil},
	{"mgtv.com", "芒果 TV", nil},
	{"acfun.cn", "AcFun", nil},
	{"huya.com", "虎牙", nil},
	{"douyu.com", "斗鱼", nil},
	{"abema.tv", "ABEMA", nil},
	{"tver.jp", "TVer", nil},
	{"nhk.or.jp", "NHK", nil},
	{"reddit.com", "Reddit", nil},
	{"dailymotion.com", "Dailymotion", nil},
	{"ted.com", "TED", nil},
	{"bbc.co.uk", "BBC", nil},
	{"soundcloud.com", "SoundCloud", nil},
	{"bandcamp.com", "Bandcamp", nil},
	{"kick.com", "Kick", nil},
	{"rumble.com", "Rumble", nil},
	{"vk.com", "VK", nil},
	{"naver.com", "NAVER", nil},
	{"afreecatv.com", "SOOP (AfreecaTV)", nil},
	{"sooplive.co.kr", "SOOP", nil},
}

func main() {
	ytdlp := flag.String("ytdlp", "yt-dlp", "the yt-dlp to list the extractors of")
	out := flag.String("out", ".", "the folder to write sites.json and logos/ into")
	withLogos := flag.Bool("logos", true, "fetch the favicons again; with -logos=false the sites keep the logos already in logos/")
	flag.Parse()
	log.SetFlags(0)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	path, err := exec.LookPath(*ytdlp)
	if err != nil {
		log.Fatal(err)
	}
	version, err := run(ctx, path, "--version")
	if err != nil {
		log.Fatalf("yt-dlp --version: %v", err)
	}
	python := pythonOf(path)
	if python == "" {
		log.Fatalf("%s is not a Python script; install yt-dlp with Homebrew or pip", path)
	}
	raw, err := run(ctx, python, "-I", "-c", dumpScript)
	if err != nil {
		log.Fatalf("listing the extractors: %v", err)
	}
	var ies []extractor
	if err := json.Unmarshal([]byte(raw), &ies); err != nil {
		log.Fatal(err)
	}
	sites := group(ies)
	logos := filepath.Join(*out, "logos")
	if *withLogos {
		if err := os.RemoveAll(logos); err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(logos, 0o755); err != nil {
			log.Fatal(err)
		}
		fetchLogos(ctx, sites, logos)
	} else {
		keepLogos(sites, logos)
	}
	data, err := json.MarshalIndent(snapshot{YTDLP: strings.TrimSpace(version), Sites: sites}, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "sites.json"), append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	have := 0
	for _, s := range sites {
		if s.Logo != "" {
			have++
		}
	}
	log.Printf("yt-dlp %s: %d extractors, %d sites, %d with a logo", strings.TrimSpace(version), len(ies), len(sites), have)
}

// group makes a site of each registrable domain an extractor's first test URL is on: the extractors' other hosts are often embeds and mirrors of other sites. A site is named after its featured name, or else its main extractor (mainExtractor); the other extractors' names are its aliases, for the search. Featured sites come first in their order, then the rest by name.
func group(ies []extractor) []site {
	folded := map[string]string{}
	for _, f := range featured {
		for _, alias := range f.aliases {
			folded[alias] = f.domain
		}
	}
	byDomain := map[string][]extractor{}
	var order []string
	for _, ie := range ies {
		if len(ie.Hosts) == 0 {
			continue
		}
		domain, ok := registrable(ie.Hosts[0])
		if !ok {
			continue
		}
		if into, ok := folded[domain]; ok {
			domain = into
		}
		if _, seen := byDomain[domain]; !seen {
			order = append(order, domain)
		}
		byDomain[domain] = append(byDomain[domain], ie)
	}
	rank := map[string]int{}
	for i, f := range featured {
		rank[f.domain] = i
	}
	var sites []site
	for _, domain := range order {
		group := byDomain[domain]
		main := mainExtractor(group)
		s := site{Domain: domain, Name: displayName(main)}
		var extra []string
		if i, ok := rank[domain]; ok {
			s.Name, s.Featured, extra = featured[i].name, true, featured[i].aliases
		}
		seen := map[string]bool{strings.ToLower(s.Name): true, domain: true}
		for _, alias := range slices.Concat(extra, mapNames(group)) {
			if key := strings.ToLower(alias); !seen[key] {
				seen[key] = true
				s.Aliases = append(s.Aliases, alias)
			}
		}
		sites = append(sites, s)
	}
	slices.SortStableFunc(sites, func(a, b site) int {
		if a.Featured != b.Featured {
			if a.Featured {
				return -1
			}
			return 1
		}
		if a.Featured {
			return rank[a.Domain] - rank[b.Domain]
		}
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.Domain, b.Domain)
	})
	return sites
}

// mapNames is the names of group's extractors before any ':' section.
func mapNames(group []extractor) []string {
	names := make([]string, len(group))
	for i, ie := range group {
		names[i] = baseName(ie.Name)
	}
	return names
}

// registrable is host's registrable domain, as youtube.com of www.youtube.com and bbc.co.uk of www.bbc.co.uk. It reports false for an IP address or a host that has none.
func registrable(host string) (string, bool) {
	host = strings.TrimSuffix(host, ".")
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return "", false
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", false
	}
	return domain, true
}

// mainExtractor is the extractor of a site whose name is the site's own: one without a ':' section, the shortest of them, and the first yt-dlp lists of the same length.
func mainExtractor(group []extractor) extractor {
	best := group[0]
	score := func(ie extractor) int {
		n := len(ie.Name)
		if strings.Contains(ie.Name, ":") {
			n += 1000
		}
		return n
	}
	for _, ie := range group[1:] {
		if score(ie) < score(best) {
			best = ie
		}
	}
	return best
}

// displayName is what a site is called after its main extractor: its description when that reads as a name, else its name before any ':' section.
func displayName(ie extractor) string {
	if d := strings.TrimSpace(ie.Desc); d != "" && len(d) <= 32 && !strings.ContainsAny(d, ";:,()\"") && !strings.Contains(d, ".") && len(strings.Fields(d)) <= 4 && !genericWords(d) {
		return d
	}
	return trimKind(baseName(ie.Name))
}

// kindSuffixes are the endings of an extractor's name that say what it takes rather than which site, as Episode of OnDemandChinaEpisode.
var kindSuffixes = []string{"AppIntl", "Episode", "Episodes", "Videos", "Video", "VOD", "Show", "Series", "Channel", "Playlist", "Embed", "Player", "Clips", "Clip", "Page", "Program", "Live"}

// trimKind takes one of kindSuffixes off a CamelCase name, when what is left is still a name of three letters or more.
func trimKind(name string) string {
	for _, suffix := range kindSuffixes {
		rest, ok := strings.CutSuffix(name, suffix)
		if ok && len(rest) >= 3 && strings.ToLower(rest) != rest {
			return rest
		}
	}
	return name
}

// genericWords reports whether a description says what an extractor takes rather than naming the site, as "YouTube playlists".
func genericWords(desc string) bool {
	for _, w := range strings.Fields(strings.ToLower(desc)) {
		switch w {
		case "video", "videos", "playlist", "playlists", "embed", "embeds", "search", "channel", "channels", "user", "users", "live", "clips", "series", "episodes", "audio", "podcast", "podcasts", "and", "of", "the":
			return true
		}
	}
	return false
}

// baseName is an extractor's name before its first ':' section.
func baseName(name string) string {
	base, _, _ := strings.Cut(name, ":")
	return strings.TrimSpace(base)
}

// fetchLogos saves each site's favicon into dir as <domain>.png or .jpg and records its file name, a few at a time. A site without one keeps no logo, and the app draws its initial.
func fetchLogos(ctx context.Context, sites []site, dir string) {
	client := &http.Client{Timeout: 20 * time.Second}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range jobs {
				name, err := fetchLogo(ctx, client, sites[i].Domain, dir)
				if err != nil {
					log.Printf("%s: %v", sites[i].Domain, err)
					continue
				}
				sites[i].Logo = name
			}
		})
	}
	for i := range sites {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

// keepLogos points each site at its logo already in dir, if there is one, and removes the logos of domains that are no longer a site.
func keepLogos(sites []site, dir string) {
	used := map[string]bool{}
	for i, s := range sites {
		for _, ext := range []string{".png", ".jpg"} {
			if _, err := os.Stat(filepath.Join(dir, s.Domain+ext)); err == nil {
				sites[i].Logo = s.Domain + ext
				used[sites[i].Logo] = true
				break
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		if !used[e.Name()] {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				log.Fatal(err)
			}
		}
	}
}

var errNoLogo = errors.New("no favicon")

// fetchLogo asks Google's favicon service for domain's 64-point favicon, trying again twice on a network error. It answers 404, with its default globe, for a domain it has none of.
func fetchLogo(ctx context.Context, client *http.Client, domain, dir string) (string, error) {
	url := "https://www.google.com/s2/favicons?sz=64&domain=" + domain
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		var data []byte
		var status int
		data, status, err = get(ctx, client, url)
		if err != nil {
			continue
		}
		if status == http.StatusNotFound {
			return "", errNoLogo
		}
		if status != http.StatusOK {
			err = fmt.Errorf("HTTP %d", status)
			continue
		}
		ext := ""
		switch http.DetectContentType(data) {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		default:
			return "", fmt.Errorf("not a PNG or JPEG: %s", http.DetectContentType(data))
		}
		name := domain + ext
		return name, os.WriteFile(filepath.Join(dir, name), data, 0o644)
	}
	return "", err
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return data, resp.StatusCode, err
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}

// pythonOf is the interpreter of a yt-dlp installed as a Python script, from its #! line, or "" for another kind of yt-dlp.
func pythonOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "#!") || !strings.Contains(line, "python") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	if filepath.Base(fields[0]) == "env" && len(fields) > 1 {
		p, err := exec.LookPath(fields[1])
		if err != nil {
			return ""
		}
		return p
	}
	return fields[0]
}
