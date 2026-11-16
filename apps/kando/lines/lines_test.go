package lines

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadSplitsAndFinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.bilingual.srt")
	srt := "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello There\n\n2\n00:00:03,000 --> 00:00:04,000\nUntranslated\n\n"
	if err := os.WriteFile(path, []byte(srt), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Start != time.Second || got[0].Translation != "你好" || got[0].Original != "Hello There" || got[1].Translation != "" || got[1].Original != "Untranslated" {
		t.Fatalf("lines %+v", got)
	}
	q := Fold("  THERE ")
	if !got[0].Holds(q) || got[1].Holds(q) || !got[0].Holds(Fold("你")) {
		t.Errorf("Holds %q", q)
	}
	if !Matches("你好", "Hello There", q) || Matches("", "Untranslated", q) {
		t.Errorf("Matches %q", q)
	}
}

func TestSeen(t *testing.T) {
	if !Seen(2500*time.Millisecond, 2*time.Second) || Seen(2501*time.Millisecond, 2*time.Second) {
		t.Error("Seen does not allow the slack exactly")
	}
}
