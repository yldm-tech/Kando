package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/remote"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// lockPassword is the password the tests give a library; lockFilm is the title of its film, which nothing may show while the library is locked.
const (
	lockPassword = "s3cret-Library-pw"
	lockFilm     = "Private Talk"
)

// lockLibrary starts the app, with the password an app lock alone (lockEncrypts), on a library of one done film, lockFilm, whose library.json and settings.json are files of their own, and another library with a film of its own the app knows beside it. It returns both libraries' folders and library.json's path.
func lockLibrary(t *testing.T) (a *app, tt *nsgotest.App, updates chan func(), dir, other, shelfPath string) {
	t.Helper()
	// These tests are of the lock in Kando; lock_vault_test.go tests the encryption macOS adds to it.
	prev := lockEncrypts
	lockEncrypts = func() bool { return false }
	t.Cleanup(func() { lockEncrypts = prev })
	dir = filepath.Join(t.TempDir(), "Private.kando")
	other = filepath.Join(t.TempDir(), "Public.kando")
	video := filepath.Join(dir, lockFilm, "talk.mp4")
	srt := filepath.Join(dir, lockFilm, "talk.bilingual.srt")
	writeFakeClip(t, video, 5, "202020", "101010")
	remoteWriteFile(t, srt, talkSubtitles)
	remoteWriteFile(t, filepath.Join(other, "Open Film", "open.mp4"), "video")
	shelfPath = filepath.Join(t.TempDir(), "library.json")
	a, tt, updates = newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.openLibrary(shelfPath)
	a.library.Jobs = []core.Job{{ID: "secret", Title: lockFilm, Source: "https://www.youtube.com/watch?v=privatevid", Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: dir, CompletedAt: time.Now()}}
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	a.edit(func(s *session) {
		s.Libraries = []string{dir, other}
		s.LibraryDir = dir
	})
	a.refreshLibrary()
	refresh(tt, a)
	return a, tt, updates, dir, other, shelfPath
}

// lockDialogSubmit types into the password dialog's fields, those of fields that are not "", presses its button and waits for the check, which runs off the engine's goroutine, to answer.
func lockDialogSubmit(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), current, next, confirm string) {
	t.Helper()
	for id, text := range map[string]string{"id:lock-current": current, "id:lock-new": next, "id:lock-confirm": confirm} {
		if text != "" {
			tt.Type(id, text)
		}
	}
	tt.Press("id:lock-submit")
	waitUntil(t, tt, a, updates, func() bool { return a.lock.dialog.waiting == 0 })
}

// lockSetPassword gives the open library lockPassword through the switcher's 设置密码…, as the user does.
func lockSetPassword(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
	t.Helper()
	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(testlocale.Messages().LockMenuSet())
	lockDialogSubmit(t, a, tt, updates, "", lockPassword, lockPassword)
	if a.lock.dialog.open {
		t.Fatalf("the dialog stayed open: %+v", a.lock.dialog.lockTry)
	}
}

// lockShows reports whether the window shows anything of the locked library's film: its title in any text, a poster of it, or its card.
func lockShows(tt *nsgotest.App) bool {
	return slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.Contains(s, lockFilm) }) || tt.Exists("id:poster-secret") || tt.Exists("id:card-secret")
}

