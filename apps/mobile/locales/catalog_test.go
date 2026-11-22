package locales

import (
	"bufio"
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
)

// Every catalog of Interface has the source's messages, and only those: a message a translation lacks would say English in the middle of another language, which nsgo only warns of.
func TestCatalogsAreComplete(t *testing.T) {
	want := keys(t, Source)
	if len(want) == 0 {
		t.Fatal("the source catalog has no messages")
	}
	for _, tag := range Interface {
		if got := keys(t, tag); !slices.Equal(got, want) {
			t.Errorf("%s.yaml: missing %q, extra %q", tag, diff(want, got), diff(got, want))
		}
	}
	got := Catalog.Languages()
	slices.Sort(got)
	if ifc := slices.Sorted(slices.Values(Interface)); !slices.Equal(got, ifc) {
		t.Errorf("the catalogs are %q, Interface %q", got, ifc)
	}
}

// Every message of the source catalog is named by the app's Go code outside this package's generated file and the tests, so translators keep no strings nobody sees. appName is NSGo's, which it reads by its key for the packages, as it reads the message of an apple.info entry of nsgo.yaml by the entry's key for the Apple packages' InfoPlist.strings, and the messages a widget's name and description name for the widget extension.
func TestEveryMessageIsUsed(t *testing.T) {
	used := map[string]bool{"AppName": true}
	manifest, err := os.ReadFile(filepath.Join("..", "nsgo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(manifest), "\n") {
		key, value, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "- "), ":")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		used[key] = true
		// A widget's name and description name their messages, which nsgo build gives the widget extension.
		if value = strings.TrimSpace(value); (key == "name" || key == "description") && value != "" {
			r, size := utf8.DecodeRuneInString(value)
			used[string(unicode.ToUpper(r))+value[size:]] = true
		}
	}
	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
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
	for _, k := range keys(t, Source) {
		r, size := utf8.DecodeRuneInString(k)
		if !used[string(unicode.ToUpper(r))+k[size:]] {
			t.Errorf("no Go file says %s; delete it from the catalogs", k)
		}
	}
}

// keys are a catalog's messages' keys, sorted, leaving out what it supplies to other packages (keys that are import paths): the lines that start with a key and a colon.
func keys(t *testing.T, tag string) []string {
	t.Helper()
	f, err := os.Open(tag + ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		line := lines.Text()
		key, _, ok := strings.Cut(line, ":")
		if !ok || key == "" || line[0] == ' ' || line[0] == '#' || strings.ContainsAny(key, "/. \"") {
			continue
		}
		out = append(out, key)
	}
	if err := lines.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
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
