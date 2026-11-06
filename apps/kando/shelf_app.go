package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kando/apps/kando/brand"
	"kando/apps/kando/films"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"
)

// The shelf: which library folders the app knows, which one is open, and following them when they are renamed in Finder. The shelf's state is the nodra session in state.go.

func (a *app) pickLibrary() {
	current := a.snap().LibraryDir
	if current == "" {
		current = a.libraryPlace()
	}
	a.pickFolder("导入资料库", "导入", current, func(dir string) {
		a.switchLibrary(dir)
		a.edit(func(s *session) { s.MakingLibrary = false })
		a.page = pageFilms
	})
}

func (a *app) pickLibraryParent() {
	a.pickFolder("资料库放在哪里", "选择", a.libraryPlace(), func(dir string) {
		a.edit(func(s *session) { s.LibraryParent = dir })
	})
}

// createLibrary makes a .kando bundle and switches to it. A folder that is already there is opened as it is.
func (a *app) createLibrary() {
	file, err := libraryFileName(a.snap().LibraryName)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	dir := filepath.Join(a.libraryPlace(), file)
	if info, statErr := os.Stat(dir); statErr == nil {
		if info.IsDir() {
			a.saveErr = "已经有这个资料库，换个名字"
		} else {
			a.saveErr = "这个位置已经有同名文件"
		}
		return
	} else if !os.IsNotExist(statErr) {
		a.saveErr = statErr.Error()
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.switchLibrary(dir)
	a.edit(func(s *session) { s.MakingLibrary = false })
	a.page = pageFilms
}

func libraryFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, media.LibraryExt)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("先写资料库的名称")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", errors.New("资料库名称不能包含路径")
	}
	return name + media.LibraryExt, nil
}

// beginNewLibrary opens the create form even when a library is already open, and picks a name that is not taken.
func (a *app) beginNewLibrary() {
	a.openKey = ""
	a.stopPlayback()
	a.page = pageFilms
	name := a.freeLibraryName()
	a.edit(func(s *session) {
		s.MakingLibrary = true
		s.LibraryName = name
	})
	a.saveErr = ""
}

func (a *app) freeLibraryName() string {
	parent := a.libraryPlace()
	base := "影视库"
	if _, err := os.Stat(filepath.Join(parent, base+media.LibraryExt)); os.IsNotExist(err) {
		return base
	}
	for n := 2; n < 100; n++ {
		name := fmt.Sprintf("%s %d", base, n)
		if _, err := os.Stat(filepath.Join(parent, name+media.LibraryExt)); os.IsNotExist(err) {
			return name
		}
	}
	return base
}

func (a *app) libraryPlace() string {
	if parent := strings.TrimSpace(a.snap().LibraryParent); parent != "" {
		return parent
	}
	return media.MoviesDir()
}

// switchLibrary points new downloads at dir and stops a job that is still writing into the previous one.
func (a *app) switchLibrary(dir string) {
	dir = filepath.Clean(dir)
	if dir == "." || dir == "" {
		return
	}
	s := a.snap()
	libs := addLibrary(s.Libraries, dir)
	a.edit(func(st *session) { st.Libraries = libs })
	if filepath.Clean(s.LibraryDir) == dir {
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
		return
	}
	a.applyLibrary(dir)
	if err := a.saveSettings(); err != nil {
		a.saveErr = "资料库无法保存：" + err.Error()
		return
	}
	a.saveErr = ""
}

// forgetLibrary drops dir from the list. The folder on disk stays.
func (a *app) forgetLibrary(dir string) {
	dir = filepath.Clean(dir)
	s := a.snap()
	kept := make([]string, 0, len(s.Libraries))
	for _, item := range s.Libraries {
		if filepath.Clean(item) != dir {
			kept = append(kept, item)
		}
	}
	a.edit(func(st *session) {
		st.Libraries = kept
		delete(st.Inodes, dir)
	})
	if filepath.Clean(s.LibraryDir) != dir {
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
		return
	}
	next := ""
	if len(kept) > 0 {
		next = kept[0]
	}
	a.applyLibrary(next)
	if err := a.saveSettings(); err != nil {
		a.saveErr = "资料库无法保存：" + err.Error()
		return
	}
	a.saveErr = ""
}

