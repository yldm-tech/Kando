package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Previewer is a Pipeline that can read what a link's video is before it is added, as Tools does with yt-dlp, so the Add sheet names it, says how long it plays and which subtitles its site has before the user adds it.
type Previewer interface {
	Preview(ctx context.Context, source string) (Preview, error)
}

// Preview is what Tools.Preview read of a video.
type Preview struct {
	// Title is the video's title, and Uploader the channel or the person that put it on the site; "" when the site gives none.
	Title    string
	Uploader string
	// Duration is how long it plays, 0 when the site did not say.
	Duration time.Duration
	// Subtitles reports whether the uploader put subtitles on it, in any language, and AutoCaptions whether the site made captions of its own: YouTube's automatic captions, bilibili's AI subtitles.
	Subtitles    bool
	AutoCaptions bool
}

// PreviewTimeout is how long Preview waits for yt-dlp: it reads the video's page and none of the video.
const PreviewTimeout = time.Minute

// Preview reads what the video at source is with yt-dlp -J, without downloading it or the playlist it is in, borrowing the sign-ins a download does, so a video only a signed-in account sees is read too. It gives up after PreviewTimeout, and ends when ctx does with its error.
func (t Tools) Preview(ctx context.Context, source string) (Preview, error) {
	if t.YTDLP == "" {
		return Preview{}, errors.New("yt-dlp is not installed")
	}
	if t.offlineNow() {
		return Preview{}, ErrOffline
	}
	ctx, cancel := context.WithTimeout(ctx, PreviewTimeout)
	defer cancel()
	args := []string{"-J", "--no-playlist", "--skip-download", "--ignore-no-formats-error", "--no-warnings"}
	args = append(args, t.ytdlpRuntimeArgs()...)
	if t.Cookies != "" {
		args = append(args, "--cookies-from-browser", t.Cookies)
	}
	if file, userAgent, done := t.siteCookiesFile(ctx, source); file != "" {
		defer done()
		args = append(args, "--cookies", file)
		if userAgent != "" {
			args = append(args, "--user-agent", userAgent)
		}
	}
	var out string
	err := withoutUnreadLogin(ctx, append(args, "--", source), func(args []string) (err error) {
		out, err = run(ctx, t.YTDLP, args...)
		return err
	})
	if err != nil {
		return Preview{}, err
	}
	preview, err := parsePreview([]byte(out))
	if err != nil {
		return Preview{}, fmt.Errorf("yt-dlp: %w", err)
	}
	return preview, nil
}

// notSubtitles are the entries yt-dlp lists among a video's subtitles that are not lines of its speech: YouTube's live chat and bilibili's danmaku.
var notSubtitles = map[string]bool{"live_chat": true, "danmaku": true}

// parsePreview reads what yt-dlp -J prints of one video. bilibili's AI subtitles, which yt-dlp lists among the uploader's as ai-zh, count as the site's own captions.
func parsePreview(data []byte) (Preview, error) {
	var info struct {
		Type              string                     `json:"_type"`
		Title             string                     `json:"title"`
		Channel           string                     `json:"channel"`
		Uploader          string                     `json:"uploader"`
		Duration          float64                    `json:"duration"`
		Subtitles         map[string]json.RawMessage `json:"subtitles"`
		AutomaticCaptions map[string]json.RawMessage `json:"automatic_captions"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return Preview{}, err
	}
	if info.Type != "" && info.Type != "video" {
		return Preview{}, errors.New("the link is not one video")
	}
	p := Preview{Title: strings.TrimSpace(info.Title), Uploader: strings.TrimSpace(info.Channel), AutoCaptions: len(info.AutomaticCaptions) > 0}
	if p.Uploader == "" {
		p.Uploader = strings.TrimSpace(info.Uploader)
	}
	if info.Duration > 0 {
		p.Duration = time.Duration(info.Duration * float64(time.Second)).Round(time.Second)
	}
	for language := range info.Subtitles {
		switch {
		case notSubtitles[language]:
		case strings.HasPrefix(language, "ai-"):
			p.AutoCaptions = true
		default:
			p.Subtitles = true
		}
	}
	return p, nil
}
