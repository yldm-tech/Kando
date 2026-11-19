package media

import (
	"context"
	"os"
	"regexp"
	"strings"

	"kando/packages/core"
)

// AutoCaptioner is a Pipeline that can fetch the captions a link's site made itself, as Tools does with yt-dlp. Run asks it, for a job whose SubtitleSource is core.SubtitlesAuto, once the speech is transcribed, for the captions in the language heard, and keeps them beside the video as the film's reference layer (ReferencePath), which the player can show over its own lines; they never become its original lines.
type AutoCaptioner interface {
	// AutoCaptions returns the captions the site made itself for source's video in language, as whisper's code (en, ja, zh): YouTube's automatic captions, bilibili's AI subtitles; none when it has none. The uploader's own subtitles are never returned.
	AutoCaptions(ctx context.Context, source, language string) ([]core.Segment, error)
}

// AutoCaptions fetches the captions the site made itself for source's video in language with yt-dlp, without the video, as PlatformSubtitles fetches the uploader's: YouTube's automatic captions (--write-auto-subs, which takes none of the uploader's), and on bilibili, whose AI subtitles yt-dlp lists among the uploader's as ai-zh, those alone. It gives up after platformFetchTimeout.
func (t Tools) AutoCaptions(ctx context.Context, source, language string) ([]core.Segment, error) {
	link, _ := core.ParseLink(source)
	return t.fetchSubtitles(ctx, source, language, func(language string) []string {
		return autoCaptionArgs(link.Platform, language)
	})
}

// autoCaptionArgs are the yt-dlp options that choose the captions a site on platform made itself in language: on bilibili its AI subtitles, which yt-dlp lists among the uploader's as ai-<language>; elsewhere the automatic captions, in the language and its variants (subtitleLanguages), as YouTube's en and en-orig.
func autoCaptionArgs(platform core.Platform, language string) []string {
	if platform == core.PlatformBilibili {
		return []string{"--write-subs", "--sub-langs", "^ai-" + regexp.QuoteMeta(language) + "(-.*)?$"}
	}
	return []string{"--write-auto-subs", "--sub-langs", subtitleLanguages(language)}
}

// ReferencePath is the hidden file beside the video at videoPath that keeps the captions its site made itself, the film's reference layer (AutoCaptioner), as SubRip.
func ReferencePath(videoPath string) string {
	return hiddenBeside(videoPath, ".reference.srt")
}

// ReadReference is the reference layer kept beside the video at videoPath (ReferencePath), none when there is none or it cannot be read.
func ReadReference(videoPath string) []core.Segment {
	if videoPath == "" {
		return nil
	}
	data, err := os.ReadFile(ReferencePath(videoPath))
	if err != nil {
		return nil
	}
	lines, err := ParseSRT(data)
	if err != nil {
		return nil
	}
	return lines
}

// keepReference fetches the captions job's site made itself in language and keeps them beside its video (ReferencePath), for a link's job whose SubtitleSource is core.SubtitlesAuto, on a pipeline that is an AutoCaptioner. A site that has none, or whose fetch fails, leaves the film without a reference layer and the job goes on; a job stopped meanwhile reports its context's error, so the caller stops it.
func keepReference(ctx context.Context, p Pipeline, job core.Job, language string) error {
	captioner, can := p.(AutoCaptioner)
	if !can || job.SubtitleSource != core.SubtitlesAuto || strings.TrimSpace(language) == "" || job.VideoPath == "" {
		return nil
	}
	if _, isLocal := localVideoPath(job.Source); isLocal {
		return nil
	}
	lines, err := captioner.AutoCaptions(ctx, job.Source, language)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil || len(lines) == 0 {
		return nil
	}
	// A folder that cannot be written to leaves the film without a reference layer, as a site without captions does.
	_ = saveTranscript(ReferencePath(job.VideoPath), lines)
	return nil
}
