package media

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"kando/packages/core"
)

// PlatformSubtitler is a Pipeline that can fetch the subtitles a link's site carries, as Tools does with yt-dlp. Run asks it, for a job whose SubtitleSource is core.SubtitlesPlatform, once the speech is transcribed, for the subtitles in the language heard, and takes them as the film's original lines where they match the speech (matchPlatform).
type PlatformSubtitler interface {
	// PlatformSubtitles returns the subtitles the uploader put on source's page in language, as whisper's code (en, ja, zh), none when it has none. Subtitles the site made itself, as YouTube's automatic captions and bilibili's AI subtitles, are not the uploader's and are never returned.
	PlatformSubtitles(ctx context.Context, source, language string) ([]core.Segment, error)
}

// platformFetchTimeout is how long fetching a link's subtitles may take: they are a few kilobytes, and a site that does not answer only means the film is heard alone.
const platformFetchTimeout = 2 * time.Minute

// platformLines are the original lines of job's film when it takes its site's subtitles (core.SubtitlesPlatform) and they match heard, the speech the job transcribed in language (matchPlatform): the subtitles, with the lines heard in place of those the speech did not match, and how many those are. ok is false for a job that does not take them, a local video, a pipeline that cannot fetch them, a site that has none in language or whose fetch failed, and subtitles that do not match the speech; the film is then heard alone.
func platformLines(ctx context.Context, p Pipeline, job core.Job, language string, heard []core.Segment) (lines []core.Segment, local int, ok bool) {
	fetcher, can := p.(PlatformSubtitler)
	if !can || job.SubtitleSource != core.SubtitlesPlatform || strings.TrimSpace(language) == "" {
		return nil, 0, false
	}
	if _, isLocal := localVideoPath(job.Source); isLocal {
		return nil, 0, false
	}
	subtitles, err := fetcher.PlatformSubtitles(ctx, job.Source, language)
	if err != nil || len(subtitles) == 0 {
		return nil, 0, false
	}
	return matchPlatform(subtitles, heard)
}

// platformSlack is how far before its start and after its end a line of the site's subtitles looks for the speech that says it, as subtitles are timed by hand and the speech recognition's own times are rough.
const platformSlack = 500 * time.Millisecond

// platformMatch is how much of a subtitle line's text the speech heard around it has to hold for the line to match: half of its pairs of letters (textPairs).
const platformMatch = 0.5

// matchPlatform checks subtitles, the lines a site carries for a film, against heard, the lines the speech recognition heard in it, and returns the film's original lines: each subtitle line whose words were heard around its time, the lines heard in place of the others, and the lines heard where the subtitles have none, in the order of their times. local counts the subtitle lines the speech did not match. A line that holds nothing to hear, as [Music] or ♪, is kept and not checked. ok is false when fewer than half of the lines checked match, as subtitles in another language than the speech, or timed for another cut of the video: the film is then heard alone.
//
// Each line heard belongs to the subtitle line it overlaps most, once at least half of it lies under subtitle lines: it is dropped for a line that matched, which says it, and kept for one that did not, at its own times. A line heard mostly where the subtitles have none is kept too.
func matchPlatform(subtitles, heard []core.Segment) (lines []core.Segment, local int, ok bool) {
	matched := make([]bool, len(subtitles))
	checked, hits := 0, 0
	for i, s := range subtitles {
		pairs := textPairs(withoutSounds(s.Text))
		if len(pairs) == 0 {
			matched[i] = true
			continue
		}
		checked++
		var around strings.Builder
		for _, h := range heard {
			if h.End > s.Start-platformSlack && h.Start < s.End+platformSlack {
				around.WriteString(h.Text)
				around.WriteByte(' ')
			}
		}
		if containment(pairs, textPairs(around.String())) >= platformMatch {
			matched[i] = true
			hits++
		}
	}
	if checked == 0 || hits*2 < checked {
		return nil, 0, false
	}
	for i, s := range subtitles {
		if matched[i] {
			lines = append(lines, s)
		}
	}
	for _, h := range heard {
		best, bestOverlap, covered := -1, time.Duration(0), time.Duration(0)
		for i, s := range subtitles {
			o := overlap(h, s)
			covered += o
			if o > bestOverlap {
				best, bestOverlap = i, o
			}
		}
		under := best >= 0 && covered*2 >= h.End-h.Start
		if !under || !matched[best] {
			lines = append(lines, h)
		}
	}
	slices.SortStableFunc(lines, func(a, b core.Segment) int { return cmp.Compare(a.Start, b.Start) })
	return lines, checked - hits, true
}

// overlap is how long segments a and b are both on.
func overlap(a, b core.Segment) time.Duration {
	return max(0, min(a.End, b.End)-max(a.Start, b.Start))
}

