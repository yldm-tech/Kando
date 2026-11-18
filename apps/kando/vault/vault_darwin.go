//go:build darwin

package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Supported reports whether a library's files can be kept in an encrypted image here: macOS, whose hdiutil makes and attaches them.
func Supported() bool { return true }

// Mounted reports whether the library's image is attached over its folder, so the library's files are there: the folder is on another device than the folder holding it, and the volume is a vault's (markerName), not some other volume mounted there.
func Mounted(dir string) bool {
	if dir == "" {
		return false
	}
	dir = filepath.Clean(dir)
	return otherDevice(dir, filepath.Dir(dir)) && isFile(filepath.Join(dir, markerName))
}

// RootOf is the folder of the open library whose image holds path, and whether one does: the nearest folder above path that is a vault's volume (Mounted).
func RootOf(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	for dir := filepath.Dir(filepath.Clean(path)); ; {
		if Mounted(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// otherDevice reports whether a and b are on different devices: a is a mount point when b is the folder that holds it.
func otherDevice(a, b string) bool {
	var sa, sb syscall.Stat_t
	if syscall.Stat(a, &sa) != nil || syscall.Stat(b, &sb) != nil {
		return false
	}
	return sa.Dev != sb.Dev
}

// maxSize is the most an image may grow to. A sparse image takes on disk only what its files take, so this is no reservation: it only has to be more than any library holds.
const maxSize = "64t"

// hdiutilWait bounds how long making, attaching, detaching or re-keying an image may take; copying files has no bound but its context. It is generous: on a slow or busy disk, as GitHub's macOS runners have, making an image took over a minute.
const hdiutilWait = 5 * time.Minute

// Create moves the files of the library at dir into a new image encrypted with password, inside dir, and leaves the library locked, its folder holding only the image. The files are copied into the image first, which then takes its place (ImageName), and only then are the originals removed: cut short before that, the library is as it was. progress hears the bytes copied.
func Create(ctx context.Context, dir, password string, progress Progress) (err error) {
	dir = filepath.Clean(dir)
	if Has(dir) {
		return ErrExists
	}
	staging, mount := filepath.Join(dir, stagingName), filepath.Join(dir, mountName)
	if err := clearLeftovers(ctx, staging, mount); err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(dir), filepath.Ext(dir))
	if _, err := hdiutil(ctx, password, "create", "-quiet", "-size", maxSize, "-type", "SPARSE", "-fs", "APFS", "-volname", name, "-encryption", "AES-256", "-stdinpass", strings.TrimSuffix(staging, ".sparseimage")); err != nil {
		return fmt.Errorf("vault: make the image: %w", err)
	}
	defer func() {
		if err != nil {
			_ = detach(context.WithoutCancel(ctx), mount, true)
			_ = os.Remove(mount)
			_ = os.Remove(staging)
		}
	}()
	if err := attach(ctx, staging, mount, password); err != nil {
		return err
	}
	// Spotlight leaves a volume with .metadata_never_index at its root alone, so no index of the films' names and subtitles is made in it. A new volume gets the file only once attached, as making the image from a folder holding it (-srcfolder) failed on the macOS of GitHub's runners; Spotlight then holds this first attach for some seconds as it detaches, the later ones not.
	for _, file := range []string{markerName, ".metadata_never_index"} {
		if err := os.WriteFile(filepath.Join(mount, file), nil, 0o600); err != nil {
			return fmt.Errorf("vault: mark the image: %w", err)
		}
	}
	entries, err := libraryEntries(dir)
	if err != nil {
		return err
	}
	if err := copyAll(ctx, dir, mount, entries, progress); err != nil {
		return err
	}
	if err := detach(ctx, mount, false); err != nil {
		return err
	}
	if err := os.Remove(mount); err != nil {
		return err
	}
	if err := os.Rename(staging, filepath.Join(dir, ImageName)); err != nil {
		return err
	}
	// The image holds every file now. Removing the originals is best effort: one left behind stays under the image while it is attached, and Open moves it in.
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(dir, entry))
	}
	return nil
}

// Open attaches the image of the library at dir over its folder with password, so its files are there again. Files found beside the image, as ones dropped into the locked folder in Finder or left by a Create cut short, are moved into the image first, as attaching over them would hide them. A library already open is left as it is.
func Open(ctx context.Context, dir, password string) error {
	dir = filepath.Clean(dir)
	if Mounted(dir) {
		return nil
	}
	image := filepath.Join(dir, ImageName)
	if !isFile(image) {
		return ErrNotVault
	}
	strays, err := libraryEntries(dir)
	if err != nil {
		return err
	}
	if len(strays) > 0 {
		if err := absorb(ctx, dir, image, password, strays); err != nil {
			return err
		}
	}
	return attach(ctx, image, dir, password)
}

