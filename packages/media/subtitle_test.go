package media

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// Trimmed from whisper-cli 1.9.5 output with -oj.
const whisperSample = `{
	"result": {"language": "en"},
	"transcription": [
		{"timestamps": {"from": "00:00:00,000", "to": "00:00:04,880"}, "offsets": {"from": 0, "to": 4880}, "text": " Hello world."},
		{"timestamps": {"from": "00:00:04,880", "to": "00:00:05,000"}, "offsets": {"from": 4880, "to": 5000}, "text": "  "},
		{"timestamps": {"from": "01:02:03,004", "to": "01:02:05,000"}, "offsets": {"from": 3723004, "to": 3725000}, "text": " Bye."}
	]
}`

func TestParseWhisperJSON(t *testing.T) {
	got, spoken, err := parseWhisperJSON([]byte(whisperSample))
	if err != nil {
		t.Fatal(err)
	}
	if spoken != "en" {
		t.Fatalf("heard %q, want en", spoken)
	}
	want := []core.Segment{
		{Start: 0, End: 4880 * time.Millisecond, Text: "Hello world."},
		{Start: 3723004 * time.Millisecond, End: 3725 * time.Second, Text: "Bye."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseWhisperJSONRejectsGarbage(t *testing.T) {
	if _, _, err := parseWhisperJSON([]byte("not json")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestExportSubtitles(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk [abc].mp4")
	original := []core.Segment{
		{Start: 0, End: 1500 * time.Millisecond, Text: "Hello."},
		{Start: 3723004 * time.Millisecond, End: 3725 * time.Second, Text: "Bye."},
	}
	translated := []core.Segment{{Text: "你好。"}, {Text: "再见。"}}

	path, err := exportSubtitles(context.Background(), video, original, translated)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(filepath.Dir(video), "talk [abc].bilingual.srt"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}

	bilingual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantBilingual := "1\n00:00:00,000 --> 00:00:01,500\n你好。\nHello.\n\n2\n01:02:03,004 --> 01:02:05,000\n再见。\nBye.\n\n"
	if string(bilingual) != wantBilingual {
		t.Fatalf("bilingual srt:\n%s", bilingual)
	}

	plain, err := os.ReadFile(filepath.Join(filepath.Dir(video), "talk [abc].srt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "1\n00:00:00,000 --> 00:00:01,500\nHello.\n\n2\n01:02:03,004 --> 01:02:05,000\nBye.\n\n"; string(plain) != want {
		t.Fatalf("original srt:\n%s", plain)
	}
}

func TestParseSRTRoundTrip(t *testing.T) {
	original := []core.Segment{
		{Start: time.Second, End: 2500 * time.Millisecond, Text: "你好。\nHello."},
		{Start: time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond, End: time.Hour + 2*time.Minute + 5*time.Second, Text: "再见。\nBye."},
	}
	got, err := ParseSRT([]byte(formatSRT(original)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("got %#v", got)
	}
}

func TestParseSRTRejectsGarbage(t *testing.T) {
	if _, err := ParseSRT([]byte("not a subtitle")); err == nil {
		t.Fatal("expected an error")
	}
	got, err := ParseSRT(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %#v", err, got)
	}
}

func TestExportSubtitlesRejectsMismatchedLines(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk.mp4")
	if _, err := exportSubtitles(context.Background(), video, []core.Segment{{Text: "a"}}, nil); err == nil {
		t.Fatal("expected an error")
	}
}

// A line the translation left untranslated is written as its original alone, which the player reads back as a cue with no translation yet, rather than as an empty line that would end the SRT cue before its text.
func TestExportSubtitlesKeepsAnUntranslatedLine(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk.mp4")
	original := []core.Segment{{Start: 0, End: time.Second, Text: "Hello."}, {Start: time.Second, End: 2 * time.Second, Text: "Bye."}}
	path, err := exportSubtitles(context.Background(), video, original, []core.Segment{{Text: ""}, {Text: "再见。"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSRT(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []core.Segment{{Start: 0, End: time.Second, Text: "Hello."}, {Start: time.Second, End: 2 * time.Second, Text: "再见。\nBye."}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read back %#v from\n%s", got, data)
	}
}

// ownSubtitles is Tools reading a local video in place, as localSteps, which exports its subtitles beside the video as Tools does.
type ownSubtitles struct{ localSteps }

func (ownSubtitles) Export(ctx context.Context, video string, original, translated []core.Segment) (string, error) {
	return exportSubtitles(ctx, video, original, translated)
}

// A local video with subtitles of the user's own beside it, Movie.srt, keeps them: Kando's transcript goes beside them as Movie.kando.srt, which the film shows while it is translated and which translating it again reads, and processing the video again later leaves Movie.srt as it is too. A video whose Movie.srt is Kando's own, exported with its bilingual subtitles, has it written again.
func TestRunLeavesTheUsersOwnSubtitles(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Movie.mp4")
	own := "1\n00:00:01,000 --> 00:00:02,000\nThe user's own line.\n\n"
	for path, body := range map[string]string{video: "v", filepath.Join(dir, "Movie.srt"): own} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	job, err := core.NewJob(core.FileURL(video))
	if err != nil {
		t.Fatal(err)
	}
	var shown []string
	done := Run(context.Background(), ownSubtitles{}, job, "简体中文", func(j core.Job) {
		if j.Stage == core.StageTranslating {
			shown = append(shown, j.SubtitlePath)
		}
	})
	if done.Stage != core.StageComplete || done.SubtitlePath != filepath.Join(dir, "Movie.bilingual.srt") {
		t.Fatalf("done %+v", done)
	}
	if read("Movie.srt") != own || !strings.Contains(read("Movie.kando.srt"), "hello") || TranscriptOf(video) != filepath.Join(dir, "Movie.kando.srt") {
		t.Fatalf("Movie.srt %q, Movie.kando.srt %q", read("Movie.srt"), read("Movie.kando.srt"))
	}
	if len(shown) == 0 || shown[0] != filepath.Join(dir, "Movie.kando.srt") {
		t.Fatalf("shown while translating %q", shown)
	}

	again := done
	again.Stage, again.Redo = core.StageQueued, core.RedoTranslation
	p := &redoPipeline{t: t}
	if final := Run(context.Background(), p, again, "日本語", func(core.Job) {}); final.Stage != core.StageComplete || !reflect.DeepEqual(p.sent, []string{"hello"}) {
		t.Fatalf("translated again %+v, sent %q", final, p.sent)
	}
	fresh, _ := core.NewJob(core.FileURL(video))
	if final := Run(context.Background(), ownSubtitles{}, fresh, "简体中文", func(core.Job) {}); final.Stage != core.StageComplete || read("Movie.srt") != own {
		t.Fatalf("processed again %+v, Movie.srt %q", final, read("Movie.srt"))
	}

	// Kando's own transcript, beside the bilingual subtitles it exported with it, is Kando's to write again.
	if err := os.Remove(filepath.Join(dir, "Movie.kando.srt")); err != nil {
		t.Fatal(err)
	}
	fresh, _ = core.NewJob(core.FileURL(video))
	if final := Run(context.Background(), ownSubtitles{}, fresh, "简体中文", func(core.Job) {}); final.Stage != core.StageComplete || !strings.Contains(read("Movie.srt"), "hello") {
		t.Fatalf("processed again %+v, Movie.srt %q", final, read("Movie.srt"))
	}
}
