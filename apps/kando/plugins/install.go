package plugins

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The prefixes of the folders Stage and Swap leave in the plugins folder while they work: a pack being copied in, and one being replaced or removed. Scan skips them; Stage removes those a crash or a failed removal left once they are a day old (staleAfter), so one another Kando is still working in is never taken from under it.
const (
	stagingPrefix = ".staging-"
	oldPrefix     = ".old-"
	staleAfter    = 24 * time.Hour
)

// finderMetadata is the file Finder writes into a folder it shows.
const finderMetadata = ".DS_Store"

// systemMetadata reports whether name is a file a file manager writes into a folder it shows: Finder's .DS_Store, and Explorer's Thumbs.db and desktop.ini (in any case, as Windows names are). Stage leaves them out of a pack copied from a folder, and Validate out of the files of a pack, rather than refusing a pack for a file its author never made; they are never read.
func systemMetadata(name string) bool {
	return name == finderMetadata || strings.EqualFold(name, "Thumbs.db") || strings.EqualFold(name, "desktop.ini")
}

// DefaultRoot is the plugins folder: plugins in Kando's folder of the user's configuration, beside settings.json and library.json. Packs are in <root>/<kind>/<id>, and state.json is in root.
func DefaultRoot() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Kando", "plugins")
}

// budget is what a pack may still take of its limits as it is read: bytes, counted as they are actually read rather than as a zip's headers declare them, and files. A dictionary's nested zips take from the same budget as the pack.
type budget struct {
	bytes   int64
	entries int
}

// take takes n files from the budget, which is ErrTooLarge when that is more than is left.
func (b *budget) take(n int) error {
	b.entries -= n
	if b.entries < 0 {
		return fmt.Errorf("%w: too many files", ErrTooLarge)
	}
	return nil
}

// copy copies src to dst until src ends, and is ErrTooLarge as soon as that is more bytes than are left; it returns the bytes copied.
func (b *budget) copy(dst io.Writer, src io.Reader) (int64, error) {
	n, err := io.Copy(dst, io.LimitReader(src, b.bytes+1))
	b.bytes -= n
	if err == nil && b.bytes < 0 {
		err = fmt.Errorf("%w: the files unpack to too many bytes", ErrTooLarge)
	}
	return n, err
}

// Stage copies the pack at src, a zip file or a folder, into a new folder .staging-<random> of root, and validates the copy (Validate), not src, so src changing while it is read cannot slip a pack past the checks. A zip's entries are unpacked with their bytes counted as they are read, never trusting the sizes its headers declare, and an entry that is a folder, a link or a name reaching outside the pack is refused; a folder is copied file by file, each looked at without following links (Lstat) and refused when it is a link, a folder or a special file. Before it copies, Stage removes what an earlier Stage or Swap left in root a day ago or more. It returns the staged folder, for Swap, and the pack; on an error it leaves nothing behind.
func Stage(root, src string) (staged string, p Pack, err error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", Pack{}, err
	}
	cleanStale(root, time.Now())
	info, err := os.Stat(src)
	if err != nil {
		return "", Pack{}, err
	}
	dir, err := os.MkdirTemp(root, stagingPrefix+"*")
	if err != nil {
		return "", Pack{}, err
	}
	switch {
	case info.IsDir():
		err = copyFolder(dir, src)
	case info.Mode().IsRegular():
		err = unzip(dir, src, info.Size())
	default:
		err = fmt.Errorf("%w: %s is not a file or a folder", ErrUnsafePath, filepath.Base(src))
	}
	if err == nil {
		p, err = Validate(dir)
	}
	if err != nil {
		os.RemoveAll(dir)
		return "", Pack{}, err
	}
	return dir, p, nil
}

// limitsFor is the limits of a pack whose manifest is data, read before the pack is copied so the copy stops at the right limit: a manifest that cannot be read, or whose kind cannot be installed, is the error.
func limitsFor(data []byte) (limits, error) {
	m, err := Parse(data)
	if err != nil {
		return limits{}, err
	}
	if !m.Kind.implemented() {
		return limits{}, fmt.Errorf("%w: %s", ErrUnsupportedKind, m.Kind)
	}
	return limitsOf(m.Kind), nil
}

