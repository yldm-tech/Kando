package catalog

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"testing"
)

func TestCatalogListsTheFeaturedSitesFirst(t *testing.T) {
	c := Load()
	if c.YTDLP == "" || len(c.Sites) < 500 {
		t.Fatalf("yt-dlp %q, %d sites", c.YTDLP, len(c.Sites))
	}
	if s := c.Sites[0]; s.Domain != "youtube.com" || s.Name != "YouTube" || !s.Featured {
		t.Fatalf("first site %+v", s)
	}
	featured := true
	for _, s := range c.Sites {
		if s.Featured && !featured {
			t.Fatalf("featured %s after the others", s.Domain)
		}
		featured = s.Featured
	}
}

func TestCatalogDomainsAreUniqueAndNamed(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Load().Sites {
		if s.Name == "" || s.Domain == "" {
			t.Fatalf("site %+v", s)
		}
		if seen[s.Domain] {
			t.Fatalf("%s twice", s.Domain)
		}
		seen[s.Domain] = true
	}
}

// Every logo a site names is embedded and decodes, and every embedded logo belongs to a site.
func TestCatalogLogosDecode(t *testing.T) {
	used := map[string]bool{}
	for _, s := range Load().Sites {
		if s.Logo == "" {
			continue
		}
		used[s.Logo] = true
		data := Logo(s)
		if data == nil {
			t.Fatalf("%s: %s is not embedded", s.Domain, s.Logo)
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("%s: %v", s.Logo, err)
		}
	}
	if len(used) == 0 {
		t.Fatal("no site has a logo")
	}
	entries, err := fs.ReadDir(files, "logos")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !used[e.Name()] {
			t.Errorf("logos/%s belongs to no site", e.Name())
		}
	}
}

func TestFilterFindsSitesByNameDomainAliasAndLink(t *testing.T) {
	c := Load()
	domains := func(query string) []string {
		var out []string
		for _, s := range c.Filter(query) {
			out = append(out, s.Domain)
		}
		return out
	}
	for _, tc := range []struct {
		query string
		want  string
	}{
		{"哔哩", "bilibili.com"},
		{"NICONICO", "nicovideo.jp"},
		{"x.com", "twitter.com"},
		{"https://www.bilibili.com/video/BV1xx411c7mD", "bilibili.com"},
		{"https://youtu.be/dQw4w9WgXcQ", "youtube.com"},
		{"www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube.com"},
		{"https://x.com/user/status/1", "twitter.com"},
		{"https://v.douyin.com/abc/", "douyin.com"},
	} {
		got := domains(tc.query)
		if len(got) == 0 || got[0] != tc.want {
			t.Errorf("%q found %v, want %s first", tc.query, got, tc.want)
		}
	}
	// A link finds only the site it is on, and a site yt-dlp does not know finds nothing.
	if got := domains("https://www.bilibili.com/video/BV1"); len(got) != 1 {
		t.Errorf("a bilibili link found %v", got)
	}
	if got := domains("https://unknown-video-site.example/watch/1"); len(got) != 0 {
		t.Errorf("an unknown site's link found %v", got)
	}
	if got := domains(""); len(got) != len(c.Sites) {
		t.Errorf("an empty search found %d of %d", len(got), len(c.Sites))
	}
}
