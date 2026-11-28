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

// PlatformSubtitler is a Pipeline that can fetch the subtitles a link's site carries, as Tools does with yt-dlp. Make asks it, for a job whose SubtitleSource is core.SubtitlesPlatform, once the language the speech is heard in is known, for the subtitles in that language, and takes them as the film's original lines where they match the speech (making.siteHeard).
type PlatformSubtitler interface {
	// PlatformSubtitles returns the subtitles the uploader put on source's page in language, as whisper's code (en, ja, zh), none when it has none. Subtitles the site made itself, as YouTube's automatic captions and bilibili's AI subtitles, are not the uploader's and are never returned.
	PlatformSubtitles(ctx context.Context, source, language string) ([]core.Segment, error)
}

// platformFetchTimeout is how long fetching a link's subtitles may take: they are a few kilobytes, and a site that does not answer only means the film is heard alone.
const platformFetchTimeout = 2 * time.Minute

// platformSlack is how far before its start and after its end a line of the site's subtitles looks for the speech that says it, as subtitles are timed by hand and the speech recognition's own times are rough.
const platformSlack = 500 * time.Millisecond

// platformMatch is how much of a subtitle line's text the speech heard around it has to hold for the line to match: half of its pairs of letters (textPairs).
const platformMatch = 0.5

// platformSaid is how much of a line heard the subtitle lines kept around it have to hold for the line to be what they say and be left out: most of its pairs of letters (textPairs), so a line heard that runs on into speech they do not have is kept, while one misheard a little is not.
const platformSaid = 0.75

// matchPlatform checks subtitles, the lines a site carries for a film, against heard, the lines the speech recognition heard in it, and returns the film's original lines, in the order of their times. ok is false when fewer than half of the subtitle lines checked match, those whose words were heard around their time, as subtitles in another language than the speech, or timed for another cut of the video: the film is then heard alone. A subtitle line that holds nothing to hear, as [Music] or ♪, matches without being checked.
//
// Every stretch of the film comes from one side, so no speech is lost and none is said twice. A line heard is left out when the subtitle lines kept around it hold its words (platformSaid); any other is kept, at its own times, and replaces every subtitle line it overlaps, as a line heard often spans two of them, so the lines heard under those are kept too. A line heard where the subtitles have none is kept the same way. A subtitle line that does not match and that no line heard replaces, as one the speech recognition did not hear at all, stays as the uploader wrote it. local counts the subtitle lines with words that lines heard replaced.
func matchPlatform(subtitles, heard []core.Segment) (lines []core.Segment, local int, ok bool) {
	matched, words, checked, hits := platformCheck(subtitles, heard)
	if checked == 0 || hits*2 < checked {
		return nil, 0, false
	}
	lines, _, local = platformMerge(subtitles, heard, matched, words)
	return lines, local, true
}

// platformCheck checks each of subtitles against heard, the lines heard around it (matchPlatform): matched says which match, words which hold words to check, and checked and hits count those with words and those of them that match.
func platformCheck(subtitles, heard []core.Segment) (matched, words []bool, checked, hits int) {
	matched = make([]bool, len(subtitles))
	words = make([]bool, len(subtitles))
	for i, s := range subtitles {
		pairs := textPairs(withoutSounds(s.Text))
		if len(pairs) == 0 {
			matched[i] = true
			continue
		}
		words[i] = true
		checked++
		var around strings.Builder
		for _, h := range heard {
			if near(h, s) {
				around.WriteString(h.Text)
				around.WriteByte(' ')
			}
		}
		if containment(pairs, textPairs(around.String())) >= platformMatch {
			matched[i] = true
			hits++
		}
	}
	return matched, words, checked, hits
}