// Close detaches the image of the library at dir from its folder, which then holds the image alone, and from mountName, where a Create, Open or Remove cut short, as by Kando quitting while it ran, left it attached with its files readable. force detaches it even while a file in it is open, as quitting does once everything that used the library has stopped; without it a library in use stays open, and Close says why.
func Close(ctx context.Context, dir string, force bool) error {
	dir = filepath.Clean(dir)
	if mount := filepath.Join(dir, mountName); otherDevice(mount, dir) {
		if err := detach(ctx, mount, force); err != nil {
			return err
		}
	}
	if !Mounted(dir) {
		return nil
	}
	return detach(ctx, dir, force)
}

// ChangePassword gives the image of the library at dir the password next in place of current. The library must be locked (Close): the image's file is hidden under its folder while it is attached there.
func ChangePassword(ctx context.Context, dir, current, next string) error {
	dir = filepath.Clean(dir)
	if Mounted(dir) {
		return ErrOpen
	}
	image := filepath.Join(dir, ImageName)
	if !isFile(image) {
		return ErrNotVault
	}
	_, err := hdiutil(ctx, current+"\x00"+next, "chpass", "-oldstdinpass", "-newstdinpass", image)
	return err
}

// Remove moves the files of the library at dir out of its image, back into its folder, and deletes the image, so the library is an ordinary folder again. The library must be locked (Close). The image goes only once every file is copied out: cut short, the library is still encrypted, and the copies made so far are moved back in by the next Open.
func Remove(ctx context.Context, dir, password string, progress Progress) (err error) {
	dir = filepath.Clean(dir)
	if Mounted(dir) {
		return ErrOpen
	}
	image, mount := filepath.Join(dir, ImageName), filepath.Join(dir, mountName)
	if !isFile(image) {
		return ErrNotVault
	}
	if err := clearLeftovers(ctx, "", mount); err != nil {
		return err
	}
	if err := attach(ctx, image, mount, password); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = detach(context.WithoutCancel(ctx), mount, true)
			_ = os.Remove(mount)
		}
	}()
	entries, err := volumeEntries(mount)
	if err != nil {
		return err
	}
	if err := copyAll(ctx, mount, dir, entries, progress); err != nil {
		return err
	}
	if err := detach(ctx, mount, false); err != nil {
		return err
	}
	if err := os.Remove(mount); err != nil {
		return err
	}
	return os.Remove(image)
}

// absorb moves strays, files in the library's folder beside its image, into the image, attached for it at mountName: a file the image does not have goes in, one it has the same, as a copy a Create or Remove cut short left, is only removed, and one it has otherwise goes in beside it under a free name, so nothing is lost.
func absorb(ctx context.Context, dir, image, password string, strays []string) (err error) {
	mount := filepath.Join(dir, mountName)
	if err := clearLeftovers(ctx, "", mount); err != nil {
		return err
	}
	if err := attach(ctx, image, mount, password); err != nil {
		return err
	}
	defer func() {
		if derr := detach(context.WithoutCancel(ctx), mount, err != nil); err == nil {
			err = derr
		}
		_ = os.Remove(mount)
	}()
	for _, name := range strays {
		if err := absorbEntry(ctx, filepath.Join(dir, name), filepath.Join(mount, name)); err != nil {
			return err
		}
	}
	return nil
}

func absorbEntry(ctx context.Context, src, dst string) error {
	si, err := os.Lstat(src)
	if err != nil {
		return err
	}
	di, err := os.Lstat(dst)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := ditto(ctx, src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	case err != nil:
		return err
	case si.IsDir() && di.IsDir():
		children, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := absorbEntry(ctx, filepath.Join(src, child.Name()), filepath.Join(dst, child.Name())); err != nil {
				return err
			}
		}
		return os.Remove(src)
	case si.Mode().IsRegular() && di.Mode().IsRegular() && si.Size() == di.Size():
		return os.Remove(src)
	}
	free := freeName(dst)
	if err := ditto(ctx, src, free); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// freeName is path, or "name 2.ext", "name 3.ext" and so on, the first that is not there.
func freeName(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for n := 2; ; n++ {
		next := fmt.Sprintf("%s %d%s", base, n, ext)
		if _, err := os.Lstat(next); errors.Is(err, fs.ErrNotExist) {
			return next
		}
	}
}

