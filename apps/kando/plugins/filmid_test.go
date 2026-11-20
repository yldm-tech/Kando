package plugins

import (
	"bytes"
	"strings"
	"testing"
)

// TestFilmID checks the ids the script plugins know the films by: one film has one id for a plugin, another plugin, another signer or another plugins folder's key gives another, the id tells nothing of the video's path, a film without a video has none, and the folder's key is kept.
func TestFilmID(t *testing.T) {
	root := t.TempDir()
	key := FilmKey(root)
	if len(key) != 32 || !bytes.Equal(FilmKey(root), key) {
		t.Fatalf("the key is not kept: %x", key)
	}
	const video = "Rust Talk/talk.mp4"
	id := FilmID(key, "acme.anki", "k1", video)
	if len(id) != len("f_")+16 || !strings.HasPrefix(id, "f_") || strings.Contains(id, "talk") {
		t.Fatalf("id %q", id)
	}
	if FilmID(key, "acme.anki", "k1", video) != id {
		t.Error("one film has two ids for one plugin")
	}
	for name, other := range map[string]string{
		"another plugin": FilmID(key, "acme.wiki", "k1", video),
		"another signer": FilmID(key, "acme.anki", "k2", video),
		"another key":    FilmID(FilmKey(t.TempDir()), "acme.anki", "k1", video),
		"another video":  FilmID(key, "acme.anki", "k1", "Rust Talk/other.mp4"),
	} {
		if other == id {
			t.Errorf("%s gives the same id %q", name, id)
		}
	}
	if got := FilmID(key, "acme.anki", "k1", ""); got != "" {
		t.Errorf("a film without a video has the id %q", got)
	}
}
