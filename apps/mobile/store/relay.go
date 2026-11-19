package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"kando/packages/sync"
)

// relayDir keeps, a file per paired computer whose storage relay the phone joined (yldm-tech/Kando#219), what the phone needs to reach that computer through the user's storage while away from its network: the links of its grant, the slots it spent, the number of its last message and the calls whose replies it waits for (sync.RelayPhone). The key its messages are sealed with is kept in the platform's secure store under RelayKeyAccount, never here.
const relayDir = "relay"

// Relay is the phone's side of a computer's storage relay, without its key.
type Relay struct {
	// Device is the phone's device ID on the computer (Desktop.DeviceID), which names its folders in the storage.
	Device string          `json:"device"`
	Grant  sync.RelayGrant `json:"grant"`
	Spent  []string        `json:"spent,omitempty"`
	Seq    int64           `json:"seq"`
	// Waiting are the calls sent whose replies the phone has not read, by slot.
	Waiting map[string]sync.RelayWaiting `json:"waiting,omitempty"`
	// Lapsed is a grant whose links the storage refused, as one past its expiry: the phone joins again once it is back on the computer's network. Failure is why the storage refused them, which the Jobs page says.
	Lapsed  bool              `json:"lapsed,omitempty"`
	Failure sync.RelayFailure `json:"failure,omitempty"`
}

// RelayKeyAccount is the account the relay key of desktop id is kept under in the platform's secure store.
func RelayKeyAccount(id string) string { return "desktop-relay-key." + id }

func (s *Store) relayPath(desktop string) string {
	return filepath.Join(s.dir, relayDir, desktop+".json")
}

// Relay is what the phone keeps of desktop's relay, false before it joined.
func (s *Store) Relay(desktop string) (Relay, bool, error) {
	var r Relay
	err := readJSON(s.relayPath(desktop), &r)
	if errors.Is(err, fs.ErrNotExist) {
		return Relay{}, false, nil
	}
	return r, err == nil, err
}

// SaveRelay keeps r as desktop's relay.
func (s *Store) SaveRelay(desktop string, r Relay) error {
	return writeJSON(s.relayPath(desktop), r)
}

// DeleteRelay forgets desktop's relay; one not kept is no error.
func (s *Store) DeleteRelay(desktop string) error {
	err := os.Remove(s.relayPath(desktop))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
