package main

import (
	"go/ast"

	"github.com/golangci/golangci-lint/pkg/golinters/goanalysis"
	"github.com/golangci/golangci-lint/pkg/lint/linter"
	"golang.org/x/tools/go/analysis"
)

func init() {
	const name = "noprint"
	const doc = "Checks for out of place print statements"

	customLinters = append(customLinters, linter.NewConfig(
		goanalysis.NewLinter(name, doc, []*analysis.Analyzer{{
			Name: name,
			Doc:  doc,
			Run:  noprint,
		}}, nil).
			WithLoadMode(goanalysis.LoadModeSyntax),
	))
}

func noprint(pass *analysis.Pass) (interface{}, error) {
	inspect := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		var name string
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			pkg, ok := fn.X.(*ast.Ident)
			if !ok {
				return true
			}
			name = pkg.Name + "." + fn.Sel.Name
		default:
			return true
		}

		switch name {
		case "print",
			"println",
			"fmt.Print",
			"fmt.Printf",
			"fmt.Println",
			"spew.Dump":
			pass.Reportf(node.Pos(), "Use a logger instead of printing")
		}

		return true
	}

	for _, f := range pass.Files {
		ast.Inspect(f, inspect)
	}
	return nil, nil
}
