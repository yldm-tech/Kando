package browserext

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The app's extension folder is among its resources: in the macOS bundle's Contents/Resources, and beside the executable elsewhere, as nsgo build puts them.
func TestShippedIsAmongTheResources(t *testing.T) {
	root := t.TempDir()
	mac := filepath.Join(root, "Kando.app", "Contents", "MacOS", "Kando")
	if got, want := filepath.Clean(Shipped("darwin", mac)), filepath.Join(root, "Kando.app", "Contents", "Resources", FolderName); got != want {
		t.Fatalf("macOS: %s, want %s", got, want)
	}
	exe := filepath.Join(root, "Kando", "Kando.exe")
	if got, want := Shipped("windows", exe), filepath.Join(root, "Kando", FolderName); got != want {
		t.Fatalf("Windows: %s, want %s", got, want)
	}
}

// Install copies the folder whole, replacing an older copy and what it held, and says when the app has none.
func TestInstallCopiesTheFolder(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "src"), filepath.Join(root, "Kando", FolderName)
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(src, dst); !errors.Is(err, ErrNotShipped) {
		t.Fatalf("without the folder: %v", err)
	}
	write(filepath.Join(src, "manifest.json"), `{"version":"1"}`)
	write(filepath.Join(src, "_locales", "en", "messages.json"), `{}`)
	write(filepath.Join(dst, "stale.js"), "old")
	if Installed(src, dst) {
		t.Fatal("an old copy counts as installed")
	}
	got, err := Install(src, dst)
	if err != nil || got != dst {
		t.Fatalf("Install = %q, %v", got, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "_locales", "en", "messages.json")); err != nil || string(data) != "{}" {
		t.Fatalf("copied %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "stale.js")); !os.IsNotExist(err) {
		t.Fatalf("the old copy's file stays: %v", err)
	}
	if !Installed(src, dst) {
		t.Fatal("the copy is not installed")
	}
	entries, _ := os.ReadDir(filepath.Dir(dst))
	if len(entries) != 1 {
		t.Fatalf("beside the copy: %v", entries)
	}
}
