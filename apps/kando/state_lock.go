package main

import (
	"context"
	"path/filepath"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/lock"
	"kando/apps/kando/remote"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// A library's password (package lock, shelf.Lock) is an app lock, not encryption: while a library that has one is locked, Kando shows nothing of its films, in the window or outside it, and its files stay ordinary files on disk. The lock is kept in library.json (shelf.Library.Locks); which library is unlocked is kept only in memory, so quitting locks it again.

// lockUI is the libraries' passwords as the window asks for them.
type lockUI struct {
	// unlocked is the library whose password was typed since the app started, by its cleaned path: the open library, or the one about to open once its password was typed in the dialog; "" while none is. One is unlocked at a time: opening another library (lockLibraryChanged), Lock Now and quitting lock it again.
	unlocked string
	// dialog is the password dialog, and screen the last try of the lock screen's field.
	dialog lockDialog
	screen lockTry
	// passed is the locked library whose password was typed for one action while that action runs (lockThen): it may act on the library, as moving a film into it, without showing it.
	passed string
	// run numbers the password checks, which run off the engine's goroutine as each takes a moment; an answer the dialog or the screen no longer waits for is dropped.
	run int
}

// lockDialog is the password dialog: whether it is open and release, which gives back the window's modal flag it holds (holdModal); what it asks for, of the library at dir (a cleaned path); then, the action an Unlock lets through once the password is right (lockThen), nil when it opens the library; and its last try.
type lockDialog struct {
	open    bool
	release func()
	mode    lock.Mode
	dir     string
	then    func()
	lockTry
}

// lockTry is a try of a password: why the last did not go through, and waiting, the run of the check it waits for, 0 when none.
type lockTry struct {
	problem lock.Problem
	detail  string
	waiting int
}

func newLockUI() lockUI { return lockUI{} }

// lockHooks locks the library left when another opens.
func (a *app) lockHooks() areaHooks {
	return areaHooks{libraryChanged: a.lockLibraryChanged}
}

// lockLibraryChanged locks the library that was unlocked when another library opened, unless the one that opened is the one just unlocked in the dialog; the lock screen's try is dropped with the library it was for.
func (a *app) lockLibraryChanged() {
	if a.lock.unlocked != "" && a.lock.unlocked != filepath.Clean(a.snap().LibraryDir) {
		a.lock.unlocked = ""
	}
	a.lock.screen = lockTry{}
}

// lockKeys: 立即锁定 has no key of its own. It is in the File menu and the palette while the open library has a password and is unlocked.
func (a *app) lockKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupFile, Label: locales.LockNow(cx), When: a.lockCanLockNow, Run: a.LockNow},
	}
}

// LibraryHasPassword reports whether the library at dir has a password.
func (a *app) LibraryHasPassword(dir string) bool {
	_, ok := a.library.LockOf(dir)
	return ok
}

// LibraryLocked reports whether the library at dir has a password that has not been typed since the app started, so nothing of its films shows.
func (a *app) LibraryLocked(dir string) bool {
	_, ok := a.library.LockOf(dir)
	return ok && filepath.Clean(dir) != a.lock.unlocked
}

// lockShut reports whether the open library is locked: the lock screen covers the window, and the films, the queue's jobs, the saved lines and the control socket show nothing of it.
func (a *app) lockShut() bool {
	dir := a.snap().LibraryDir
	return dir != "" && a.LibraryLocked(dir)
}

// lockHidesJob reports whether job belongs to a library that is locked, the open one or another the queue runs it for, so neither the window nor the system nor the control socket shows it.
func (a *app) lockHidesJob(job kcore.Job) bool {
	for dir := range a.library.Locks {
		if dir != a.lock.unlocked && films.InLibrary(dir, job) {
			return true
		}
	}
	return false
}

// lockVisible is jobs without those of locked libraries (lockHidesJob); jobs itself while no library is locked.
func (a *app) lockVisible(jobs []kcore.Job) []kcore.Job {
	locked := false
	for dir := range a.library.Locks {
		if dir != a.lock.unlocked {
			locked = true
			break
		}
	}
	if !locked {
		return jobs
	}
	out := make([]kcore.Job, 0, len(jobs))
	for _, job := range jobs {
		if !a.lockHidesJob(job) {
			out = append(out, job)
		}
	}
	return out
}

