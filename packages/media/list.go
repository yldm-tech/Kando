package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kando/packages/core"
)

// Lister is a Pipeline that can also list the videos of a playlist or a channel, so the app adds them one job each: Tools does, with yt-dlp.
type Lister interface {
	ListVideos(ctx context.Context, source string, limit int) (Listing, error)
}

// Listing is what a playlist or a channel holds, as ListVideos read it.
type Listing struct {
	// Title is the list's name, or the channel's, "" when the site gives none.
	Title string
	// Videos are its videos in the site's order: a playlist's own, a channel's newest first.
	Videos []ListedVideo
}

// ListedVideo is one video of a Listing: the link a job downloads it from, its title, and how long it plays, 0 when the site did not say.
type ListedVideo struct {
	URL      string
	Title    string
	Duration time.Duration
}

// ErrOneVideo is ListVideos' error for a link that lists as one video rather than a list, as a bilibili upload of one part does (core.Link.Parts).
var ErrOneVideo = errors.New("the link is one video, not a list")

// ListTimeout is how long ListVideos waits for yt-dlp to list a playlist or a channel: yt-dlp reads a long playlist a page of a hundred videos at a time.
const ListTimeout = 3 * time.Minute

// ErrListTimeout is ListVideos' error when yt-dlp did not finish listing within ListTimeout. It is not the caller's own cancel, which is context.Canceled.
var ErrListTimeout = errors.New("listing the videos took too long")

// ListVideos lists the videos of source, a playlist or a channel, with yt-dlp --flat-playlist, which reads the list without looking up each video, so it takes seconds rather than minutes. limit, when above 0, keeps the first limit videos, a channel's most recent; 0 lists them all. A YouTube channel's link is read as its Videos tab (listSource). Entries that are not one video, as another list on a channel's page, and YouTube's private and deleted videos, which cannot be downloaded, are left out, and a video listed twice is listed once. It borrows the browser's login (Cookies) and the app's pages' cookies (SiteCookies) as Download does, so a list only a signed-in account sees, as Watch Later, can be read. It gives up after ListTimeout with ErrListTimeout, and ends when ctx does with its error.
func (t Tools) ListVideos(ctx context.Context, source string, limit int) (Listing, error) {
	if t.YTDLP == "" {
		return Listing{}, errors.New("yt-dlp is not installed")
	}
	if t.offlineNow() {
		return Listing{}, ErrOffline
	}
	timed, cancel := context.WithTimeout(ctx, ListTimeout)
	defer cancel()
	args := []string{"--flat-playlist", "-J", "--no-warnings"}
	if limit > 0 {
		args = append(args, "--playlist-end", strconv.Itoa(limit))
	}
	if t.Cookies != "" {
		args = append(args, "--cookies-from-browser", t.Cookies)
	}
	if file, userAgent, done := t.siteCookiesFile(timed, source); file != "" {
		defer done()
		args = append(args, "--cookies", file)
		if userAgent != "" {
			args = append(args, "--user-agent", userAgent)
		}
	}
	out, err := runStage(timed, t.YTDLP, append(args, "--", listSource(source))...)
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			return Listing{}, ErrListTimeout
		}
		return Listing{}, err
	}
	listing, err := parseListing([]byte(out))
	if err != nil {
		return Listing{}, fmt.Errorf("yt-dlp: %w", err)
	}
	if limit > 0 && len(listing.Videos) > limit {
		listing.Videos = listing.Videos[:limit]
	}
	return listing, nil
}

// listSource is the link yt-dlp lists for source: a YouTube channel's own page, which yt-dlp reads as one list per tab (Videos, Shorts, Live) rather than as videos, becomes its Videos tab; any other link stays as it is.
func listSource(source string) string {
	link, ok := core.ParseLink(source)
	if !ok || link.Platform != core.PlatformYouTube || link.Kind != core.LinkChannel {
		return source
	}
	u, err := url.Parse(link.URL)
	if err != nil {
		return source
	}
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	tabless := len(parts) == 1 && strings.HasPrefix(parts[0], "@") || len(parts) == 2 && !strings.HasPrefix(parts[0], "@")
	if !tabless {
		return source
	}
	u.Path = "/" + strings.Join(parts, "/") + "/videos"
	return u.String()
}

// youTubeUnavailable are the titles YouTube lists a private or deleted video of a playlist under, which no one can download.
var youTubeUnavailable = map[string]bool{"[Private video]": true, "[Deleted video]": true}

// parseListing reads what yt-dlp -J --flat-playlist prints of a list.
func parseListing(data []byte) (Listing, error) {
	var list struct {
		Title    string `json:"title"`
		Channel  string `json:"channel"`
		Uploader string `json:"uploader"`
		Type     string `json:"_type"`
		Entries  []struct {
			Type       string  `json:"_type"`
			URL        string  `json:"url"`
			WebpageURL string  `json:"webpage_url"`
			Title      string  `json:"title"`
			Duration   float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return Listing{}, err
	}
	if list.Type != "" && list.Type != "playlist" {
		return Listing{}, ErrOneVideo
	}
	listing := Listing{Title: strings.TrimSpace(list.Title)}
	// A channel's Videos tab is titled "<channel> - Videos"; the channel's own name says it better.
	if channel := strings.TrimSpace(list.Channel); channel != "" && strings.HasPrefix(listing.Title, channel+" - ") {
		listing.Title = channel
	}
	if listing.Title == "" {
		listing.Title = strings.TrimSpace(list.Uploader)
	}
	seen := map[string]bool{}
	for _, e := range list.Entries {
		link := e.WebpageURL
		if !webLink(link) {
			link = e.URL
		}
		if !webLink(link) || e.Type == "playlist" || youTubeUnavailable[e.Title] || seen[link] {
			continue
		}
		if parsed, ok := core.ParseLink(link); !ok || !parsed.OneVideo() {
			continue
		}
		seen[link] = true
		listing.Videos = append(listing.Videos, ListedVideo{URL: link, Title: strings.TrimSpace(e.Title), Duration: time.Duration(e.Duration * float64(time.Second))})
	}
	return listing, nil
}

// webLink reports whether s is an http or https link, as a flat entry's url is for most sites; some give only the video's ID there.
func webLink(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}