func (a *app) applyLibrary(dir string) {
	defer a.areasLibraryChanged()
	a.areasLeavingLibrary()
	a.openKey = ""
	a.stopPlayback()
	if dir == "" {
		a.edit(func(s *session) {
			s.LibraryDir = ""
			s.Videos = nil
		})
		if tools, ok := a.pipeline.(media.Tools); ok {
			tools.OutputDir = ""
			a.pipeline = tools
		}
		a.restartLibraryWatch()
		return
	}
	if _, err := os.Stat(dir); err != nil && filepath.Clean(dir) == filepath.Clean(a.snap().LibraryDir) {
		if next, ok := media.FindDir(filepath.Dir(dir), a.inode(dir)); ok {
			a.adoptRenamedLibrary(next)
			dir = next
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.saveErr = err.Error()
		return
	}
	id := media.IdentifyDir(dir)
	a.edit(func(s *session) {
		s.LibraryDir = dir
		putInode(s, dir, id)
	})
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.OutputDir = dir
		a.pipeline = tools
	}
	brand.Ensure(dir, appIconPNG)
	a.refreshLibrary()
	a.restartLibraryWatch()
}

// adoptRenamedLibrary points the shelf at a library folder that was renamed in Finder. The folder on disk is the same one; only its path changed.
func (a *app) adoptRenamedLibrary(next string) {
	prev := a.snap().LibraryDir
	if filepath.Clean(prev) == filepath.Clean(next) {
		return
	}
	a.renameLibraryPath(prev, next)
	if err := a.saveSettings(); err != nil {
		a.saveErr = "资料库无法保存：" + err.Error()
	}
}

func (a *app) retargetLibraryPrefix(oldRoot, newRoot string) {
	changed := false
	for i, job := range a.library.Jobs {
		if job.LibraryDir != "" && filepath.Clean(job.LibraryDir) == filepath.Clean(oldRoot) {
			a.library.Jobs[i].LibraryDir = newRoot
			changed = true
		}
		if next, ok := replacePathPrefix(oldRoot, newRoot, job.VideoPath); ok {
			a.library.Jobs[i].VideoPath = next
			changed = true
		}
		if next, ok := replacePathPrefix(oldRoot, newRoot, job.SubtitlePath); ok {
			a.library.Jobs[i].SubtitlePath = next
			changed = true
		}
	}
	if changed {
		if err := a.library.Save(); err != nil {
			a.saveErr = "视频库无法保存：" + err.Error()
		}
	}
}

