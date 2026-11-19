package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/storage"
	"kando/packages/core"
)

// 移动视频到 moves a done film's video out of the library's folder to a storage location, a NAS location of 设置 › 账号与上传 › 存储位置 or the object store set up there, and 将视频移回资料库 brings it back. Only the video moves: its subtitles, transcript, corrections, terms and the other files beside it (media.Companions) stay in the library's folder, named after the video's path there, which the film is still known by, so its lines are read, studied, searched, exported and translated as before. library.json keeps where the video went (shelf.Library.Stored). The player plays it through a proxy on the loopback address (storage.Proxy), Phone Sync serves it from the location, and what needs the file on disk, as processing the film again, refuses with why. A move copies the video, checks the copy's size, and only then removes where it was, so a move that fails or is stopped leaves it where it was. One move runs at a time, in the message bar with its progress and 取消.

// storedUI is the state of the moves of videos to storage locations: the providers opened (pool) and the move going on (run, nil while none is).
type storedUI struct {
	pool *storage.Pool
	run  *storedRun
}

func newStoredUI() storedUI { return storedUI{pool: storage.NewPool(nil)} }

// storedRun is a video being moved: the film's video path, its name and the location's, whether it goes back to the library (back), how far it got in percent, and cancel, which stops it.
type storedRun struct {
	video, name, place string
	back               bool
	percent            int
	cancel             context.CancelFunc
}

// storedStorePrefix starts the ID of an object store as a location: "store:" and its kind (cloud.KindR2 and the others).
const storedStorePrefix = "store:"

// storedTargets are the locations a video can be moved to now, in the menu's order: the NAS locations of the settings, then the object store in use when it is set up.
func (a *app) storedTargets() []storage.Target {
	var out []storage.Target
	for _, l := range a.locations.list {
		out = append(out, storage.Target{ID: l.ID, Remote: &l.Remote})
	}
	if cfg := a.currentStore(); storeReady(cfg) {
		out = append(out, storage.Target{ID: storedStorePrefix + cfg.Kind, Store: &cfg})
	}
	return out
}

// storedTargetOf is the location of ID id as the settings have it now, false when it is no longer there: a NAS location removed, or an object store no longer set up.
func (a *app) storedTargetOf(id string) (storage.Target, bool) {
	if kind, ok := strings.CutPrefix(id, storedStorePrefix); ok {
		cfg := a.storeFor(kind)
		if cfg.Kind != kind || !storeReady(cfg) {
			return storage.Target{}, false
		}
		return storage.Target{ID: id, Store: &cfg}, true
	}
	for _, l := range a.locations.list {
		if l.ID == id {
			return storage.Target{ID: id, Remote: &l.Remote}, true
		}
	}
	return storage.Target{}, false
}

// storedName is the name of the location of ID id in m's language: a NAS location's own name, an object store's brand (uploadStoreName), or, for one no longer in the settings, words saying so.
func (a *app) storedName(m locales.Messages, id string) string {
	if kind, ok := strings.CutPrefix(id, storedStorePrefix); ok {
		return m.StoredObjectStore(uploadStoreName(m, kind))
	}
	for _, l := range a.locations.list {
		if l.ID == id {
			return l.Name
		}
	}
	return m.StoredLocationGone()
}

// storedOf is where the video at path was moved to, false for a video in the library's folder.
func (a *app) storedOf(path string) (shelf.Stored, bool) {
	if path == "" {
		return shelf.Stored{}, false
	}
	s, ok := a.library.Stored[path]
	return s, ok
}

// storedMark sets Stored on each of items whose video was moved to a storage location (films.Film.Stored), by stored, library.json's records.
func storedMark(items []films.Film, stored map[string]shelf.Stored) []films.Film {
	if len(stored) == 0 {
		return items
	}
	for i := range items {
		if s, ok := stored[items[i].VideoPath]; ok && items[i].VideoPath != "" {
			items[i].Stored = s.Location
		}
	}
	return items
}

// storedHooks: the length of each moved video, which no file tells now, is known again from library.json as the shelf is read; a quit stops the move going on, which leaves the video where it was, and closes the locations opened and the proxy once the app has quit.
func (a *app) storedHooks() areaHooks {
	return areaHooks{
		shelfLoaded:  a.storedLengths,
		quitStarting: a.storedCancel,
		quitting:     a.stored.pool.Close,
	}
}

// storedLengths keeps the length library.json noted of each moved video as how long it plays (app.lib.durations), as the poster's probe cannot read it from a file that is not here.
func (a *app) storedLengths() {
	for path, s := range a.library.Stored {
		if s.Duration > 0 {
			a.libraryLearnDuration(path, s.Duration)
		}
	}
}