// unzip unpacks the zip at src, size bytes long, into the folder dst.
func unzip(dst, src string, size int64) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	if size > limitsOf(KindDictionary).zip {
		return fmt.Errorf("%w: the zip is over %d MiB", ErrTooLarge, limitsOf(KindDictionary).zip>>20)
	}
	zr, err := openZip(f, size)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(zr.File, func(zf *zip.File) bool { return zf.Name == ManifestName })
	if i < 0 {
		return fmt.Errorf("%w: no %s at the top of the zip", ErrBadManifest, ManifestName)
	}
	data, err := readEntry(zr.File[i], manifestMax)
	if err != nil {
		return err
	}
	lim, err := limitsFor(data)
	if err != nil {
		return err
	}
	if size > lim.zip {
		return fmt.Errorf("%w: the zip is over %d MiB", ErrTooLarge, lim.zip>>20)
	}
	b := &budget{bytes: lim.bytes, entries: lim.entries}
	seen := map[string]bool{}
	for _, zf := range zr.File {
		if zf.Mode().IsDir() {
			return fmt.Errorf("%w: %s is a folder", ErrBadShape, strings.TrimSuffix(zf.Name, "/"))
		}
		if err := checkFileName(zf.Name); err != nil {
			return err
		}
		if !zf.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is a link or a special file", ErrUnsafePath, zf.Name)
		}
		// Two names a case-insensitive disk would make one file.
		key := strings.ToLower(zf.Name)
		if seen[key] {
			return fmt.Errorf("%w: %s is in the zip twice", ErrBadShape, zf.Name)
		}
		seen[key] = true
		if err := b.take(1); err != nil {
			return err
		}
		if err := unzipEntry(zf, filepath.Join(dst, zf.Name), b); err != nil {
			return err
		}
	}
	return nil
}

// readEntry reads the zip entry zf, which may unpack to at most max bytes.
func readEntry(zf *zip.File, max int64) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w: %s is over %d KiB", ErrTooLarge, zf.Name, max>>10)
	}
	return data, nil
}

// unzipEntry unpacks the zip entry zf into a new file at path, its bytes taken from b. An entry whose bytes do not match its checksum or declared size, or cannot be decompressed, is ErrBadArchive.
func unzipEntry(zf *zip.File, path string, b *budget) error {
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer rc.Close()
	err = writeNew(path, rc, b)
	var pathErr *os.PathError
	if err != nil && !errors.Is(err, ErrTooLarge) && !errors.As(err, &pathErr) {
		err = fmt.Errorf("%w: %s: %v", ErrBadArchive, zf.Name, err)
	}
	return err
}

// writeNew writes src into a new file at path, its bytes taken from b.
func writeNew(path string, src io.Reader, b *budget) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = b.copy(out, src)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

