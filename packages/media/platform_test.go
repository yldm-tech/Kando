package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

func seg(from, to float64, text string) core.Segment {
	return core.Segment{Start: time.Duration(from * float64(time.Second)), End: time.Duration(to * float64(time.Second)), Text: text}
}

func texts(lines []core.Segment) []string {
	out := make([]string, len(lines))
	for i, s := range lines {
		out[i] = s.Text
	}
	return out
}

// Subtitles whose words were heard at their times are the film's lines as the uploader wrote them, a line the speech did not say is replaced by what was heard there, and speech the subtitles leave out is kept.
func TestMatchPlatformKeepsTheUploadersLines(t *testing.T) {
	subtitles := []core.Segment{
		seg(1, 3, "Hello, everyone."),
		seg(3, 5, "Welcome to the show!"),
		seg(5, 7, "[Music]"),
		seg(8, 10, "This line was never said."),
		seg(10, 12, "See you next week."),
	}
	heard := []core.Segment{
		seg(1, 3, "hello everyone"),
		seg(3.2, 5.1, "welcome to the show"),
		seg(8, 10, "and now the weather"),
		seg(10, 12, "see you next week"),
		seg(20, 22, "one more thing"),
	}
	lines, local, ok := matchPlatform(subtitles, heard)
	if !ok || local != 1 {
		t.Fatalf("ok %v, local %d", ok, local)
	}
	want := []string{"Hello, everyone.", "Welcome to the show!", "[Music]", "and now the weather", "See you next week.", "one more thing"}
	if got := texts(lines); !reflect.DeepEqual(got, want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
}

// Japanese subtitles, with no spaces, match the speech heard in other characters' places and punctuation.
func TestMatchPlatformComparesAnyScript(t *testing.T) {
	subtitles := []core.Segment{seg(0, 2, "おはようございます。"), seg(2, 4, "今日はいい天気ですね。")}
	heard := []core.Segment{seg(0, 4, "おはようございます 今日はいい天気ですね")}
	lines, local, ok := matchPlatform(subtitles, heard)
	if !ok || local != 0 || !reflect.DeepEqual(texts(lines), []string{"おはようございます。", "今日はいい天気ですね。"}) {
		t.Fatalf("lines %q, local %d, ok %v", texts(lines), local, ok)
	}
}

// Subtitles that do not say what was heard, as ones in another language or timed for another cut, are not taken at all.
func TestMatchPlatformRefusesSubtitlesThatDoNotMatch(t *testing.T) {
	heard := []core.Segment{seg(0, 2, "hello everyone"), seg(2, 4, "welcome to the show"), seg(4, 6, "see you next week")}
	other := []core.Segment{seg(0, 2, "大家好"), seg(2, 4, "欢迎收看"), seg(4, 6, "下周见")}
	if _, _, ok := matchPlatform(other, heard); ok {
		t.Fatal("took subtitles in another language")
	}
	shifted := []core.Segment{seg(30, 32, "hello everyone"), seg(32, 34, "welcome to the show"), seg(34, 36, "see you next week")}
	if _, _, ok := matchPlatform(shifted, heard); ok {
		t.Fatal("took subtitles timed for another video")
	}
	if _, _, ok := matchPlatform([]core.Segment{seg(0, 2, "♪")}, heard); ok {
		t.Fatal("took subtitles with nothing to check")
	}
}

// A line heard that spans a subtitle line that matched and one that did not replaces both, so its speech is neither lost nor said twice, and a line the speech recognition did not hear at all stays as the uploader wrote it.
func TestMatchPlatformSpanningLines(t *testing.T) {
	cases := []struct {
		name             string
		subtitles, heard []core.Segment
		want             []string
		local            int
	}{
		{"runs on past a matched line",
			[]core.Segment{seg(0, 2, "hello everyone"), seg(2, 4, "totally different words"), seg(4, 6, "see you next week")},
			[]core.Segment{seg(0, 3.5, "hello everyone the weather is nice"), seg(4, 6, "see you next week")},
			[]string{"hello everyone the weather is nice", "see you next week"}, 2},
		{"starts under a matched line",
			[]core.Segment{seg(0, 2, "hello everyone"), seg(2, 4, "totally different words"), seg(4, 6, "see you next week")},
			[]core.Segment{seg(0, 1.2, "hello"), seg(1.2, 4, "everyone the weather is nice"), seg(4, 6, "see you next week")},
			[]string{"hello", "everyone the weather is nice", "see you next week"}, 2},
		{"spans two matched lines",
			[]core.Segment{seg(0, 2, "Hello, everyone."), seg(2, 4, "Welcome to the show!")},
			[]core.Segment{seg(0, 4, "hello everyone welcome to the show")},
			[]string{"Hello, everyone.", "Welcome to the show!"}, 0},
		{"a line not heard",
			[]core.Segment{seg(0, 2, "hello everyone"), seg(2, 4, "welcome to the show"), seg(10, 12, "a whisper too quiet to hear")},
			[]core.Segment{seg(0, 2, "hello everyone"), seg(2, 4, "welcome to the show")},
			[]string{"hello everyone", "welcome to the show", "a whisper too quiet to hear"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines, local, ok := matchPlatform(c.subtitles, c.heard)
			if !ok || local != c.local || !reflect.DeepEqual(texts(lines), c.want) {
				t.Fatalf("lines %q, local %d, ok %v; want %q, local %d", texts(lines), local, ok, c.want, c.local)
			}
			for i := 1; i < len(lines); i++ {
				if lines[i].Start < lines[i-1].End {
					t.Fatalf("lines %+v overlap", lines)
				}
			}
		})
	}
}

// platformPipeline is a pipeline whose video is in a folder of the test and whose site carries subtitles, counting the transcriptions, fetches and proofreadings and keeping what was exported. Its proofreading writes "um so" as "um, so", and onFetch, when set, runs as it fetches.
type platformPipeline struct {
	t           *testing.T
	video       string
	heard       []core.Segment
	subtitles   []core.Segment
	fetchErr    error
	onFetch     func()
	transcribed int
	fetched     []string
	proofread   int
	exported    []core.Segment
}

func (p *platformPipeline) Check() error { return nil }

func (p *platformPipeline) Download(context.Context, string) (string, error) {
	if err := os.WriteFile(p.video, []byte("video"), 0o644); err != nil {
		p.t.Fatal(err)
	}
	return p.video, nil
}

func (p *platformPipeline) Transcribe(ctx context.Context, _ string) ([]core.Segment, error) {
	p.transcribed++
	reportSpoken(ctx, "en")
	return p.heard, nil
}

func (p *platformPipeline) PlatformSubtitles(_ context.Context, source, language string) ([]core.Segment, error) {
	p.fetched = append(p.fetched, language)
	if p.onFetch != nil {
		p.onFetch()
	}
	return p.subtitles, p.fetchErr
}

func (p *platformPipeline) Proofread(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	p.proofread++
	fixed := slices.Clone(segments)
	for i := range fixed {
		fixed[i].Text = strings.ReplaceAll(fixed[i].Text, "um so", "um, so")
	}
	return fixed, nil
}

func (p *platformPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

func (p *platformPipeline) Export(ctx context.Context, video string, original, translated []core.Segment) (string, error) {
	p.exported = original
	return exportSubtitles(ctx, video, original, translated)
}

func newPlatformPipeline(t *testing.T) *platformPipeline {
	return &platformPipeline{t: t, video: filepath.Join(t.TempDir(), "video.mp4"),
		heard:     []core.Segment{seg(0, 2, "hello everyone"), seg(2, 4, "welcome to the shoe"), seg(4, 6, "um so")},
		subtitles: []core.Segment{seg(0, 2, "Hello, everyone."), seg(2, 4, "Welcome to the show!"), seg(4, 6, "Totally different words.")},
	}
}

// A link's job that takes its site's subtitles makes the film of the uploader's lines where they match the speech, with the speech, proofread, in place of the one that does not, and keeps them as the transcript, so a job run again reads them back without fetching them or proofreading again.
func TestRunTakesTheSitesSubtitles(t *testing.T) {
	p := newPlatformPipeline(t)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.SubtitleSource = core.SubtitlesPlatform
	done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if done.Stage != core.StageComplete || !done.PlatformSubtitles || done.LocalLines != 1 {
		t.Fatalf("job %+v", done)
	}
	if want := []string{"Hello, everyone.", "Welcome to the show!", "um, so"}; !reflect.DeepEqual(texts(p.exported), want) {
		t.Fatalf("exported %q", texts(p.exported))
	}
	if !reflect.DeepEqual(p.fetched, []string{"en"}) || p.proofread != 1 {
		t.Fatalf("fetched %q, proofread %d times", p.fetched, p.proofread)
	}
	if got := mustLoad(t, TranscriptOf(p.video)); got[0].Text != "Hello, everyone." {
		t.Fatalf("transcript %+v", got)
	}

	// Translated again, the film's lines are still the uploader's and are not proofread again.
	done.Redo, done.Stage = core.RedoTranslation, core.StageQueued
	again := Run(context.Background(), p, done, "日本語", func(core.Job) {})
	if again.Stage != core.StageComplete || !again.PlatformSubtitles || again.LocalLines != 1 || p.proofread != 1 || len(p.fetched) != 1 {
		t.Fatalf("again %+v, proofread %d, fetched %q", again, p.proofread, p.fetched)
	}
}

// A job heard alone, one whose site has no subtitles or whose fetch failed, and subtitles that do not match the speech make the film of the speech heard, which is proofread as before.
func TestRunHearsTheFilmWithoutTheSitesSubtitles(t *testing.T) {
	cases := map[string]func(*platformPipeline, *core.Job){
		"heard alone":  func(_ *platformPipeline, job *core.Job) { job.SubtitleSource = core.SubtitlesLocal },
		"not chosen":   func(_ *platformPipeline, job *core.Job) { job.SubtitleSource = "" },
		"none":         func(p *platformPipeline, _ *core.Job) { p.subtitles = nil },
		"fetch failed": func(p *platformPipeline, _ *core.Job) { p.fetchErr = errors.New("HTTP Error 429") },
		"do not match": func(p *platformPipeline, _ *core.Job) {
			p.subtitles = []core.Segment{seg(0, 2, "大家好"), seg(2, 4, "欢迎")}
		},
		"a local video": func(_ *platformPipeline, job *core.Job) { job.Source = "file:///films/talk.mp4" },
	}
	for name, tweak := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPlatformPipeline(t)
			job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
			job.SubtitleSource = core.SubtitlesPlatform
			tweak(p, &job)
			done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
			if done.Stage != core.StageComplete || done.PlatformSubtitles || done.LocalLines != 0 || p.proofread != 1 {
				t.Fatalf("job %+v, proofread %d", done, p.proofread)
			}
			if want := []string{"hello everyone", "welcome to the shoe", "um, so"}; !reflect.DeepEqual(texts(p.exported), want) {
				t.Fatalf("exported %q", texts(p.exported))
			}
		})
	}
}

