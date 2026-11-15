package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

// TestRevealAsksFinderForTheAbsolutePath checks that a relative path reaches open -R absolute, so it cannot read as one of open's options.
func TestRevealAsksFinderForTheAbsolutePath(t *testing.T) {
	ran := fakeTools(t, nil)
	t.Chdir(t.TempDir())
	if err := revealItem("-clip.mp4"); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs("-clip.mp4")
	if len(*ran) != 1 || !slices.Equal((*ran)[0].Args, []string{"/usr/bin/open", "-R", abs}) {
		t.Fatalf("ran %v", *ran)
	}
}

// TestTrashAsksNSFileManager checks that Trash moves the item with NSFileManager through osascript, given the absolute path, and that a failure leaves the item where it is and answers ErrNotTrashed, without what osascript wrote, which goes to the log.
func TestTrashAsksNSFileManager(t *testing.T) {
	film := filepath.Join(t.TempDir(), "Spring Clip.mp4")
	if err := os.WriteFile(film, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := fakeTools(t, nil)
	if err := trashItem(film); err != nil || len(*ran) != 1 || !slices.Equal((*ran)[0].Args, fileManagerTrashCommand(film).Args) {
		t.Fatalf("trash answered %v, ran %v", err, *ran)
	}

	ran = fakeTools(t, errors.New("osascript: exit status 1: execution error: Error: The volume has no Trash. (-2700)"))
	var err error
	app := start(t, map[string]func(*core.Scope){"Trash": func(cx *core.Scope) { err = Trash(cx, film) }})
	app.Press("text:Trash")
	app.Settle()
	if err != ErrNotTrashed || len(*ran) != 1 || (*ran)[0].Path != "/usr/bin/osascript" || (*ran)[0].Args[len((*ran)[0].Args)-1] != film {
		t.Fatalf("Trash answered %v, ran %v", err, *ran)
	}
	if _, statErr := os.Stat(film); statErr != nil {
		t.Fatalf("the item is gone: %v", statErr)
	}
}