func replacePathPrefix(oldRoot, newRoot, path string) (string, bool) {
	if path == "" || oldRoot == "" {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(oldRoot), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(newRoot, rel), true
}

func dirIDSet(id media.DirID) bool {
	_, _, ok := id.Parts()
	return ok
}

func (a *app) rememberLibrary(dir string) {
	a.rememberID(dir, media.IdentifyDir(dir))
}

func (a *app) inode(dir string) media.DirID {
	return a.snap().Inodes[filepath.Clean(dir)]
}

func inodesForSave(ids map[string]media.DirID) map[string]savedInode {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]savedInode, len(ids))
	for path, id := range ids {
		dev, ino, ok := id.Parts()
		if !ok {
			continue
		}
		out[path] = savedInode{Dev: dev, Ino: ino}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (a *app) restoreLibraryInodes(saved map[string]savedInode) {
	if len(saved) == 0 {
		return
	}
	a.edit(func(s *session) {
		for path, id := range saved {
			putInode(s, path, media.MakeDirID(id.Dev, id.Ino))
		}
	})
}

func (a *app) rememberID(dir string, id media.DirID) {
	if !dirIDSet(id) {
		return
	}
	a.edit(func(s *session) { putInode(s, dir, id) })
}

// putInode records dir's identity on the session being edited. The map is the clone nodra hands the update.
func putInode(s *session, dir string, id media.DirID) {
	if s == nil || !dirIDSet(id) {
		return
	}
	dir = filepath.Clean(dir)
	if dir == "" || dir == "." {
		return
	}
	if s.Inodes == nil {
		s.Inodes = map[string]media.DirID{}
	}
	s.Inodes[dir] = id
}

// renameLibraryPath keeps a library that was renamed in Finder, including one that is not the library currently open.
func (a *app) renameLibraryPath(old, next string) {
	old, next = filepath.Clean(old), filepath.Clean(next)
	if old == "" || next == "" || old == next {
		return
	}
	id := a.inode(old)
	if !dirIDSet(id) {
		id = media.IdentifyDir(next)
	}
	opened := filepath.Clean(a.snap().LibraryDir) == old
	a.edit(func(st *session) {
		delete(st.Inodes, old)
		putInode(st, next, id)
		st.Libraries = addLibrary(dropLibrary(st.Libraries, old), next)
		if filepath.Clean(st.LibraryDir) == old {
			st.LibraryDir = next
		}
	})
	if opened {
		if tools, ok := a.pipeline.(media.Tools); ok {
			tools.OutputDir = next
			a.pipeline = tools
		}
	}
	a.retargetLibraryPrefix(old, next)
}

// reconcileMissingLibraries points a saved path that is gone at the renamed bundle. A saved inode wins. Without one, a single missing library and a single new .kando bundle in the same folder are the same library.
func (a *app) reconcileMissingLibraries() bool {
	changed := false
	for _, dir := range a.knownLibraries() {
		if _, err := os.Stat(dir); err == nil {
			a.rememberLibrary(dir)
			continue
		}
		if id := a.inode(dir); dirIDSet(id) {
			if next, ok := media.FindDir(filepath.Dir(dir), id); ok {
				a.renameLibraryPath(dir, next)
				changed = true
				continue
			}
		}
	}
	var missing []string
	for _, dir := range a.knownLibraries() {
		if _, err := os.Stat(dir); err != nil {
			missing = append(missing, dir)
		}
	}
	if len(missing) != 1 {
		return changed
	}
	extras := unlistedBundles(filepath.Dir(missing[0]), a.knownLibraries())
	if len(extras) != 1 {
		return changed
	}
	a.renameLibraryPath(missing[0], extras[0])
	return true
}

func unlistedBundles(parent string, known []string) []string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for _, dir := range known {
		have[filepath.Clean(dir)] = true
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() || filepath.Ext(entry.Name()) != media.LibraryExt {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		if !have[filepath.Clean(path)] {
			out = append(out, path)
		}
	}
	return out
}

// syncRenamedLibraries notices a library folder renamed in Finder, whether or not it is the one open.
func (a *app) syncRenamedLibraries() bool {
	changed := false
	for _, dir := range a.knownLibraries() {
		if _, err := os.Stat(dir); err == nil {
			a.rememberLibrary(dir)
			continue
		}
		id := a.inode(dir)
		next, ok := media.FindDir(filepath.Dir(dir), id)
		if !ok || filepath.Clean(next) == filepath.Clean(dir) {
			continue
		}
		a.renameLibraryPath(dir, next)
		changed = true
	}
	if changed {
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
	}
	return changed
}

// restartLibraryWatch follows the current library folder. A new file, a deletion, a rewrite, or a rename of the folder itself refreshes the shelf on the UI thread.
func (a *app) restartLibraryWatch() {
	if a.libraryWatch != nil {
		close(a.libraryWatch)
		a.libraryWatch = nil
	}
	dir := a.snap().LibraryDir
	if dir == "" || a.update == nil {
		return
	}
	stop := make(chan struct{})
	a.libraryWatch = stop
	update := a.update
	go media.WatchLibrary(dir, time.Second, stop, func(next string) {
		update(func() {
			if a.libraryWatch != stop {
				return
			}
			if next != "" && filepath.Clean(next) != filepath.Clean(a.snap().LibraryDir) {
				a.adoptRenamedLibrary(next)
			}
			a.refreshLibrary()
			a.reloadOpenSubtitles()
		})
	})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				update(func() {
					if a.libraryWatch != stop {
						return
					}
					if a.syncRenamedLibraries() && filepath.Clean(a.snap().LibraryDir) != "" {
						a.refreshLibrary()
					}
				})
			}
		}
	}()
}

func (a *app) knownLibraries() []string {
	s := a.snap()
	out := append([]string{}, s.Libraries...)
	if s.LibraryDir != "" {
		out = addLibrary(out, s.LibraryDir)
	}
	return out
}

func dropLibrary(libraries []string, dir string) []string {
	dir = filepath.Clean(dir)
	kept := make([]string, 0, len(libraries))
	for _, item := range libraries {
		if filepath.Clean(item) != dir {
			kept = append(kept, item)
		}
	}
	return kept
}

func addLibrary(libraries []string, dir string) []string {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" || dir == "." {
		return libraries
	}
	for _, item := range libraries {
		if filepath.Clean(item) == dir {
			return libraries
		}
	}
	return append(libraries, dir)
}

type savedInode struct {
	Dev uint64 `json:"dev"`
	Ino uint64 `json:"ino"`
}

// savedShelf is the shelf's part of settings.json: the open library, the libraries the app knows and each one's inode.
type savedShelf struct {
	LibraryDir    string                `json:"libraryDir,omitempty"`
	Libraries     []string              `json:"libraries,omitempty"`
	LibraryInodes map[string]savedInode `json:"libraryInodes,omitempty"`
}

func (s *savedShelf) clean() {
	s.LibraryDir = strings.TrimSpace(s.LibraryDir)
	s.Libraries = cleanLibraries(s.Libraries)
}

func (a *app) saveShelf() savedShelf {
	shelf := a.snap()
	return savedShelf{
		LibraryDir:    shelf.LibraryDir,
		Libraries:     append([]string{}, a.knownLibraries()...),
		LibraryInodes: inodesForSave(shelf.Inodes),
	}
}

