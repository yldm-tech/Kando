package core

import (
	"errors"
	"strings"
	"testing"
)

// Kando's own sites are read alike by Go and JavaScript, which is what ValidateVideoSites holds every site to.
func TestVideoSitesAreValid(t *testing.T) {
	if err := ValidateVideoSites(VideoSites); err != nil {
		t.Fatal(err)
	}
}

// site is a site of one shape whose path is path.
func site(path string) VideoSite {
	return VideoSite{Domain: "example.com", Videos: []VideoShape{{Path: path}}}
}

// A pattern a backtracking engine, as JavaScript's, takes exponential or high polynomial time on is refused, as are those Go and JavaScript read apart: flags, named groups, lookaround, backreferences, Unicode classes, braces that are no count and a class that starts with ]. So are a pattern over MaxVideoPattern and one that is not anchored.
func TestValidateVideoSiteRefusesPatterns(t *testing.T) {
	for _, p := range []string{
		`^/(a+)+$`,
		`^/(a*)*$`,
		`^/(a|aa)+$`,
		`^/(\w+\d+)*$`,
		`^/([a-z]+/)+$`,
		`^/(a{2,})+$`,
		`^/(.*a){10}$`,
		`^/(a?){20}$`,
		`^/(ab|cd)*$`,
		`^/\w*\w*\w*x$`,
		`^/(a)\1$`,
		`^/(?=a)a$`,
		`^/(?!a)b$`,
		`^/(?<id>[0-9]+)$`,
		`^/(?i)abc$`,
		`^/\pL+$`,
		`^/\s+$`,
		`^/\bword$`,
		`^/a{,5}$`,
		`^/a{1000000}$`,
		`^/({2})$`,
		`^/a}$`,
		`^/[]a]$`,
		`^/[^]a]$`,
		`^/[a-z$`,
		`^/` + strings.Repeat("a", MaxVideoPattern) + `$`,
		`/video/[0-9]+$`,
		`^/video/[0-9]+`,
		`^/vidéo$`,
		`^/a b$`,
		`^/a#b$`,
		`^/(\w|\d)+$`,
		`^/(a|a)+$`,
		`^/(a|b)+$`,
		`^/(ab|ab)+$`,
		`^/((a|b)c)+$`,
		`^/(x(a)?)*$`,
		`^/\w{0,999}\w{0,999}\w{0,999}x$`,
		`^/[a-z]{0,100}[a-z]{0,100}[a-z]+x$`,
		`^*/$`,
		`^/$*`,
		`^/a$+`,
		`^/(*a)$`,
		`^/(a|*b)$`,
		`^/a**$`,
		`^/a+??$`,
		`^/(a$`,
		`^/a)$`,
	} {
		err := ValidateVideoSite(site(p))
		if !errors.Is(err, ErrVideoSite) {
			t.Errorf("%q: %v, want ErrVideoSite", p, err)
		}
	}
}

