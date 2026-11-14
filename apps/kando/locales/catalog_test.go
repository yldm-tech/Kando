package locales

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Every catalog of Interface has the source's messages, and only those, none of them empty: a message a translation lacks would say English in the middle of another language. nsgo only warns of one.
func TestCatalogsAreComplete(t *testing.T) {
	source := readCatalog(t, Source)
	want := slices.Sorted(func(yield func(string) bool) {
		for k := range source {
			if !yield(k) {
				return
			}
		}
	})
	if len(want) == 0 {
		t.Fatal("the source catalog has no messages")
	}
	for _, tag := range Interface {
		m := readCatalog(t, tag)
		var got []string
		for k := range m {
			got = append(got, k)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s.yaml: missing %q, extra %q", tag, diff(want, got), diff(got, want))
		}
		for _, k := range want {
			if s, ok := m[k]; ok && strings.TrimSpace(s) == "" {
				t.Errorf("%s.yaml: %s is empty", tag, k)
			}
		}
	}
}

// Every language of Interface has a catalog, and every catalog is a language of Interface, so the settings list what nsgo compiles.
func TestInterfaceIsTheCatalogs(t *testing.T) {
	got := Catalog.Languages()
	if !slices.Contains(Interface, Default) || got[0] != Source {
		t.Errorf("Default %s, the catalogs %q, Interface %q", Default, got, Interface)
	}
	slices.Sort(got)
	want := slices.Sorted(slices.Values(Interface))
	if !slices.Equal(got, want) {
		t.Errorf("the catalogs are %q, Interface %q", got, want)
	}
}

// readCatalog returns a catalog's messages by key, leaving out what it supplies to other packages (keys that are import paths).
func readCatalog(t *testing.T, tag string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(tag + ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s.yaml: %v", tag, err)
	}
	msgs := map[string]string{}
	for k, v := range doc {
		if strings.ContainsAny(k, "/.") {
			continue
		}
		switch v.Kind {
		case yaml.ScalarNode:
			msgs[k] = v.Value
		case yaml.MappingNode:
			var m struct{ Message string }
			if err := v.Decode(&m); err != nil {
				t.Fatalf("%s.yaml: %s: %v", tag, k, err)
			}
			msgs[k] = m.Message
		default:
			t.Fatalf("%s.yaml: %s is not a message", tag, k)
		}
	}
	return msgs
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