// A job stopped while it fetches the subtitles keeps the speech it heard, so the one run again fetches them without transcribing the film again.
func TestRunStoppedFetchingKeepsTheSpeech(t *testing.T) {
	p := newPlatformPipeline(t)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.SubtitleSource = core.SubtitlesPlatform
	ctx, cancel := context.WithCancel(context.Background())
	p.onFetch = cancel
	stopped := Run(ctx, p, job, "简体中文", func(core.Job) {})
	if stopped.Stage != core.StageFailed || !errors.Is(stopped.Err, context.Canceled) || !stopped.Transcribed || stopped.PlatformSubtitles {
		t.Fatalf("stopped %+v", stopped)
	}
	if got := texts(mustLoad(t, TranscriptOf(p.video))); !reflect.DeepEqual(got, []string{"hello everyone", "welcome to the shoe", "um, so"}) {
		t.Fatalf("transcript kept %q", got)
	}

	p.onFetch = nil
	stopped.Stage, stopped.Err = core.StageQueued, nil
	done := Run(context.Background(), p, stopped, "简体中文", func(core.Job) {})
	if done.Stage != core.StageComplete || !done.PlatformSubtitles || done.LocalLines != 1 || p.transcribed != 1 || p.proofread != 1 || len(p.fetched) != 2 {
		t.Fatalf("done %+v, transcribed %d, proofread %d, fetched %q", done, p.transcribed, p.proofread, p.fetched)
	}
	if want := []string{"Hello, everyone.", "Welcome to the show!", "um, so"}; !reflect.DeepEqual(texts(p.exported), want) {
		t.Fatalf("exported %q", texts(p.exported))
	}
}