// lockLibraryLabel is name, the name a menu gives the library at dir, with the mark of a locked library when it is locked.
func (a *app) lockLibraryLabel(cx *core.Scope, dir, name string) string {
	if a.LibraryLocked(dir) {
		return locales.LockLibraryLocked(cx, name)
	}
	return name
}

// lockAsksFirst opens the password dialog for the library at dir and reports true when it is locked and is not the one open, so switching to it waits for its password (Choose). It reports true too while the dialog is open, which holds back a switch.
func (a *app) lockAsksFirst(dir string) bool {
	if a.lock.dialog.open {
		return true
	}
	if dir == "" || !a.LibraryLocked(dir) || filepath.Clean(dir) == filepath.Clean(a.snap().LibraryDir) {
		return false
	}
	a.lockAsk(lock.Unlock, dir)
	return true
}

// lockBars reports whether the library at dir is locked for an action on it, as moving a film into or out of it: it is locked and its password was not typed for this action (lockThen).
func (a *app) lockBars(dir string) bool {
	return a.LibraryLocked(dir) && filepath.Clean(dir) != a.lock.passed
}

// lockThen runs fn, an action on the library at dir, at once when the library is not locked, and else once its password is typed in the dialog, as switching to it asks for it (lockAsksFirst). The password lets fn through alone: the library stays locked, and nothing of its films shows. While the dialog is open for something else, fn does not run.
func (a *app) lockThen(dir string, fn func()) {
	if !a.LibraryLocked(dir) {
		fn()
		return
	}
	if a.lock.dialog.open {
		return
	}
	a.lockAsk(lock.Unlock, dir)
	a.lock.dialog.then = fn
}

// SetPassword, ChangePassword and RemovePassword open the password dialog for the library at dir, the switcher's 设置密码…, 更改密码… and 移除密码…; the last two ask for the current password.
func (a *app) SetPassword(dir string) {
	if !a.LibraryHasPassword(dir) {
		a.lockAsk(lock.Set, dir)
	}
}

func (a *app) ChangePassword(dir string) {
	if a.LibraryHasPassword(dir) {
		a.lockAsk(lock.Change, dir)
	}
}

func (a *app) RemovePassword(dir string) {
	if a.LibraryHasPassword(dir) {
		a.lockAsk(lock.Remove, dir)
	}
}

// lockCanLockNow reports whether 立即锁定 applies: the open library has a password and is unlocked.
func (a *app) lockCanLockNow() bool {
	dir := a.snap().LibraryDir
	return dir != "" && a.LibraryHasPassword(dir) && !a.LibraryLocked(dir)
}

// LockNow locks the open library again: the film open closes, and the lock screen asks for its password.
func (a *app) LockNow() {
	if !a.lockCanLockNow() {
		return
	}
	a.lock.unlocked = ""
	a.lock.screen = lockTry{}
	a.openKey = ""
	a.stopPlayback()
	a.areasShelfChanged()
}

// lockAsk opens the password dialog for mode on the library at dir, unless it is open already.
func (a *app) lockAsk(mode lock.Mode, dir string) {
	if a.lock.dialog.open || dir == "" {
		return
	}
	a.lock.dialog = lockDialog{open: true, release: a.holdModal(), mode: mode, dir: filepath.Clean(dir)}
}

// lockClose closes the password dialog and gives back the window's modal flag; the answer of a check still going is dropped. Cancel, Escape and a try that went through close it.
func (a *app) lockClose() {
	if a.lock.dialog.release != nil {
		a.lock.dialog.release()
	}
	a.lock.dialog = lockDialog{}
}

