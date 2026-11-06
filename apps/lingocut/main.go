package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"lingocut/apps/lingocut/brand"
	"lingocut/apps/lingocut/cloud"
	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/shelf"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/yldm-tech/nodra"
)

type app struct {
	source           string
	language         string
	translatePrompt  string
	translateContext bool
	status           string
	selectedID       string
	saveErr          string
	page             string
	openKey          string
	store            *nodra.Store[session]
	libraryWatch     chan struct{}
	settingsPath     string
	modelsDir        string
	extraModelDirs   []string
	downloading      string
	downloadWritten  int64
	downloadTotal    int64
	downloadErr      string
	downloadCancel   context.CancelFunc
	llmProvider      string
	llmBase          string
	llmModel         string
	llmKey           string
	showLLMKey       bool
	llmCredential    func(context.Context, bool) (media.Endpoint, error)
	testingLLM       bool
	llmTestResult    string
	toolVersions     map[string]string
	versionGen       int
	posterDir        string
	posters          map[string]ui.ImageSource
	posterMiss       map[string]bool
	posterBusy       map[string]bool
	googleEmail      string
	googleRefresh    string
	googleBusy       bool
	googleErr        string
	googleLogin      func(context.Context, cloud.GoogleConfig) (cloud.Account, error)
	storage          string
	r2Endpoint       string
	r2Bucket         string
	r2AccessKey      string
	r2Secret         string
	s3Region         string
	s3Bucket         string
	s3AccessKey      string
	s3Secret         string
	ossRegion        string
	ossBucket        string
	ossAccessKey     string
	ossSecret        string
	cosRegion        string
	cosBucket        string
	cosAccessKey     string
	cosSecret        string
	compatEndpoint   string
	compatRegion     string
	compatBucket     string
	compatAccessKey  string
	compatSecret     string
	compatPath       bool
	showR2Secret     bool
	uploadPath       string
	uploadWritten    int64
	uploadTotal      int64
	uploadErr        string
	uploadErrPath    string
	uploaded         map[string]string
	uploader         func(context.Context, cloud.Store, []cloud.File, func(written, total int64)) error
	removed          map[string]bool
	job              *core.Job
	library          shelf.Library
	pipeline         media.Pipeline
	win              *mygo.Window
	lib              libraryUI
	player           playerUI
	overlay          overlayUI
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())
	cancel context.CancelFunc
}

func (a *app) pickVideos() {
	win := a.win
	update := a.update
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       "选择视频",
			ButtonLabel: "打开",
			Multiple:    true,
			Filters:     []mygo.FileFilter{{Name: "视频", Extensions: []string{"mp4", "m4v", "mov", "mkv", "webm", "avi", "flv"}}},
		})
		update(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			if len(paths) > 0 {
				a.importLocal(paths[0])
			}
		})
	}()
}

// importLocal points the next job at a video already on disk. The pipeline reads it directly and does not download it.
func (a *app) importLocal(path string) {
	if !media.IsVideoFile(path) {
		a.status = "没有可导入的视频"
		return
	}
	a.source = (&url.URL{Scheme: "file", Path: path}).String()
	a.status = "已选择本地视频"
	a.page = pageFilms
	a.openKey = ""
}

// startFilm processes a video that is already in the library and stays on it.
func (a *app) startFilm(item films.Film) {
	a.importLocal(item.VideoPath)
	if a.status != "已选择本地视频" {
		return
	}
	a.start()
	if a.job == nil {
		return
	}
	a.openKey = a.job.ID
	a.loadSubtitles(a.job.ID, a.job.SubtitlePath)
}

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

func (a *app) pickFolder(title, button, current string, done func(string)) {
	win := a.win
	update := a.update
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            win,
			Title:             title,
			ButtonLabel:       button,
			Directory:         true,
			CreateDirectories: true,
			DefaultPath:       current,
		})
		update(func() {
			if err != nil {
				a.saveErr = err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			done(paths[0])
		})
	}()
}

// createLibrary makes a .lingocut bundle and switches to it. A folder that is already there is opened as it is.
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
	if a.running() {
		a.stop()
	}
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

// reconcileMissingLibraries points a saved path that is gone at the renamed bundle. A saved inode wins. Without one, a single missing library and a single new .lingocut bundle in the same folder are the same library.
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

