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

	path, err := exportSubtitles(video, original, translated)
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

// A line the translation left untranslated is written as its original alone, which the player reads back as a cue with no translation yet, rather than as an empty line that would end the SRT cue before its text.
func TestExportSubtitlesKeepsAnUntranslatedLine(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk.mp4")
	original := []core.Segment{{Start: 0, End: time.Second, Text: "Hello."}, {Start: time.Second, End: 2 * time.Second, Text: "Bye."}}
	path, err := exportSubtitles(video, original, []core.Segment{{Text: ""}, {Text: "再见。"}})
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

// A local video with subtitles of the user's own beside it, Movie.srt, keeps them: Kando's transcript goes beside them as Movie.kando.srt, which translating it again reads, and processing the video again later leaves Movie.srt as it is too. A video whose Movie.srt is Kando's own, exported with its bilingual subtitles, has it written again.
func TestMakeLeavesTheUsersOwnSubtitles(t *testing.T) {
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
	done := Make(context.Background(), localSteps{}, job, "简体中文", "mac", nil, func(core.Job) {})
	if done.Stage != core.StageComplete || done.SubtitlePath != filepath.Join(dir, "Movie.bilingual.srt") {
		t.Fatalf("done %+v", done)
	}
	if read("Movie.srt") != own || !strings.Contains(read("Movie.kando.srt"), "hello") || TranscriptOf(video) != filepath.Join(dir, "Movie.kando.srt") {
		t.Fatalf("Movie.srt %q, Movie.kando.srt %q", read("Movie.srt"), read("Movie.kando.srt"))
	}

	again := done
	again.Stage, again.Redo = core.StageQueued, core.RedoTranslation
	p := &redoPipeline{t: t}
	if final, _ := remake(p, again, "日本語"); final.Stage != core.StageComplete || !reflect.DeepEqual(p.sent, []string{"hello"}) {
		t.Fatalf("translated again %+v, sent %q", final, p.sent)
	}
	fresh, _ := core.NewJob(core.FileURL(video))
	if final := Make(context.Background(), localSteps{}, fresh, "简体中文", "mac", nil, func(core.Job) {}); final.Stage != core.StageComplete || read("Movie.srt") != own {
		t.Fatalf("processed again %+v, Movie.srt %q", final, read("Movie.srt"))
	}

	// Kando's own transcript, beside the bilingual subtitles it exported with it, is Kando's to write again.
	if err := os.Remove(filepath.Join(dir, "Movie.kando.srt")); err != nil {
		t.Fatal(err)
	}
	fresh, _ = core.NewJob(core.FileURL(video))
	if final := Make(context.Background(), localSteps{}, fresh, "简体中文", "mac", nil, func(core.Job) {}); final.Stage != core.StageComplete || !strings.Contains(read("Movie.srt"), "hello") {
		t.Fatalf("processed again %+v, Movie.srt %q", final, read("Movie.srt"))
	}
}

// A film's subtitles that are being read while they are written out are replaced once the reader is done with them. Windows refuses to replace a file another handle holds open, as Go's os.ReadFile does without letting it be deleted, so on Windows the write waits for the reader rather than failing: the player that shows a film while it is made reads its subtitles each time more lines are written out, and a write that failed so failed the film's job (TestOpenFilmStartsLocalVideo, whose film follows its job, closed the film and lost its job on the Windows runner so); elsewhere it replaces the file at once.
func TestSubtitlesBeingReadAreReplacedOnceTheReaderIsDone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "talk.bilingual.srt")
	if err := writeFileAtomic(path, []byte("before")); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(100 * time.Millisecond)
		reader.Close()
	}()
	if err := writeFileAtomic(path, []byte("after")); err != nil {
		t.Fatalf("the write failed while the subtitles were read: %v", err)
	}
	<-done
	if data, err := os.ReadFile(path); err != nil || string(data) != "after" {
		t.Fatalf("read back %q, %v", data, err)
	}
}
