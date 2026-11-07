package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kando/packages/core"
)

// YouTubeOEmbed is YouTube's oEmbed endpoint. It names a public video, and one that is age-restricted or not shown where this machine is, without a sign-in; it knows nothing of a private or removed one.
const YouTubeOEmbed = "https://www.youtube.com/oembed"

// oembedTimeout bounds the lookup, which only names a card whose download already failed.
const oembedTimeout = 10 * time.Second

// lookupOEmbed asks endpoint for the title and thumbnail of the YouTube video source links to. It reports false for a link that is not to a YouTube video and when the endpoint does not answer with a title.
func lookupOEmbed(ctx context.Context, endpoint, source string) (Info, bool) {
	link, ok := core.ParseLink(source)
	if !ok || link.Platform != core.PlatformYouTube || link.Kind != core.LinkVideo || link.ID == "" {
		return Info{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, oembedTimeout)
	defer cancel()
	query := url.Values{"format": {"json"}, "url": {"https://www.youtube.com/watch?v=" + link.ID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return Info{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Info{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Info{}, false
	}
	var reply struct {
		Title        string `json:"title"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply); err != nil {
		return Info{}, false
	}
	title := strings.TrimSpace(reply.Title)
	if title == "" {
		return Info{}, false
	}
	return Info{Title: title, Thumbnail: reply.ThumbnailURL}, true
}
