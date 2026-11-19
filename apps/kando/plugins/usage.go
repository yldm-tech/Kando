package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// usageName is the file in the plugins folder that keeps what the task packs spent today.
const usageName = "usage.json"

// Usage is the tokens of the user's AI service each task pack spent on Day (a local date, 2006-01-02), by pack id, kept in usage.json of the plugins folder so a restart does not give a pack its budget again. The app is its single writer. It is never synced.
type Usage struct {
	Day    string         `json:"day"`
	Tokens map[string]int `json:"tokens"`
}

// LoadUsage reads usage.json of the plugins folder root. A missing file, one that cannot be read and one of another day than day are all the usage of a day nothing was spent on yet.
func LoadUsage(root, day string) Usage {
	var u Usage
	data, err := os.ReadFile(filepath.Join(root, usageName))
	if err != nil || json.Unmarshal(data, &u) != nil || u.Day != day {
		return Usage{Day: day}
	}
	return u
}

// Spent is how many tokens the pack id spent on day.
func (u Usage) Spent(day, id string) int {
	if u.Day != day {
		return 0
	}
	return u.Tokens[id]
}

// Add counts n tokens the pack id spent on day, starting a new day's count when day is not u's.
func (u *Usage) Add(day, id string, n int) {
	if u.Day != day {
		*u = Usage{Day: day}
	}
	if u.Tokens == nil {
		u.Tokens = map[string]int{}
	}
	u.Tokens[id] += max(n, 0)
}

// Forget drops what the pack id spent, as when it is uninstalled.
func (u *Usage) Forget(id string) { delete(u.Tokens, id) }

// Save writes u to usage.json of root through a file renamed over it.
func (u Usage) Save(root string) error {
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, usageName), append(data, '\n'))
}
