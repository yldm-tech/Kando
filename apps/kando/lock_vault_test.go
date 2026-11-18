package main

import (
	"bytes"
	"os"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/shelf"
	"kando/apps/kando/vault"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// lockWaitLong applies background updates until done holds, for as long as encrypting a library may take: hdiutil makes, attaches and detaches images in seconds each.
func lockWaitLong(t *testing.T, tt *nsgotest.App, a *app, updates <-chan func(), what string, done func() bool) {
	t.Helper()
	timeout := time.After(2 * time.Minute)
	for !done() {
		select {
		case fn := <-updates:
			onEngine(a, func() {
				fn()
				a.Changed()
			})
		case <-time.After(100 * time.Millisecond):
			// lockSettle detaches an image after a change; one that waited for a retry runs from a timer.
			onEngine(a, a.Changed)
		case <-timeout:
			t.Fatalf("timed out waiting until %s", what)
		}
	}
	refresh(tt, a)
}

// lockFolder is what the library's folder holds, but Finder's own file.
func lockFolder(t *testing.T, dir string) []string {
	t.Helper()
	items, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, item := range items {
		if item.Name() != ".DS_Store" {
			out = append(out, item.Name())
		}
	}
	return out
}

// On macOS a library's password encrypts its files: setting it moves them into an encrypted image and keeps the library open; locking it leaves only the image in its folder, so the film cannot be opened from Finder, while the shelf keeps the film and the queue holds the library's jobs; its password opens it again, a wrong one does not; quitting detaches the image; and removing the password puts the files back. One test, as each image Kando makes takes seconds, more on CI runners.
func TestLockEncryptsTheLibraryOnDisk(t *testing.T) {
	if !vault.Supported() {
		t.Skip("only macOS encrypts a library's files")
	}
	a, tt, updates, dir, _, shelfPath := lockLibrary(t)
	lockEncrypts = vault.Supported
	t.Cleanup(a.lockVaultQuit)
	m := testlocale.Messages()
	job, _ := a.library.Find("secret")
	video := job.VideoPath
	picture, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}

	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuSet())
	if !hasText(tt, m.LockSetNoteEncrypted()) {
		t.Fatalf("the set dialog does not say the files are encrypted: %q", tt.Texts())
	}
	tt.Type("id:lock-new", lockPassword)
	tt.Type("id:lock-confirm", lockPassword)
	tt.Press("id:lock-submit")
	lockWaitLong(t, tt, a, updates, "the library is encrypted", func() bool { return a.lock.dialog.waiting == 0 })
	if a.lock.dialog.open || !vault.Mounted(dir) || a.LibraryLocked(dir) || !lockShows(tt) {
		t.Fatalf("after setting the password: dialog %t %+v, attached %t, locked %t, texts %q", a.lock.dialog.open, a.lock.dialog.lockTry, vault.Mounted(dir), a.LibraryLocked(dir), tt.Texts())
	}
	if got, err := os.ReadFile(video); err != nil || !bytes.Equal(got, picture) {
		t.Fatalf("the film reads %d bytes, %v, once encrypted and open", len(got), err)
	}

	apply(tt, a, a.LockNow)
	lockWaitLong(t, tt, a, updates, "the image is detached", func() bool { return !vault.Mounted(dir) && len(a.lock.vault.closing) == 0 })
	if got := lockFolder(t, dir); !slices.Equal(got, []string{vault.ImageName}) {
		t.Fatalf("the locked library's folder holds %v, want only the image", got)
	}
	if _, err := os.Stat(video); err == nil {
		t.Fatal("the film can be opened from its folder while the library is locked")
	}
	// The shelf reads the folder again: the film is not gone, only locked away.
	apply(tt, a, a.refreshLibrary)
	if _, ok := a.library.Find("secret"); !ok {
		t.Fatal("the locked library's film was dropped from the shelf")
	}
	if saved, err := shelf.Load(shelfPath); err != nil || !slices.ContainsFunc(saved.Jobs, func(j core.Job) bool { return j.ID == "secret" }) {
		t.Fatalf("library.json lost the locked library's film: %v", err)
	}
	if !a.lockShut() || lockShows(tt) {
		t.Fatal("the library shows its film while locked")
	}
	if a.queueCanRun(core.Job{ID: "waiting", Stage: core.StageQueued, LibraryDir: dir}) {
		t.Fatal("a job may start in the locked library, whose folder holds only its image")
	}

	tt.Type("id:lock-current", "wrong horse")
	tt.Press("id:lock-screen-unlock")
	lockWaitLong(t, tt, a, updates, "the wrong password is checked", func() bool { return a.lock.screen.waiting == 0 })
	if !hasText(tt, m.LockWrong()) || vault.Mounted(dir) {
		t.Fatal("a wrong password opened the encrypted library")
	}
	tt.Type("id:lock-current", lockPassword)
	tt.Press("id:lock-screen-unlock")
	lockWaitLong(t, tt, a, updates, "the library is unlocked", func() bool { return !a.lockShut() })
	if !vault.Mounted(dir) || !lockShows(tt) {
		t.Fatalf("the password did not open the library: attached %t, texts %q", vault.Mounted(dir), tt.Texts())
	}
	if got, err := os.ReadFile(video); err != nil || !bytes.Equal(got, picture) {
		t.Fatalf("the film reads %d bytes, %v, once unlocked", len(got), err)
	}

	// Quitting detaches the image of the open library, so its files are not left readable after Kando is gone.
	a.lockVaultQuit()
	if vault.Mounted(dir) {
		t.Fatal("the library's image is still attached after quitting")
	}
	if got := lockFolder(t, dir); !slices.Equal(got, []string{vault.ImageName}) {
		t.Fatalf("the folder holds %v after quitting, want only the image", got)
	}

	// Removing the password moves the files back out of the locked image.
	tt.Press("id:sidebar-switcher")
	tt.ChoosePopup(m.LockMenuRemove())
	tt.Type("id:lock-current", lockPassword)
	tt.Press("id:lock-submit")
	lockWaitLong(t, tt, a, updates, "the password is removed", func() bool { return a.lock.dialog.waiting == 0 })
	if a.lock.dialog.open || a.LibraryHasPassword(dir) || vault.Has(dir) || !lockShows(tt) {
		t.Fatalf("after removing the password: dialog %t %+v, password %t, texts %q", a.lock.dialog.open, a.lock.dialog.lockTry, a.LibraryHasPassword(dir), tt.Texts())
	}
	if got := lockFolder(t, dir); slices.Contains(got, vault.ImageName) || !slices.Contains(got, lockFilm) {
		t.Fatalf("the folder holds %v after the password was removed", got)
	}
	if got, err := os.ReadFile(video); err != nil || !bytes.Equal(got, picture) {
		t.Fatalf("the film reads %d bytes, %v, once decrypted", len(got), err)
	}
}
