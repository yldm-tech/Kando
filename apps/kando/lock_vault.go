package main

import (
	"context"
	"errors"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/lock"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/vault"
)

// On macOS a library's password encrypts its files (package vault): its folder holds an encrypted image, attached over the folder while Kando uses the library and detached once it does not, so while it is locked its films are not on disk to open in Finder or any other app. Kando attaches it when its password is typed (lockScreenUnlock, the dialog's Unlock), and keeps it attached while the library is unlocked, while a job downloads or processes a film into it, while a film is moved into or out of it, and while its files are moved into or out of the image; lockSettle detaches it once none of these holds. Quitting detaches every image (lockVaultQuit).

// lockVault is the state of the libraries' encrypted images.
type lockVault struct {
	// work is the library whose files are being moved into or out of its image, or whose image is getting another password, by its cleaned path, while that runs; "" while nothing is. copying is whether files are being moved, percent how far that has gone.
	work    string
	copying bool
	percent int
	// opened are the libraries whose image Kando attached, by cleaned path, until it is detached again: lockSettle looks only at these, as looking at every library's folder after each change would wait on slow and network disks.
	opened map[string]bool
	// closing are the libraries whose image is being detached; retryAt is when lockSettle tries again to detach one that another app keeps open, and fails how many tries failed, so the message bar says it once.
	closing map[string]bool
	retryAt map[string]time.Time
	fails   map[string]int
}

// lockEncrypts reports whether setting a password encrypts a library's files here (vault.Supported). Tests of the lock itself replace it, as encrypting takes a while.
var lockEncrypts = vault.Supported

// lockVaultRetry is how long lockSettle waits before it tries again to detach an image another app keeps open, as a video player opened from Finder.
const lockVaultRetry = 3 * time.Second

// lockVaultHooks: an image left attached by a run that ended without detaching it is detached once the window exists, as the library is locked again; quitting detaches every image. They run last (areas), once the queue and the player have stopped using the libraries' files.
func (a *app) lockVaultHooks() areaHooks {
	return areaHooks{
		ready: func() {
			for _, dir := range a.knownLibraries() {
				if vault.Mounted(dir) {
					a.lockVaultOpened(dir)
				}
			}
			a.lockSettle()
		},
		quitting: a.lockVaultQuit,
	}
}

// lockFilesAway reports whether the files of the library at dir are not in its folder now: it is encrypted and locked, its folder holding the image alone, or its files are being moved into or out of the image. The shelf then neither lists its folder nor takes its films for gone (refreshLibrary), and the queue starts no job in it (queueCanRun).
func (a *app) lockFilesAway(dir string) bool {
	if dir == "" {
		return false
	}
	dir = filepath.Clean(dir)
	return dir == a.lock.vault.work || (vault.Has(dir) && !vault.Mounted(dir))
}

// lockVaultOpened notes that the image of the library at dir is attached, so lockSettle detaches it once it is not needed.
func (a *app) lockVaultOpened(dir string) {
	if a.lock.vault.opened == nil {
		a.lock.vault.opened = map[string]bool{}
	}
	a.lock.vault.opened[filepath.Clean(dir)] = true
}

// lockNeeds reports whether the files of the library at dir are in use, so its image stays attached: it is the unlocked library, its files are being moved into or out of its image, a film is being moved into or out of it, or the queue's running job downloads or processes a film into it.
func (a *app) lockNeeds(dir string) bool {
	if dir == a.lock.unlocked || dir == a.lock.passed || dir == a.lock.vault.work {
		return true
	}
	return a.lockMoving(dir) || a.lockJobIn(dir)
}

// lockJobIn reports whether the queue's running job downloads or processes a film into the library at dir.
func (a *app) lockJobIn(dir string) bool {
	if a.queue.active == "" {
		return false
	}
	job, ok := a.library.Find(a.queue.active)
	if !ok {
		return false
	}
	lib := job.LibraryDir
	if lib == "" {
		lib = a.snap().LibraryDir
	}
	return filepath.Clean(lib) == dir
}

// lockSettle detaches, off the engine's goroutine, the image of each library Kando attached that nothing needs any more (lockNeeds): one locked again with Lock Now or as another library opened, one whose running job or move ended. It runs after every change (lockHooks' afterChange) and changes nothing a binding reads.
func (a *app) lockSettle() bool {
	v := &a.lock.vault
	for _, dir := range slices.Sorted(maps.Keys(v.opened)) {
		if v.closing[dir] || a.lockNeeds(dir) || time.Now().Before(v.retryAt[dir]) {
			continue
		}
		if !vault.Mounted(dir) {
			delete(v.opened, dir)
			continue
		}
		if v.closing == nil {
			v.closing = map[string]bool{}
		}
		v.closing[dir] = true
		update := a.update
		go func() {
			err := vault.Close(context.Background(), dir, false)
			update(func() { a.lockVaultClosed(dir, err) })
		}()
	}
	return false
}

