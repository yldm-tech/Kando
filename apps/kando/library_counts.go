package main

import (
	"path/filepath"
	"slices"

	"kando/apps/kando/films"
	"kando/packages/core"
)

// countLibraries reads, off the UI thread, how many films each known library that is not open holds, as its shelf would list them, and keeps them in the sidebar's state for the library switcher's menu, unless a later run started meanwhile. The open library's count is the shelf's own.
func (a *app) countLibraries() {
	if a.update == nil {
		return
	}
	open := filepath.Clean(a.snap().LibraryDir)
	var dirs []string
	for _, dir := range a.knownLibraries() {
		// A locked library is not counted: its menu item shows its name alone.
		if filepath.Clean(dir) != open && !a.LibraryLocked(dir) {
			dirs = append(dirs, filepath.Clean(dir))
		}
	}
	a.side.counting++
	run := a.side.counting
	if len(dirs) == 0 {
		return
	}
	jobs, added := slices.Clone(a.library.Jobs), a.library.AddedCopy()
	update := a.update
	go func() {
		counts := make(map[string]int, len(dirs))
		noName := func(core.Job) string { return "" }
		for _, dir := range dirs {
			videos, err := films.Videos(dir, jobs, nil, added[dir])
			if err != nil {
				continue
			}
			counts[dir] = len(films.List(dir, jobs, videos, nil, noName, noName))
		}
		update(func() {
			if run != a.side.counting {
				return
			}
			a.side.libraryCounts = counts
		})
	}()
}
