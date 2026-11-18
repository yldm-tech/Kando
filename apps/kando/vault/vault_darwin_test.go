//go:build darwin

package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// library makes a library folder holding a film in a folder of its own, with a hidden file beside it, and a loose video, and detaches its image when the test ends.
func library(t *testing.T) string {
	t.Helper()
	// Making and attaching images takes seconds here and minutes on GitHub's macOS runners, more than the Test job's time for a package, so these run where KANDO_VAULT_TESTS is set: by hand, and in CI's Vault job.
	if os.Getenv("KANDO_VAULT_TESTS") == "" {
		t.Skip("set KANDO_VAULT_TESTS=1 to make real encrypted images")
	}
	dir := filepath.Join(t.TempDir(), "Talks.kando")
	files := map[string]string{
		"Film/film.mp4":               "picture",
		"Film/film.srt":               "1\n00:00:00,000 --> 00:00:01,000\nhi\n",
		"Film/.film.translation.json": "{}",
		"loose.mp4":                   "loose picture",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = Close(context.Background(), dir, true)
		_ = detach(context.Background(), filepath.Join(dir, mountName), true)
	})
	return dir
}

// names are the names in dir, but Finder's and the image volume's own.
func names(t *testing.T, dir string) []string {
	t.Helper()
	items, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, item := range items {
		switch item.Name() {
		case markerName, ".metadata_never_index", ".fseventsd", ".Spotlight-V100", ".Trashes", ".TemporaryItems", ".DS_Store":
			continue
		}
		out = append(out, item.Name())
	}
	return out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A library goes into its image and comes out again, as it gets a password, is locked and unlocked, gets another password and has it removed. One test, as making an image takes seconds, more on CI runners.
func TestALibraryLocksIntoItsImageAndOpensAgain(t *testing.T) {
	ctx := context.Background()
	dir := library(t)
	if Has(dir) {
		t.Fatal("a plain folder counts as encrypted")
	}
	var last [2]int64
	if err := Create(ctx, dir, "right horse", func(done, total int64) { last = [2]int64{done, total} }); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); !slices.Equal(got, []string{ImageName}) {
		t.Fatalf("the locked folder holds %v, want only the image", got)
	}
	if !Has(dir) || Mounted(dir) {
		t.Fatalf("after Create: Has %v, Mounted %v; want an encrypted, locked library", Has(dir), Mounted(dir))
	}
	if last[0] != last[1] || last[1] == 0 {
		t.Fatalf("progress ended at %d of %d bytes", last[0], last[1])
	}
	if err := Create(ctx, dir, "again", nil); !errors.Is(err, ErrExists) {
		t.Fatalf("a second Create: %v, want ErrExists", err)
	}

	if err := Open(ctx, dir, "wrong horse"); !errors.Is(err, ErrNotOpened) {
		t.Fatalf("Open with a wrong password: %v, want ErrNotOpened", err)
	}
	if Mounted(dir) {
		t.Fatal("a wrong password opened the library")
	}
	if err := Open(ctx, dir, "right horse"); err != nil {
		t.Fatal(err)
	}
	if !Mounted(dir) || !Has(dir) {
		t.Fatal("the library is not open after Open")
	}
	if got := read(t, filepath.Join(dir, "Film", "film.mp4")); got != "picture" {
		t.Fatalf("film.mp4 holds %q after Open", got)
	}
	if got := read(t, filepath.Join(dir, "Film", ".film.translation.json")); got != "{}" {
		t.Fatalf("the hidden file holds %q after Open", got)
	}
	if got := names(t, dir); !slices.Equal(got, []string{"Film", "loose.mp4"}) {
		t.Fatalf("the open library holds %v", got)
	}
	// What is written while it is open is kept in the image.
	if err := os.WriteFile(filepath.Join(dir, "new.mp4"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Close(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); !slices.Equal(got, []string{ImageName}) {
		t.Fatalf("the folder holds %v after Close, want only the image", got)
	}
	if err := Open(ctx, dir, "right horse"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "new.mp4")); got != "new" {
		t.Fatalf("new.mp4 holds %q after opening again", got)
	}
	if err := Close(ctx, dir, false); err != nil {
		t.Fatal(err)
	}

	// Another password: the old one opens it no more.
	if err := ChangePassword(ctx, dir, "right horse", "new horse"); err != nil {
		t.Fatal(err)
	}
	if err := Open(ctx, dir, "right horse"); !errors.Is(err, ErrNotOpened) {
		t.Fatalf("Open with the old password: %v, want ErrNotOpened", err)
	}

	// An image a Create, Open or Remove cut short left attached at the mount folder, as quitting Kando while one ran would, is detached by Close, so its files are not left readable.
	mount := filepath.Join(dir, mountName)
	if err := attach(ctx, filepath.Join(dir, ImageName), mount, "new horse"); err != nil {
		t.Fatal(err)
	}
	if err := Close(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	if otherDevice(mount, dir) {
		t.Fatal("the image is still attached at the mount folder after Close")
	}

	// No password: the files go back into the folder, once it is locked.
	if err := Open(ctx, dir, "new horse"); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, dir, "new horse", nil); !errors.Is(err, ErrOpen) {
		t.Fatalf("Remove while open: %v, want ErrOpen", err)
	}
	if err := Close(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, dir, "new horse", nil); err != nil {
		t.Fatal(err)
	}
	if Has(dir) || Mounted(dir) {
		t.Fatal("the library is still encrypted after Remove")
	}
	if got := names(t, dir); !slices.Equal(got, []string{"Film", "loose.mp4", "new.mp4"}) {
		t.Fatalf("the folder holds %v after Remove", got)
	}
	if got := read(t, filepath.Join(dir, "Film", ".film.translation.json")); got != "{}" {
		t.Fatalf("the hidden file holds %q after Remove", got)
	}
}

func TestOpenTakesInWhatWasLeftBesideTheImage(t *testing.T) {
	ctx := context.Background()
	dir := library(t)
	if err := Create(ctx, dir, "pw", nil); err != nil {
		t.Fatal(err)
	}
	// A copy left by a Create cut short, the same as the image's, and a file dropped into the locked folder in Finder: one the image has otherwise, and a new one.
	for name, body := range map[string]string{"loose.mp4": "loose picture", "Film/film.srt": "edited", "dropped.mp4": "dropped"} {
		path := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Open(ctx, dir, "pw"); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); !slices.Equal(got, []string{"Film", "dropped.mp4", "loose.mp4"}) {
		t.Fatalf("the open library holds %v", got)
	}
	if got := read(t, filepath.Join(dir, "Film", "film.srt")); got == "edited" {
		t.Fatal("a stray file took the place of the image's")
	}
	if got := read(t, filepath.Join(dir, "Film", "film 2.srt")); got != "edited" {
		t.Fatalf("the stray file that differs holds %q beside the image's, want it kept", got)
	}
	if err := Close(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); !slices.Equal(got, []string{ImageName}) {
		t.Fatalf("the folder holds %v after Close, want only the image", got)
	}
}

// infoDevice finds an image's whole disk in hdiutil info's output, past other images' sections, and across /var and /private/var.
func TestInfoDeviceFindsTheImagesWholeDisk(t *testing.T) {
	info := "framework       : 671.0.1\ndriver          : 671.0.1\n================================================\nimage-path      : /System/Library/AssetsV2/x.dmg\nshadow-path     : <none>\nimage-type      : read-only disk image\n/dev/disk2\tGUID_partition_scheme\t\n/dev/disk3s1\t41504653-0000-11AA-AA11-00306543ECAC\t/Library/Developer\n================================================\nimage-path      : /private/var/folders/x/T/Talks.kando/.kando-vault-new.sparseimage\nshadow-path     : <none>\nimage-encrypted : TRUE\n/dev/disk4\tGUID_partition_scheme\t\n/dev/disk4s1\tEF57347C-0000-11AA-AA11-00306543ECAC\t\n/dev/disk9s1\t41504653-0000-11AA-AA11-00306543ECAC\t/var/folders/x/T/Talks.kando/.kando-vault-mount\n"
	if dev, ok := infoDevice(info, "/var/folders/x/T/Talks.kando/.kando-vault-new.sparseimage"); !ok || dev != "/dev/disk4" {
		t.Fatalf("infoDevice = %q, %v; want /dev/disk4", dev, ok)
	}
	if dev, ok := infoDevice(info, "/var/folders/x/T/Other.kando/.kando-vault.sparseimage"); ok {
		t.Fatalf("an image not attached has device %q", dev)
	}
}
