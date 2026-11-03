package main

import (
	"slices"

	"lingocut/packages/media"

	"github.com/yldm-tech/nodra"
)

// session is the library shelf the window draws. Nodra publishes a snapshot when it changes, and the window redraws from that snapshot.
type session struct {
	LibraryDir    string
	Libraries     []string
	LibraryName   string
	LibraryParent string
	MakingLibrary bool
	Videos        []media.Video
}

func newSessionStore() *nodra.Store[session] {
	return nodra.NewWithClone(session{LibraryName: "影视库"}, cloneSession)
}

func cloneSession(s session) session {
	s.Libraries = slices.Clone(s.Libraries)
	s.Videos = slices.Clone(s.Videos)
	return s
}

func (a *app) snap() session {
	if a.store == nil {
		return session{LibraryName: "影视库"}
	}
	return a.store.Get()
}

func (a *app) edit(fn func(*session)) {
	if a.store == nil || fn == nil {
		return
	}
	_ = a.store.Update(fn)
}
