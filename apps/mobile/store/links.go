package store

import (
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	"kando/packages/sync"
)

// linksFile keeps the links sent from this phone that their computer has not taken yet.
const linksFile = "links.json"

// Link is a link the user sent from this phone to a computer, kept until the computer answers it: one sent while the computer is off waits on the phone and goes out once it answers again, under the same call ID (sync.Client.CallID), so a link sent twice is taken once.
type Link struct {
	// ID is the call's ID (sync.NewCallID), which names the link on the phone too.
	ID      string `json:"id"`
	Desktop string `json:"desktop"`
	URL     string `json:"url"`
	// Options are how the user asked the computer to process it, kept so a link sent again goes with them.
	Options sync.JobOptions `json:"options,omitzero"`
	Added   time.Time       `json:"added"`
	// Refusal is why the computer refused the link, nil while it waits for the computer: kept, so a link refused before the app was closed still says why rather than going out again on its own.
	Refusal *sync.ErrorBody `json:"refusal,omitempty"`
}

// Links are the links not taken yet, oldest first, none before any were kept.
func (s *Store) Links() ([]Link, error) {
	var out []Link
	err := readJSON(filepath.Join(s.dir, linksFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

// SaveLinks keeps links.
func (s *Store) SaveLinks(links []Link) error {
	return writeJSON(filepath.Join(s.dir, linksFile), links)
}
