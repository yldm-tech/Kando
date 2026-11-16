package browserext

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FolderName is the extension's folder among the app's resources (apps/kando/resources), and the name of the copy Install makes beside Kando's settings.
const FolderName = "browser-extension"

// Shipped is the extension's folder the app was built with, for the executable exe on goos: among the resources nsgo build puts in Contents/Resources of the macOS bundle, and beside the executable on Windows and Linux (nsgo's cmd/nsgo/resources.go and internal/platform/imageload's ResourceRoot). exe is the app's own executable, not the AppImage it may run from.
func Shipped(goos, exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if goos == "darwin" {
		return filepath.Join(filepath.Dir(exe), "..", "Resources", FolderName)
	}
	return filepath.Join(filepath.Dir(exe), FolderName)
}

// ErrNotShipped is Install's error when the app has no extension folder, as a build that is not packaged (go build, go run).
var ErrNotShipped = errors.New("this build of Kando has no browser extension folder")

// Install copies the extension's folder src to dst, replacing what dst held, and returns dst. The browsers load the extension from dst, which stays where it is as Kando updates or moves, and runs from a place that goes away, as the AppImage's folder, which is another each time it runs. It reads and writes the disk, so it runs off the UI thread.
func Install(src, dst string) (string, error) {
	if _, err := os.Stat(filepath.Join(src, "manifest.json")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrNotShipped
		}
		return "", err
	}
	// The copy is made beside dst and renamed over it, so a browser reloading the extension meanwhile finds the old one or the new one whole.
	tmp := dst + ".new"
	if err := os.RemoveAll(tmp); err != nil {
		return "", err
	}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(tmp, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	old := dst + ".old"
	if err := os.RemoveAll(old); err != nil {
		return "", err
	}
	if err := os.Rename(dst, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		os.RemoveAll(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, os.RemoveAll(old)
}

// Installed reports whether dst holds the extension as src ships it, file for file.
func Installed(src, dst string) bool {
	same := true
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		a, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil || string(a) != string(b) {
			same = false
			return filepath.SkipAll
		}
		return nil
	})
	return err == nil && same
}
