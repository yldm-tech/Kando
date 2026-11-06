package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"lingocut/apps/lingocut/film"
)

// settings.json: what saveSettings writes and loadSavedSettings reads.

func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.settingsPath), 0o700); err != nil {
		return err
	}
	var saved savedSettings
	saved.savedShelf = a.saveShelf()
	a.areasSave(&saved)
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := a.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.settingsPath)
}

// restoreSettings takes back what settings.json holds at startup: the shelf first, then each area's part (areaHooks.restore). A library renamed while the app was closed is saved under its new path only after that, because saveSettings writes every area's state and, before the areas restore theirs, that state is the defaults.
func (a *app) restoreSettings(saved savedSettings) {
	moved := a.restoreShelf(saved)
	a.areasRestore(saved)
	if moved {
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
	}
}

func loadSavedSettings(path string) savedSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		return savedSettings{}
	}
	// A saved look is read over the default, so a field it lacks, such as one added after it was saved, keeps the default.
	look := film.DefaultLook()
	saved := savedSettings{savedPlayer: savedPlayer{Look: &look}}
	if json.Unmarshal(data, &saved) != nil {
		return savedSettings{}
	}
	saved.clean()
	return saved
}

// savedSettings is settings.json. Each part is one area's, declared in that area's file next to the code that saves and restores it (areaHooks.save and restore), and encoding/json writes the parts' fields side by side, as one object. An area adds a setting to its own part, not here; two parts must not use the same key (TestSavedSettingsKeysAreUnique).
type savedSettings struct {
	savedShelf     // shelf_app.go
	savedSidebar   // state_sidebar.go
	savedLibrary   // state_library.go
	savedQueue     // queue.go
	savedSpeech    // settings_speech.go
	savedLLM       // settings_ai.go
	savedTranslate // state_settings.go
	savedCloud     // state_cloud.go
	savedPlayer    // state_player.go
	savedOverlay   // state_overlay.go
	savedMenu      // state_menu.go
}

// clean trims what was read. A part whose text needs trimming has a clean of its own, called here.
func (s *savedSettings) clean() {
	s.savedShelf.clean()
	s.savedSpeech.clean()
	s.savedLLM.clean()
	s.savedTranslate.clean()
	s.savedCloud.clean()
}