// Processing a film again entirely fetches the subtitles again, and until it completes the film keeps what its own job found.
func TestRunRedoAllFetchesTheSubtitlesAgain(t *testing.T) {
	p := newPlatformPipeline(t)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.SubtitleSource = core.SubtitlesLocal
	done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if done.PlatformSubtitles {
		t.Fatalf("job %+v", done)
	}
	done.Redo, done.Stage, done.SubtitleSource = core.RedoAll, core.StageQueued, core.SubtitlesPlatform
	var seen []core.Job
	again := Run(context.Background(), p, done, "简体中文", func(j core.Job) { seen = append(seen, j) })
	if again.Stage != core.StageComplete || !again.PlatformSubtitles || again.LocalLines != 1 {
		t.Fatalf("again %+v", again)
	}
	for _, j := range seen[:len(seen)-1] {
		if j.PlatformSubtitles {
			t.Fatalf("the film took the subtitles before the redo completed: %+v", j)
		}
	}
}

// The download asks for the picture the job chose: the best unless 720P was.
func TestDownloadAsksForTheQuality(t *testing.T) {
	tool, args := argsYTDLP(t)
	tools := Tools{YTDLP: tool, OutputDir: t.TempDir()}
	for quality, want := range map[core.Quality]string{
		"":                "-f\nbv*+ba/b\n",
		core.QualityBest:  "-f\nbv*+ba/b\n",
		core.Quality720:   "-f\nbv*[height<=720]+ba/b[height<=720]/bv*+ba/b\n",
		core.Quality("x"): "-f\nbv*+ba/b\n",
	} {
		tools.Download(withQuality(context.Background(), quality), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
		if data, _ := os.ReadFile(args); !strings.Contains(string(data), want) {
			t.Fatalf("quality %q: args %s", quality, data)
		}
	}
}

// PlatformSubtitles asks yt-dlp for the uploader's subtitles alone, in the language and its variants, takes the language's own before a variant's and leaves out the tags of style.
func TestPlatformSubtitlesFetchesTheUploadersOwn(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	srt := func(text string) string { return "1\n00:00:01,000 --> 00:00:02,500\n" + text + "\n\n" }
	tool := writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, Act: "subtitles", Files: map[string]string{
		"subs.en-GB.srt": srt("Colour"),
		"subs.en.srt":    srt("<i>Hello</i> {\\an8}there"),
	}})
	tools := Tools{YTDLP: tool, FFmpeg: "/usr/bin/ffmpeg", Cookies: "firefox"}
	lines, err := tools.PlatformSubtitles(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw", "en")
	if err != nil {
		t.Fatal(err)
	}
	if want := []core.Segment{{Start: time.Second, End: 2500 * time.Millisecond, Text: "Hello there"}}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines %+v", lines)
	}
	data, _ := os.ReadFile(args)
	got := string(data)
	for _, want := range []string{"--skip-download\n", "--write-subs\n", "--sub-langs\n^en(-.*)?$\n", "--convert-subs\nsrt\n", "--cookies-from-browser\nfirefox\n", "--ffmpeg-location\n/usr/bin/ffmpeg\n", "--\nhttps://www.youtube.com/watch?v=jNQXAC9IVRw\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("args lack %q: %s", want, got)
		}
	}
	if strings.Contains(got, "--write-auto-subs") {
		t.Fatalf("asked for the automatic subtitles: %s", got)
	}

	// A site with none in the language gives none, and no error.
	empty := writeFakeTool(t, filepath.Join(dir, "empty"), "yt-dlp", fakeTool{Act: "subtitles"})
	if lines, err := (Tools{YTDLP: empty}).PlatformSubtitles(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw", "ja"); err != nil || len(lines) != 0 {
		t.Fatalf("lines %+v, err %v", lines, err)
	}
}

func TestSubtitleLanguages(t *testing.T) {
	if got := subtitleLanguages("he"); got != "^he(-.*)?$,^iw(-.*)?$" {
		t.Fatalf("he: %s", got)
	}
	if got := subtitleLanguages("zh"); got != "^zh(-.*)?$" {
		t.Fatalf("zh: %s", got)
	}
}
