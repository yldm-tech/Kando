package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"kando/packages/track"
)

// FilmMove moves one film's files from a library folder into a folder of its own in another library folder, as Move to Library does. Within one volume the files are renamed (Rename); across volumes they are copied (Copy) and removed from where they were once the copy is whole (RemoveSources), so a copy that fails or is stopped leaves them as they were.
type FilmMove struct {
	// Sources are what moves: the film's folder when Folder is set, otherwise its video and the files beside it that are its (Companions).
	Sources []string
	Folder  bool
	// Dest is the film's folder in the other library once moved: the folder itself, or the new folder its files go into.
	Dest string
}

// PlanFilmMove is the move of a film into the library folder dir: sources are its folder when folder is set, else its video first and the files beside it. The film's folder in dir takes the folder's name, or the video's without its extension, followed by " 2", " 3" and so on while that name is taken there.
func PlanFilmMove(sources []string, folder bool, dir string) FilmMove {
	name := filepath.Base(sources[0])
	if !folder {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return FilmMove{Sources: sources, Folder: folder, Dest: freeName(dir, name)}
}

// freeName is the path of name in dir, or of "name 2", "name 3" and so on, the first that nothing in dir has, hidden staging folders included.
func freeName(dir, name string) string {
	path := filepath.Join(dir, name)
	for n := 2; ; n++ {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s %d", name, n))
	}
}

// Companions are the files beside the video at video that are its, those that exist: the subtitles Kando reads and writes by its name (<video>.srt, <video>.kando.srt and <video>.bilingual.srt) and the hidden files a job keeps beside it while it works, the translation done so far (translationCheckpointPath) and the speech a redo keeps aside (pendingTranscriptPath, pendingHeardPath) and the transcript a translating redo proofread (proofreadTranscriptPath), the user's corrections of its lines (CorrectionsPath), the chapters its site marks (ChaptersPath), the captions its site made itself (ReferencePath), its own glossary (TermsPath), the final proofread's changes (ProofsPath), the terms suggested for its glossary (SuggestionsPath), the sound a link's subtitles are made from until its video comes (soundPath) and every device's log of its subtitles (track.Logs). The media files of its name beside it are its too (SameNamed), and go where these go when the film moves or is deleted.
func Companions(video string) []string {
	base := strings.TrimSuffix(video, filepath.Ext(video))
	var out []string
	for _, path := range []string{transcriptPath(video), asideTranscriptPath(video), base + ".bilingual.srt", translationCheckpointPath(video), pendingTranscriptPath(video), pendingHeardPath(video), proofreadTranscriptPath(video), CorrectionsPath(video), ChaptersPath(video), ReferencePath(video), TermsPath(video), ProofsPath(video), SuggestionsPath(video)} {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			out = append(out, path)
		}
	}
	out = append(out, soundPaths(video)...)
	return append(out, track.Logs(video)...)
}

// Path is where the file at path is once the move is done, and whether the move moves it: a file under the folder that moves, or one of the files that move.
func (m FilmMove) Path(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	clean := filepath.Clean(path)
	if m.Folder {
		rel, err := filepath.Rel(filepath.Clean(m.Sources[0]), clean)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		if rel == "." {
			return m.Dest, true
		}
		return filepath.Join(m.Dest, rel), true
	}
	for _, source := range m.Sources {
		if filepath.Clean(source) == clean {
			return filepath.Join(m.Dest, filepath.Base(source)), true
		}
	}
	return "", false
}

// Rename moves the sources to Dest with os.Rename, which works within one volume. When a rename fails it puts back what it had moved and returns the error, so the caller can copy them instead (Copy).
func (m FilmMove) Rename() error {
	if _, err := os.Lstat(m.Dest); err == nil {
		return fmt.Errorf("%s: %w", m.Dest, fs.ErrExist)
	}
	if m.Folder {
		return os.Rename(m.Sources[0], m.Dest)
	}
	if err := os.Mkdir(m.Dest, 0o755); err != nil {
		return err
	}
	var done []string
	for _, source := range m.Sources {
		next, _ := m.Path(source)
		if err := os.Rename(source, next); err != nil {
			for _, back := range done {
				moved, _ := m.Path(back)
				os.Rename(moved, back)
			}
			os.Remove(m.Dest)
			return err
		}
		done = append(done, source)
	}
	return nil
}

