package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
)

// settings.json: what saveSettings writes and loadSavedSettings reads.

func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	data, err := a.settingsData()
	if err != nil {
		return err
	}
	return a.writeSettings(data)
}

// saveSettingsChanged saves the settings when they differ from what settings.json held when the app read it or last wrote it, as the app does when it quits, for what changed without a save of its own, such as where the windows are. It never writes over a settings.json the app could neither read nor copy aside at startup (settingsUnsaved): the user's libraries, keys and sign-ins in it would be lost to the defaults.
func (a *app) saveSettingsChanged() error {
	if a.settingsPath == "" || a.settingsUnsaved {
		return nil
	}
	data, err := a.settingsData()
	if err != nil || bytes.Equal(data, a.settingsKept) {
		return err
	}
	return a.writeSettings(data)
}

// settingsData is settings.json as saveSettings writes it.
func (a *app) settingsData() ([]byte, error) {
	data, err := json.MarshalIndent(a.settingsFile(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// writeSettings writes data to settings.json through a file beside it, and remembers it as what the file holds. A settings.json written in full no longer holds anything a write could lose, so writes need no longer wait for one the user asked for (settingsUnsaved).
func (a *app) writeSettings(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(a.settingsPath), 0o700); err != nil {
		return err
	}
	tmp := a.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, a.settingsPath); err != nil {
		return err
	}
	a.settingsKept, a.settingsUnsaved = data, false
	return nil
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
	// What the file holds now, as the app would write it: a quit saves only what changed since (saveSettingsChanged).
	a.settingsKept, _ = a.settingsData()
	if moved {
		if err := a.saveSettings(); err != nil {
			a.saveErr = a.messages().SettingsLibrarySaveFailed(err.Error())
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
	// The words are kept in the app's state, in the interface language the file names when what could be read names one, as the areas have not restored theirs yet.
	msgs := a.messages()
	if locales.Supported(saved.InterfaceLanguage) {
		msgs = locales.In(saved.InterfaceLanguage)
	}
	if found == settingsDamaged && data == nil {
		a.saveErr = msgs.SettingsFileUnreadable(readErr.Error())
		a.settingsUnsaved = true
	} else if found == settingsDamaged {
		bad := a.settingsPath + ".bad"
		if err := os.WriteFile(bad, data, 0o600); err != nil {
			a.saveErr = msgs.SettingsFileDamagedUnsaved(err.Error())
			a.settingsUnsaved = true
		} else {
			a.saveErr = msgs.SettingsFileDamaged()
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
	savedInterface  // state_settings.go
	savedSpeech     // settings_speech.go
	savedLLM        // settings_ai.go
	savedTranslate  // state_settings.go
	savedCloud      // state_cloud.go
	savedPlayer     // state_player.go
	savedOverlay    // state_overlay.go
	savedMenu       // state_menu.go
	savedOnboarding // state_onboarding.go
	savedWindow     // state_window.go
}

// clean tidies what was read. Every part has a clean of its own, in its own file, which trims its text or does nothing, and each is called here, so an area changes only its own clean.
func (s *savedSettings) clean() {
	s.savedShelf.clean()
	s.savedSidebar.clean()
	s.savedLibrary.clean()
	s.savedQueue.clean()
	s.savedInterface.clean()
	s.savedSpeech.clean()
	s.savedLLM.clean()
	s.savedTranslate.clean()
	s.savedCloud.clean()
	s.savedPlayer.clean()
	s.savedOverlay.clean()
	s.savedMenu.clean()
	s.savedOnboarding.clean()
	s.savedWindow.clean()
}
