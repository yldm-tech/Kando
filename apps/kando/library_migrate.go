package main

import (
	"os"
	"path/filepath"

	"kando/packages/media"
)

// migrateLibrary moves the open library out of where earlier versions kept it: Kando's videos out of Vimpo's library into a .kando bundle. main runs it at startup, after the settings are restored; KANDO_OUTPUT_DIR turns it off.
func (a *app) migrateLibrary() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if os.Getenv("KANDO_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.VimpoLibraryDir()) {
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
}