func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.settingsPath), 0o700); err != nil {
		return err
	}
	contextOn := a.translateContext
	yieldSubs := a.player.yieldSubs
	shelf := a.snap()
	saved := savedSettings{
		LibraryDir:       shelf.LibraryDir,
		Libraries:        append([]string{}, a.knownLibraries()...),
		LibraryInodes:    inodesForSave(shelf.Inodes),
		LLMProvider:      a.llmProvider,
		Language:         a.language,
		TranslatePrompt:  a.translatePrompt,
		TranslateContext: &contextOn,
		GoogleEmail:      a.googleEmail,
		GoogleRefresh:    a.googleRefresh,
		Storage:          a.storageKind(),
		R2Endpoint:       a.r2Endpoint,
		R2Bucket:         a.r2Bucket,
		R2AccessKey:      a.r2AccessKey,
		R2Secret:         a.r2Secret,
		S3Region:         a.s3Region,
		S3Bucket:         a.s3Bucket,
		S3AccessKey:      a.s3AccessKey,
		S3Secret:         a.s3Secret,
		OSSRegion:        a.ossRegion,
		OSSBucket:        a.ossBucket,
		OSSAccessKey:     a.ossAccessKey,
		OSSSecret:        a.ossSecret,
		COSRegion:        a.cosRegion,
		COSBucket:        a.cosBucket,
		COSAccessKey:     a.cosAccessKey,
		COSSecret:        a.cosSecret,
		CompatEndpoint:   a.compatEndpoint,
		CompatRegion:     a.compatRegion,
		CompatBucket:     a.compatBucket,
		CompatAccessKey:  a.compatAccessKey,
		CompatSecret:     a.compatSecret,
		CompatPathStyle:  a.compatPath,
		Look:             changedLook(a.player.look),
		SubMode:          a.player.subMode,
		SubOrder:         a.player.subOrder,
		Untranslated:     a.player.untranslated,
		YieldSubs:        &yieldSubs,
		Onboarded:        a.overlay.onboarded,
	}
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.WhisperModel = tools.WhisperModel
		saved.LLMBaseURL = tools.Translator.BaseURL
		saved.LLMModel = tools.Translator.Model
		saved.LLMAPIKey = tools.Translator.APIKey
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := a.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.settingsPath)
}

func loadSavedSettings(path string) savedSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		return savedSettings{}
	}
	// A saved look is read over the default, so a field it lacks, such as one added after it was saved, keeps the default.
	look := film.DefaultLook()
	saved := savedSettings{Look: &look}
	if json.Unmarshal(data, &saved) != nil {
		return savedSettings{}
	}
	saved.LibraryDir = strings.TrimSpace(saved.LibraryDir)
	saved.Libraries = cleanLibraries(saved.Libraries)
	saved.WhisperModel = strings.TrimSpace(saved.WhisperModel)
	saved.LLMBaseURL = strings.TrimSpace(saved.LLMBaseURL)
	saved.LLMModel = strings.TrimSpace(saved.LLMModel)
	saved.LLMAPIKey = strings.TrimSpace(saved.LLMAPIKey)
	saved.LLMProvider = strings.TrimSpace(saved.LLMProvider)
	saved.Language = strings.TrimSpace(saved.Language)
	saved.TranslatePrompt = strings.TrimSpace(saved.TranslatePrompt)
	saved.GoogleEmail = strings.TrimSpace(saved.GoogleEmail)
	saved.GoogleRefresh = strings.TrimSpace(saved.GoogleRefresh)
	saved.Storage = strings.TrimSpace(saved.Storage)
	saved.R2Endpoint = strings.TrimSpace(saved.R2Endpoint)
	saved.R2Bucket = strings.TrimSpace(saved.R2Bucket)
	saved.R2AccessKey = strings.TrimSpace(saved.R2AccessKey)
	saved.R2Secret = strings.TrimSpace(saved.R2Secret)
	saved.S3Region = strings.TrimSpace(saved.S3Region)
	saved.S3Bucket = strings.TrimSpace(saved.S3Bucket)
	saved.S3AccessKey = strings.TrimSpace(saved.S3AccessKey)
	saved.S3Secret = strings.TrimSpace(saved.S3Secret)
	saved.OSSRegion = strings.TrimSpace(saved.OSSRegion)
	saved.OSSBucket = strings.TrimSpace(saved.OSSBucket)
	saved.OSSAccessKey = strings.TrimSpace(saved.OSSAccessKey)
	saved.OSSSecret = strings.TrimSpace(saved.OSSSecret)
	saved.COSRegion = strings.TrimSpace(saved.COSRegion)
	saved.COSBucket = strings.TrimSpace(saved.COSBucket)
	saved.COSAccessKey = strings.TrimSpace(saved.COSAccessKey)
	saved.COSSecret = strings.TrimSpace(saved.COSSecret)
	saved.CompatEndpoint = strings.TrimSpace(saved.CompatEndpoint)
	saved.CompatRegion = strings.TrimSpace(saved.CompatRegion)
	saved.CompatBucket = strings.TrimSpace(saved.CompatBucket)
	saved.CompatAccessKey = strings.TrimSpace(saved.CompatAccessKey)
	saved.CompatSecret = strings.TrimSpace(saved.CompatSecret)
	return saved
}

