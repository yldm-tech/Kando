package locales

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// Every catalog of Interface has the source's messages, and only those, none of them empty: a message a translation lacks would say English in the middle of another language.
func TestCatalogsAreComplete(t *testing.T) {
	read := func(tag string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(tag + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s.json: %v", tag, err)
		}
		return m
	}
	messages := func(m map[string]any) []string {
		var keys []string
		for k := range m {
			if !strings.HasPrefix(k, "@") && !strings.HasPrefix(k, "mygo.") {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		return keys
	}
	source := read(Source)
	want := messages(source)
	if len(want) == 0 {
		t.Fatal("the source catalog has no messages")
	}
	for _, tag := range Interface {
		m := read(tag)
		if got := messages(m); !slices.Equal(got, want) {
			missing, extra := diff(want, got), diff(got, want)
			t.Errorf("%s.json: missing %q, extra %q", tag, missing, extra)
		}
		for _, k := range want {
			if s, _ := m[k].(string); strings.TrimSpace(s) == "" {
				t.Errorf("%s.json: %s is empty", tag, k)
			}
		}
	}
}

func diff(a, b []string) []string {
	var out []string
	for _, k := range a {
		if !slices.Contains(b, k) {
			out = append(out, k)
		}
	}
	return out
}
