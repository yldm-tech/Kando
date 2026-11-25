package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kando/packages/remotefs"
)

// Local is a remotefs.Provider over a folder of this computer, or of a NAS mounted on it. Every operation goes through an os.Root of the folder, so neither a path nor a symlink in the folder reaches outside it.
type Local struct {
	dir string
}

// NewLocal is a Local over the folder dir, which must exist by the time it is used.
func NewLocal(dir string) *Local { return &Local{dir: dir} }

// Dir is the folder Local keeps its files in.
func (l *Local) Dir() string { return l.dir }

// local is p cleaned and in the form os.Root takes, "." for the root.
func local(p string) (string, error) {
	p, err := remotefs.CleanPath(p)
	if err != nil {
		return "", err
	}
	if p == "" {
		return ".", nil
	}
	name := filepath.FromSlash(p)
	if !filepath.IsLocal(name) {
		// A Windows device name such as NUL, or a drive-relative path such as C:x.
		return "", fmt.Errorf("%q: %w", p, remotefs.ErrInvalidPath)
	}
	return name, nil
}

// localFile is local for a path that must not be the root.
func localFile(p string) (string, error) {
	if _, err := remotefs.CleanFile(p); err != nil {
		return "", err
	}
	return local(p)
}

// within runs fn with the root opened, and has an error that says the path holds nothing read as ErrNotExist.
func (l *Local) within(fn func(root *os.Root) error) error {
	root, err := os.OpenRoot(l.dir)
	if err != nil {
		return notExist(err)
	}
	defer root.Close()
	return notExist(fn(root))
}

func notExist(err error) error {
	if err != nil && errors.Is(err, fs.ErrNotExist) && !errors.Is(err, remotefs.ErrNotExist) {
		return fmt.Errorf("%w: %v", remotefs.ErrNotExist, err)
	}
	return err
}

func localInfo(name string, fi fs.FileInfo) remotefs.Info {
	info := remotefs.Info{Name: name, Modified: fi.ModTime(), Dir: fi.IsDir()}
	if !info.Dir {
		info.Size = fi.Size()
	}
	return info
}

// Stat is the file or folder at p.
func (l *Local) Stat(_ context.Context, p string) (remotefs.Info, error) {
	name, err := local(p)
	if err != nil {
		return remotefs.Info{}, err
	}
	var info remotefs.Info
	err = l.within(func(root *os.Root) error {
		fi, err := root.Stat(name)
		if err != nil {
			return err
		}
		info = localInfo(path.Base(filepath.ToSlash(name)), fi)
		if name == "." {
			info.Name = ""
		}
		return nil
	})
	return info, err
}

// List is what lies directly in the folder dir, leaving out the files a Write is still writing.
func (l *Local) List(_ context.Context, dir string) ([]remotefs.Info, error) {
	name, err := local(dir)
	if err != nil {
		return nil, err
	}
	var out []remotefs.Info
	err = l.within(func(root *os.Root) error {
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		entries, err := f.ReadDir(-1)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), remotefs.TempPrefix) {
				continue
			}
			fi, err := e.Info()
			if errors.Is(err, fs.ErrNotExist) {
				// Removed since the folder was read.
				continue
			}
			if err != nil {
				return err
			}
			out = append(out, localInfo(e.Name(), fi))
		}
		return nil
	})
	return out, err
}

// Open reads the file at p from offset.
func (l *Local) Open(_ context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length < -1 {
		return nil, fmt.Errorf("%s: bad range %d+%d", p, offset, length)
	}
	name, err := local(p)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(l.dir)
	if err != nil {
		return nil, notExist(err)
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, notExist(err)
	}
	fi, err := f.Stat()
	if err == nil && fi.IsDir() {
		err = fmt.Errorf("%s: %w", p, remotefs.ErrIsDir)
	}
	if err == nil && offset > 0 {
		_, err = f.Seek(offset, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	if length < 0 {
		return f, nil
	}
	return limitedFile{Reader: io.LimitReader(f, length), Closer: f}, nil
}

// limitedFile is a file read up to a length, closed as the file is.
type limitedFile struct {
	io.Reader
	io.Closer
}

// Write writes r into a new file beside p and renames it into place, so p is either what it was or all of r.
func (l *Local) Write(_ context.Context, p string, r io.Reader, size int64) error {
	name, err := localFile(p)
	if err != nil {
		return err
	}
	if size < 0 {
		return fmt.Errorf("%s: unknown size", p)
	}
	return l.within(func(root *os.Root) error {
		dir := filepath.Dir(name)
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		temp := filepath.Join(dir, remotefs.TempPrefix+rand.Text())
		f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		n, err := io.Copy(f, io.LimitReader(r, size))
		if err == nil && n < size {
			err = fmt.Errorf("%s: %d of %d bytes: %w", p, n, size, io.ErrUnexpectedEOF)
		}
		if err == nil {
			err = f.Sync()
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = root.Rename(temp, name)
		}
		if err != nil {
			_ = root.Remove(temp)
		}
		return err
	})
}

// Remove removes the file or empty folder at p.
func (l *Local) Remove(_ context.Context, p string) error {
	name, err := localFile(p)
	if err != nil {
		return err
	}
	err = l.within(func(root *os.Root) error {
		fi, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			f, err := root.Open(name)
			if err != nil {
				return err
			}
			names, err := f.Readdirnames(1)
			f.Close()
			if len(names) > 0 {
				return fmt.Errorf("%s: %w", p, remotefs.ErrNotEmpty)
			}
			if err != nil && err != io.EOF {
				return err
			}
		}
		return root.Remove(name)
	})
	if errors.Is(err, remotefs.ErrNotExist) {
		return nil
	}
	return err
}

// Move renames from to to, which on one disk is atomic; across disks, as a NAS mounted inside the folder would be, the system refuses it.
func (l *Local) Move(_ context.Context, from, to string) error {
	src, err := localFile(from)
	if err != nil {
		return err
	}
	dst, err := localFile(to)
	if err != nil {
		return err
	}
	return l.within(func(root *os.Root) error {
		if _, err := root.Lstat(src); err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return root.Rename(src, dst)
	})
}

// MkdirAll makes the folder dir and those above it.
func (l *Local) MkdirAll(_ context.Context, dir string) error {
	name, err := local(dir)
	if err != nil {
		return err
	}
	return l.within(func(root *os.Root) error { return root.MkdirAll(name, 0o755) })
}