type savedSettings struct {
	LibraryDir       string                `json:"libraryDir,omitempty"`
	Libraries        []string              `json:"libraries,omitempty"`
	LibraryInodes    map[string]savedInode `json:"libraryInodes,omitempty"`
	WhisperModel     string                `json:"whisperModel,omitempty"`
	LLMProvider      string                `json:"llmProvider,omitempty"`
	LLMBaseURL       string                `json:"llmBaseURL,omitempty"`
	LLMModel         string                `json:"llmModel,omitempty"`
	LLMAPIKey        string                `json:"llmAPIKey,omitempty"`
	Language         string                `json:"language,omitempty"`
	TranslatePrompt  string                `json:"translatePrompt,omitempty"`
	TranslateContext *bool                 `json:"translateContext,omitempty"`
	GoogleEmail      string                `json:"googleEmail,omitempty"`
	GoogleRefresh    string                `json:"googleRefresh,omitempty"`
	Storage          string                `json:"storage,omitempty"`
	R2Endpoint       string                `json:"r2Endpoint,omitempty"`
	R2Bucket         string                `json:"r2Bucket,omitempty"`
	R2AccessKey      string                `json:"r2AccessKey,omitempty"`
	R2Secret         string                `json:"r2Secret,omitempty"`
	S3Region         string                `json:"s3Region,omitempty"`
	S3Bucket         string                `json:"s3Bucket,omitempty"`
	S3AccessKey      string                `json:"s3AccessKey,omitempty"`
	S3Secret         string                `json:"s3Secret,omitempty"`
	OSSRegion        string                `json:"ossRegion,omitempty"`
	OSSBucket        string                `json:"ossBucket,omitempty"`
	OSSAccessKey     string                `json:"ossAccessKey,omitempty"`
	OSSSecret        string                `json:"ossSecret,omitempty"`
	COSRegion        string                `json:"cosRegion,omitempty"`
	COSBucket        string                `json:"cosBucket,omitempty"`
	COSAccessKey     string                `json:"cosAccessKey,omitempty"`
	COSSecret        string                `json:"cosSecret,omitempty"`
	CompatEndpoint   string                `json:"s3CompatEndpoint,omitempty"`
	CompatRegion     string                `json:"s3CompatRegion,omitempty"`
	CompatBucket     string                `json:"s3CompatBucket,omitempty"`
	CompatAccessKey  string                `json:"s3CompatAccessKey,omitempty"`
	CompatSecret     string                `json:"s3CompatSecret,omitempty"`
	CompatPathStyle  bool                  `json:"s3CompatPathStyle,omitempty"`
	Look             *film.Look            `json:"look,omitempty"`
	SubMode          string                `json:"subMode,omitempty"`
	SubOrder         string                `json:"subOrder,omitempty"`
	Untranslated     string                `json:"untranslated,omitempty"`
	YieldSubs        *bool                 `json:"yieldSubs,omitempty"`
	Onboarded        bool                  `json:"onboarded,omitempty"`
}

type savedInode struct {
	Dev uint64 `json:"dev"`
	Ino uint64 `json:"ino"`
}

type llmPreset struct {
	ID, Name, Blurb, BaseURL, Model, Group string
}

