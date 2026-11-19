package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The reasons a script plugin was turned off by Kando rather than by the user (Health.Reason): it failed as it loaded (init), too many of its calls failed (failed), it kept the window from answering or ran past the time a call may take (slow), it held more memory than a plugin may (memory), a call of it was still running when Kando last stopped, so it may have hung or crashed it (hung), or its signer or package was revoked (revoked).
const (
	ReasonInit    = "init"
	ReasonFailed  = "failed"
	ReasonSlow    = "slow"
	ReasonMemory  = "memory"
	ReasonHung    = "hung"
	ReasonRevoked = "revoked"
)

// healthName is health.json's name in the plugins folder, inFlightDir the folder of the in-flight marks.
const (
	healthName  = "health.json"
	inFlightDir = ".inflight"
)

// Health is why Kando turned the script plugin ID off for its content Hash, and when. It is kept in health.json, the reason alone, never what the plugin did; a new content of the plugin has none.
type Health struct {
	ID     string    `json:"id"`
	Hash   string    `json:"hash"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// LoadHealth reads the health.json of root: none when there is no file. A file that cannot be read as JSON is an error, and reads as none.
func LoadHealth(root string) ([]Health, error) {
	data, err := os.ReadFile(filepath.Join(root, healthName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Health
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveHealth writes hs to the health.json of root, through a file renamed over it.
func SaveHealth(root string, hs []Health) error {
	if hs == nil {
		hs = []Health{}
	}
	data, err := json.MarshalIndent(hs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, healthName), append(data, '\n'))
}

// SetHealth is hs with h in place of the entry of h's id, or without the id's entry when h.Reason is "".
func SetHealth(hs []Health, h Health) []Health {
	out := slices.DeleteFunc(slices.Clone(hs), func(o Health) bool { return o.ID == h.ID })
	if h.Reason != "" {
		out = append(out, h)
	}
	return out
}

// HealthOf is the entry of hs for the plugin id's content hash, and whether it has one.
func HealthOf(hs []Health, id, hash string) (Health, bool) {
	i := slices.IndexFunc(hs, func(h Health) bool { return h.ID == id && h.Hash == hash })
	if i < 0 {
		return Health{}, false
	}
	return hs[i], true
}

// MarkInFlight marks the script plugin id's content hash as running a call, in a file of root's .inflight folder named after it, until ClearInFlight. A mark still there when Kando starts again (InFlight) is a call that hung or crashed Kando. It is written through a file renamed into place but not flushed to disk, which takes a tenth of a millisecond where a flush can take tens (docs/plugin-system.md 5.2): the system writes it out even when Kando is killed or crashes; only a power cut loses it, which loses one chance to turn the plugin off.
func MarkInFlight(root, id, hash string) error {
	dir := filepath.Join(root, inFlightDir)
	if !validID(id) {
		return ErrBadID
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".mark-*")
	if err != nil {
		return err
	}
	_, err = tmp.WriteString(hash)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, id))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// ClearInFlight removes the in-flight mark of the plugin id.
func ClearInFlight(root, id string) error {
	if !validID(id) {
		return ErrBadID
	}
	err := os.Remove(filepath.Join(root, inFlightDir, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// InFlight is the marks left in root: the plugins whose call was running when Kando last stopped, by id, with the content hash each was for. It removes them as it reads them, so each is reported once.
func InFlight(root string) map[string]string {
	dir := filepath.Join(root, inFlightDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		if strings.HasPrefix(name, ".") || !validID(name) {
			os.Remove(path)
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			out[name] = strings.TrimSpace(string(data))
		}
		os.Remove(path)
	}
	return out
}

// MergeHealth is hs with each entry of more whose id hs has none of.
func MergeHealth(hs, more []Health) []Health {
	out := slices.Clone(hs)
	for _, h := range more {
		if !slices.ContainsFunc(out, func(o Health) bool { return o.ID == h.ID }) {
			out = append(out, h)
		}
	}
	return out
}