// The shapes of Kando's own sites, and patterns like them, are taken.
func TestValidateVideoSiteTakesPatterns(t *testing.T) {
	for _, p := range []string{
		`^/watch/?$`,
		`^/(shorts|live)/[\w-]{6,}/?$`,
		`^/(` + BilibiliLocale + `/)?video/(BV[0-9A-Za-z]{10}|av[0-9]+)/?$`,
		`^/@[^/]+/video/[0-9]+/?$`,
		`^/talks/[\w-]+/?$`,
		`^/v/[0-9a-f]{8}-[0-9a-f]{4}$`,
		`^/watch/\d+\.html$`,
		`^/v/[0-9]+?/?$`,
		`^/(v|video)/[0-9]{1,16}$`,
	} {
		if err := ValidateVideoSite(site(p)); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	ok := VideoSite{Domain: "videos.example.org", Not: []string{"live.videos.example.org"}, Videos: []VideoShape{{Path: `^/watch$`, Param: "v", Value: `^[0-9]{3,12}$`}}}
	if err := ValidateVideoSite(ok); err != nil {
		t.Fatal(err)
	}
}

// A site's domain is a host name, its Not hosts under it, and a shape names a parameter exactly when it names a value, anchored too; in a list, no domain is another's or under it.
func TestValidateVideoSiteRefusesSites(t *testing.T) {
	shape := []VideoShape{{Path: `^/v/[0-9]+$`}}
	for name, s := range map[string]VideoSite{
		"no domain":         {Videos: shape},
		"one label":         {Domain: "localhost", Videos: shape},
		"uppercase":         {Domain: "Example.com", Videos: shape},
		"address":           {Domain: "127.0.0.1", Videos: shape},
		"port":              {Domain: "example.com:8080", Videos: shape},
		"scheme":            {Domain: "https://example.com", Videos: shape},
		"trailing dot":      {Domain: "example.com.", Videos: shape},
		"hyphen":            {Domain: "-example.com", Videos: shape},
		"not elsewhere":     {Domain: "example.com", Not: []string{"other.org"}, Videos: shape},
		"not the domain":    {Domain: "example.com", Not: []string{"example.com"}, Videos: shape},
		"not twice":         {Domain: "example.com", Not: []string{"a.example.com", "a.example.com"}, Videos: shape},
		"no shape":          {Domain: "example.com"},
		"param no value":    {Domain: "example.com", Videos: []VideoShape{{Path: `^/watch$`, Param: "v"}}},
		"value no param":    {Domain: "example.com", Videos: []VideoShape{{Path: `^/watch$`, Value: `^[0-9]+$`}}},
		"bad param":         {Domain: "example.com", Videos: []VideoShape{{Path: `^/watch$`, Param: "v=1", Value: `^[0-9]+$`}}},
		"value not ^...$":   {Domain: "example.com", Videos: []VideoShape{{Path: `^/watch$`, Param: "v", Value: `[0-9]+`}}},
		"value nested":      {Domain: "example.com", Videos: []VideoShape{{Path: `^/watch$`, Param: "v", Value: `^([0-9]+)+$`}}},
		"empty path":        {Domain: "example.com", Videos: []VideoShape{{}}},
		"path without root": {Domain: "example.com", Videos: []VideoShape{{Path: `^watch$`}}},
	} {
		if err := ValidateVideoSite(s); !errors.Is(err, ErrVideoSite) {
			t.Errorf("%s: %v, want ErrVideoSite", name, err)
		}
	}
	a := VideoSite{Domain: "example.com", Videos: shape}
	for _, other := range []string{"example.com", "v.example.com"} {
		b := VideoSite{Domain: other, Videos: shape}
		if err := ValidateVideoSites([]VideoSite{a, b}); !errors.Is(err, ErrVideoSite) {
			t.Errorf("example.com and %s: %v, want ErrVideoSite", other, err)
		}
		if err := ValidateVideoSites([]VideoSite{b, a}); !errors.Is(err, ErrVideoSite) {
			t.Errorf("%s and example.com: %v, want ErrVideoSite", other, err)
		}
	}
	if err := ValidateVideoSites([]VideoSite{a, {Domain: "example.org", Videos: shape}}); err != nil {
		t.Fatal(err)
	}
}

// A VideoMatcher finds the site of a link and whether it is one video's, as VideoLink does for Kando's sites, without VideoLink knowing its sites.
func TestVideoMatcherFindsItsSites(t *testing.T) {
	m, err := NewVideoMatcher([]VideoSite{{Domain: "videos.example.org", Not: []string{"live.videos.example.org"}, Videos: []VideoShape{{Path: `^/v/[0-9]+/?$`}, {Path: `^/watch$`, Param: "id", Value: `^[0-9]+$`}}}})
	if err != nil {
		t.Fatal(err)
	}
	for u, want := range map[string]struct {
		domain string
		video  bool
	}{
		"https://videos.example.org/v/42":                                       {"videos.example.org", true},
		"https://www.videos.example.org/v/42/":                                  {"videos.example.org", true},
		"http://videos.example.org/watch?id=7&t=3":                              {"videos.example.org", true},
		"https://videos.example.org/watch?id=x":                                 {"videos.example.org", false},
		"https://videos.example.org/":                                           {"videos.example.org", false},
		"https://live.videos.example.org/v/42":                                  {"", false},
		"https://videos.example.org.evil.test/v/42":                             {"", false},
		"https://example.org/v/42":                                              {"", false},
		"ftp://videos.example.org/v/42":                                         {"", false},
		"https://videos.example.org/v/42?x=" + strings.Repeat("a", MaxVideoURL): {"", false},
	} {
		domain, video := m.Find(u)
		if domain != want.domain || video != want.video {
			t.Errorf("Find(%q) = %q, %v, want %q, %v", u, domain, video, want.domain, want.video)
		}
		if VideoLink(u) {
			t.Errorf("VideoLink(%q) knows a site of a matcher", u)
		}
	}
	if _, err := NewVideoMatcher([]VideoSite{site(`^/(a+)+$`)}); !errors.Is(err, ErrVideoSite) {
		t.Fatalf("NewVideoMatcher took a nested quantifier: %v", err)
	}
}