func llmPresets() []llmPreset {
	return []llmPreset{
		{ID: "everyapi", Name: "EveryAPI", Blurb: "本机已登录的 EveryAPI", BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Group: "云服务"},
		{ID: "openai", Name: "OpenAI", Blurb: "OpenAI 官方接口", BaseURL: "https://api.openai.com/v1", Group: "云服务"},
		{ID: "openrouter", Name: "OpenRouter", Blurb: "一个接口调用多家模型", BaseURL: "https://openrouter.ai/api/v1", Group: "云服务"},
		{ID: "groq", Name: "Groq", Blurb: "Groq 的 OpenAI 兼容接口", BaseURL: "https://api.groq.com/openai/v1", Group: "云服务"},
		{ID: "deepseek", Name: "DeepSeek", Blurb: "DeepSeek 的 OpenAI 兼容接口", BaseURL: "https://api.deepseek.com/v1", Group: "云服务"},
		{ID: "xai", Name: "xAI", Blurb: "Grok 的 OpenAI 兼容接口", BaseURL: "https://api.x.ai/v1", Group: "云服务"},
		{ID: "mistral", Name: "Mistral", Blurb: "Mistral 的 OpenAI 兼容接口", BaseURL: "https://api.mistral.ai/v1", Group: "云服务"},
		{ID: "dashscope", Name: "通义千问", Blurb: "阿里云 DashScope 兼容接口", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Group: "云服务"},
		{ID: "moonshot", Name: "Kimi", Blurb: "Moonshot 的 OpenAI 兼容接口", BaseURL: "https://api.moonshot.cn/v1", Group: "云服务"},
		{ID: "zhipu", Name: "智谱 GLM", Blurb: "智谱的 OpenAI 兼容接口", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Group: "云服务"},
		{ID: "siliconflow", Name: "硅基流动", Blurb: "硅基流动的 OpenAI 兼容接口", BaseURL: "https://api.siliconflow.cn/v1", Group: "云服务"},
		{ID: "ollama", Name: "Ollama", Blurb: "本机 Ollama", BaseURL: "http://127.0.0.1:11434/v1", Group: "本机"},
		{ID: "lmstudio", Name: "LM Studio", Blurb: "本机 LM Studio", BaseURL: "http://127.0.0.1:1234/v1", Group: "本机"},
		{ID: "custom", Name: "OpenAI 兼容", Blurb: "任何兼容 OpenAI Chat Completions 的接口", Group: "其他"},
	}
}

func llmPresetByID(id string) (llmPreset, bool) {
	for _, preset := range llmPresets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return llmPreset{}, false
}

func llmProviderID(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, preset := range llmPresets() {
		if preset.ID == "custom" || preset.BaseURL == "" {
			continue
		}
		if strings.TrimRight(preset.BaseURL, "/") == base {
			return preset.ID
		}
	}
	return "custom"
}

func (a *app) syncLLM(tools media.Tools) {
	a.pipeline = tools
	a.llmBase = tools.Translator.BaseURL
	a.llmModel = tools.Translator.Model
	a.llmKey = tools.Translator.APIKey
	a.llmCredential = tools.Translator.Credential
	a.llmProvider = llmProviderID(tools.Translator.BaseURL)
}

func (a *app) selectLLM(id string) {
	if a.llmProvider == id {
		return
	}
	preset, ok := llmPresetByID(id)
	if !ok {
		return
	}
	a.llmProvider = id
	if preset.BaseURL != "" {
		a.llmBase = preset.BaseURL
	}
	if preset.Model != "" {
		a.llmModel = preset.Model
	}
	a.applyLLM()
}

func (a *app) applyLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.Translator.BaseURL = strings.TrimSpace(a.llmBase)
	tools.Translator.Model = strings.TrimSpace(a.llmModel)
	tools.Translator.APIKey = strings.TrimSpace(a.llmKey)
	tools.Translator.Credential = nil
	if tools.Translator.BaseURL == media.EveryAPIBaseURL && tools.Translator.APIKey == "" {
		tools.Translator.Credential = a.llmCredential
	}
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = "翻译服务无法保存：" + err.Error()
	}
}

func (a *app) testLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.testingLLM {
		return
	}
	a.testingLLM = true
	a.llmTestResult = ""
	translator := tools.Translator
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := translator.Ping(ctx)
		update(func() {
			a.testingLLM = false
			if err != nil {
				a.llmTestResult = err.Error()
				return
			}
			a.llmTestResult = "连接成功"
		})
	}()
}