// platformMerge is the film's original lines from subtitles, checked against heard (platformCheck), and the lines heard (matchPlatform), in the order of their times: site says, line for line, which are the uploader's, and local counts the subtitle lines with words that lines heard replaced.
func platformMerge(subtitles, heard []core.Segment, matched, words []bool) (lines []core.Segment, site []bool, local int) {
	// around[j] are the subtitle lines near heard line j, whose words it may be, under[j] those it overlaps, and nearBy[i] the lines heard near subtitle line i, which have to be looked at again once it is replaced.
	around := make([][]int, len(heard))
	under := make([][]int, len(heard))
	nearBy := make([][]int, len(subtitles))
	for j, h := range heard {
		for i, s := range subtitles {
			if near(h, s) {
				around[j] = append(around[j], i)
				nearBy[i] = append(nearBy[i], j)
			}
			if overlap(h, s) > 0 {
				under[j] = append(under[j], i)
			}
		}
	}
	replaced := make([]bool, len(subtitles))
	kept := make([]bool, len(heard))
	// said is whether heard line j is what the subtitle lines still kept around it say. A line with no words says nothing to keep unless it is where the subtitles have none.
	said := func(j int) bool {
		pairs := textPairs(heard[j].Text)
		if len(pairs) == 0 {
			return len(under[j]) > 0
		}
		var text strings.Builder
		for _, i := range around[j] {
			if matched[i] && !replaced[i] {
				text.WriteString(subtitles[i].Text)
				text.WriteByte(' ')
			}
		}
		return containment(pairs, textPairs(text.String())) >= platformSaid
	}
	// Replacing a subtitle line that matched can leave a line heard near it no longer said, so those are looked at again until nothing changes; every line heard is kept at most once, which bounds the work.
	queue := make([]int, len(heard))
	for j := range heard {
		queue[j] = j
	}
	for len(queue) > 0 {
		j := queue[0]
		queue = queue[1:]
		if kept[j] || said(j) {
			continue
		}
		kept[j] = true
		for _, i := range under[j] {
			if replaced[i] {
				continue
			}
			replaced[i] = true
			if matched[i] {
				queue = append(queue, nearBy[i]...)
			}
		}
	}
	type line struct {
		core.Segment
		site bool
	}
	var out []line
	for i, s := range subtitles {
		switch {
		case !replaced[i]:
			out = append(out, line{s, true})
		case words[i]:
			local++
		}
	}
	for j, h := range heard {
		if kept[j] {
			out = append(out, line{h, false})
		}
	}
	slices.SortStableFunc(out, func(a, b line) int { return cmp.Compare(a.Start, b.Start) })
	for _, l := range out {
		lines, site = append(lines, l.Segment), append(site, l.site)
	}
	return lines, site, local
}

// near is whether heard is on around subtitle's time, within platformSlack of it.
func near(heard, subtitle core.Segment) bool {
	return heard.End > subtitle.Start-platformSlack && heard.Start < subtitle.End+platformSlack
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
	// --write-subs takes the uploader's subtitles alone; the site's own automatic ones would need --write-auto-subs (AutoCaptions).
	return t.fetchSubtitles(ctx, source, language, func(language string) []string {
		return []string{"--write-subs", "--sub-langs", subtitleLanguages(language)}
	})
}

// fetchSubtitles fetches the subtitles of source's page in language with yt-dlp, without the video, as PlatformSubtitles and AutoCaptions do: which gives the options that choose them, for language lowered and trimmed. It borrows the sign-ins a download does and gives up after platformFetchTimeout.
func (t Tools) fetchSubtitles(ctx context.Context, source, language string, which func(language string) []string) ([]core.Segment, error) {
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
	}
	args = append(args, which(language)...)
	args = append(args,
		"--sub-format", "srt/vtt/best",
		"--convert-subs", "srt",
		"-P", dir,
		"-o", "subtitle:"+platformSubtitlesName+".%(ext)s",
	)
	if t.FFmpeg != "" {
		args = append(args, "--ffmpeg-location", t.FFmpeg)
	}
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
	err = withoutUnreadLogin(ctx, append(args, "--", source), func(args []string) error {
		_, _, err := runFollowing(ctx, false, func(string) {}, t.YTDLP, args...)
		return err
	})
	if err != nil {
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

// formatFor is yt-dlp's -f for the quality ctx asks for (withQuality): the best video and sound, merged, or the best single file; for 720P the same no taller than 720 lines, falling back to the best when the site has nothing that small; for 仅字幕, as for the sound a link's subtitles are made from first, the best sound alone in an M4A, which every platform's player plays while the film's video downloads (core.Job.SoundPath), else the best sound alone, or the best single file of a site that serves none apart.
func formatFor(ctx context.Context) string {
	q, _ := ctx.Value(qualityKey{}).(core.Quality)
	switch q {
	case core.Quality720:
		return "bv*[height<=720]+ba/b[height<=720]/bv*+ba/b"
	case core.QualitySubtitles:
		return "ba[ext=m4a]/ba/b"
	}
	return "bv*+ba/b"
}