// Setting a password asks for it twice, keeps the library open, and stores the key's parameters; changing and removing it ask for the current one, and a wrong one changes nothing and says so.
func TestLockSetChangeAndRemoveAskForThePassword(t *testing.T) {
	a, tt, updates, dir, _, shelfPath := lockLibrary(t)
	m := testlocale.Messages()
	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuSet())
	if !hasText(tt, m.LockSetTitle("Private")) || !hasText(tt, m.LockSetNote()) || !a.modalOpen() {
		t.Fatalf("the set dialog shows %q", tt.Texts())
	}
	if !tt.Find("id:lock-new").Input.Masked || !tt.Find("id:lock-confirm").Input.Masked {
		t.Fatal("the password fields show what is typed")
	}
	lockDialogSubmit(t, a, tt, updates, "", lockPassword, "something else")
	if !hasText(tt, m.LockMismatch()) || a.LibraryHasPassword(dir) {
		t.Fatalf("a mismatch set the password, texts %q", tt.Texts())
	}
	lockDialogSubmit(t, a, tt, updates, "", lockPassword, lockPassword)
	if a.lock.dialog.open || a.modalOpen() || !a.LibraryHasPassword(dir) || a.LibraryLocked(dir) || !lockShows(tt) {
		t.Fatalf("after setting it: dialog %t, locked %t, texts %q", a.lock.dialog.open, a.LibraryLocked(dir), tt.Texts())
	}
	saved, err := shelf.Load(shelfPath)
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := saved.LockOf(dir); !ok || l.Algorithm != shelf.LockAlgorithm || l.Iterations < 600_000 || !l.Matches(lockPassword) {
		t.Fatalf("library.json keeps %+v, %t", l, ok)
	}

	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuChange())
	lockDialogSubmit(t, a, tt, updates, "wrong", "next-pw", "next-pw")
	if !hasText(tt, m.LockWrong()) || !a.lock.dialog.open {
		t.Fatalf("a wrong current password: texts %q", tt.Texts())
	}
	if l, _ := a.library.LockOf(dir); !l.Matches(lockPassword) {
		t.Fatal("a wrong current password changed the password")
	}
	lockDialogSubmit(t, a, tt, updates, lockPassword, "next-pw", "next-pw")
	if l, _ := a.library.LockOf(dir); a.lock.dialog.open || !l.Matches("next-pw") || l.Matches(lockPassword) {
		t.Fatalf("the password did not change: dialog %t", a.lock.dialog.open)
	}

	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuRemove())
	lockDialogSubmit(t, a, tt, updates, lockPassword, "", "")
	if !hasText(tt, m.LockWrong()) || !a.LibraryHasPassword(dir) {
		t.Fatal("the old password removed the password")
	}
	tt.Press("id:lock-cancel")
	if a.lock.dialog.open || a.modalOpen() || !a.LibraryHasPassword(dir) {
		t.Fatal("取消 did not close the dialog, or removed the password")
	}
	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuRemove())
	lockDialogSubmit(t, a, tt, updates, "next-pw", "", "")
	if a.LibraryHasPassword(dir) || a.lock.dialog.open {
		t.Fatal("the password was not removed")
	}
	if saved, _ := shelf.Load(shelfPath); len(saved.Locks) != 0 {
		t.Fatalf("library.json keeps %v", saved.Locks)
	}
}

// Switching to a locked library asks for its password first; a wrong one says so and leaves the open library as it is, and the right one opens it. Switching away locks it again.
func TestLockSwitchAsksForThePasswordAndRelocks(t *testing.T) {
	a, tt, updates, dir, other, _ := lockLibrary(t)
	m := testlocale.Messages()
	lockSetPassword(t, a, tt, updates)
	apply(tt, a, func() { a.Choose(other) })
	if !a.LibraryLocked(dir) || lockShows(tt) {
		t.Fatalf("leaving the library left it unlocked, texts %q", tt.Texts())
	}
	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockLibraryLocked("Private"))
	if !a.lock.dialog.open || !hasText(tt, m.LockUnlockTitle("Private")) || a.snap().LibraryDir != other {
		t.Fatalf("choosing the locked library: dialog %t, open %s", a.lock.dialog.open, a.snap().LibraryDir)
	}
	for _, wrong := range []string{"s3cret", strings.ToUpper(lockPassword), lockPassword + " "} {
		lockDialogSubmit(t, a, tt, updates, wrong, "", "")
		if !hasText(tt, m.LockWrong()) || a.snap().LibraryDir != other || !a.LibraryLocked(dir) || lockShows(tt) {
			t.Fatalf("the wrong password %q: open %s, locked %t", wrong, a.snap().LibraryDir, a.LibraryLocked(dir))
		}
	}
	lockDialogSubmit(t, a, tt, updates, lockPassword, "", "")
	if a.lock.dialog.open || a.snap().LibraryDir != dir || a.LibraryLocked(dir) || !lockShows(tt) {
		t.Fatalf("the right password: dialog %t, open %s, texts %q", a.lock.dialog.open, a.snap().LibraryDir, tt.Texts())
	}
	apply(tt, a, func() { a.Choose(other) })
	apply(tt, a, func() { a.Choose(dir) })
	if !a.lock.dialog.open || a.snap().LibraryDir != other {
		t.Fatal("the library opened again without its password")
	}
}

