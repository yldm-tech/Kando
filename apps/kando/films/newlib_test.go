package films

import (
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

// TestLibraryFolder checks the folder name of a new library: its name with .kando, a name that is empty or holds a path refused everywhere, and on Windows, where the folder is made, the characters and device names Windows refuses in a file's name refused with words that say so, rather than the system's error once the folder cannot be made.
func TestLibraryFolder(t *testing.T) {
	m := testlocale.Messages()
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if got, err := LibraryFolder(m, goos, "  英语听力.kando "); err != nil || got != "英语听力.kando" {
			t.Errorf("%s: LibraryFolder = %q, %v", goos, got, err)
		}
		if _, err := LibraryFolder(m, goos, " "); err == nil || err.Error() != m.LibraryNameEmpty() {
			t.Errorf("%s: an empty name: %v", goos, err)
		}
		if _, err := LibraryFolder(m, goos, `a\b`); err == nil || err.Error() != m.LibraryNameHasPath() {
			t.Errorf("%s: a path: %v", goos, err)
		}
	}
	for _, name := range []string{"Talks: 2026", `"Quoted"`, "What?", "a|b", "a*", "<x>", "tab\there", "CON", "con", "Nul.old", "COM1", "lpt9", "COM¹", "AUX .x"} {
		if _, err := LibraryFolder(m, "windows", name); err == nil || err.Error() != m.LibraryNameNotAllowed() {
			t.Errorf("windows: LibraryFolder(%q) = %v, want %q", name, err, m.LibraryNameNotAllowed())
		}
		if strings.ContainsAny(name, "/\\") {
			continue
		}
		if got, err := LibraryFolder(m, "darwin", name); err != nil || got != strings.TrimSpace(name)+".kando" {
			t.Errorf("darwin: LibraryFolder(%q) = %q, %v", name, got, err)
		}
	}
	for _, name := range []string{"CONSOLE", "COM10", "Console", "Comics", "Videos 1"} {
		if _, err := LibraryFolder(m, "windows", name); err != nil {
			t.Errorf("windows: LibraryFolder(%q): %v", name, err)
		}
	}
}
