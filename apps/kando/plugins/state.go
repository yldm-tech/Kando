package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StateFormat is the format of state.json this version reads and writes. It goes up only when an older Kando must not read the file as its own, as when the grants of script plugins are added to it (docs/plugin-system.md 5.1).
const StateFormat = 1

// stateName is the state file's name in the plugins folder.
const stateName = "state.json"

// State is the plugins folder's state.json: which packs the user turned off or on, by id, with the hash of the content they did it for. It is never synced and never in settings.json. The app is its single writer (Save); the command line only reads it. Revision goes up by one each time it is written.
type State struct {
	Format   int           `json:"format"`
	Revision int           `json:"revision"`
	Plugins  []PluginState `json:"plugins"`
	// off is set when the file could not be read as this format, and every pack then reads as turned off (Enabled) until it is written again (Save).
	off bool
}

// PluginState is one pack's entry of State.
type PluginState struct {
	ID      string `json:"id"`
	Hash    string `json:"hash"`
	Enabled bool   `json:"enabled"`
}

// LoadState reads the state.json of the plugins folder root. A missing file is the state of a folder where no pack was ever turned off. A file that cannot be read, is not JSON or has no format (format 0 or less, as a file cut short to {} or edited by hand) is ErrBadState, and one of a newer format ErrStateFormat; either way the state returned has every pack turned off, as such a file may say anything about them. A newer format's state is not written over (Save); a bad one is.
func LoadState(root string) (State, error) {
	data, err := os.ReadFile(filepath.Join(root, stateName))
	if errors.Is(err, os.ErrNotExist) {
		return State{Format: StateFormat}, nil
	}
	if err != nil {
		return State{Format: StateFormat, off: true}, fmt.Errorf("%w: %v", ErrBadState, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{Format: StateFormat, off: true}, fmt.Errorf("%w: %v", ErrBadState, err)
	}
	if s.Format < 1 {
		return State{Format: StateFormat, off: true}, fmt.Errorf("%w: format %d", ErrBadState, s.Format)
	}
	if s.Format > StateFormat {
		s.off = true
		for i := range s.Plugins {
			s.Plugins[i].Enabled = false
		}
		return s, fmt.Errorf("%w: format %d", ErrStateFormat, s.Format)
	}
	return s, nil
}

// Enabled reports whether the pack id is turned on: as its entry says, and when it has none, on, as a data pack is turned on once it is installed (docs/plugin-system.md 6.2). Every pack is off in a state that could not be read as this format.
func (s State) Enabled(id string) bool {
	if s.off {
		return false
	}
	for _, p := range s.Plugins {
		if p.ID == id {
			return p.Enabled
		}
	}
	return true
}

// Save writes s to the state.json of root in this format, with its revision one higher, through a file beside it renamed over it, so the file holds either the old state or all of the new one. It does not write over a newer format's state, which is ErrStateFormat. Saving a state read as every pack off (a bad state.json) writes only the entries it has, and every pack without one reads as on again afterwards: a writer that must keep the packs off, such as the app saving its first change after it found state.json damaged, first adds an entry turned off for each pack it has.
func (s *State) Save(root string) error {
	if s.Format > StateFormat {
		return fmt.Errorf("%w: format %d", ErrStateFormat, s.Format)
	}
	next := *s
	next.Format = StateFormat
	next.Revision++
	if next.Plugins == nil {
		next.Plugins = []PluginState{}
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(root, stateName), append(data, '\n')); err != nil {
		return err
	}
	next.off = false
	*s = next
	return nil
}

// writeFileAtomic writes data to path through a hidden file beside it, flushed to disk and then renamed over it.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
