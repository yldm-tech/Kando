package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// valuesDir is the folder of the plugins folder where the values of script plugins' settings are kept, one file per plugin, outside its pack so they count in no hash and an update keeps them.
const valuesDir = ".settings"

// Values are the values the user set for a script plugin's settings, by key; a setting the user never set is not in them.
type Values map[string]any

// valuesPath is the file of the plugin id's values, or ErrBadID for an id that is not one.
func valuesPath(root, id string) (string, error) {
	if !validID(id) {
		return "", ErrBadID
	}
	return filepath.Join(root, valuesDir, id+".json"), nil
}

// LoadValues reads the values of the plugin id's settings: none when it has no file, and none of a file that cannot be read as JSON, which the next SaveValues replaces.
func LoadValues(root, id string) (Values, error) {
	path, err := valuesPath(root, id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Values{}, nil
	}
	if err != nil {
		return Values{}, err
	}
	var v Values
	if json.Unmarshal(data, &v) != nil || v == nil {
		return Values{}, nil
	}
	return v, nil
}

// SaveValues writes the values of the plugin id's settings, through a file renamed over its file.
func SaveValues(root, id string, v Values) error {
	path, err := valuesPath(root, id)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// RemoveValues removes the values of the plugin id's settings, as it is uninstalled.
func RemoveValues(root, id string) error {
	path, err := valuesPath(root, id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Resolve is the value of each of the settings of m but its secrets, by key: the one in v when it is a value of the setting (Setting.Value), else its Zero. It is what kando.settings.get answers.
func (m Manifest) Resolve(v Values) map[string]any {
	out := map[string]any{}
	for _, s := range m.Settings {
		if s.Type == SettingSecret {
			continue
		}
		val, ok := s.Value(v[s.Key])
		if !ok {
			val = s.Zero()
		}
		out[s.Key] = val
	}
	return out
}

// Clone is a copy of v to change.
func (v Values) Clone() Values {
	out := Values{}
	for k, x := range v {
		out[k] = x
	}
	return out
}
