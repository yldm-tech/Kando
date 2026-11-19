package script

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// tsxPath is the import path of NSGo's TSX runtime.
const tsxPath = "github.com/nsgo-dev/nsgo/tsx"

// TestScriptRuntimeEntrances scans every Go file of the repository, tests included, for the ways into tsx that would run code with more than a plugin is granted (docs/plugin-system.md 3.3, 9): tsx.Sandboxed, which with neither capabilities nor grant runs a bundle with the app's own authority, file:// included, appears nowhere, developer mode included; tsx.View, whose sandbox fills a nil grant with what the package declares, appears nowhere either, as no plugin shows a view yet; and tsx.Load appears only in this package's own code, each time with the sandbox of Sandbox or probeSandbox, whose grants are made from nothing.
func TestScriptRuntimeEntrances(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	here, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	files, loads := 0, 0
	err = filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != repo && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		name := ""
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == tsxPath {
				name = "tsx"
				if imp.Name != nil {
					name = imp.Name.Name
				}
			}
		}
		if name == "" {
			return nil
		}
		ours := filepath.Dir(path) == here && !strings.HasSuffix(path, "_test.go")
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				if call, ok := n.(*ast.CallExpr); ok {
					if s, ok := call.Fun.(*ast.SelectorExpr); ok && isTSX(s, name, "Load") {
						loads++
						if !ours {
							t.Errorf("%s: tsx.Load outside plugins/script", fset.Position(call.Pos()))
						} else if len(call.Args) != 3 || !callsSandbox(call.Args[2]) {
							t.Errorf("%s: tsx.Load without Sandbox or probeSandbox", fset.Position(call.Pos()))
						}
					}
				}
				return true
			}
			switch {
			case isTSX(sel, name, "Sandboxed"):
				t.Errorf("%s: tsx.Sandboxed runs a bundle with the app's authority", fset.Position(sel.Pos()))
			case isTSX(sel, name, "View"):
				t.Errorf("%s: tsx.View fills a nil grant with what the package declares", fset.Position(sel.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 || loads == 0 {
		t.Fatalf("scanned %d files and %d loads: is the repository where the test looks (%s)?", files, loads, repo)
	}
}

// isTSX reports whether sel is the tsx package's member, imported as name.
func isTSX(sel *ast.SelectorExpr, name, member string) bool {
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == name && sel.Sel.Name == member
}

// callsSandbox reports whether e is a call of Sandbox or probeSandbox.
func callsSandbox(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && (id.Name == "Sandbox" || id.Name == "probeSandbox")
}
