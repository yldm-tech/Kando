package main

import (
	"os"
	"path/filepath"

	"kando/packages/media"
)

// migrateLibrary moves the open library out of where earlier versions kept it: LingoCut's videos out of Vimpo's library, and the old plain folder into a .lingocut bundle. main runs it at startup, after the settings are restored; LINGOCUT_OUTPUT_DIR turns it off.
func (a *app) migrateLibrary() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.VimpoLibraryDir()) {
		bundle := media.DefaultLibraryDir()
		moved, err := media.DetachOwnVideos(a.snap().LibraryDir, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.Save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "资料库无法搬出：" + err.Error()
		} else {
			libs := dropLibrary(a.snap().Libraries, media.VimpoLibraryDir())
			libs = addLibrary(libs, bundle)
			a.edit(func(s *session) {
				s.Libraries = libs
				s.LibraryDir = bundle
			})
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.LegacyLibraryDir()) {
		from := a.snap().LibraryDir
		bundle := media.DefaultLibraryDir()
		moved, err := media.AdoptLibrary(from, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.Save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "影视库无法整理：" + err.Error()
		} else {
			media.RemoveEmptyLibrary(from)
			libs := addLibrary(a.snap().Libraries, bundle)
			a.edit(func(s *session) {
				s.Libraries = libs
				s.LibraryDir = bundle
			})
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
}