// A library that was open and locked when the app quit starts behind the lock screen, which shows nothing of its films and opens another library; its password shows them, and Lock Now locks it again.
func TestLockStartsLockedAndLocksNow(t *testing.T) {
	a, tt, updates, dir, other, shelfPath := lockLibrary(t)
	m := testlocale.Messages()
	lockSetPassword(t, a, tt, updates)
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}

	// The next launch reads the same library.json and settings.json.
	b, bt, bupdates := newTestApp(t, &fakePipeline{})
	b.settingsPath = a.settingsPath
	b.restoreSettings(b.loadSettings())
	b.openLibrary(shelfPath)
	b.refreshLibrary()
	refresh(bt, b)
	if b.snap().LibraryDir != dir || !b.lockShut() || !bt.Exists("id:lock-screen") || !hasText(bt, m.LockScreenTitle("Private")) {
		t.Fatalf("the restart did not start locked: open %s, texts %q", b.snap().LibraryDir, bt.Texts())
	}
	if lockShows(bt) || b.FilmCount() != 0 || len(b.QueueFilms()) != 0 {
		t.Fatalf("the locked library shows its film: texts %q", bt.Texts())
	}
	if !bt.Find("id:lock-current").Input.Masked {
		t.Fatal("the lock screen's field shows what is typed")
	}
	bt.Type("id:lock-current", "nope")
	bt.Press("id:lock-screen-unlock")
	waitUntil(t, bt, b, bupdates, func() bool { return b.lock.screen.waiting == 0 })
	if !hasText(bt, m.LockWrong()) || !b.lockShut() {
		t.Fatal("a wrong password opened the library")
	}
	bt.Type("id:lock-current", lockPassword)
	bt.Press("id:lock-screen-unlock")
	waitUntil(t, bt, b, bupdates, func() bool { return !b.lockShut() })
	if bt.Exists("id:lock-screen") || !lockShows(bt) {
		t.Fatalf("the password did not show the library: texts %q", bt.Texts())
	}

	bt.Press("id:sidebar-switcher")
	bt.ChoosePopup(m.LockNow())
	if !b.lockShut() || !bt.Exists("id:lock-screen") || lockShows(bt) {
		t.Fatal("Lock Now did not lock the library")
	}
	bt.Press("id:lock-screen-others")
	var labels []string
	for _, it := range bt.PopupMenu {
		labels = append(labels, it.Label)
	}
	if want := []string{"Public", "", m.SidebarNewLibrary(), m.SidebarOpenLibrary()}; !slices.Equal(labels, want) {
		t.Fatalf("the lock screen's other libraries %q, want %q", labels, want)
	}
	bt.ChoosePopup("Public")
	if b.snap().LibraryDir != other || bt.Exists("id:lock-screen") || !b.LibraryLocked(dir) {
		t.Fatalf("opening another library from the lock screen: open %s", b.snap().LibraryDir)
	}
}

