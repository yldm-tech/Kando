package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestRevealNeedsGDBus checks that without gdbus Reveal's selecting fails, so it opens the folder instead, and with it calls ShowItems.
func TestRevealNeedsGDBus(t *testing.T) {
	ran := fakeTools(t, nil)
	film := filepath.Join(t.TempDir(), "clip.mp4")
	if err := revealItem(film); err != nil || len(*ran) != 1 || (*ran)[0].Path != "/fake/bin/gdbus" {
		t.Fatalf("reveal answered %v, ran %v", err, *ran)
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if err := revealItem(film); !errors.Is(err, errNoTool) || len(*ran) != 1 {
		t.Fatalf("reveal without gdbus answered %v, ran %v", err, *ran)
	}
}

// TestTrashUsesGio checks that gio trash moves the item, and that without gio Trash fails as unsupported and leaves the item where it is.
func TestTrashUsesGio(t *testing.T) {
	film := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(film, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := fakeTools(t, nil)
	if err := trashItem(film); err != nil || len(*ran) != 1 || !slices.Equal((*ran)[0].Args, []string{"/fake/bin/gio", "trash", film}) {
		t.Fatalf("trash answered %v, ran %v", err, *ran)
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if err := trashItem(film); !errors.Is(err, errors.ErrUnsupported) || len(*ran) != 1 {
		t.Fatalf("trash without gio answered %v, ran %v", err, *ran)
	}
	if _, err := os.Stat(film); err != nil {
		t.Fatalf("the item is gone: %v", err)
	}
}