func (a *app) speechSearchDirs() []string {
	dirs := []string{a.modelsDir}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		dirs = append(dirs, filepath.Dir(tools.WhisperModel))
	}
	return append(dirs, a.extraModelDirs...)
}

func (a *app) useSpeechModel(path string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.WhisperModel = path
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = "模型无法保存：" + err.Error()
	}
}

func (a *app) downloadSpeechModel(id string) {
	model, ok := media.SpeechModelByID(id)
	if !ok || a.downloading != "" {
		return
	}
	if a.modelsDir == "" {
		a.downloadErr = "还没有模型目录"
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloading = id
	a.downloadWritten = 0
	a.downloadTotal = 0
	a.downloadErr = ""
	a.downloadCancel = cancel
	dir := a.modelsDir
	update := a.update
	go func() {
		var last time.Time
		path, err := media.DownloadSpeechModel(ctx, media.WhisperModelBaseURL, model.File, dir, func(written, total int64) {
			if time.Since(last) < 200*time.Millisecond && written != total {
				return
			}
			last = time.Now()
			update(func() {
				if a.downloading != id {
					return
				}
				a.downloadWritten = written
				a.downloadTotal = total
			})
		})
		update(func() {
			if a.downloading != id {
				return
			}
			a.downloading = ""
			a.downloadCancel = nil
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.downloadErr = err.Error()
				}
				return
			}
			a.useSpeechModel(path)
		})
	}()
}

func (a *app) cancelDownload() {
	if a.downloadCancel != nil {
		a.downloadCancel()
	}
}

func (a *app) redetectTools() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	a.pipeline = tools.Relook()
	a.loadToolVersions()
}

// loadToolVersions asks each installed tool for its version off the UI thread.
func (a *app) loadToolVersions() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	paths := []string{tools.YTDLP, tools.FFmpeg, tools.Whisper}
	a.versionGen++
	gen := a.versionGen
	update := a.update
	go func() {
		versions := map[string]string{}
		for _, path := range paths {
			if path == "" {
				continue
			}
			if version := toolVersion(path); version != "" {
				versions[path] = version
			}
		}
		update(func() {
			if a.versionGen != gen {
				return
			}
			a.toolVersions = versions
		})
	}()
}

func toolVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"--version"}
	if strings.Contains(strings.ToLower(filepath.Base(path)), "ffmpeg") {
		args = []string{"-version"}
	}
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if len(out) == 0 || (err != nil && ctx.Err() != nil) {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return shortVersion(line)
}

func shortVersion(line string) string {
	fields := strings.Fields(line)
	if len(fields) >= 3 && strings.EqualFold(fields[0], "ffmpeg") && strings.EqualFold(fields[1], "version") {
		return fields[2]
	}
	lower := strings.ToLower(line)
	if i := strings.Index(lower, "version"); i >= 0 {
		rest := strings.Trim(line[i+len("version"):], " :\t")
		if parts := strings.Fields(rest); len(parts) > 0 {
			return strings.Trim(parts[0], ":")
		}
	}
	if len(line) > 48 {
		return line[:48]
	}
	return line
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

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{
		language: "简体中文", translatePrompt: media.DefaultSubtitlePrompt, translateContext: true,
		lib:      newLibraryUI(),
		player:   newPlayerUI(),
		store:    newSessionStore(),
		pipeline: pipeline, update: update,
	}
}

func (a *app) openLibrary(path string) {
	lib, err := shelf.Load(path)
	a.library = lib
	if err != nil {
		a.saveErr = "视频库无法读取：" + err.Error()
	}
}

func (a *app) running() bool {
	for _, job := range a.library.Jobs {
		if job.Stage != "" && !job.Stage.Done() {
			return true
		}
	}
	return false
}

func (a *app) remove(id string) {
	if a.removed == nil {
		a.removed = map[string]bool{}
	}
	a.removed[id] = true
	kept := make([]core.Job, 0, len(a.library.Jobs))
	for _, job := range a.library.Jobs {
		if job.ID != id {
			kept = append(kept, job)
		}
	}
	a.library.Jobs = kept
	if a.selectedID == id {
		a.newDraft()
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = "视频库无法保存：" + err.Error()
	}
}

