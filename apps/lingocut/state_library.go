package main

import "github.com/egoist/mygo/ui"

// libraryUI is the state of the library page that is not the shelf itself (the shelf is the nodra session in state.go).
type libraryUI struct {
	// posters are the cards' frames by video path, posterMiss the videos that have none and posterBusy those being grabbed; posterDir is where frames of videos outside a folder of their own are kept (library_posters.go).
	posterDir  string
	posters    map[string]ui.ImageSource
	posterMiss map[string]bool
	posterBusy map[string]bool
}

func newLibraryUI() libraryUI {
	return libraryUI{}
}

// savedLibrary is the library page's part of settings.json. The libraries themselves are the shelf's (savedShelf).
type savedLibrary struct{}

func (a *app) libraryHooks() areaHooks { return areaHooks{} }
