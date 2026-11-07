package media

import (
	"os"
	"path/filepath"
	"testing"

	"kando/packages/core"
)

// The stderr files in testdata/ytdlp/errors were captured from the real yt-dlp (2026.08.19, and 2024.03.10 for outdated-youtube) run against real failing links on 2026-11-07, with the home folder in paths replaced.
func TestClassifyRealYTDLPErrors(t *testing.T) {
	for file, want := range map[string]core.FailKind{
		"outdated-youtube.stderr":        core.FailOutdated,
		"private-youtube.stderr":         core.FailPrivate,
		"removed-youtube.stderr":         core.FailPrivate,
		"notfound-vimeo.stderr":          core.FailPrivate,
		"age-youtube.stderr":             core.FailAge,
		"region-bilibili.stderr":         core.FailRegion,
		"drm.stderr":                     core.FailDRM,
		"unsupported.stderr":             core.FailUnsupported,
		"login-instagram.stderr":         core.FailLogin,
		"network-dns.stderr":             core.FailNetwork,
		"network-refused.stderr":         core.FailNetwork,
		"cookies-safari-denied.stderr":   core.FailCookies,
		"cookies-firefox-missing.stderr": core.FailCookies,
	} {
		data, err := os.ReadFile(filepath.Join("testdata", "ytdlp", "errors", file))
		if err != nil {
			t.Fatal(err)
		}
		if got := ClassifyDownload(string(data)); got != want {
			t.Errorf("%s: %q, want %q", file, got, want)
		}
	}
}

// These messages could not be made to happen from this machine (no members-only video, no rate limit, no bot check), so they are the texts yt-dlp's own extractors raise, from extractor/bilibili.py and extractor/youtube/_video.py of 2026.08.19, and the reason YouTube gives in its player response.
func TestClassifyMessagesFromTheYTDLPSource(t *testing.T) {
	for msg, want := range map[string]core.FailKind{
		"ERROR: [BiliBili] BV1xx411c7mD: This video is for premium members only. Use --cookies-from-browser or --cookies for the authentication.": core.FailMembers,
		"ERROR: [youtube] abc: Join this channel to get access to members-only content like this video, and other exclusive perks.":               core.FailMembers,
		"ERROR: [youtube] abc: Video unavailable. The uploader has not made this video available in your country":                                 core.FailRegion,
		"ERROR: [youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication.":              core.FailRateLimit,
		"ERROR: [youtube] abc: This content isn't available, try again later. The current session has been rate-limited by YouTube":               core.FailRateLimit,
		"ERROR: unable to download video data: HTTP Error 429: Too Many Requests":                                                                 core.FailRateLimit,
		"ERROR: [vimeo] 1: This video is only available for registered users. Use --cookies-from-browser or --cookies for the authentication.":    core.FailLogin,
		"ERROR: something nobody has seen": "",
	} {
		if got := ClassifyDownload(msg); got != want {
			t.Errorf("%q: %q, want %q", msg, got, want)
		}
	}
}

// A warning that asks to update yt-dlp does not make a failure for another reason an outdated one: only the ERROR lines count.
func TestClassifyReadsOnlyTheErrorLines(t *testing.T) {
	stderr := "WARNING: [youtube] unable to extract yt initial data; please report this issue on  https://github.com/yt-dlp/yt-dlp/issues?q= , filling out the appropriate issue template. Confirm you are on the latest version using  yt-dlp -U\nERROR: [youtube] yZIXLfi8CZQ: Private video\n"
	if got := ClassifyDownload(stderr); got != core.FailPrivate {
		t.Fatalf("got %q", got)
	}
}
