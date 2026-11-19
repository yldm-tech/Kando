package plugins

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// filmKeyName is the file of the plugins folder that holds the secret the films' ids for the script plugins are derived from.
const filmKeyName = ".filmkey"

// FilmKey is the secret of the plugins folder root the films' ids for the script plugins are derived from (FilmID), 32 random bytes made the first time; a secret that cannot be kept lasts as long as Kando runs.
func FilmKey(root string) []byte {
	path := filepath.Join(root, filmKeyName)
	if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
		return data
	}
	key := make([]byte, 32)
	rand.Read(key)
	os.MkdirAll(root, 0o755)
	os.WriteFile(path, key, 0o600)
	return key
}

// FilmID is the id the script plugin id, signed by signer, knows the film of video by: derived from the folder's secret key, the plugin, its signer and the video (HMAC), so two plugins cannot match their films up, and none learns where the video is. A film without a video has none.
func FilmID(key []byte, id, signer, video string) string {
	if video == "" {
		return ""
	}
	k := hmac.New(sha256.New, key)
	k.Write([]byte(id + "\x00" + signer))
	mac := hmac.New(sha256.New, k.Sum(nil))
	mac.Write([]byte(video))
	return "f_" + hex.EncodeToString(mac.Sum(nil))[:16]
}

// LineValue is a line of a film as a script plugin reads it: its index, times in milliseconds, original and translation.
func LineValue(index int, startMs, endMs int64, original, translation string) map[string]any {
	return map[string]any{"index": index, "startMs": startMs, "endMs": endMs, "original": original, "translation": translation}
}
