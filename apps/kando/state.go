package main

import (
	"maps"
	"slices"

	"kando/packages/media"

	"github.com/yldm-tech/nodra"
)

// session is the library shelf the window draws. Paths and the inode of each folder stay in this one value, so a rename updates both together. Nodra publishes a snapshot when it changes, and the window redraws from that snapshot.
type session struct {
	LibraryDir    string
	Libraries     []string
	LibraryName   string
	LibraryParent string
	MakingLibrary bool
	Videos        []media.Video
	Inodes        map[string]media.DirID
}

func newSessionStore() *nodra.Store[session] {
	return nodra.NewWithClone(session{LibraryName: "影视库"}, cloneSession)
}

func cloneSession(s session) session {
	s.Libraries = slices.Clone(s.Libraries)
	s.Videos = slices.Clone(s.Videos)
	s.Inodes = maps.Clone(s.Inodes)
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
