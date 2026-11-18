package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kando/packages/core"
)

// A film that keeps no video (core.Job.SubtitlesOnly) is watched by streaming it from its site: StreamURL asks yt-dlp for an address the system's player opens as it opens a file, which only a stream with the picture and the sound together, behind no request headers, is (Streams).

// streamFormat is yt-dlp's -f for the stream a film that keeps no video is watched from: the best HLS stream that carries the picture and the sound together, as YouTube serves one.
const streamFormat = "b[protocol^=m3u8]"

// streamTimeout is how long looking up a film's stream may take before the player says it cannot be streamed.
const streamTimeout = time.Minute

// Streams reports whether a film that keeps no video, from source, can be streamed on this computer (StreamURL).
func Streams(source string) bool { return streamsOn(runtime.GOOS, source) }

// streamsOn is Streams on goos: only a YouTube video, whose HLS stream carries the picture and the sound together, and only on macOS, whose player opens such a stream by its address.
func streamsOn(goos, source string) bool {
	// TODO(nsgo#681): stream the other sites and platforms once core.Video takes request headers and a separate audio stream: bilibili's streams need its Referer, and most sites serve the picture and the sound apart (DASH) https://github.com/nsgo-dev/nsgo/issues/681
	link, ok := core.ParseLink(source)
	return goos == "darwin" && ok && link.Platform == core.PlatformYouTube
}

// StreamURL is the address of the stream the system's player plays source from (streamFormat), borrowing the browser's login the tools borrow (Cookies). The address expires after some hours, so it is looked up each time the film is opened. Offline mode refuses it (ErrOffline).
func (t Tools) StreamURL(ctx context.Context, source string) (string, error) {
	if t.offlineNow() {
		return "", ErrOffline
	}
	if t.YTDLP == "" {
		return "", errors.New("yt-dlp is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, streamTimeout)
	defer cancel()
	args := []string{"--no-playlist", "--no-warnings", "-f", streamFormat, "--get-url"}
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
	out, err := run(ctx, t.YTDLP, append(args, "--", source)...)
	if err != nil {
		return "", withClosedBrowserData(err, t.Cookies)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			return line, nil
		}
	}
	return "", errors.New("yt-dlp gave no stream address")
}

// placeVideo moves video, which a film that kept none just downloaded (core.RedoVideo), to the name its subtitles go by: anchor, the path the film's sound was downloaded to, with video's extension, as the site may have renamed the video since and yt-dlp names its folder after the title. The folder the download made for the new name goes once it holds nothing else. It returns where the video is.
func placeVideo(anchor, video string) (string, error) {
	if anchor == "" {
		return video, nil
	}
	want := strings.TrimSuffix(anchor, filepath.Ext(anchor)) + filepath.Ext(video)
	if want == video {
		return video, nil
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(video, want); err != nil {
		return "", err
	}
	if filepath.Dir(video) != filepath.Dir(want) {
		_ = os.Remove(filepath.Dir(video))
	}
	return want, nil
}

// removeSound removes the sound a film at core.QualitySubtitles downloaded to make its subtitles, once they are written, and reports whether it is gone.
func removeSound(path string) bool {
	err := os.Remove(path)
	return err == nil || errors.Is(err, os.ErrNotExist)
}
