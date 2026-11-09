// Package catalog is the list of websites Kando downloads from through yt-dlp, with their logos, as gen.go took it from a yt-dlp release. It imports no area of the window, so any area can list the sites.
package catalog

import (
	"embed"
	"encoding/json"
	"net/url"
	"path"
	"strings"
	"sync"
)

//go:generate go run gen.go

//go:embed sites.json logos
var files embed.FS

// Site is a website yt-dlp downloads from: what it is called, its registrable domain, more names the search finds it by (its yt-dlp extractors' names, and other domains), and the file of its logo in logos/, or "" when it has none.
type Site struct {
	Name    string   `json:"name"`
	Domain  string   `json:"domain"`
	Aliases []string `json:"aliases,omitempty"`
	Logo    string   `json:"logo,omitempty"`
	// Featured marks the well-known sites listed first.
	Featured bool `json:"featured,omitempty"`
}

// Catalog is the snapshot: the yt-dlp version it was taken from and its sites, the featured ones first.
type Catalog struct {
	YTDLP string `json:"ytdlp"`
	Sites []Site `json:"sites"`
}

// Load is the catalog embedded in the app, read once.
var Load = sync.OnceValue(func() Catalog {
	data, err := files.ReadFile("sites.json")
	if err != nil {
		panic("catalog: " + err.Error())
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		panic("catalog: sites.json: " + err.Error())
	}
	return c
})

// Logo is the bytes of s's logo, a PNG or JPEG, or nil when it has none.
func Logo(s Site) []byte {
	if s.Logo == "" {
		return nil
	}
	data, err := files.ReadFile(path.Join("logos", s.Logo))
	if err != nil {
		return nil
	}
	return data
}

// Matches reports whether s is found by query. A link finds the site it is on: its host is the site's domain, or a domain among its aliases, or under one, as www.bilibili.com/video/… finds bilibili.com and youtu.be/… YouTube. Other text finds a site when every word of it, ignoring case, is part of its name, its domain or one of its aliases. An empty query finds every site.
func (s Site) Matches(query string) bool {
	if host := linkHost(query); host != "" {
		for _, domain := range append([]string{s.Domain}, s.Aliases...) {
			if strings.Contains(domain, ".") && (host == domain || strings.HasSuffix(host, "."+domain)) {
				return true
			}
		}
		return false
	}
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return true
	}
	hay := strings.ToLower(s.Name + " " + s.Domain + " " + strings.Join(s.Aliases, " "))
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// linkHost is the host of query when it is a link, with or without its scheme, as https://www.youtube.com/watch?v=… or youtu.be/…; "" otherwise.
func linkHost(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if strings.ContainsAny(q, " \t") || !strings.Contains(q, ".") {
		return ""
	}
	if !strings.Contains(q, "://") {
		if !strings.Contains(q, "/") {
			return ""
		}
		q = "https://" + q
	}
	u, err := url.Parse(q)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u.Hostname(), ".")
}

// Filter is the sites of c that query finds (Matches), in their order.
func (c Catalog) Filter(query string) []Site {
	if strings.TrimSpace(query) == "" {
		return c.Sites
	}
	var out []Site
	for _, s := range c.Sites {
		if s.Matches(query) {
			out = append(out, s)
		}
	}
	return out
}
