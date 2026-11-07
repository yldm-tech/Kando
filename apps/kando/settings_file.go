package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"kando/apps/kando/film"
)

// settings.json: what saveSettings writes and loadSavedSettings reads.

func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.settingsPath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.settingsFile(), "", "  ")
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

// settingsFile is the settings as saveSettings writes them.
func (a *app) settingsFile() savedSettings {
	var saved savedSettings
	saved.savedShelf = a.saveShelf()
	a.areasSave(&saved)
	return saved
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

// settingsFound is what the app found at settingsPath when it started (loadSettings).
type settingsFound int

const (
	// settingsUnread is an app that did not read settings.json, as in tests that restore settings of their own.
	settingsUnread settingsFound = iota
	// settingsNone is no settings.json at all: Kando has not run with this home before. Only then does the first-run guide open by itself and make a library (state_onboarding.go).
	settingsNone
	settingsRead
	// settingsDamaged is a settings.json that could not be read in full: one cut short, or with a value of another type, as another version may write. What could be read is kept, and the file is copied to settings.json.bad before anything writes over it.
	settingsDamaged
)

// loadSettings reads settings.json at startup, as main does, and records what it found in a.settingsFound. A file it cannot read in full is copied beside itself as settings.json.bad, and the sidebar says so, so a save that follows does not lose what the file held.
func (a *app) loadSettings() savedSettings {
	saved, found, data, readErr := readSavedSettings(a.settingsPath)
	a.settingsFound = found
	if found == settingsDamaged && data == nil {
		a.saveErr = "设置文件无法读取：" + readErr.Error()
	} else if found == settingsDamaged {
		bad := a.settingsPath + ".bad"
		if err := os.WriteFile(bad, data, 0o600); err != nil {
			a.saveErr = "设置文件有读不懂的内容，也无法另存：" + err.Error()
		} else {
			a.saveErr = "设置文件有读不懂的内容，原文件已另存为 settings.json.bad"
		}
	}
	return saved
}

func loadSavedSettings(path string) savedSettings {
	saved, _, _, _ := readSavedSettings(path)
	return saved
}

// readSavedSettings reads settings.json at path and says what it found, with the file's bytes and the error when it is damaged; a file that is there but cannot be read has no bytes. A value of another type leaves only that field at its default, as encoding/json goes on with the rest; a file that is not JSON at all reads as the defaults.
func readSavedSettings(path string) (savedSettings, settingsFound, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return savedSettings{}, settingsNone, nil, nil
	}
	if err != nil {
		return savedSettings{}, settingsDamaged, nil, err
	}
	// A saved look is read over the default, so a field it lacks, such as one added after it was saved, keeps the default.
	look := film.DefaultLook()
	saved := savedSettings{savedPlayer: savedPlayer{Look: &look}}
	if err := json.Unmarshal(data, &saved); err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			saved = savedSettings{}
		}
		saved.clean()
		return saved, settingsDamaged, data, err
	}
	saved.clean()
	return saved, settingsRead, nil, nil
}

// savedSettings is settings.json. Each part is one area's, declared in that area's file next to the code that saves and restores it (areaHooks.save and restore), and encoding/json writes the parts' fields side by side, as one object. An area adds a setting to its own part, not here; two parts must not use the same key (TestSavedSettingsKeysAreUnique), and no part may have a MarshalJSON, UnmarshalJSON, MarshalText or UnmarshalText of its own, because Go promotes it to savedSettings and it would then encode the whole file (TestSavedSettingsPartsHaveNoMarshalers).
type savedSettings struct {
	savedShelf      // shelf_app.go
	savedSidebar    // state_sidebar.go
	savedLibrary    // state_library.go
	savedQueue      // queue.go
	savedSpeech     // settings_speech.go
	savedLLM        // settings_ai.go
	savedTranslate  // state_settings.go
	savedCloud      // state_cloud.go
	savedPlayer     // state_player.go
	savedOverlay    // state_overlay.go
	savedMenu       // state_menu.go
	savedOnboarding // state_onboarding.go
}

// clean tidies what was read. Every part has a clean of its own, in its own file, which trims its text or does nothing, and each is called here, so an area changes only its own clean.
func (s *savedSettings) clean() {
	s.savedShelf.clean()
	s.savedSidebar.clean()
	s.savedLibrary.clean()
	s.savedQueue.clean()
	s.savedSpeech.clean()
	s.savedLLM.clean()
	s.savedTranslate.clean()
	s.savedCloud.clean()
	s.savedPlayer.clean()
	s.savedOverlay.clean()
	s.savedMenu.clean()
	s.savedOnboarding.clean()
}