// lockSubmit is the dialog's button with what its fields held: the current password, and the new one twice. The new ones are compared at once; the current one is checked, and the new one's key made, off the engine's goroutine, and lockAnswered takes the answer. Nothing keeps the passwords: they go once the check has run.
func (a *app) lockSubmit(current, next, confirm string) {
	d := &a.lock.dialog
	if !d.open || d.waiting != 0 {
		return
	}
	mode, dir := d.mode, d.dir
	makes := mode == lock.Set || mode == lock.Change
	if makes && next != confirm {
		d.problem, d.detail = lock.Mismatch, ""
		return
	}
	old, has := a.library.LockOf(dir)
	if mode != lock.Set && !has {
		a.lockClose()
		return
	}
	a.lock.run++
	run := a.lock.run
	d.problem, d.detail, d.waiting = lock.NoProblem, "", run
	update := a.update
	go func() {
		ok := mode == lock.Set || old.Matches(current)
		var made shelf.Lock
		var err error
		if ok && makes {
			made, err = shelf.NewLock(next)
		}
		update(func() { a.lockAnswered(run, ok, made, err) })
	}()
}

// lockAnswered takes the answer of the dialog's check run: a wrong current password says so and lets the user try again; a right one unlocks the library and opens it, or gives it the key made of the new password, or takes its password away, and saves library.json. A lock that cannot be saved stays as it was, and the dialog says why. Engine goroutine only.
func (a *app) lockAnswered(run int, ok bool, made shelf.Lock, err error) {
	d := &a.lock.dialog
	if !d.open || d.waiting != run {
		return
	}
	d.waiting = 0
	switch {
	case !ok:
		d.problem = lock.Wrong
		return
	case err != nil:
		d.problem, d.detail = lock.SaveFailed, a.messages().LibraryShelfSaveFailed(err.Error())
		return
	}
	mode, dir := d.mode, d.dir
	if then := d.then; mode == lock.Unlock && then != nil {
		a.lockClose()
		a.lock.passed = dir
		defer func() { a.lock.passed = "" }()
		then()
		return
	}
	if mode == lock.Unlock {
		a.lockClose()
		a.lock.unlocked = dir
		if dir != filepath.Clean(a.snap().LibraryDir) {
			a.Choose(dir)
		}
		return
	}
	if mode == lock.Remove {
		made = shelf.Lock{}
	}
	if why := a.lockKeep(dir, made); why != "" {
		d.problem, d.detail = lock.SaveFailed, why
		return
	}
	// The library whose password was just set stays open: it locks at Lock Now, on quitting or when another library opens.
	if mode == lock.Set && dir == filepath.Clean(a.snap().LibraryDir) {
		a.lock.unlocked = dir
	}
	a.lockClose()
	a.countLibraries()
}

