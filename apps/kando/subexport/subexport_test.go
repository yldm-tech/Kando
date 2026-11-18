package subexport

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/packages/core"
)

func TestFileNameNeverTakesKandosOwn(t *testing.T) {
	for _, tc := range []struct {
		c    Choice
		want string
	}{
		{Choice{Format: core.FormatSRT, Content: core.SubBilingual}, "video.dual.srt"},
		{Choice{Format: core.FormatSRT, Content: core.SubOriginal}, "video.original.srt"},
		{Choice{Format: core.FormatVTT, Content: core.SubTranslation}, "video.translation.vtt"},
		{Choice{Format: core.FormatASS, Content: core.SubBilingual}, "video.dual.ass"},
	} {
		got := tc.c.FileName(filepath.Join("films", "a", "video.mp4"))
		if got != tc.want {
			t.Fatalf("%+v names %q, want %q", tc.c, got, tc.want)
		}
		for _, own := range []string{"video.srt", "video.kando.srt", "video.bilingual.srt"} {
			if got == own {
				t.Fatalf("%+v takes Kando's %q", tc.c, own)
			}
		}
	}
}

func TestCleanKeepsOfferedValues(t *testing.T) {
	if got := (Choice{Format: "docx", Content: "?", Order: "", Missing: "x", Delay: false}).Clean(); got != (Choice{Format: core.FormatSRT, Content: core.SubBilingual, Order: core.SubTranslationFirst, Missing: core.MissingSkip}) {
		t.Fatalf("cleaned %+v", got)
	}
	c := Choice{Format: core.FormatASS, Content: core.SubOriginal, Order: core.SubOriginalFirst, Missing: core.MissingOriginal, Delay: true}
	if got := c.Clean(); got != c {
		t.Fatalf("cleaned %+v, want it as it was", got)
	}
}

func TestExportTakesTheDelayOnlyWhenChosen(t *testing.T) {
	if e := (Choice{Delay: true}).Export(time.Second, "?"); e.Delay != time.Second || e.Placeholder != "?" {
		t.Fatalf("export %+v", e)
	}
	if e := (Choice{}).Export(time.Second, "?"); e.Delay != 0 {
		t.Fatalf("export %+v moves the times", e)
	}
}

func TestWriteReplacesTheFileWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.dual.srt")
	if err := os.WriteFile(path, []byte("old and longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	entries, _ := os.ReadDir(filepath.Dir(path))
	if string(data) != "new" || len(entries) != 1 {
		t.Fatalf("file holds %q, folder %d entries", data, len(entries))
	}
}