func (a *app) retry(job core.Job) {
	if a.running() {
		return
	}
	job.Err = nil
	job.Stage = core.StageQueued
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.language)
	}
	if job.Language == "" {
		a.status = "请填写目标语言"
		return
	}
	a.page = pageFilms
	a.run(job)
}

func (a *app) newDraft() {
	a.selectedID = ""
	a.job = nil
	a.source = ""
	a.status = ""
}

func (a *app) start() {
	if a.running() {
		return
	}
	job, err := a.newJob(a.source)
	if err != nil {
		a.job = nil
		a.status = err.Error()
		return
	}
	a.run(job)
}

// run keeps job.ID, so a retry updates the card already on screen.
func (a *app) run(job core.Job) {
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.Translator.Prompt = a.translatePrompt
		tools.Translator.Context = a.translateContext
		a.pipeline = tools
	}
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	a.selectedID = job.ID

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.show(job)
	pipeline, update := a.pipeline, a.update
	go func() {
		defer cancel()
		media.Run(ctx, pipeline, job, job.Language, func(j core.Job) {
			update(func() { a.show(j) })
		})
	}()
}

func (a *app) stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *app) show(job core.Job) {
	if a.removed[job.ID] {
		return
	}
	a.library.Upsert(job)
	if err := a.library.Save(); err != nil {
		a.saveErr = "视频库无法保存：" + err.Error()
	} else {
		a.saveErr = ""
	}
	if a.selectedID == job.ID {
		a.job = &job
		a.status = statusText(job)
	}
	if a.openKey == job.ID {
		a.loadSubtitles(job.ID, job.SubtitlePath)
	}
}

// subtitleView is the subtitle file open on the film page. list keeps the scroll position across frames.
type subtitleView struct {
	key   string
	path  string
	cues  []core.Segment
	err   string
	ready bool
	list  ui.ListState
	clips ui.ListState
	mod   time.Time
	size  int64
	seen  bool
}

// loadSubtitles reads path off the UI thread. An empty path means this film has no subtitle yet.
func (a *app) loadSubtitles(key, path string) {
	if a.player.subs.key == key && a.player.subs.path == path {
		return
	}
	a.player.subs = subtitleView{key: key, path: path}
	if path == "" {
		a.player.subs.ready = true
		return
	}
	update := a.update
	if update == nil {
		a.player.subs.err = "字幕无法读取"
		a.player.subs.ready = true
		return
	}
	go func() {
		data, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		var cues []core.Segment
		msg := ""
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				msg = "字幕文件不在了"
			} else {
				msg = "字幕无法读取"
			}
		} else if cues, err = media.ParseSRT(data); err != nil {
			msg = "字幕无法读取"
			cues = nil
		}
		update(func() {
			if a.player.subs.key != key || a.player.subs.path != path {
				return
			}
			a.player.subs.cues = cues
			a.player.subs.err = msg
			a.player.subs.ready = true
			if statErr == nil {
				a.player.subs.mod = info.ModTime()
				a.player.subs.size = info.Size()
				a.player.subs.seen = true
			}
		})
	}()
}

