package plugins

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	s, err := LoadState(root)
	if err != nil || s.Format != StateFormat || !s.Enabled("example.jlpt") {
		t.Fatalf("missing state = %+v, %v", s, err)
	}
	s.Plugins = []PluginState{{ID: "example.jlpt", Hash: "abc", Enabled: false}, {ID: "example.terms", Hash: "def", Enabled: true}}
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || len(got.Plugins) != 2 || got.Enabled("example.jlpt") || !got.Enabled("example.terms") || !got.Enabled("example.new") {
		t.Errorf("state = %+v", got)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Errorf("files in root: %v", entries)
	}
}

// TestStateNewerFormat checks that a state.json of a newer Kando turns every pack off and is not written over.
func TestStateNewerFormat(t *testing.T) {
	root := t.TempDir()
	newer := `{"format": 2, "revision": 7, "plugins": [{"id": "example.jlpt", "hash": "abc", "enabled": true, "grant": {}}]}`
	if err := os.WriteFile(filepath.Join(root, stateName), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadState(root)
	wantErr(t, "newer", err, ErrStateFormat)
	if s.Enabled("example.jlpt") || s.Enabled("example.other") {
		t.Errorf("a pack is on in a newer state: %+v", s)
	}
	wantErr(t, "save over newer", s.Save(root), ErrStateFormat)
	if data, _ := os.ReadFile(filepath.Join(root, stateName)); string(data) != newer {
		t.Errorf("state.json was written over: %s", data)
	}
}

// TestStateNewerFormatWrittenSince checks that a state read in this format is not saved over a state.json a newer Kando wrote after it was read, and that a save takes its revision past the file's, so a state read before another of this format was saved still counts up.
func TestStateNewerFormatWrittenSince(t *testing.T) {
	root := t.TempDir()
	s, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	newer := `{"format": 2, "revision": 7, "plugins": []}`
	if err := os.WriteFile(filepath.Join(root, stateName), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "save over a newer state written since", s.Save(root), ErrStateFormat)
	if data, _ := os.ReadFile(filepath.Join(root, stateName)); string(data) != newer {
		t.Errorf("state.json was written over: %s", data)
	}

	if err := os.WriteFile(filepath.Join(root, stateName), []byte(`{"format": 1, "revision": 9, "plugins": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadState(root); err != nil || got.Revision != 10 {
		t.Errorf("after a save over revision 9: %+v, %v", got, err)
	}
}

// TestStateDamaged checks that a state.json that is not JSON turns every pack off until the state is written again.
func TestStateDamaged(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, stateName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadState(root)
	wantErr(t, "damaged", err, ErrBadState)
	if s.Enabled("example.jlpt") {
		t.Error("a pack is on in a damaged state")
	}
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	if !s.Enabled("example.jlpt") {
		t.Error("the state written again still has every pack off")
	}
	if _, err := LoadState(root); err != nil {
		t.Errorf("the state written again: %v", err)
	}
}

// TestStateNoFormat checks that a state.json without a format, as one cut short to {}, is a bad state rather than a newer one, and is written over.
func TestStateNoFormat(t *testing.T) {
	for _, data := range []string{"{}", `{"format": 0, "plugins": [{"id": "example.jlpt", "enabled": true}]}`} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, stateName), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := LoadState(root)
		wantErr(t, data, err, ErrBadState)
		if s.Enabled("example.jlpt") {
			t.Errorf("%s: a pack is on", data)
		}
		if err := s.Save(root); err != nil {
			t.Errorf("%s: save: %v", data, err)
		}
		if _, err := LoadState(root); err != nil {
			t.Errorf("%s: the state written again: %v", data, err)
		}
	}
}

// TestStateSet checks that Set replaces the entry of its id where it is and adds one of a new id after the others.
func TestStateSet(t *testing.T) {
	s := State{Plugins: []PluginState{{ID: "example.a", Hash: "1", Enabled: true}, {ID: "example.b", Hash: "2", Enabled: true}}}
	s.Set(PluginState{ID: "example.a", Hash: "3", Enabled: false})
	s.Set(PluginState{ID: "example.c", Hash: "4", Enabled: true})
	want := []PluginState{{ID: "example.a", Hash: "3", Enabled: false}, {ID: "example.b", Hash: "2", Enabled: true}, {ID: "example.c", Hash: "4", Enabled: true}}
	if !slices.Equal(s.Plugins, want) {
		t.Fatalf("Set made %+v, want %+v", s.Plugins, want)
	}
}
