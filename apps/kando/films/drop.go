package films

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kando/packages/media"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// ConfirmAbove is how many films a drop may add before the window asks first how to add them (Drop): a few are queued at once, as they always were, while a large folder could cost minutes and the AI service's charges per film.
const ConfirmAbove = 20

// DropLimit is how many films Dropped finds at most, and dropLooks how many files and folders it looks at, so a whole drive dropped by mistake is not read to its end: what is past either is left out, and Drop.Cut says so.
const (
	DropLimit = 5000
	dropLooks = 200_000
)

// Drop is what the files and folders dropped on the window, picked or opened with Kando hold (Dropped).
type Drop struct {
	// Videos are the films' files, in the order they are to be queued or added.
	Videos []string
	// Skipped is how many files were left out because they are neither videos nor sounds, but for the disguised ones.
	Skipped int
	// Disguised are the programs named as videos are, as 第01话.mp4.exe, whose name shows a video's where the system hides the extension, or one written backwards with U+202E. They are never added; the window says to leave them unopened.
	Disguised []string
	// Size is how many bytes Videos take, those that can be read.
	Size int64
	// Cut is whether DropLimit was reached, or the files and folders Dropped looks at, and what is past it left out.
	Cut bool
}

// Dropped is the films among paths, the files and folders dropped on the window, picked or opened with Kando. A file is kept when its name is a video's or a sound's, as before folders could be dropped, so one that cannot be read still makes a job that says why; a sound with a video of its name beside it is that video's, which is kept in its place, once (media.FilmOf). A folder gives the videos directly in it, then those of each folder in it, however deep, as a course keeps its lessons in a folder per section and a collection a folder per show and per season: a large folder dropped is not queued whole without asking, as the window asks first past ConfirmAbove. A folder's videos and folders come in their names' order as tag, the interface's language, orders words, numbers by value, so "Episode 2" comes before "Episode 10" and episodes come in order. Hidden files and folders are neither added nor counted, nor are the folders a system or a NAS keeps its own things in, another library of Kando's (media.LibraryExt) inside what was dropped, or an app, a folder that cannot be read, or a link to a folder, which may lead back up.
func Dropped(paths []string, tag string) Drop {
	col := collate.New(language.Make(tag), collate.Numeric)
	seen := map[string]bool{}
	var d Drop
	looked := 0
	// add keeps the film of the media file at path, once, and counts any other file.
	add := func(path string) {
		if d.Cut {
			return
		}
		if disguised(filepath.Base(path)) {
			d.Disguised = append(d.Disguised, path)
			return
		}
		film, ok := media.FilmOf(path)
		switch {
		case !ok:
			if !media.IsMediaFile(path) {
				d.Skipped++
			}
		case !seen[film]:
			if len(d.Videos) == DropLimit {
				d.Cut = true
				return
			}
			seen[film] = true
			d.Videos = append(d.Videos, film)
			if info, err := os.Stat(film); err == nil {
				d.Size += info.Size()
			}
		}
	}
	byName := func(x, y os.DirEntry) int { return col.CompareString(x.Name(), y.Name()) }
	// folder adds dir's videos, then those of its folders.
	var folder func(dir string)
	folder = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		if looked += len(entries); looked > dropLooks {
			d.Cut = true
			return
		}
		slices.SortStableFunc(entries, byName)
		var folders []string
		for _, entry := range entries {
			name := entry.Name()
			switch {
			case strings.HasPrefix(name, "."):
			case entry.IsDir() && disguised(name):
				d.Disguised = append(d.Disguised, filepath.Join(dir, name))
			case entry.IsDir():
				if !passedOver(name) {
					folders = append(folders, filepath.Join(dir, name))
				}
			case entry.Type()&os.ModeSymlink != 0:
				// A link is added for the file it leads to, never followed into a folder.
				if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
					add(filepath.Join(dir, name))
				}
			default:
				add(filepath.Join(dir, name))
			}
		}
		for _, sub := range folders {
			if d.Cut {
				return
			}
			folder(sub)
		}
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			// A library of Kando's dropped is looked into, as its films are what it was dropped for; a system's folder or a package is not.
			if name := filepath.Base(filepath.Clean(path)); strings.EqualFold(filepath.Ext(name), media.LibraryExt) || !passedOver(name) {
				folder(path)
			}
			continue
		}
		add(path)
	}
	return d
}

// systemFolders are the names, in lower case, of the folders a system or a NAS keeps its own things in: Windows' Recycle Bin, restore points and upgrade files, Linux's recovered files, and the Recycle Bins, thumbnails and snapshots of Synology's and QNAP's shares.
var systemFolders = []string{"$recycle.bin", "system volume information", "$windows.~bt", "$windows.~ws", "lost+found", "@eadir", "#recycle", "#snapshot", "@recycle", "@recently-snapshot", ".@__thumb"}

// passedOver reports whether a folder named name is one Dropped does not look into: one a system or a NAS keeps its own things in (systemFolders), another library of Kando's, or a package macOS shows as one file, as an app or a Photos library.
func passedOver(name string) bool {
	if slices.Contains(systemFolders, strings.ToLower(name)) {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case media.LibraryExt, ".app", ".photoslibrary", ".fcpbundle", ".imovielibrary", ".tvlibrary", ".musiclibrary", ".bundle", ".framework":
		return true
	}
	return false
}

// programExtensions are the extensions of files Windows and macOS run when they are opened.
var programExtensions = []string{".exe", ".scr", ".com", ".pif", ".bat", ".cmd", ".msi", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".hta", ".cpl", ".ps1", ".jar", ".app", ".command"}

// disguised reports whether name, a file's or a folder's, is a program's that shows a video's or a sound's: a media extension followed by a program's, as 第01话.mp4.exe, which shows as 第01话.mp4 where the system hides extensions, or any name with U+202E, which writes what follows it backwards, as 第01话<U+202E>4pm.exe shows as 第01话exe.mp4.
func disguised(name string) bool {
	if strings.ContainsRune(name, '\u202e') {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	return slices.Contains(programExtensions, ext) && media.IsMediaFile(strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name))))
}