// reloadOpenSubtitles reads the open subtitle again when the file on disk was rewritten or removed.
func (a *app) reloadOpenSubtitles() {
	if a.player.subs.key == "" || a.player.subs.path == "" {
		return
	}
	info, err := os.Stat(a.player.subs.path)
	if err != nil {
		if !a.player.subs.ready || a.player.subs.err != "" {
			return
		}
	} else if a.player.subs.seen && a.player.subs.size == info.Size() && a.player.subs.mod.Equal(info.ModTime()) {
		return
	}
	key, path := a.player.subs.key, a.player.subs.path
	a.player.subs.path = ""
	a.loadSubtitles(key, path)
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

func jobTitle(job core.Job) string {
	return shorten(jobName(job), 22)
}

func jobName(job core.Job) string {
	if job.VideoPath != "" {
		return displayVideoName(job.VideoPath)
	}
	parsed, err := url.Parse(job.Source)
	if err != nil || parsed.Host == "" {
		return job.Source
	}
	host := strings.TrimPrefix(parsed.Hostname(), "www.")
	if host == "youtu.be" {
		if id := strings.Trim(parsed.Path, "/"); id != "" {
			return "YouTube · " + id
		}
	}
	if host == "youtube.com" || host == "m.youtube.com" || host == "music.youtube.com" {
		if id := parsed.Query().Get("v"); id != "" {
			return "YouTube · " + id
		}
	}
	title := host
	if parsed.Path != "" && parsed.Path != "/" {
		title += parsed.Path
	}
	return title
}

func displayVideoName(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name != "video" {
		return name
	}
	parent := filepath.Base(filepath.Dir(path))
	if parent == "" || parent == "." || parent == string(filepath.Separator) {
		return name
	}
	return parent
}

func (a *app) posterOf(video string) ui.ImageSource {
	if video == "" {
		return nil
	}
	return a.posters[video]
}

// ensurePoster grabs one frame the first time a card needs it. A miss is remembered so the card does not retry every frame.
func (a *app) ensurePoster(video string) {
	if video == "" || a.update == nil || a.posters[video] != nil || a.posterMiss[video] || a.posterBusy[video] {
		return
	}
	dest := posterDest(a.posterDir, video)
	if dest == "" {
		a.markPosterMiss(video)
		return
	}
	if _, err := os.Stat(video); err != nil {
		a.markPosterMiss(video)
		return
	}
	tools, ok := a.pipeline.(media.Tools)
	replace := false
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		replace = media.PosterTooDark(dest)
		if !replace {
			ok = true
			tools.FFmpeg = ""
		}
	}
	if !ok || (tools.FFmpeg == "" && !fileExists(dest)) {
		a.markPosterMiss(video)
		return
	}
	if a.posterBusy == nil {
		a.posterBusy = map[string]bool{}
	}
	a.posterBusy[video] = true
	update, ffmpeg := a.update, tools.FFmpeg
	go func() {
		var err error
		if _, statErr := os.Stat(dest); statErr != nil || replace {
			err = media.GrabPoster(context.Background(), ffmpeg, video, dest)
		}
		var img ui.ImageSource
		if err == nil {
			data, readErr := os.ReadFile(dest)
			if readErr != nil {
				err = readErr
			} else {
				img, err = ui.DecodeImage(data)
			}
		}
		update(func() {
			delete(a.posterBusy, video)
			if err != nil || img == nil {
				a.markPosterMiss(video)
				return
			}
			if a.posters == nil {
				a.posters = map[string]ui.ImageSource{}
			}
			a.posters[video] = img
		})
	}()
}

func (a *app) markPosterMiss(video string) {
	if a.posterMiss == nil {
		a.posterMiss = map[string]bool{}
	}
	a.posterMiss[video] = true
}

func posterKey(video string) string {
	sum := sha256.Sum256([]byte(video))
	return hex.EncodeToString(sum[:8])
}

func posterDest(posterDir, video string) string {
	if strings.TrimSuffix(filepath.Base(video), filepath.Ext(video)) == "video" {
		return filepath.Join(filepath.Dir(video), "cover.jpg")
	}
	if posterDir == "" {
		return ""
	}
	return filepath.Join(posterDir, posterKey(video)+".jpg")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func statusText(job core.Job) string {
	switch job.Stage {
	case core.StageQueued:
		return "任务已加入队列"
	case core.StageDownloading:
		return "正在下载视频…"
	case core.StageTranscribing:
		return "正在本地转录…"
	case core.StageTranslating:
		return "正在翻译字幕…"
	case core.StageExporting:
		return "正在导出双语字幕…"
	case core.StageComplete:
		return "完成：" + job.SubtitlePath
	case core.StageFailed:
		if errors.Is(job.Err, context.Canceled) {
			return "已取消"
		}
		return "失败：" + job.Err.Error()
	}
	return string(job.Stage)
}

// defaultExtraModelDirs lists places other local apps keep whisper.cpp models, so one already on disk can be selected without copying it.
func defaultExtraModelDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Application Support", "Vimpo", "models"),
		filepath.Join(home, ".cache", "hyperframes", "whisper", "models"),
	}
}