// Copy copies the sources into a hidden folder beside Dest, which the library does not list, and renames it to Dest once every file is there, each with its mode and modification time, so the other library never shows a film half copied. progress, which may be nil, hears how many bytes of how many are copied as the copy goes. When ctx ends or a file cannot be copied, the hidden folder is removed and the sources are left as they are; ctx's error is returned for a copy that was stopped.
func (m FilmMove) Copy(ctx context.Context, progress func(done, total int64)) (err error) {
	if _, err := os.Lstat(m.Dest); err == nil {
		return fmt.Errorf("%s: %w", m.Dest, fs.ErrExist)
	}
	stage, err := os.MkdirTemp(filepath.Dir(m.Dest), "."+filepath.Base(m.Dest)+".moving-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(stage)
		}
	}()
	// files are the copies to make, from the source to the path under stage.
	type file struct {
		from, to string
		info     fs.FileInfo
	}
	var files []file
	var dirs []file
	var total int64
	add := func(from, to string, info fs.FileInfo) {
		if info.IsDir() {
			dirs = append(dirs, file{from, to, info})
			return
		}
		files = append(files, file{from, to, info})
		if info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	if m.Folder {
		root := m.Sources[0]
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			add(path, filepath.Join(stage, rel), info)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		dirs = append(dirs, file{to: stage})
		for _, source := range m.Sources {
			info, err := os.Lstat(source)
			if err != nil {
				return err
			}
			add(source, filepath.Join(stage, filepath.Base(source)), info)
		}
	}
	for _, d := range dirs {
		if d.info != nil {
			if err := os.MkdirAll(d.to, d.info.Mode().Perm()|0o700); err != nil {
				return err
			}
		}
	}
	var done int64
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyFile(ctx, f.from, f.to, f.info, func(n int64) {
			done += n
			if progress != nil {
				progress(done, total)
			}
		}); err != nil {
			return err
		}
	}
	// A folder's time is set once its files are in it, as writing them changes it.
	for i := len(dirs) - 1; i >= 0; i-- {
		if d := dirs[i]; d.info != nil {
			os.Chmod(d.to, d.info.Mode().Perm())
			os.Chtimes(d.to, d.info.ModTime(), d.info.ModTime())
		}
	}
	if _, err := os.Lstat(m.Dest); err == nil {
		return fmt.Errorf("%s: %w", m.Dest, fs.ErrExist)
	}
	return os.Rename(stage, m.Dest)
}

// copyFile copies the file at from, whose Lstat is info, to to with its mode and modification time, telling copied how many bytes each write added. A symbolic link is made again as the same link. It stops with ctx's error once ctx ends.
func copyFile(ctx context.Context, from, to string, info fs.FileInfo, copied func(n int64)) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", from)
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, &copyReader{ctx: ctx, r: in, copied: copied}); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(to, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

// copyReader reads r for io.Copy, telling copied how many bytes each read gave, and ends with ctx's error once ctx ends.
type copyReader struct {
	ctx    context.Context
	r      io.Reader
	copied func(n int64)
}

func (c *copyReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := c.r.Read(p)
	if n > 0 {
		c.copied(int64(n))
	}
	return n, err
}

// RemoveSources removes the sources, once a copy of them is at Dest (Copy).
func (m FilmMove) RemoveSources() error {
	if m.Folder {
		return os.RemoveAll(m.Sources[0])
	}
	var errs []error
	for _, source := range m.Sources {
		if err := os.Remove(source); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RemoveCopy removes what a whole copy made at Dest (Copy), when the move does not go ahead after all; the sources are as they were.
func (m FilmMove) RemoveCopy() error { return os.RemoveAll(m.Dest) }