// While its library is locked, none of a film's title, poster or count shows anywhere: the switcher's counts, the palette, the saved lines, the system's notifications, the Dock's badge and Now Playing, the built-in browser's buttons, and the control socket the command line and the MCP server read, which refuses the films, the jobs, opening and adding.
func TestLockedLibraryLeaksNothing(t *testing.T) {
	a, tt, updates, dir, other, _ := lockLibrary(t)
	// The queue's job downloads until it is stopped, so it is still at work in the locked library once another opens.
	a.pipeline = &fakePipeline{block: true}
	lockSetPassword(t, a, tt, updates)
	// A job of the locked library waits in the queue, and its film is playing, shown where the system shows what plays.
	queued := core.Job{ID: "waiting", Title: "Private Waiting", Source: "https://www.youtube.com/watch?v=privwait", Stage: core.StageQueued, LibraryDir: dir}
	a.library.Jobs = append(a.library.Jobs, queued)
	refresh(tt, a)
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	item, ok := films.Find(a.films(), "secret")
	if !ok {
		t.Fatal("the film is not on the unlocked shelf")
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 5*time.Second)
	if rec.last(t).kind != "set" {
		t.Fatalf("the film does not show where the system shows what plays: %+v", rec.events)
	}
	var notified []string
	var badge []int
	a.queueUseDesktop(queueDesktop{
		active: func() bool { return false },
		notify: func(id, title, body string) { notified = append(notified, title+" "+body) },
		badge:  func(n int) { badge = append(badge, n) },
	})
	if badge[len(badge)-1] != 1 {
		t.Fatalf("the unlocked library's badge %v", badge)
	}

	apply(tt, a, a.LockNow)
	if a.openKey != "" || a.player.play != nil || rec.last(t).kind != "clear" {
		t.Fatalf("Lock Now left the film open: key %q, now playing %+v", a.openKey, rec.last(t))
	}
	if lockShows(tt) {
		t.Fatalf("the window shows the locked film: %q", tt.Texts())
	}
	a.queueTellSystem()
	if badge[len(badge)-1] != 0 {
		t.Fatalf("the badge counts the locked library's job: %v", badge)
	}
	a.queue.active = "secret"
	a.queueNotifyEnded(core.StageTranslating, a.library.Jobs[0])
	a.queue.active = ""
	if len(notified) != 0 {
		t.Fatalf("a notification told of the locked library's film: %q", notified)
	}
	a.palette.query = "Private"
	if found := a.paletteFilmsFound(); len(found) != 0 || len(a.paletteScope()) != 0 {
		t.Fatalf("the palette finds %+v", found)
	}
	a.palette.query = ""
	if len(a.savedBook().Lines) != 0 || len(a.browseHaves()) != 0 {
		t.Fatal("the saved lines or the browser's buttons read the locked library")
	}
	if _, ok := a.browseJob("https://www.youtube.com/watch?v=privatevid"); ok {
		t.Fatal("the browser's toolbar finds the locked library's film")
	}
	if err := a.Enqueue("https://www.youtube.com/watch?v=another"); err == nil || len(a.library.Jobs) != 2 {
		t.Fatalf("a job was added to the locked library: %v", err)
	}

	remoteServe(t, a)
	locked := remoteMessages().LockLibraryIsLocked("Private")
	for method, params := range map[string]any{
		remote.MethodFilms:   remote.FilmsParams{},
		remote.MethodFilm:    remote.IDParams{ID: "secret"},
		remote.MethodSearch:  remote.SearchParams{Query: "Hello"},
		remote.MethodJobs:    nil,
		remote.MethodJob:     remote.IDParams{ID: "waiting"},
		remote.MethodAdd:     remote.AddParams{Sources: []string{"https://www.youtube.com/watch?v=another"}},
		remote.MethodShow:    remote.ShowParams{ID: "secret"},
		remote.MethodRemove:  remote.IDParams{ID: "secret"},
		remote.MethodPlugins: nil,
	} {
		_, err := remoteCall[any](t, updates, method, params)
		remoteWantCode(t, err, remote.CodeNotReady)
		if !strings.Contains(err.Error(), locked) {
			t.Errorf("%s answered %v, want %q", method, err, locked)
		}
	}
	status, err := remoteCall[remote.Status](t, updates, remote.MethodStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Libraries) != 2 || !status.Libraries[0].Locked || status.Libraries[0].Films != 0 || status.Libraries[1].Locked || status.Queued != 0 {
		t.Fatalf("status %+v", status)
	}
	if len(a.library.Jobs) != 2 {
		t.Fatalf("the socket changed the locked library's jobs: %+v", a.library.Jobs)
	}

	// With another library open, the locked one's job the queue still runs is not in 处理中, nor in the socket's jobs, and the switcher does not count its films.
	apply(tt, a, func() {
		a.Choose(other)
		a.pump()
	})
	if a.queue.active != "waiting" {
		t.Fatalf("the queue runs %q", a.queue.active)
	}
	if slices.ContainsFunc(a.QueueFilms(), func(f films.Film) bool { return f.JobID == "waiting" }) || lockShows(tt) || tt.Exists(`text:Private Waiting`) {
		t.Fatalf("处理中 shows the locked library's job: %q", tt.Texts())
	}
	jobs, err := remoteCall[[]remote.Job](t, updates, remote.MethodJobs, nil)
	if err != nil || slices.ContainsFunc(jobs, func(j remote.Job) bool { return j.ID == "waiting" || j.ID == "secret" }) {
		t.Fatalf("jobs.list %+v, %v", jobs, err)
	}
	_, err = remoteCall[remote.Job](t, updates, remote.MethodJob, remote.IDParams{ID: "waiting"})
	remoteWantCode(t, err, remote.CodeNotFound)
	if _, ok := a.LibraryCount(dir); ok {
		t.Fatal("the switcher counts the locked library's films")
	}
	tt.Press("id:sidebar-switcher")
	for _, it := range tt.PopupMenu {
		if strings.HasPrefix(it.Label, "Private") && it.Label != testlocale.Messages().LockLibraryLocked("Private") {
			t.Fatalf("the switcher lists the locked library as %q", it.Label)
		}
	}
}

