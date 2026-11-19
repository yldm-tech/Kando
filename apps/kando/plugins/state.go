package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// StateFormat is the newest format of state.json this version reads and writes. It goes up only when an older Kando must not read the file as its own: format 2 holds the grants of script plugins (PluginState.Grant, Signer, OldHash), which a Kando of format 1 would drop as it saved the file. Save writes format 1 while no entry has any of them, so a Kando that never ran a script plugin leaves a file older versions still read (docs/plugin-system.md 5.1).
const StateFormat = 2

// grantsFormat is the format whose entries may hold grants; formats before it have only id, hash and enabled.
const grantsFormat = 2

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

// PluginState is one pack's entry of State. Sites are the domains of a site pack the user allowed, each on its own (docs/plugin-system.md 6.2); a file written before them has none, which allows no domain. A script plugin's also says who signed the content it is for (Signer, the release key's ID), what the user granted it (Grant), and, after an update that asks for more than the version before, that version's hash (OldHash), which tells "updated, look at the grant again" from "never granted". Budget is the most tokens of the user's AI service a task pack may spend a day, DefaultBudget when 0; other kinds have none.
type PluginState struct {
	ID      string   `json:"id"`
	Hash    string   `json:"hash"`
	Enabled bool     `json:"enabled"`
	Sites   []string `json:"sites,omitempty"`
	Budget  int      `json:"budget,omitempty"`
	OldHash string   `json:"oldHash,omitempty"`
	Signer  string   `json:"signer,omitempty"`
	Grant   Grant    `json:"grant,omitzero"`
}

// scripted reports whether e holds what only format 2 has: the sites of a site pack are format 1's.
func (e PluginState) scripted() bool {
	return e.OldHash != "" || e.Signer != "" || len(e.Grant.Perms) > 0 || len(e.Grant.Network) > 0 || len(e.Grant.Credentials) > 0
}

// DefaultBudget is a task pack's daily budget of tokens until the user sets another (docs/plugin-system.md 6.2); MinBudget and MaxBudget bound what the user may set.
const (
	DefaultBudget = 50000
	MinBudget     = 1000
	MaxBudget     = 1000000
)

// optIn reports whether packs of kind k are off until the user turns them on: a task pack spends the user's tokens, so installing it is not agreeing to that (docs/plugin-system.md 6.2).
func (k Kind) optIn() bool { return k == KindTask }

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

// Enabled reports whether the pack id is turned on: as its entry says, and when it has none, on, as a data pack is turned on once it is installed (docs/plugin-system.md 6.2). Every pack is off in a state that could not be read as this format. A script plugin is EnabledPack's.
func (s State) Enabled(id string) bool {
	if s.off {
		return false
	}
	if e, ok := s.entry(id); ok {
		return e.Enabled
	}
	return true
}

// Budget is the daily budget of tokens of the task pack id: its entry's, or DefaultBudget.
func (s State) Budget(id string) int {
	if e, _ := s.Entry(id); e.Budget > 0 {
		return e.Budget
	}
	return DefaultBudget
}

// EnabledPack reports whether the pack p is turned on: a data pack as Enabled says, but a kind the user turns on first (a task pack, which spends the user's tokens; docs/plugin-system.md 6.2) only when its entry turned it on; a script plugin only when its entry turned it on for this content and signer, as one is off until the user turns it on, and a content or signer the user never turned on is off too.
func (s State) EnabledPack(p Pack) bool {
	if p.Kind.optIn() {
		e, ok := s.entry(p.ID)
		return ok && !s.off && e.Enabled
	}
	if p.Kind != KindScript {
		return s.Enabled(p.ID)
	}
	e, ok := s.entry(p.ID)
	return ok && !s.off && e.Enabled && e.Hash == p.Hash && e.Signer == p.Signer
}

// Entry is the entry of the pack id, and whether it has one.
func (s State) Entry(id string) (PluginState, bool) { return s.entry(id) }

func (s State) entry(id string) (PluginState, bool) {
	i := slices.IndexFunc(s.Plugins, func(p PluginState) bool { return p.ID == id })
	if i < 0 {
		return PluginState{}, false
	}
	return s.Plugins[i], true
}

// Allowed reports whether the user allowed domain of the site pack id: its entry lists it. A pack without an entry allows none, unlike Enabled, as each domain needs its own consent; so does every pack in a state that could not be read as this format.
func (s State) Allowed(id, domain string) bool {
	if s.off {
		return false
	}
	e, ok := s.Entry(id)
	return ok && slices.Contains(e.Sites, domain)
}

// Set puts e in s in place of the entry of its id, or after the others.
func (s *State) Set(e PluginState) {
	if i := slices.IndexFunc(s.Plugins, func(o PluginState) bool { return o.ID == e.ID }); i >= 0 {
		s.Plugins[i] = e
		return
	}
	s.Plugins = append(s.Plugins, e)
}

// Save writes s to the state.json of root in the oldest format that holds it (StateFormat), with its revision one higher than both its own and that of the file on disk, through a file beside it renamed over it, so the file holds either the old state or all of the new one. It does not write over a newer format's state, which is ErrStateFormat: neither one s was read from nor one a newer Kando wrote since s was read, which Save reads the file again for. Saving a state read as every pack off (a bad state.json) writes only the entries it has, and every pack without one reads as on again afterwards: a writer that must keep the packs off, such as the app saving its first change after it found state.json damaged, first adds an entry turned off for each pack it has.
func (s *State) Save(root string) error {
	if s.Format > StateFormat {
		return fmt.Errorf("%w: format %d", ErrStateFormat, s.Format)
	}
	next := *s
	next.Format = 1
	if slices.ContainsFunc(next.Plugins, PluginState.scripted) {
		next.Format = grantsFormat
	}
	// The file as it is now: a newer Kando may have written it since s was read. One that cannot be read as a state is written over, as LoadState reads it as bad.
	if data, err := os.ReadFile(filepath.Join(root, stateName)); err == nil {
		var disk struct {
			Format   int `json:"format"`
			Revision int `json:"revision"`
		}
		if json.Unmarshal(data, &disk) == nil {
			if disk.Format > StateFormat {
				return fmt.Errorf("%w: format %d", ErrStateFormat, disk.Format)
			}
			next.Revision = max(next.Revision, disk.Revision)
		}
	}
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
