package locales

import (
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

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

// unusedAllowed are the messages no Go file of the app names that stay in the catalogs: appName, which NSGo reads by its key for the menu bar and the packages, and the words of what waits on NSGo, which the TODO(nsgo#N) at each place names.
var unusedAllowed = []string{
	"appName",
	// nsgo#199: titles, buttons and filters of the file and folder panels (dialogs.go).
	"libraryPickVideosTitle", "libraryPickVideosButton", "libraryVideoFilter", "libraryImportTitle", "libraryImportButton", "libraryPlaceTitle", "libraryPlaceButton",
	// nsgo#118: the system menu items NSGo has no role for (menu.go).
	"menuServices", "menuPasteAndMatchStyle", "menuDelete", "menuSpeech", "menuStartSpeaking", "menuStopSpeaking", "menuBringAllToFront",
	// nsgo#32: the updater's state and switches under Version (settings/general.go).
	"settingsGeneralUpdateAvailable", "settingsGeneralLastCheck", "settingsGeneralAutoCheck", "settingsGeneralAutoDownload", "settingsGeneralAutoDownloadHint",
}

// Every message of the source catalog is named by the app's Go code outside this package's generated file and the tests, so translators keep no strings nobody sees. A message no longer said is deleted from all five catalogs; one waiting on NSGo goes in unusedAllowed.
func TestEveryMessageIsUsed(t *testing.T) {
	used := map[string]bool{}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "catalog_gen.go" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var s scanner.Scanner
		fset := token.NewFileSet()
		s.Init(fset.AddFile(path, -1, len(src)), src, nil, 0)
		for {
			_, tok, lit := s.Scan()
			if tok == token.EOF {
				return nil
			}
			if tok == token.IDENT {
				used[lit] = true
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for k := range readCatalog(t, Source) {
		r, size := utf8.DecodeRuneInString(k)
		if !used[string(unicode.ToUpper(r))+k[size:]] && !slices.Contains(unusedAllowed, k) {
			t.Errorf("no Go file says %s; delete it from the catalogs", k)
		}
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