// libraryEntries are the names in the library's folder that are the library's own files: all but the image, its staging image and mount folder, and Finder's .DS_Store.
func libraryEntries(dir string) ([]string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, item := range items {
		switch item.Name() {
		case ImageName, stagingName, mountName, ".DS_Store":
			continue
		}
		out = append(out, item.Name())
	}
	return out, nil
}

// volumeEntries are the names at the root of an image's volume that are the library's files: all but the marker and what macOS keeps at a volume's root.
func volumeEntries(mount string) ([]string, error) {
	items, err := os.ReadDir(mount)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, item := range items {
		switch item.Name() {
		case markerName, ".metadata_never_index", ".fseventsd", ".Spotlight-V100", ".Trashes", ".TemporaryItems", ".DS_Store":
			continue
		}
		out = append(out, item.Name())
	}
	return out, nil
}

// spare is the room copyAll leaves on the disk beyond the files it copies, for the image's own bookkeeping and whatever else writes there meanwhile.
const spare = 1 << 30

// copyAll copies the entries of from into to with ditto, which keeps their dates, permissions and extended attributes, telling progress the bytes copied after each. It copies nothing when to's disk has no room for them all (ErrNoSpace): from and to are on the same disk, the image being a file on it.
func copyAll(ctx context.Context, from, to string, entries []string, progress Progress) error {
	sizes := make([]int64, len(entries))
	var total int64
	for i, name := range entries {
		sizes[i] = treeSize(filepath.Join(from, name))
		total += sizes[i]
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(to, &st); err == nil && int64(st.Bavail)*int64(st.Bsize) < total+spare {
		return fmt.Errorf("%w: %d bytes to copy, %d free", ErrNoSpace, total, int64(st.Bavail)*int64(st.Bsize))
	}
	var done int64
	if progress != nil {
		progress(0, total)
	}
	for i, name := range entries {
		if err := ditto(ctx, filepath.Join(from, name), filepath.Join(to, name)); err != nil {
			return err
		}
		done += sizes[i]
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
}

// treeSize is how many bytes the regular files at path and under it hold.
func treeSize(path string) int64 {
	var n int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// clearLeftovers removes what a Create, Open or Remove cut short may have left: the staging image, when one is named, and the mount folder, detached first if the image is still attached there.
func clearLeftovers(ctx context.Context, staging, mount string) error {
	if otherDevice(mount, filepath.Dir(mount)) {
		if err := detach(ctx, mount, true); err != nil {
			return err
		}
	}
	if err := os.Remove(mount); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("vault: clear %s: %w", mountName, err)
	}
	if staging != "" {
		if err := os.Remove(staging); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("vault: clear %s: %w", stagingName, err)
		}
	}
	return nil
}

// attach attaches image at mount with password, made first when it is not there, hidden from Finder's sidebar and the Desktop (-nobrowse) and with the files owned as they are written (-owners on).
func attach(ctx context.Context, image, mount, password string) error {
	if err := os.MkdirAll(mount, 0o755); err != nil {
		return err
	}
	if _, err := hdiutil(ctx, password, "attach", "-quiet", "-stdinpass", "-nobrowse", "-noautoopen", "-owners", "on", "-mountpoint", mount, image); err != nil {
		return fmt.Errorf("%w: %w", ErrNotOpened, err)
	}
	return nil
}

func detach(ctx context.Context, mount string, force bool) error {
	args := []string{"detach", "-quiet", mount}
	if force {
		args = append(args, "-force")
	}
	_, err := hdiutil(ctx, "", args...)
	return err
}

// hdiutil runs hdiutil with args, giving it stdin, the password or passwords, on its standard input; its error carries what hdiutil wrote, which names no password.
func hdiutil(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, hdiutilWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/hdiutil", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("hdiutil %s: %w: %s", args[0], err, strings.TrimSpace(deprecations(out.String())))
	}
	return out.Bytes(), nil
}

// deprecations is hdiutil's output without the warnings newer macOS versions write that its verbs are deprecated in favour of diskutil image, which older ones do not have.
func deprecations(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "is deprecated") {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}

// ditto copies src to dst, a file or a folder merged into one that is there, keeping dates, permissions and extended attributes.
func ditto(ctx context.Context, src, dst string) error {
	out, err := exec.CommandContext(ctx, "/usr/bin/ditto", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("vault: copy %s: %w: %s", filepath.Base(src), err, strings.TrimSpace(string(out)))
	}
	return nil
}
