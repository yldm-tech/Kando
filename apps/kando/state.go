package main

import (
	"maps"
	"slices"

	"kando/apps/kando/locales"
	"kando/packages/media"

	"github.com/yldm-tech/nodra"
)

// Track and Changed make the app the live.State every area's App interface embeds: a binding reads the app's state through live.Func, and whatever changes it on the engine's goroutine says so, a handler through live.Do or live.On, a background job through update (appUpdate). Both do nothing before run gives the revision its graph.
func (a *app) Track()   { a.rev.Track() }
func (a *app) Changed() { a.rev.Changed() }

// appUpdate is the app's update: it runs fn on the engine's goroutine, then what the areas do after each report (areasAfterUpdate), then lets every binding follow. Any goroutine may call it; before the engine starts, the report waits for it.
func (a *app) appUpdate(fn func()) {
	a.rev.Post(func() {
		fn()
		a.areasAfterUpdate()
	})
}

// messages says Kando's words in the interface's language, for what is said outside a scope and kept as it was said: a message stored in the app's state, such as the library's message bar (saveErr, warn), a notification, or a name written to disk. Kando always runs in its interface language (SetInterfaceLanguage), so these are the words the windows show. Words a component shows come from locales.Xxx(cx) in its bindings instead, which follow the language as it changes.
func (a *app) messages() locales.Messages { return locales.In(a.prefs.interfaceLanguage) }

// session is the library shelf the window draws. Paths and the inode of each folder stay in this one value, so a rename updates both together. Nodra publishes a snapshot when it changes, and the revision (run's nodra.InvalidateTarget) lets the bindings read it again.
type session struct {
	LibraryDir    string
	Libraries     []string
	LibraryName   string
	LibraryParent string
	MakingLibrary bool
	Videos        []media.Video
	Inodes        map[string]media.DirID
	// Renamed is the libraries restoreShelf followed to a new path before library.json was read, oldest first. openLibrary points the jobs, and what is kept by video path, at the new paths once it has read the shelf, then empties it.
	Renamed []libraryRename
}

// newSessionStore is the store of a new app, its library named in the interface's language Kando starts in, which newApp's settings default to, before restoreSettings reads the saved one.
func newSessionStore() *nodra.Store[session] {
	return nodra.NewWithClone(session{LibraryName: locales.In(locales.Default).LibraryDefaultName()}, cloneSession)
}

func cloneSession(s session) session {
	s.Libraries = slices.Clone(s.Libraries)
	s.Videos = slices.Clone(s.Videos)
	s.Inodes = maps.Clone(s.Inodes)
	s.Renamed = slices.Clone(s.Renamed)
	return s
}

func (a *app) snap() session {
	if a.store == nil {
		return session{LibraryName: a.messages().LibraryDefaultName()}
	}
	return a.store.Get()
}

func (a *app) edit(fn func(*session)) {
	if a.store == nil || fn == nil {
		return
	}
	_ = a.store.Update(fn)
}
