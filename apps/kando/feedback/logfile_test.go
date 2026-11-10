package feedback

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLogFileRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), LogName)
	w, err := OpenLogFile(path, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	for i := range 12 {
		if _, err := w.Write([]byte(strings.Repeat(string(rune('a'+i)), 39) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	// Two 40-byte entries fit in 100 bytes, so the twelve fill six files: kando.log keeps the last two, and the two before it are kept.
	for file, want := range map[string]string{LogName: "k", "kando.1.log": "i", "kando.2.log": "g"} {
		got := readLog(t, filepath.Join(filepath.Dir(path), file))
		if len(got) != 80 || got[0] != want[0] {
			t.Fatalf("%s holds %q", file, got)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "kando.3.log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("kando.3.log was kept: %v", err)
	}
}

// A log file that was already full when the app quit is moved aside when the app opens it again.
func TestFullLogFileIsMovedAsideOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LogName)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenLogFile(path, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	w.Write([]byte("new\n"))
	if got := readLog(t, path); got != "new\n" {
		t.Fatalf("kando.log holds %q", got)
	}
	if got := readLog(t, filepath.Join(dir, "kando.1.log")); got != strings.Repeat("x", 100) {
		t.Fatalf("kando.1.log holds %q", got)
	}
}

// Whatever is shaped like a credential reaches neither the file nor stderr.
func TestLogFileHidesTokens(t *testing.T) {
	var echo strings.Builder
	path := filepath.Join(t.TempDir(), LogName)
	w, err := OpenLogFile(path, LogLimit, &echo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	w.Write([]byte("Authorization: Bearer abcdefghijklmnop; key sk-proj-abcdefghijklmnopqrstuv\n"))
	for _, got := range []string{readLog(t, path), echo.String()} {
		if strings.Contains(got, "abcdefghijklmnop") || !strings.Contains(got, "Bearer ***") {
			t.Fatalf("logged %q", got)
		}
	}
}
