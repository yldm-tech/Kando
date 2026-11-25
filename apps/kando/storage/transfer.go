package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kando/packages/remotefs"
)

// ErrSizeMismatch is Put's and Get's error for a copy that does not come out as long as what it was copied from, as when a store kept less than it was sent; the copy is not used.
var ErrSizeMismatch = errors.New("storage: the copy's size does not match")

// Progress hears how many of total bytes a copy has moved so far.
type Progress func(done, total int64)

// Put copies the file at local on this computer to p on provider, telling progress how far it got, and checks that the provider then holds as many bytes at p as the file has; it returns that size. The file is left as it is, whatever happens. A copy stopped by ctx, or one the provider stored fewer bytes of, fails: with the latter (ErrSizeMismatch) Put removes what the provider holds at p, as it is not the file.
func Put(ctx context.Context, provider remotefs.Provider, p, local string, progress Progress) (int64, error) {
	f, err := os.Open(local)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("%s: %w", local, remotefs.ErrIsDir)
	}
	size := info.Size()
	if err := provider.Write(ctx, p, &watchedReader{ctx: ctx, r: f, total: size, progress: progress}, size); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	got, err := provider.Stat(ctx, p)
	if err != nil {
		return 0, err
	}
	if got.Dir || got.Size != size {
		_ = provider.Remove(context.WithoutCancel(ctx), p)
		return 0, fmt.Errorf("%s: %d of %d bytes: %w", p, got.Size, size, ErrSizeMismatch)
	}
	return size, nil
}

// Get copies the file at p of reader, a provider or a media source, to local on this computer, telling progress how far it got: into a hidden file beside local, renamed to local once it holds as many bytes as the reader says the file has, so local is never a part of it. It does not write over a file already at local. On any failure, a stop by ctx among them, nothing is left beside local.
func Get(ctx context.Context, reader remotefs.Reader, p, local string, progress Progress) error {
	info, err := reader.Stat(ctx, p)
	if err != nil {
		return err
	}
	if info.Dir {
		return fmt.Errorf("%s: %w", p, remotefs.ErrIsDir)
	}
	if _, err := os.Lstat(local); err == nil {
		return fmt.Errorf("%s: %w", local, os.ErrExist)
	}
	r, err := reader.Open(ctx, p, 0, -1)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(local), remotefs.TempPrefix+rand.Text())
	f, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, &watchedReader{ctx: ctx, r: r, total: info.Size, progress: progress})
	if err == nil && n != info.Size {
		err = fmt.Errorf("%s: %d of %d bytes: %w", p, n, info.Size, ErrSizeMismatch)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		// A file that turned up at local meanwhile is not written over.
		if _, statErr := os.Lstat(local); statErr == nil {
			err = fmt.Errorf("%s: %w", local, os.ErrExist)
		}
	}
	if err == nil {
		err = os.Rename(temp, local)
	}
	if err != nil {
		_ = os.Remove(temp)
	}
	return err
}

// watchedReader reads r, telling progress how many of total bytes it has read, and fails once ctx is done, so a provider that does not watch ctx while it reads its body, as Local does not, stops too.
type watchedReader struct {
	ctx      context.Context
	r        io.Reader
	n, total int64
	progress Progress
}

func (w *watchedReader) Read(b []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.r.Read(b)
	w.n += int64(n)
	if n > 0 && w.progress != nil {
		w.progress(w.n, w.total)
	}
	return n, err
}

// FilmPath is where Kando keeps a film's video moved out of a library on a provider: in a folder Kando, then one named after the library, at the path rel the video had in the library's folder (with the separators of this system). It is "" for a rel that leaves the library's folder.
func FilmPath(library, rel string) string {
	rel = filepath.ToSlash(rel)
	library = strings.TrimSpace(strings.NewReplacer("/", "-", "\\", "-").Replace(library))
	if library == "" || library == "." || library == ".." {
		library = "Library"
	}
	p, err := remotefs.CleanFile(path.Join("Kando", library, rel))
	if err != nil || !strings.HasPrefix(p, "Kando/"+library+"/") {
		return ""
	}
	return p
}
