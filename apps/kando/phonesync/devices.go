package phonesync

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	psync "kando/packages/sync"
)

// DevicesFile is the file Kando keeps its paired phones in, in its config folder, readable by the user alone. It holds each device token's SHA-256, never the token.
const DevicesFile = "phone-sync-devices.json"

// Device is a phone paired with this desktop, as Settings › Phone Sync lists it.
type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Platform string    `json:"platform"`
	Paired   time.Time `json:"paired"`
	LastSeen time.Time `json:"last_seen"`
	// TokenHash is the SHA-256 of the device's token, in hex.
	TokenHash string `json:"token_sha256"`
	// RelayKey is the key the device's objects in the storage relay are sealed with, made when it first joins the relay (psync.OpRelayJoin), none before. Devices does not give it out.
	RelayKey []byte `json:"relay_key,omitempty"`
}

// seenEvery is how often a device's LastSeen is written to disk while it makes requests, so streaming a video does not write the file at every range.
const seenEvery = time.Minute

// devices is the paired phones, kept in a file.
type devices struct {
	path string
	mu   sync.Mutex
	list []Device
}

type devicesData struct {
	Devices []Device `json:"devices"`
}

// loadDevices reads the device list at path; a missing file is no device.
func loadDevices(path string) (*devices, error) {
	d := &devices{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	var saved devicesData
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	for _, dev := range saved.Devices {
		if dev.ID != "" && len(dev.TokenHash) == 2*sha256.Size {
			d.list = append(d.list, dev)
		}
	}
	return d, nil
}

// save writes the list; the caller holds mu.
func (d *devices) save() error {
	data, err := json.MarshalIndent(devicesData{Devices: d.list}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return err
	}
	return writePrivate(d.path, data)
}

// add pairs a new device named name on platform, and returns it with its token, which only the phone keeps.
func (d *devices) add(name, platform string, now time.Time) (Device, string, error) {
	token := rand.Text() + rand.Text()
	dev := Device{ID: rand.Text()[:16], Name: deviceName(name), Platform: devicePlatform(platform), Paired: now, LastSeen: now, TokenHash: tokenHash(token)}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.list = append(d.list, dev)
	if err := d.save(); err != nil {
		d.list = d.list[:len(d.list)-1]
		return Device{}, "", err
	}
	return dev, token, nil
}

// check finds the device whose token is token and notes it was seen at now. saved reports whether that was written to disk, as it is once every seenEvery.
func (d *devices) check(token string, now time.Time) (dev Device, ok, saved bool) {
	if token == "" {
		return Device{}, false, false
	}
	hash := tokenHash(token)
	d.mu.Lock()
	defer d.mu.Unlock()
	i := slices.IndexFunc(d.list, func(dev Device) bool { return dev.TokenHash == hash })
	if i < 0 {
		return Device{}, false, false
	}
	if now.Sub(d.list[i].LastSeen) >= seenEvery {
		d.list[i].LastSeen = now
		saved = d.save() == nil
	}
	return d.list[i], true, saved
}

// revoke unpairs device id; its token is refused from then on.
func (d *devices) revoke(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := slices.IndexFunc(d.list, func(dev Device) bool { return dev.ID == id })
	if i < 0 {
		return nil
	}
	removed := d.list[i]
	d.list = slices.Delete(d.list, i, i+1)
	if err := d.save(); err != nil {
		d.list = slices.Insert(d.list, i, removed)
		return err
	}
	return nil
}

// all is the devices, the latest paired first, without their relay keys.
func (d *devices) all() []Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := slices.Clone(d.list)
	for i := range out {
		out[i].RelayKey = nil
	}
	slices.Reverse(out)
	return out
}

// relayKey is device id's relay key, made and saved the first time it is asked for.
func (d *devices) relayKey(id string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := slices.IndexFunc(d.list, func(dev Device) bool { return dev.ID == id })
	if i < 0 {
		return nil, psync.Errorf(psync.CodeUnauthorized, "no device %s", id)
	}
	if len(d.list[i].RelayKey) == psync.RelayKeySize {
		return slices.Clone(d.list[i].RelayKey), nil
	}
	key := psync.NewRelayKey()
	d.list[i].RelayKey = key
	if err := d.save(); err != nil {
		d.list[i].RelayKey = nil
		return nil, err
	}
	return slices.Clone(key), nil
}

// relayDevices are the devices that joined the relay, with their keys.
func (d *devices) relayDevices() []psync.RelayDevice {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []psync.RelayDevice
	for _, dev := range d.list {
		if len(dev.RelayKey) == psync.RelayKeySize {
			out = append(out, psync.RelayDevice{ID: dev.ID, Name: dev.Name, Key: slices.Clone(dev.RelayKey)})
		}
	}
	return out
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// deviceName is the name a phone gives itself, as the list shows it: on one line, at most 64 characters, "Phone" when it gives none.
func deviceName(name string) string {
	name = strings.Join(strings.Fields(strings.ToValidUTF8(name, "")), " ")
	if utf8.RuneCountInString(name) > 64 {
		name = string([]rune(name)[:64])
	}
	if name == "" {
		return "Phone"
	}
	return name
}

// devicePlatform is one of the protocol's platforms, "" for any other.
func devicePlatform(platform string) string {
	switch p := strings.ToLower(strings.TrimSpace(platform)); p {
	case psync.PlatformIOS, psync.PlatformAndroid:
		return p
	}
	return ""
}
