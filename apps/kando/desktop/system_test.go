package desktop

import (
	"slices"
	"testing"
)

// TestCommandsTakeThePathAsItIs checks the command lines Reveal and Trash run: each tool is the system's own, and the path reaches it as one argument of its own, quotes, spaces and a leading dash included, never through a shell or the text of a script.
func TestCommandsTakeThePathAsItIs(t *testing.T) {
	path := `/Users/me/Movies/It's "a" -clip\.mp4`
	for _, c := range []struct {
		name string
		args []string
		want []string
	}{
		{"open -R", openRevealCommand(path).Args, []string{"/usr/bin/open", "-R", path}},
		{"trash", macTrashCommand(path).Args, []string{"/usr/bin/trash", path}},
		{"Finder", finderTrashCommand(path).Args, []string{"/usr/bin/osascript", "-e", "on run argv", "-e", "set target to POSIX file (item 1 of argv) as alias", "-e", `tell application "Finder" to delete target`, "-e", "end run", path}},
		{"gio trash", gioTrashCommand("/usr/bin/gio", path).Args, []string{"/usr/bin/gio", "trash", path}},
	} {
		if !slices.Equal(c.args, c.want) {
			t.Errorf("%s runs %q, want %q", c.name, c.args, c.want)
		}
	}
}

// TestShowItemsNamesTheItemByItsURI checks the FileManager1 call: the item as a file: URI with its special characters escaped, written as GVariant text whose quotes and backslashes are escaped too.
func TestShowItemsNamesTheItemByItsURI(t *testing.T) {
	got := showItemsCommand("/usr/bin/gdbus", "/home/me/Videos/It's a\\clip.mp4").Args
	want := []string{"/usr/bin/gdbus", "call", "--session", "--dest", "org.freedesktop.FileManager1", "--object-path", "/org/freedesktop/FileManager1", "--method", "org.freedesktop.FileManager1.ShowItems", `['file:///home/me/Videos/It\'s%20a%5Cclip.mp4']`, "''"}
	if !slices.Equal(got, want) {
		t.Fatalf("gdbus runs\n%q\nwant\n%q", got, want)
	}
}

// TestExplorerSelectsTheQuotedPath checks File Explorer's command line: /select, with the path quoted, so a path with spaces stays one.
func TestExplorerSelectsTheQuotedPath(t *testing.T) {
	if got, want := explorerSelectLine(`C:\Users\me\My Films\talk.mp4`), `explorer.exe /select,"C:\Users\me\My Films\talk.mp4"`; got != want {
		t.Fatalf("explorer runs %s, want %s", got, want)
	}
}