// storedCancel stops the move going on, as the message bar's 取消 does (CancelMove): its copy stops, and the video stays where it was.
func (a *app) storedCancel() {
	if r := a.stored.run; r != nil && r.cancel != nil {
		r.cancel()
	}
}

// storedCan reports whether item's video can be moved to a storage location now, or why not in m's words: it is a done film of a job (a video no job processed is listed by its file alone, which would leave the shelf with it), keeps a video, is in the open library's folder, not one with a password, whose videos stay inside it, and is not being processed.
func (a *app) storedCan(m locales.Messages, item films.Film) error {
	name := films.Name(item)
	root := a.snap().LibraryDir
	switch {
	case item.JobID == "" || item.Stage != core.StageComplete || item.SubtitlesOnly || item.VideoPath == "":
		return errors.New(m.StoredNotDone(name))
	case !films.InDir(root, item.VideoPath):
		return errors.New(m.StoredOutside(name))
	case a.LibraryHasPassword(root):
		return errors.New(m.StoredLocked())
	case a.libraryMoveBusy(item):
		return errors.New(m.StoredProcessing(name))
	}
	return nil
}

// MoveVideoTo moves item's video to the location of ID id, as 移动视频到 ▸ does: off the engine's goroutine, the message bar saying how far it got with 取消. Once the copy is whole and of the video's size, library.json notes where it went, and the video is removed from the library's folder; a film open then closes. Why it cannot, or how it ended, the message bar says.
func (a *app) MoveVideoTo(item films.Film, id string) {
	m := a.messages()
	if _, stored := a.storedOf(item.VideoPath); stored {
		return
	}
	if a.stored.run != nil {
		a.warn(m.StoredBusy(), "")
		return
	}
	if err := a.storedCan(m, item); err != nil {
		a.warn(err.Error(), "")
		return
	}
	t, ok := a.storedTargetOf(id)
	switch {
	case !ok:
		a.warn(m.StoredLocationGone(), "")
		return
	case t.Store != nil && a.privacy.offline:
		a.warn(m.PrivacyNoUpload(), "")
		return
	}
	info, err := os.Stat(item.VideoPath)
	if err != nil || info.IsDir() {
		a.warn(m.LibraryFileMissing(), "")
		return
	}
	root := a.snap().LibraryDir
	rel, err := filepath.Rel(root, item.VideoPath)
	dest := storage.FilmPath(sidebar.Title(root), rel)
	if err != nil || dest == "" {
		a.warn(m.StoredOutside(films.Name(item)), "")
		return
	}
	run := a.storedStart(item, id, false)
	pool, update, video := a.stored.pool, a.update, item.VideoPath
	progress := a.storedProgress(update, run)
	ctx, cancel := context.WithCancel(context.Background())
	run.cancel = cancel
	go func() {
		defer cancel()
		size, err := func() (int64, error) {
			provider, err := pool.Provider(ctx, t)
			if err != nil {
				return 0, err
			}
			return storage.Put(ctx, provider, dest, video, progress)
		}()
		update(func() { a.storedUploaded(run, t, dest, size, err) })
	}()
}

// storedStart puts run, the move of item's video to or back from the location of ID id, as the move going on, and says so in the message bar.
func (a *app) storedStart(item films.Film, id string, back bool) *storedRun {
	run := &storedRun{video: item.VideoPath, name: films.Name(item), place: id, back: back}
	a.stored.run = run
	a.storedShow()
	return run
}

// storedProgress is what tells the move run, from its copy's goroutine, how far it got: each change of a whole percent goes to the engine's goroutine.
func (a *app) storedProgress(update func(func()), run *storedRun) storage.Progress {
	var mu sync.Mutex
	shown := 0
	return func(done, total int64) {
		if total <= 0 {
			return
		}
		percent := int(done * 100 / total)
		mu.Lock()
		same := percent <= shown
		if !same {
			shown = percent
		}
		mu.Unlock()
		if same {
			return
		}
		update(func() {
			if a.stored.run == run {
				run.percent = percent
				a.storedShow()
			}
		})
	}
}

// storedShow says in the message bar how far the move going on got, with 取消, unless the user closed the bar while it goes on.
func (a *app) storedShow() {
	r := a.stored.run
	if r == nil {
		return
	}
	m := a.messages()
	text := m.StoredMovingTo(r.name, a.storedName(m, r.place), r.percent)
	if r.back {
		text = m.StoredMovingBack(r.name, r.percent)
	}
	if n := &a.lib.notice; n.Open && n.Stop {
		n.Text = text
		return
	}
	if r.percent == 0 {
		a.libraryShow(films.Notice{Kind: "success", Text: text, Stop: true, Open: true})
	}
}