// Neither library.json nor settings.json ever holds the password, whatever the dialog did with it.
func TestLockKeepsNoPasswordOnDisk(t *testing.T) {
	a, tt, updates, dir, other, shelfPath := lockLibrary(t)
	m := testlocale.Messages()
	lockSetPassword(t, a, tt, updates)
	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuChange())
	lockDialogSubmit(t, a, tt, updates, "wrong-"+lockPassword, lockPassword+"-2", lockPassword+"-2")
	lockDialogSubmit(t, a, tt, updates, lockPassword, lockPassword+"-2", lockPassword+"-2")
	apply(tt, a, func() { a.Choose(other) })
	apply(tt, a, func() { a.Choose(dir) })
	lockDialogSubmit(t, a, tt, updates, lockPassword+"-2", "", "")
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{shelfPath, a.settingsPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(lockPassword)) || bytes.Contains(data, []byte("wrong-")) {
			t.Fatalf("%s holds the password:\n%s", filepath.Base(path), data)
		}
	}
	data, _ := os.ReadFile(shelfPath)
	if !bytes.Contains(data, []byte(`"locks"`)) || !bytes.Contains(data, []byte(`"iterations"`)) || !bytes.Contains(data, []byte(`"salt"`)) {
		t.Fatalf("library.json does not keep the key's parameters:\n%s", data)
	}
}

// A library renamed in Finder keeps its password under its new path, and stays unlocked when it was.
func TestLockFollowsARenamedLibrary(t *testing.T) {
	a, tt, updates, dir, _, shelfPath := lockLibrary(t)
	lockSetPassword(t, a, tt, updates)
	next := filepath.Join(filepath.Dir(dir), "Renamed.kando")
	if err := os.Rename(dir, next); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.renameLibraryPath(dir, next) })
	if !a.LibraryHasPassword(next) || a.LibraryHasPassword(dir) || a.LibraryLocked(next) {
		t.Fatalf("after the rename: password %t, locked %t", a.LibraryHasPassword(next), a.LibraryLocked(next))
	}
	if saved, _ := shelf.Load(shelfPath); !saved.Locks[filepath.Clean(next)].Matches(lockPassword) {
		t.Fatal("library.json did not follow the rename")
	}
	if got := sidebar.Title(a.snap().LibraryDir); got != "Renamed" {
		t.Fatalf("open library %q", got)
	}
}