// lockKeep gives the library at dir l, or takes its lock away for the zero Lock, and saves library.json. When it cannot be saved the lock is put back as it was, and the answer says why in the interface's language; "" when it was saved.
func (a *app) lockKeep(dir string, l shelf.Lock) string {
	old, had := a.library.LockOf(dir)
	a.library.SetLock(dir, l)
	if err := a.library.Save(); err != nil {
		if !had {
			old = shelf.Lock{}
		}
		a.library.SetLock(dir, old)
		return a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
	return ""
}

// lockScreenUnlock is the lock screen's button with the password typed: it is checked off the engine's goroutine, and a right one shows the library.
func (a *app) lockScreenUnlock(password string) {
	dir := filepath.Clean(a.snap().LibraryDir)
	l, ok := a.library.LockOf(dir)
	if !ok || !a.lockShut() || a.lock.screen.waiting != 0 {
		return
	}
	a.lock.run++
	run := a.lock.run
	a.lock.screen = lockTry{waiting: run}
	update := a.update
	go func() {
		ok := l.Matches(password)
		update(func() { a.lockScreenAnswered(run, dir, ok) })
	}()
}

// lockScreenAnswered takes the answer of the lock screen's check run for the library at dir, unless another library opened meanwhile. Engine goroutine only.
func (a *app) lockScreenAnswered(run int, dir string, ok bool) {
	if a.lock.screen.waiting != run || filepath.Clean(a.snap().LibraryDir) != dir {
		return
	}
	if !ok {
		a.lock.screen = lockTry{problem: lock.Wrong}
		return
	}
	a.lock.screen = lockTry{}
	a.lock.unlocked = dir
	a.areasShelfChanged()
}

// lockMoved keys the lock of a library whose folder was renamed in Finder, and which library is unlocked, by its new path, and reports whether library.json changed (retargetLibraryPrefix).
func (a *app) lockMoved(old, next string) bool {
	if a.lock.unlocked != "" && a.lock.unlocked == filepath.Clean(old) {
		a.lock.unlocked = filepath.Clean(next)
	}
	return a.library.MoveLock(old, next)
}

// lockRemoteGate answers the control socket's methods that read or change the open library while it is locked, as the command line's films, jobs, open and add, with why they cannot: it is locked, and only the window unlocks it.
func (a *app) lockRemoteGate(ctx context.Context) error {
	_, err := remote.OnUI(ctx, a.update, func() (struct{}, error) {
		if a.lockShut() {
			return struct{}{}, remote.Errorf(remote.CodeNotReady, "%s", remoteMessages().LockLibraryIsLocked(sidebar.Title(a.snap().LibraryDir)))
		}
		return struct{}{}, nil
	})
	return err
}

// lockLook is what the dialog shows.
func (a *app) lockLook() lock.Look {
	d := a.lock.dialog
	return lock.Look{Mode: d.mode, Name: sidebar.Title(d.dir), Problem: d.problem, Detail: d.detail, Busy: d.waiting != 0}
}

// lockScreenLook is what the lock screen shows.
func (a *app) lockScreenLook() lock.Look {
	s := a.lock.screen
	return lock.Look{Mode: lock.Unlock, Name: sidebar.Title(a.snap().LibraryDir), Problem: s.problem, Busy: s.waiting != 0}
}

// lockScreenShown reports whether the lock screen covers the window: the open library is locked, and no new library is being made, which the library page's form does under it.
func (a *app) lockScreenShown() bool { return a.lockShut() && !a.snap().MakingLibrary }

// lockOthers are the items of the lock screen's 打开其他资料库… menu: the other libraries the app knows, a locked one marked, which ask for its password first (Choose), then 新建资料库… and 打开已有资料库….
func (a *app) lockOthers(cx *core.Scope) []nsdesktop.MenuItem {
	open := filepath.Clean(a.snap().LibraryDir)
	var items []nsdesktop.MenuItem
	for _, dir := range a.knownLibraries() {
		if filepath.Clean(dir) == open {
			continue
		}
		items = append(items, nsdesktop.MenuItem{Label: a.lockLibraryLabel(cx, dir, sidebar.Title(dir)), OnSelect: live.Do(a, func() { a.Choose(dir) })})
	}
	if len(items) > 0 {
		items = append(items, nsdesktop.MenuItem{Separator: true})
	}
	return append(items,
		nsdesktop.MenuItem{Label: locales.SidebarNewLibrary(cx), OnSelect: live.Do(a, a.BeginNew)},
		nsdesktop.MenuItem{Label: locales.SidebarOpenLibrary(cx), OnSelect: live.Do(a, a.PickLibrary)},
	)
}

// lockOverlay is the lock screen over the window while the open library is locked, and the password dialog over it while it is open (state_overlay.go).
func (a *app) lockOverlay(cx *core.Scope) core.Node {
	return core.Group{
		lock.Screen{
			Shown:    live.Func(a, a.lockScreenShown),
			Look:     live.Func(a, a.lockScreenLook),
			OnUnlock: live.On(a, a.lockScreenUnlock),
			Others:   func() []nsdesktop.MenuItem { return a.lockOthers(cx) },
		},
		lock.Dialog{
			Open:     live.Func(a, func() bool { return a.lock.dialog.open }),
			Look:     live.Func(a, a.lockLook),
			OnSubmit: func(current, next, confirm string) { live.Do(a, func() { a.lockSubmit(current, next, confirm) })() },
			OnCancel: live.Do(a, a.lockClose),
		},
	}
}