// copyFolder copies the files of the folder src into dst, refusing a folder, a link or a special file in it (listFiles looks at each without following links, and openRegular opens it only if it is still the regular file it looked at). A file manager's files (systemMetadata) are left out.
func copyFolder(dst, src string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if systemMetadata(e.Name()) {
			continue
		}
		switch {
		case e.IsDir():
			return fmt.Errorf("%w: %s is a folder", ErrBadShape, e.Name())
		case !e.Type().IsRegular():
			return fmt.Errorf("%w: %s is a link or a special file", ErrUnsafePath, e.Name())
		}
		if err := checkFileName(e.Name()); err != nil {
			return err
		}
		names = append(names, e.Name())
	}
	if !slices.Contains(names, ManifestName) {
		return fmt.Errorf("%w: no %s in the folder", ErrBadManifest, ManifestName)
	}
	data, err := readSmall(filepath.Join(src, ManifestName), manifestMax)
	if err != nil {
		return err
	}
	lim, err := limitsFor(data)
	if err != nil {
		return err
	}
	b := &budget{bytes: lim.bytes, entries: lim.entries}
	if err := b.take(len(names)); err != nil {
		return err
	}
	for _, name := range names {
		if err := copyFile(filepath.Join(dst, name), filepath.Join(src, name), b); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(dst, src string, b *budget) error {
	in, err := openRegular(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeNew(dst, in, b)
}

// packDir is the folder of the pack id of kind in root, root/<kind>/<id>, checked before it is returned: id is a valid id and a local name (filepath.IsLocal), and the path made of it is inside root/<kind> (filepath.Rel).
func packDir(root string, kind Kind, id string) (string, error) {
	if !kind.known() || !validID(id) {
		return "", fmt.Errorf("%w: %q", ErrBadID, id)
	}
	base := filepath.Join(root, string(kind))
	dir := filepath.Join(base, id)
	rel, err := filepath.Rel(base, dir)
	if err != nil || rel != id || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %q", ErrBadID, id)
	}
	return dir, nil
}

// randomName is prefix and 16 random hex digits, the name of a folder Stage or Swap works in.
func randomName(prefix string) string {
	var b [8]byte
	rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// Swap puts the pack p, staged by Stage, in its place, root/<kind>/<id>, and sets p.Dir to it: the pack already there, if any, is first renamed out of the way to .old-<random> (a folder cannot be renamed over one that is not empty), then the staged folder is renamed into place, and the old one is removed. When the staged folder cannot be put in place the old pack is put back. An id is one pack whatever its kind, as the state and Remove know a pack by its id alone: when a pack of another kind has p's id, Swap refuses with ErrIDTaken and changes nothing, and that pack has to be removed first. The caller closes whatever it holds open in the old pack first. Removing the old folder may fail, as when a file of it is still open on Windows; it is then left for a later Stage to remove (cleanStale), and Swap still succeeds.
func Swap(root, staged string, p *Pack) error {
	if filepath.Dir(staged) != filepath.Clean(root) || !strings.HasPrefix(filepath.Base(staged), stagingPrefix) {
		return fmt.Errorf("plugins: %s is not a folder Stage made in %s", staged, root)
	}
	dst, err := packDir(root, p.Kind, p.ID)
	if err != nil {
		return err
	}
	for _, kind := range kinds {
		if kind == p.Kind {
			continue
		}
		other, err := packDir(root, kind, p.ID)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(other); err == nil {
			return fmt.Errorf("%w: %s is installed as a %s", ErrIDTaken, p.ID, kind)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	old := ""
	if _, err := os.Lstat(dst); err == nil {
		old = filepath.Join(root, randomName(oldPrefix))
		if err := os.Rename(dst, old); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(staged, dst); err != nil {
		if old != "" {
			if back := os.Rename(old, dst); back != nil {
				err = errors.Join(err, back)
			}
		}
		return err
	}
	if old != "" {
		os.RemoveAll(old)
	}
	p.Dir = dst
	return nil
}

// Remove takes the pack id out of root, whatever its kind: it is renamed out of the way to .old-<random> first, so it is gone at once even when removing its files fails or is cut short, and what is left then is removed by a later Stage. id is checked (packDir) before any path is made of it. An id no pack has is ErrNotInstalled.
func Remove(root, id string) error {
	for _, kind := range kinds {
		dir, err := packDir(root, kind, id)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		old := filepath.Join(root, randomName(oldPrefix))
		if err := os.Rename(dir, old); err != nil {
			return err
		}
		os.RemoveAll(old)
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNotInstalled, id)
}

// Scan validates every pack in root (Validate), the same checks a pack passed when it was installed, and returns those that pass, by id, and an Issue for each folder that does not: a pack whose folder is not named after its kind and id (ErrMismatch), and a pack whose id a pack of a kind before it (in the order of kinds) already has (ErrIDTaken), which Swap never installs but a folder copied in by hand may be. Folders whose names start with a dot, those Stage and Swap work in, are skipped.
func Scan(root string) ([]Pack, []Issue) {
	var packs []Pack
	var issues []Issue
	seen := map[string]Kind{}
	for _, kind := range kinds {
		base := filepath.Join(root, string(kind))
		entries, err := os.ReadDir(base)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			issues = append(issues, Issue{Dir: base, Err: err})
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dir := filepath.Join(base, e.Name())
			if !e.IsDir() {
				issues = append(issues, Issue{Dir: dir, Err: fmt.Errorf("%w: not a folder", ErrBadShape)})
				continue
			}
			p, err := Validate(dir)
			if err == nil && (p.ID != e.Name() || p.Kind != kind) {
				err = fmt.Errorf("%w: %s %s in %s/%s", ErrMismatch, p.Kind, p.ID, kind, e.Name())
			}
			if other, ok := seen[p.ID]; err == nil && ok {
				err = fmt.Errorf("%w: %s is installed as a %s too", ErrIDTaken, p.ID, other)
			}
			if err != nil {
				issues = append(issues, Issue{Dir: dir, Err: err})
				continue
			}
			seen[p.ID] = kind
			packs = append(packs, p)
		}
	}
	slices.SortFunc(packs, func(a, b Pack) int { return strings.Compare(a.ID, b.ID) })
	return packs, issues
}

// cleanStale removes the folders Stage and Swap work in (.staging-, .old-) that are in root and were last changed staleAfter or more before now: those a crash, an install never confirmed or a removal that failed left behind. Younger ones may still be in use, by this Kando or another, and stay. What cannot be removed stays for the next time.
func cleanStale(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, stagingPrefix) && !strings.HasPrefix(name, oldPrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < staleAfter {
			continue
		}
		os.RemoveAll(filepath.Join(root, name))
	}
}