// TestLockScreenFollowsTheLanguage checks that the lock screen says another interface language in place: its title, its field and its buttons change, and the field keeps the keyboard and what was typed in it.
func TestLockScreenFollowsTheLanguage(t *testing.T) {
	a, tt, updates, _, _, shelfPath := lockLibrary(t)
	lockSetPassword(t, a, tt, updates)
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	b, bt, _ := newTestApp(t, &fakePipeline{})
	b.settingsPath = a.settingsPath
	b.restoreSettings(b.loadSettings())
	b.openLibrary(shelfPath)
	b.refreshLibrary()
	refresh(bt, b)
	bt.Type("id:lock-current", "half")
	field := bt.Find("id:lock-current")
	b.cx.SetLocale("en")
	refresh(bt, b)
	en := locales.In("en")
	if got := bt.Find("id:lock-current"); got.ID != field.ID || !got.Focused || got.Text != "half" || got.Props.Label != en.LockFieldPassword() {
		t.Fatalf("in English the field is view %d (was %d), focused %v, holds %q, named %q", got.ID, field.ID, got.Focused, got.Text, got.Props.Label)
	}
	for _, want := range []string{en.LockScreenTitle("Private"), en.LockUnlockButton(), en.LockScreenOthers()} {
		if !hasText(bt, want) {
			t.Fatalf("the lock screen does not say %q in English: %q", want, bt.Texts())
		}
	}
	if got := bt.Find("id:lock-screen").Props.Label; got != en.LockScreenTitle("Private") {
		t.Fatalf("in English the lock screen is named %q", got)
	}
}

// Lock Now is a command of the File menu and of the palette while the open library has a password and is unlocked, with no key of its own; File › 切换资料库 marks a locked library.
func TestLockNowIsInTheMenuBarAndThePalette(t *testing.T) {
	a, tt, updates, dir, other, _ := lockLibrary(t)
	m := testlocale.Messages()
	inPalette := func() bool {
		return slices.ContainsFunc(a.paletteCommands(a.cx), func(c paletteCommand) bool { return c.label == m.LockNow() })
	}
	if it := findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupFile), m.LockNow()); !it.Disabled || it.Shortcut != "" || inPalette() {
		t.Fatalf("Lock Now applies to a library without a password: %+v", it)
	}
	lockSetPassword(t, a, tt, updates)
	it := findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupFile), m.LockNow())
	if it.Disabled || !inPalette() {
		t.Fatalf("Lock Now is not offered: %+v", it)
	}
	choose(tt, a, it)
	if !a.lockShut() || !tt.Exists("id:lock-screen") {
		t.Fatal("File › Lock Now did not lock the library")
	}
	apply(tt, a, func() { a.Choose(other) })
	var switchTo []string
	for _, m := range a.menuBar(a.cx) {
		for _, it := range m.Items {
			for _, sub := range it.Items {
				switchTo = append(switchTo, sub.Label)
			}
		}
	}
	if !slices.Contains(switchTo, m.LockLibraryLocked("Private")) || !a.LibraryLocked(dir) {
		t.Fatalf("切换资料库 lists %q", switchTo)
	}
}

// lockMoveFixture is newMoveFixture with 英语听力, the library a film moves to, locked with lockPassword.
func lockMoveFixture(t *testing.T) *moveFixture {
	t.Helper()
	f := newMoveFixture(t)
	l, err := shelf.NewLock(lockPassword)
	if err != nil {
		t.Fatal(err)
	}
	f.a.library.SetLock(f.to, l)
	refresh(f.tt, f.a)
	return f
}