// lockVaultClosed takes the end of detaching the image of the library at dir. One that another app keeps open stays attached and is tried again after lockVaultRetry; the third failure says so in the message bar, once. Engine goroutine only.
func (a *app) lockVaultClosed(dir string, err error) {
	v := &a.lock.vault
	delete(v.closing, dir)
	if err == nil {
		delete(v.opened, dir)
		delete(v.fails, dir)
		delete(v.retryAt, dir)
		return
	}
	log.Printf("lock: could not close the encrypted library %s: %v", filepath.Base(dir), err)
	if v.fails == nil {
		v.fails, v.retryAt = map[string]int{}, map[string]time.Time{}
	}
	v.fails[dir]++
	v.retryAt[dir] = time.Now().Add(lockVaultRetry)
	if v.fails[dir] == 3 {
		a.warn(a.messages().LockStillInUse(sidebar.Title(dir)), "")
	}
	if a.cx != nil {
		a.cx.AfterFunc(lockVaultRetry, func() { a.lockSettle() })
	}
}

// lockVaultQuit detaches the image of every library Kando attached, and of any known library left attached, over its folder or where a password being set or removed was moving its files (vault.Close), as Kando quits: everything that used their files has stopped by now, so one still held open by another app is detached anyway, which that app then reads as the file being gone.
func (a *app) lockVaultQuit() {
	dirs := a.knownLibraries()
	for dir := range a.lock.vault.opened {
		dirs = addLibrary(dirs, dir)
	}
	for _, dir := range dirs {
		// The gentle try has its time, and the forced one a time of its own, so a gentle try that used all of its time still leaves the image detached.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := vault.Close(ctx, dir, false)
		cancel()
		if err == nil {
			continue
		}
		log.Printf("lock: closing the encrypted library %s as Kando quits: %v; closing it anyway", filepath.Base(dir), err)
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		if err := vault.Close(ctx, dir, true); err != nil {
			log.Printf("lock: could not close the encrypted library %s: %v", filepath.Base(dir), err)
		}
		cancel()
	}
}

// lockVaultTask is what the password dialog does to the image of the library at dir for mode, off the engine's goroutine, once the password was checked against library.json's key, or, for an image no key was kept for, by the image itself: Unlock attaches it, Set moves the library's files into a new image (encrypt), and attaches it again when the library is the open one (reopen), Change gives the image the new password and Remove moves the files back out. An image that was attached for the open library (open) is detached for Change and Remove, whose hdiutil needs the image's file, which the attached volume hides, and attached again after Change, with whichever password it has then.
type lockVaultTask struct {
	mode                  lock.Mode
	dir                   string
	encrypt, reopen, open bool
	current, next         string
	progress              vault.Progress
}

func (t lockVaultTask) run(ctx context.Context) error {
	switch t.mode {
	case lock.Unlock:
		if vault.Has(t.dir) && !vault.Mounted(t.dir) {
			return vault.Open(ctx, t.dir, t.current)
		}
	case lock.Set:
		if !t.encrypt {
			return nil
		}
		if err := vault.Create(ctx, t.dir, t.next, t.progress); err != nil {
			return err
		}
		if t.reopen {
			return vault.Open(ctx, t.dir, t.next)
		}
	case lock.Change:
		if !t.encrypt {
			return nil
		}
		if t.open {
			if err := vault.Close(ctx, t.dir, false); err != nil {
				return err
			}
		}
		err := vault.ChangePassword(ctx, t.dir, t.current, t.next)
		if t.open {
			password := t.next
			if err != nil {
				password = t.current
			}
			if oerr := vault.Open(ctx, t.dir, password); err == nil {
				err = oerr
			}
		}
		return err
	case lock.Remove:
		if !t.encrypt {
			return nil
		}
		if t.open {
			if err := vault.Close(ctx, t.dir, false); err != nil {
				return err
			}
		}
		err := vault.Remove(ctx, t.dir, t.current, t.progress)
		if err != nil && t.open {
			_ = vault.Open(ctx, t.dir, t.current)
		}
		return err
	}
	return nil
}

// lockVaultProblem is what the dialog or the lock screen says for err, the end of a vault task on the library, which has a key in library.json or not (hasKey): an image that would not open for a library without a key says the password is wrong, as only the image could check it; the details go to the log.
func lockVaultProblem(dir string, err error, hasKey bool) lock.Problem {
	switch {
	case !hasKey && errors.Is(err, vault.ErrNotOpened):
		return lock.Wrong
	case errors.Is(err, vault.ErrNoSpace):
		return lock.NoSpace
	}
	log.Printf("lock: the encrypted library %s: %v", filepath.Base(dir), err)
	return lock.VaultFailed
}

// lockForgetCache removes the posters and site thumbnails of the library at dir's films from the app's cache, which is not encrypted, once the library is: they are made again inside it (posterFolder) the next time the library shows.
func (a *app) lockForgetCache(dir string) {
	if a.lib.posterDir == "" {
		return
	}
	var videos []string
	for _, job := range a.library.Jobs {
		if !films.InLibrary(dir, job) {
			continue
		}
		if job.VideoPath != "" {
			videos = append(videos, job.VideoPath)
		}
		if job.Thumbnail != "" {
			_ = os.Remove(siteThumbnailPath(a.lib.posterDir, job.Thumbnail))
			delete(a.lib.thumbs, job.Thumbnail)
		}
	}
	for path := range a.library.Names {
		videos = append(videos, path)
	}
	for _, video := range a.snap().Videos {
		videos = append(videos, video.Path)
	}
	for _, video := range videos {
		if !films.InDir(dir, video) {
			continue
		}
		_ = os.Remove(filepath.Join(a.lib.posterDir, posterKey(video)+".jpg"))
		delete(a.lib.posters, video)
		delete(a.lib.posterMiss, video)
	}
}