func main() {
	var win *mygo.Window
	tools := media.ToolsFromEnv()
	a := newApp(tools, func(fn func()) { win.Update(fn) })
	defer func() { _ = a.store.Close() }()
	a.settingsPath = filepath.Join(filepath.Dir(shelf.DefaultPath()), "settings.json")
	a.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.extraModelDirs = defaultExtraModelDirs()
	saved := loadSavedSettings(a.settingsPath)
	libs := cleanLibraries(saved.Libraries)
	if saved.LibraryDir != "" {
		libs = addLibrary(libs, saved.LibraryDir)
	}
	libraryDir := ""
	if env := os.Getenv("LINGOCUT_OUTPUT_DIR"); env != "" {
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
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
	}
	tools.OutputDir = libraryDir
	a.pipeline = tools
	if os.Getenv("LINGOCUT_WHISPER_MODEL") == "" && saved.WhisperModel != "" {
		if info, err := os.Stat(saved.WhisperModel); err == nil && !info.IsDir() && info.Size() > 0 {
			tools.WhisperModel = saved.WhisperModel
			a.pipeline = tools
		}
	}
	if os.Getenv("LINGOCUT_LLM_BASE_URL") == "" && saved.LLMBaseURL != "" {
		tools.Translator.BaseURL = saved.LLMBaseURL
	}
	if os.Getenv("LINGOCUT_LLM_MODEL") == "" && saved.LLMModel != "" {
		tools.Translator.Model = saved.LLMModel
	}
	if os.Getenv("LINGOCUT_LLM_API_KEY") == "" && os.Getenv("EVERYAPI_API_KEY") == "" && saved.LLMAPIKey != "" {
		tools.Translator.APIKey = saved.LLMAPIKey
	}
	credential := tools.Translator.Credential
	if tools.Translator.BaseURL != media.EveryAPIBaseURL || tools.Translator.APIKey != "" {
		tools.Translator.Credential = nil
	}
	a.syncLLM(tools)
	a.llmCredential = credential
	if saved.Language != "" {
		a.language = saved.Language
	}
	if saved.TranslatePrompt != "" {
		a.translatePrompt = saved.TranslatePrompt
	}
	if saved.TranslateContext != nil {
		a.translateContext = *saved.TranslateContext
	}
	applyCloudEnv(a, saved)
	a.restorePlayer(saved)
	a.overlay.onboarded = saved.Onboarded
	if cache, err := os.UserCacheDir(); err == nil {
		a.posterDir = filepath.Join(cache, "LingoCut", "posters")
	}
	a.openLibrary(shelf.DefaultPath())
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.VimpoLibraryDir()) {
		bundle := media.DefaultLibraryDir()
		moved, err := media.DetachOwnVideos(a.snap().LibraryDir, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.Save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "资料库无法搬出：" + err.Error()
		} else {
			libs := dropLibrary(a.snap().Libraries, media.VimpoLibraryDir())
			libs = addLibrary(libs, bundle)
			a.edit(func(s *session) {
				s.Libraries = libs
				s.LibraryDir = bundle
			})
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.LegacyLibraryDir()) {
		from := a.snap().LibraryDir
		bundle := media.DefaultLibraryDir()
		moved, err := media.AdoptLibrary(from, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.Save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "影视库无法整理：" + err.Error()
		} else {
			media.RemoveEmptyLibrary(from)
			libs := addLibrary(a.snap().Libraries, bundle)
			a.edit(func(s *session) {
				s.Libraries = libs
				s.LibraryDir = bundle
			})
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	for _, dir := range a.knownLibraries() {
		brand.Ensure(dir, appIconPNG)
	}
	a.refreshLibrary()
	a.restartLibraryWatch()
	mygo.Theme.SetSource(mygo.ThemeLight)
	mygo.App.WhenReady(func() {
		// The sidebar's material: AppKit's source list on macOS and Mica on Windows, whose NavigationView pane is transparent over it, as Onevra's shell chose.
		vibrancy := mygo.VibrancyNone
		switch runtime.GOOS {
		case "darwin":
			vibrancy = mygo.VibrancySidebar
		case "windows":
			vibrancy = mygo.VibrancyMica
		}
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:                "LingoCut",
			Width:                1100,
			Height:               720,
			MinWidth:             900,
			MinHeight:            560,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
			Transparent:          runtime.GOOS == "darwin",
			Vibrancy:             vibrancy,
			Content:              ui.View(a.view),
		})
		a.win = win
		nodra.Invalidate(a.store, win)
		a.loadToolVersions()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
