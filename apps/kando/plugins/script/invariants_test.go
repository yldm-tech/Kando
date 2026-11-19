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

// TestScriptRuntimeEntrances scans every Go file of the repository, tests included, for the ways into tsx that would run code with more than a plugin is granted (docs/plugin-system.md 3.3, 9): tsx.Sandboxed, which with neither capabilities nor grant runs a bundle with the app's own authority, file:// included, appears nowhere, developer mode included; tsx.View, whose sandbox fills a nil grant with what the package declares, appears only in this package's own code, each time as a literal whose Sandbox is Sandbox's (a panel's view, Runner.Panel), never probeSandbox's nor one written out, rendered where it is made (tsx.View{…}.Render), so no field of it is set after it was checked; and tsx.Load appears only in this package's own code, each time with the sandbox of Sandbox or probeSandbox, whose grants are made from nothing.
func TestScriptRuntimeEntrances(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	here, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	files, loads, views := 0, 0, 0
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
		// The selectors of tsx.View that are the type of a literal checked here, which the selector's own visit lets pass.
		literals := map[*ast.SelectorExpr]bool{}
		// The tsx.View literals rendered where they are made, as tsx.View{…}.Render: none is kept in a variable whose fields could be set after the literal was checked.
		rendered := map[*ast.CompositeLit]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "Render" {
				if lit, ok := s.X.(*ast.CompositeLit); ok {
					rendered[lit] = true
				}
			}
			return true
		})
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok {
				if s, ok := lit.Type.(*ast.SelectorExpr); ok && isTSX(s, name, "View") {
					literals[s] = true
					views++
					if !ours {
						t.Errorf("%s: tsx.View outside plugins/script", fset.Position(lit.Pos()))
					} else if !viewSandboxed(lit) {
						t.Errorf("%s: tsx.View without the Sandbox of Sandbox", fset.Position(lit.Pos()))
					} else if !rendered[lit] {
						t.Errorf("%s: tsx.View not rendered where it is made (tsx.View{…}.Render)", fset.Position(lit.Pos()))
					}
				}
			}
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
			case isTSX(sel, name, "View") && !literals[sel]:
				t.Errorf("%s: tsx.View fills a nil grant with what the package declares", fset.Position(sel.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 || loads == 0 || views == 0 {
		t.Fatalf("scanned %d files, %d loads and %d views: is the repository where the test looks (%s)?", files, loads, views, repo)
	}
}

// viewSandboxed reports whether lit, a tsx.View literal, sets its Sandbox, by key, to a call of Sandbox, and nothing else but its Package.
func viewSandboxed(lit *ast.CompositeLit) bool {
	sandboxed := false
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, _ := kv.Key.(*ast.Ident)
		switch {
		case key == nil:
			return false
		case key.Name == "Sandbox":
			call, ok := kv.Value.(*ast.CallExpr)
			if !ok {
				return false
			}
			fn, ok := call.Fun.(*ast.Ident)
			sandboxed = ok && fn.Name == "Sandbox"
		case key.Name != "Package":
			return false
		}
	}
	return sandboxed
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
