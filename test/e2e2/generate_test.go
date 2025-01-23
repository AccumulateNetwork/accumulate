// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e2

import (
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/russross/blackfriday/v2"
	"github.com/stretchr/testify/require"
	"gitlab.com/firelizzard/go-script/pkg/script"
)

//go:generate go test -run=^TestGenerate$
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w generated

func TestGenerate(t *testing.T) {
	dir, err := os.ReadDir(".")
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll("generated"))
	require.NoError(t, os.MkdirAll("generated", 0755))

	for _, ent := range dir {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), "_test.md") {
			continue
		}

		name := strings.TrimSuffix(ent.Name(), "_test.md")
		name = reSnake.ReplaceAllStringFunc(name, func(s string) string {
			return strings.ToUpper(strings.TrimPrefix(s, "_"))
		})

		t.Run(name, func(t *testing.T) {
			generateTest(t, name, ent.Name())
		})
	}
}

var reSnake = regexp.MustCompile(`(^|_)\w`)
var reCodeFence = regexp.MustCompile(`^([^\s\{]*)(\{[^\n]*\})?`)

func generateTest(t *testing.T, name, filename string) {
	fset := token.NewFileSet()
	imports, contents := parseTestMd(t, fset, filename)

	body, err := script.ResolveImports(&ast.BlockStmt{
		List: contents,
	}, func(path string) (ast.Node, error) {
		imports, contents := parseTestMd(t, fset, path)
		return &ast.File{
			Decls: []ast.Decl{
				&ast.GenDecl{
					Tok:   token.IMPORT,
					Specs: imports,
				},
				&ast.FuncDecl{
					Name: &ast.Ident{Name: "main"},
					Body: &ast.BlockStmt{List: contents},
				},
			},
		}, nil
	})
	require.NoError(t, err)

	f := &ast.File{
		Name: &ast.Ident{Name: "e2e2"},
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{Path: &ast.BasicLit{Value: `"testing"`, Kind: token.STRING}},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: `"gitlab.com/accumulatenetwork/accumulate/pkg/build"`, Kind: token.STRING}},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: `"gitlab.com/accumulatenetwork/accumulate/protocol"`, Kind: token.STRING}, Name: &ast.Ident{Name: "."}},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: `"gitlab.com/accumulatenetwork/accumulate/test/harness"`, Kind: token.STRING}, Name: &ast.Ident{Name: "."}},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: `"gitlab.com/accumulatenetwork/accumulate/test/simulator"`, Kind: token.STRING}},
				},
			},
			&ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: imports,
			},
			&ast.FuncDecl{
				Name: &ast.Ident{Name: "Test" + name},
				Type: &ast.FuncType{
					Params: &ast.FieldList{
						List: []*ast.Field{{
							Names: []*ast.Ident{{Name: "t"}},
							Type: &ast.StarExpr{
								X: &ast.SelectorExpr{
									X:   &ast.Ident{Name: "testing"},
									Sel: &ast.Ident{Name: "T"},
								},
							},
						}},
					},
				},
				Body: body,
			},
		},
	}

	out, err := os.Create(filepath.Join("generated", strings.TrimSuffix(filename, ".md")+".go"))
	require.NoError(t, err)
	err = printer.Fprint(out, fset, f)
	require.NoError(t, err)
}

func parseTestMd(t *testing.T, fset *token.FileSet, filename string) ([]ast.Spec, []ast.Stmt) {
	// Read the file
	src, err := os.ReadFile(filename)
	require.NoError(t, err)

	// Parse the markdown
	parser := blackfriday.New(blackfriday.WithExtensions(blackfriday.FencedCode))
	doc := parser.Parse(src)

	// Extract code blocks
	var imports []ast.Spec
	var contents []ast.Stmt
	doc.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		// Is it a code block?
		if node.Type != blackfriday.CodeBlock {
			return blackfriday.GoToNext
		}

		// Is it Go?
		m := reCodeFence.FindSubmatch(node.Info)
		if len(m) < 2 || string(m[1]) != "go" {
			return blackfriday.GoToNext
		}

		// Parse it
		f, err := script.Read(fset, t.Name(), string(node.Literal))
		require.NoError(t, err)

		for _, i := range f.Imports {
			imports = append(imports, i)
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "main" {
				contents = append(contents, &ast.DeclStmt{Decl: decl})
			} else {
				contents = append(contents, fn.Body.List...)
			}
		}

		return blackfriday.GoToNext
	})

	return imports, contents
}