// storedErrText is why a move to or from t failed with err, in m's words: never err's own words for a location, which are English and may name the server.
func storedErrText(m locales.Messages, t storage.Target, err error) string {
	switch {
	case errors.Is(err, storage.ErrSizeMismatch):
		return m.StoredSizeMismatch()
	case errors.Is(err, os.ErrExist):
		return m.StoredLocalTaken()
	case t.Remote != nil:
		return account.Failure(m, *t.Remote, err)
	case t.Store != nil:
		return uploadFailure(m, err, *t.Store)
	}
	return err.Error()
}

// storedUploaded ends run, the move of a video to t, whose copy at dest of size bytes ended with err. A whole copy is noted in library.json, then the video is removed from the library's folder; when either fails the note is taken back and the copy removed, and the video stays.
func (a *app) storedUploaded(run *storedRun, t storage.Target, dest string, size int64, err error) {
	if a.stored.run != run {
		return
	}
	a.stored.run = nil
	m := a.messages()
	place := a.storedName(m, run.place)
	switch {
	case errors.Is(err, context.Canceled):
		a.tell(m.StoredStopped(run.name))
		return
	case err != nil:
		log.Printf("storage: could not move %s to a location: %v", filepath.Base(run.video), err)
		a.warn(m.StoredMoveFailed(run.name, place, storedErrText(m, t, err)), "")
		return
	}
	item, found := a.storedFilm(run.video)
	if !found || a.libraryMoveBusy(item) {
		a.storedRemoveCopy(t, dest)
		a.warn(m.StoredProcessing(run.name), "")
		return
	}
	if a.library.Stored == nil {
		a.library.Stored = map[string]shelf.Stored{}
	}
	a.library.Stored[run.video] = shelf.Stored{Location: run.place, Path: dest, Size: size, Duration: a.lib.durations[run.video], Moved: time.Now().UTC()}
	if err := a.library.Save(); err != nil {
		delete(a.library.Stored, run.video)
		a.storedRemoveCopy(t, dest)
		a.warn(a.shelfSaveErr(err, m.LibraryShelfSaveFailed), "")
		return
	}
	if a.openKey == item.Key {
		a.closeFilm()
	}
	if err := os.Remove(run.video); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("storage: could not remove %s after moving it: %v", filepath.Base(run.video), err)
		delete(a.library.Stored, run.video)
		if err := a.library.Save(); err != nil {
			a.saveErr = a.shelfSaveErr(err, m.LibraryShelfSaveFailed)
		}
		a.storedRemoveCopy(t, dest)
		a.warn(m.StoredRemoveFailed(run.name, place), "")
		return
	}
	a.refreshLibrary()
	a.tell(m.StoredMoved(run.name, place))
}

// storedFilm is the film of the job whose video is video, among the open library's jobs.
func (a *app) storedFilm(video string) (films.Film, bool) {
	for _, job := range a.library.Jobs {
		if job.VideoPath == video {
			return a.JobFilm(job.ID)
		}
	}
	return films.Film{}, false
}

// storedRemoveCopy removes the file at path of t off the engine's goroutine, as when a move to t is taken back; one it cannot remove is logged and left.
func (a *app) storedRemoveCopy(t storage.Target, path string) {
	pool := a.stored.pool
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), storage.PoolOpenTimeout)
		defer cancel()
		provider, err := pool.Provider(ctx, t)
		if err == nil {
			err = provider.Remove(ctx, path)
		}
		if err != nil {
			log.Printf("storage: could not remove a video's copy from its location: %v", err)
		}
	}()
}

// MoveVideoBack moves item's video from the storage location it was moved to back to where it was in the library's folder, as 将视频移回资料库 does: copied down off the engine's goroutine, the message bar saying how far it got with 取消, into a hidden file renamed into place once whole and of the size the location tells (storage.Get). Then library.json forgets where it was, and its copy there is removed; the message bar says when that copy could not be.
func (a *app) MoveVideoBack(item films.Film) {
	m := a.messages()
	s, ok := a.storedOf(item.VideoPath)
	if !ok {
		return
	}
	switch {
	case a.stored.run != nil:
		a.warn(m.StoredBusy(), "")
		return
	case a.libraryMoveBusy(item):
		a.warn(m.StoredProcessing(films.Name(item)), "")
		return
	}
	t, found := a.storedTargetOf(s.Location)
	switch {
	case !found:
		a.warn(m.StoredLocationGone(), "")
		return
	case t.Store != nil && a.privacy.offline:
		a.warn(m.PrivacyNoUpload(), "")
		return
	}
	run := a.storedStart(item, s.Location, true)
	pool, update, video := a.stored.pool, a.update, item.VideoPath
	progress := a.storedProgress(update, run)
	ctx, cancel := context.WithCancel(context.Background())
	run.cancel = cancel
	go func() {
		defer cancel()
		err := func() error {
			provider, err := pool.Provider(ctx, t)
			if err != nil {
				return err
			}
			return storage.Get(ctx, provider, s.Path, video, progress)
		}()
		update(func() { a.storedDownloaded(run, t, s, err) })
	}()
}

