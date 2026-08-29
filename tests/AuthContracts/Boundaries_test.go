package authcontracts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"testing"
)

// Native authentication and view contracts come from Hesape. The application
// owns its domain and screens; framework/modules remains the namespace for
// external community modules. Framework's view package is allowed once at the
// composition root only for NewModule: that adapter connects the Framework
// v0.40 kernel router to Hesape's renderer and asset registry.
func TestApplicationDoesNotImportLegacyAuthenticationOrViewPackages(t *testing.T) {
	legacyAuth := "github.com/arandu-io/framework/" + "modules/auth"
	viewBridge := "github.com/arandu-io/framework/" + "view"
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	bridgeImports := 0
	for _, directory := range []string{"app", "assets", "bootstrap", "config", "database", "routes", "tests"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imported := range file.Imports {
				name, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				relative, relErr := filepath.Rel(root, path)
				if relErr != nil {
					return relErr
				}
				if name == legacyAuth {
					t.Errorf("%s imports retired package %s", relative, name)
				}
				if name == viewBridge {
					bridgeImports++
					if relative != filepath.Join("bootstrap", "app.go") || imported.Name == nil || imported.Name.Name != "fwview" {
						t.Errorf("%s imports the kernel view bridge outside the canonical fwview composition seam", relative)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", directory, err)
		}
	}
	if bridgeImports != 1 {
		t.Fatalf("kernel view bridge imports = %d, want exactly one in bootstrap/app.go", bridgeImports)
	}

	bootstrapPath := filepath.Join(root, "bootstrap", "app.go")
	file, err := parser.ParseFile(token.NewFileSet(), bootstrapPath, nil, 0)
	if err != nil {
		t.Fatalf("parsing bootstrap/app.go: %v", err)
	}
	uses := 0
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "fwview" {
			return true
		}
		uses++
		if selector.Sel.Name != "NewModule" {
			t.Errorf("bootstrap/app.go uses fwview.%s, want only fwview.NewModule", selector.Sel.Name)
		}
		return true
	})
	if uses != 1 {
		t.Fatalf("kernel view bridge uses = %d, want exactly fwview.NewModule once", uses)
	}
}
