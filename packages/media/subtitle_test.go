package media

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"lingocut/packages/core"
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
	got, err := parseWhisperJSON([]byte(whisperSample))
	if err != nil {
		t.Fatal(err)
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
	if _, err := parseWhisperJSON([]byte("not json")); err == nil {
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

func TestExportSubtitlesRejectsMismatchedLines(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk.mp4")
	if _, err := exportSubtitles(video, []core.Segment{{Text: "a"}}, nil); err == nil {
		t.Fatal("expected an error")
	}
}