// Moving a film into a locked library asks for its password first, as switching to it does: the 移到资料库 submenu marks it, a wrong password moves nothing, and the right one moves the film without opening the library, which stays locked, so Undo asks again.
func TestLockMoveIntoALockedLibraryAsksForItsPassword(t *testing.T) {
	f := lockMoveFixture(t)
	a, tt, updates := f.a, f.tt, f.updates
	m := testlocale.Messages()
	tt.Press("id:film-more >> nth=0")
	tt.ChoosePopup(m.FilmsMoveToLibrary(), m.LockLibraryLocked("英语听力"))
	if !a.lock.dialog.open || !hasText(tt, m.LockUnlockTitle("英语听力")) {
		t.Fatalf("moving into the locked library did not ask for its password: %q", tt.Texts())
	}
	lockDialogSubmit(t, a, tt, updates, "wrong", "", "")
	if !hasText(tt, m.LockWrong()) {
		t.Fatal("a wrong password did not say so")
	}
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("a wrong password moved the film: %v", err)
	}
	lockDialogSubmit(t, a, tt, updates, lockPassword, "", "")
	if a.lock.dialog.open {
		t.Fatal("the right password left the dialog open")
	}
	if _, err := os.Stat(filepath.Join(f.to, "Talk", "video.mp4")); err != nil {
		t.Fatalf("the right password did not move the film: %v", err)
	}
	if a.snap().LibraryDir != f.from || !a.LibraryLocked(f.to) || a.lock.passed != "" {
		t.Fatalf("the move opened or unlocked the library: open %s, locked %t, passed %q", a.snap().LibraryDir, a.LibraryLocked(f.to), a.lock.passed)
	}
	tt.Press("id:notice-undo")
	if !a.lock.dialog.open {
		t.Fatal("Undo out of the locked library did not ask for its password")
	}
	lockDialogSubmit(t, a, tt, updates, lockPassword, "", "")
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("Undo with the password did not bring the film back: %v", err)
	}
}

// The move itself refuses a locked library, the one the film leaves or the one it goes to, whatever asks it; films.move, which kando move and kando_move_film call, answers not_ready with the words every other method says of a locked library.
func TestLockMoveRefusesALockedLibrary(t *testing.T) {
	f := lockMoveFixture(t)
	a := f.a
	item, ok := films.Find(a.films(), "film")
	if !ok {
		t.Fatal("the film is not on the shelf")
	}
	err := a.libraryMoveFilm(remoteMessages(), item, f.from, f.to, false, nil)
	remoteWantCode(t, remoteMoveErr(err), remote.CodeNotReady)
	if err.Error() != remoteMessages().LockLibraryIsLocked("英语听力") {
		t.Fatalf("moving into the locked library: %v", err)
	}
	l, _ := a.library.LockOf(f.to)
	a.library.SetLock(f.from, l)
	a.library.SetLock(f.to, shelf.Lock{})
	if err := a.libraryMoveFilm(remoteMessages(), item, f.from, f.to, false, nil); err == nil || err.Error() != remoteMessages().LockLibraryIsLocked("我的影片") {
		t.Fatalf("moving out of the locked library: %v", err)
	}
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("a refused move moved the film: %v", err)
	}

	// Over the socket: out of the open library while it is locked, and into a locked one.
	remoteServe(t, a)
	_, err = remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: "英语听力"})
	remoteWantCode(t, err, remote.CodeNotReady)
	if !strings.Contains(err.Error(), remoteMessages().LockLibraryIsLocked("我的影片")) {
		t.Fatalf("films.move out of the locked library: %v", err)
	}
	a.library.SetLock(f.from, shelf.Lock{})
	a.library.SetLock(f.to, l)
	_, err = remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: "英语听力"})
	remoteWantCode(t, err, remote.CodeNotReady)
	if !strings.Contains(err.Error(), remoteMessages().LockLibraryIsLocked("英语听力")) || a.lock.dialog.open {
		t.Fatalf("films.move into the locked library: %v, dialog %t", err, a.lock.dialog.open)
	}
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("the socket moved the film: %v", err)
	}
}