// restoreShelf points the shelf at the library saved last time, or the one KANDO_OUTPUT_DIR names, and follows libraries renamed while the app was closed. It reports whether it followed one, so settings.json needs saving; restoreSettings saves it once the areas have restored their own settings, as a save before that would write their defaults.
func (a *app) restoreShelf(saved savedSettings) (moved bool) {
	libs := cleanLibraries(saved.Libraries)
	if saved.LibraryDir != "" {
		libs = addLibrary(libs, saved.LibraryDir)
	}
	libraryDir := ""
	if env := os.Getenv("KANDO_OUTPUT_DIR"); env != "" {
		libraryDir = env
	} else if saved.LibraryDir != "" {
		libraryDir = saved.LibraryDir
	} else if len(libs) > 0 {
		libraryDir = libs[0]
	}
	a.edit(func(s *session) {
		s.Libraries = libs
		s.LibraryDir = libraryDir
	})
	a.restoreLibraryInodes(saved.LibraryInodes)
	if a.reconcileMissingLibraries() {
		libraryDir = a.snap().LibraryDir
		moved = true
	}
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.OutputDir = libraryDir
		a.pipeline = tools
	}
	return moved
}

func (a *app) refreshLibrary() {
	dir := a.snap().LibraryDir
	if dir == "" {
		return
	}
	moved, err := media.GatherVideos(dir)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	if a.retargetLibrary(moved) || a.relocateMissing() || a.retargetRenamed() || a.dropMissing() {
		if err := a.library.Save(); err != nil {
			a.saveErr = "视频库无法保存：" + err.Error()
		}
	}
	videos, err := media.ListVideos(dir)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.edit(func(s *session) { s.Videos = videos })
	a.areasShelfChanged()
}

func (a *app) retargetRenamed() bool {
	root := a.snap().LibraryDir
	var missing, live []string
	for _, job := range a.library.Jobs {
		for _, path := range []string{job.VideoPath, job.SubtitlePath} {
			if path == "" || !films.InDir(root, path) {
				continue
			}
			if _, err := os.Stat(path); err != nil {
				missing = append(missing, path)
			} else {
				live = append(live, path)
			}
		}
	}
	return a.retargetLibrary(media.RetargetRenamed(root, missing, live))
}

// retargetLibrary points jobs at files GatherVideos moved into per-video folders.
func (a *app) retargetLibrary(moved map[string]string) bool {
	if len(moved) == 0 {
		return false
	}
	changed := false
	for i, job := range a.library.Jobs {
		if next, ok := moved[job.VideoPath]; ok {
			a.library.Jobs[i].VideoPath = next
			changed = true
		}
		if next, ok := moved[job.SubtitlePath]; ok {
			a.library.Jobs[i].SubtitlePath = next
			changed = true
		}
	}
	return changed
}

func (a *app) dropMissing() bool {
	kept := make([]core.Job, 0, len(a.library.Jobs))
	changed := false
	for _, job := range a.library.Jobs {
		if job.VideoPath != "" && job.Stage.Done() {
			if _, err := os.Stat(job.VideoPath); err != nil {
				changed = true
				if a.openKey == job.ID {
					a.openKey = ""
					a.stopPlayback()
				}
				continue
			}
		}
		kept = append(kept, job)
	}
	if changed {
		a.library.Jobs = kept
	}
	return changed
}

func (a *app) relocateMissing() bool {
	changed := false
	for i, job := range a.library.Jobs {
		root := a.snap().LibraryDir
		if next, ok := media.Relocate(root, job.VideoPath); ok {
			a.library.Jobs[i].VideoPath = next
			changed = true
		}
		if next, ok := media.Relocate(root, job.SubtitlePath); ok {
			a.library.Jobs[i].SubtitlePath = next
			changed = true
		}
	}
	return changed
}

func cleanLibraries(dirs []string) []string {
	var out []string
	for _, dir := range dirs {
		out = addLibrary(out, dir)
	}
	return out
}

// openLibrary reads the shelf's library.json at path into app.library, then runs the areas' shelfLoaded.
func (a *app) openLibrary(path string) {
	lib, err := shelf.Load(path)
	a.library = lib
	if err != nil {
		a.saveErr = "视频库无法读取：" + err.Error()
	}
	a.areasShelfLoaded()
}

// revealInFolder shows path in the system file manager. Tests replace it.
var revealInFolder = revealPath

func revealPath(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,", path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func (a *app) reveal(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		a.saveErr = "文件不在了"
		return
	}
	if err := revealInFolder(path); err != nil {
		a.saveErr = "无法打开所在文件夹：" + err.Error()
		return
	}
	a.saveErr = ""
}