// storedDownloaded ends run, the move of a video back from t, where s says it was, whose copy down ended with err. A whole copy makes library.json forget where it was, and its copy on t is then removed off the engine's goroutine.
func (a *app) storedDownloaded(run *storedRun, t storage.Target, s shelf.Stored, err error) {
	if a.stored.run != run {
		return
	}
	a.stored.run = nil
	m := a.messages()
	place := a.storedName(m, run.place)
	switch {
	case errors.Is(err, context.Canceled):
		a.tell(m.StoredStopped(run.name))
		return
	case err != nil:
		log.Printf("storage: could not move %s back from its location: %v", filepath.Base(run.video), err)
		a.warn(m.StoredMoveBackFailed(run.name, place, storedErrText(m, t, err)), "")
		return
	}
	if a.openKey != "" {
		if item, ok := a.openFilm(); ok && item.VideoPath == run.video {
			a.closeFilm()
		}
	}
	delete(a.library.Stored, run.video)
	if err := a.library.Save(); err != nil {
		// The video is in the library's folder again, which the player and everything else take first; its copy on t stays, as the next start reads library.json as it was.
		a.warn(a.shelfSaveErr(err, m.LibraryShelfSaveFailed), "")
		a.refreshLibrary()
		return
	}
	a.refreshLibrary()
	a.tell(m.StoredMovedBack(run.name))
	pool, update, name := a.stored.pool, a.update, run.name
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), storage.PoolOpenTimeout)
		defer cancel()
		provider, err := pool.Provider(ctx, t)
		if err == nil {
			err = provider.Remove(ctx, s.Path)
		}
		if err == nil {
			return
		}
		log.Printf("storage: could not remove a video's copy from its location after moving it back: %v", err)
		update(func() { a.warn(m.StoredMovedBackLeft(name, place), "") })
	}()
}

// storedForget forgets where the video of item, a film taken off the shelf (libraryForget), was moved to, and removes its copy there off the engine's goroutine, as the confirmation of 从资料库移除 said (films.Film.Stored). A copy on a location no longer in the settings cannot be reached and stays.
func (a *app) storedForget(item films.Film) {
	s, ok := a.storedOf(item.VideoPath)
	if !ok {
		return
	}
	delete(a.library.Stored, item.VideoPath)
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
	if t, found := a.storedTargetOf(s.Location); found {
		a.storedRemoveCopy(t, s.Path)
	}
}

// storedRefusal is the error of something that needs a film's video on disk, refused for a film whose video is on a storage location: say is why, in m's words.
type storedRefusal struct{ text string }

func (e *storedRefusal) Error() string { return e.text }

// storedNeedsFile is why what needs the video at path on disk cannot run now that the video is on a storage location, said by say with the film's name and the location's, in m's words; nil for a video in the library's folder.
func (a *app) storedNeedsFile(m locales.Messages, path, name string, say func(m locales.Messages, name, place string) string) error {
	s, ok := a.storedOf(path)
	if !ok {
		return nil
	}
	return &storedRefusal{text: say(m, name, a.storedName(m, s.Location))}
}

// storedUploadRefused is why the video at path cannot be uploaded to the object store (cloud_app.go) while it is on a storage location, in m's words; nil for a video in the library's folder.
func (a *app) storedUploadRefused(m locales.Messages, path string) error {
	return a.storedNeedsFile(m, path, "", func(m locales.Messages, _, place string) string { return m.StoredUpload(place) })
}

// storedHas reports whether job's video was moved to a storage location, which the library's folder then holds its subtitles of alone, as a film that keeps no video does (dropMissing, refreshLibrary's pairing of renamed folders).
func (a *app) storedHas(job core.Job) bool {
	_, ok := a.storedOf(job.VideoPath)
	return ok
}

// storedPlayable is the IDs of the locations the films whose videos were moved can be played from now, for Phone Sync, taken on the engine's goroutine.
func (a *app) storedPlayable() map[string]storage.Target {
	out := map[string]storage.Target{}
	for _, s := range a.library.Stored {
		if _, seen := out[s.Location]; seen {
			continue
		}
		if t, ok := a.storedTargetOf(s.Location); ok {
			out[s.Location] = t
		}
	}
	return out
}