// sounds are the parts of a subtitle line that name what is heard rather than say it: [Music], (laughs), （笑）, 【拍手】 and the notes around song lyrics.
var sounds = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|（[^）]*）|【[^】]*】|[♪♫]`)

// withoutSounds is text without the parts that only name a sound (sounds).
func withoutSounds(text string) string {
	return sounds.ReplaceAllString(text, " ")
}

// textPairs counts the pairs of letters and digits next to each other in text, lowercased, with everything else left out, so the same words compare alike whatever their spaces, punctuation and case, in any script. A text of one letter is that letter.
func textPairs(text string) map[string]int {
	var letters []rune
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			letters = append(letters, r)
		}
	}
	pairs := map[string]int{}
	if len(letters) == 1 {
		pairs[string(letters)]++
	}
	for i := 0; i+1 < len(letters); i++ {
		pairs[string(letters[i:i+2])]++
	}
	return pairs
}

// containment is how much of want, pairs of letters, have holds: 1 when every pair is there as often, 0 when none is.
func containment(want, have map[string]int) float64 {
	total, found := 0, 0
	for pair, n := range want {
		total += n
		found += min(n, have[pair])
	}
	if total == 0 {
		return 0
	}
	return float64(found) / float64(total)
}

// PlatformSubtitles fetches the subtitles the uploader put on source's page in language with yt-dlp, without the video, borrowing the same sign-ins a download does, as bilibili shows its CC subtitles only to a viewer signed in. It runs apart from Download, so a site that refuses the subtitles, as YouTube does to a client asking too often, never fails the download; it gives up after platformFetchTimeout. Subtitles in a variant of language, as en-GB or zh-Hans, are taken when the language itself has none.
func (t Tools) PlatformSubtitles(ctx context.Context, source, language string) ([]core.Segment, error) {
	if t.YTDLP == "" {
		return nil, errors.New("yt-dlp is not installed")
	}
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return nil, errors.New("no language to fetch subtitles in")
	}
	ctx, cancel := context.WithTimeout(ctx, platformFetchTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "kando-subtitles-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	args := []string{
		"--skip-download",
		"--no-playlist",
		"--ignore-no-formats-error",
		// --write-subs takes the uploader's subtitles alone; the site's own automatic ones would need --write-auto-subs.
		"--write-subs",
		"--sub-langs", subtitleLanguages(language),
		"--sub-format", "srt/vtt/best",
		"--convert-subs", "srt",
		"-P", dir,
		"-o", "subtitle:" + platformSubtitlesName + ".%(ext)s",
	}
	if t.FFmpeg != "" {
		args = append(args, "--ffmpeg-location", t.FFmpeg)
	}
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
	if _, _, err := runFollowing(ctx, false, func(string) {}, t.YTDLP, append(args, "--", source)...); err != nil {
		return nil, err
	}
	return readPlatformSubtitles(dir, language)
}

// platformSubtitlesName is the name yt-dlp gives the subtitles PlatformSubtitles fetches, before the language and the extension: subs.en.srt.
const platformSubtitlesName = "subs"

// subtitleLanguages is the --sub-langs of the subtitles in language, as whisper's code: the language and its variants, as en, en-US and en-GB, or zh, zh-Hans, zh-CN and zh-TW. YouTube writes Hebrew iw, as whisper does not. The AI subtitles bilibili makes (ai-zh) do not start with the language and are left out.
func subtitleLanguages(language string) string {
	codes := []string{language}
	if language == "he" {
		codes = append(codes, "iw")
	}
	for i, code := range codes {
		codes[i] = "^" + regexp.QuoteMeta(code) + "(-.*)?$"
	}
	return strings.Join(codes, ",")
}

// readPlatformSubtitles reads the subtitles yt-dlp wrote in dir: those of language itself before a variant of it, and of the variants the first by name. Tags of style, as <i> and {\an8}, are left out of the lines, and lines left empty dropped.
func readPlatformSubtitles(dir, language string) ([]core.Segment, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, platformSubtitlesName+".") && strings.EqualFold(filepath.Ext(name), ".srt") {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	exact := platformSubtitlesName + "." + language + ".srt"
	slices.SortFunc(files, func(a, b string) int {
		switch {
		case strings.EqualFold(a, exact) && !strings.EqualFold(b, exact):
			return -1
		case strings.EqualFold(b, exact) && !strings.EqualFold(a, exact):
			return 1
		}
		return strings.Compare(a, b)
	})
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		return nil, err
	}
	segments, err := ParseSRT(data)
	if err != nil {
		return nil, err
	}
	out := segments[:0]
	for _, s := range segments {
		s.Text = strings.TrimSpace(styleTags.ReplaceAllString(s.Text, ""))
		if s.Text != "" && s.End >= s.Start {
			out = append(out, s)
		}
	}
	return out, nil
}

// styleTags are the tags of style subtitles carry: HTML's, as <i> and <font color>, and ASS's, as {\an8}.
var styleTags = regexp.MustCompile(`<[^>]*>|\{\\[^}]*\}`)

// qualityKey is the context key of the quality a job's download asks for (withQuality).
type qualityKey struct{}

// withQuality has Download ask for q (core.Job.Quality); empty, or one Kando does not know, asks for the best.
func withQuality(ctx context.Context, q core.Quality) context.Context {
	return context.WithValue(ctx, qualityKey{}, q)
}

// formatFor is yt-dlp's -f for the quality ctx asks for (withQuality): the best video and sound, merged, or the best single file; for 720P the same no taller than 720 lines, falling back to the best when the site has nothing that small.
func formatFor(ctx context.Context) string {
	q, _ := ctx.Value(qualityKey{}).(core.Quality)
	if q == core.Quality720 {
		return "bv*[height<=720]+ba/b[height<=720]/bv*+ba/b"
	}
	return "bv*+ba/b"
}
