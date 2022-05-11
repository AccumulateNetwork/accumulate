package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

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
			"fmt.Println":
			pass.Reportf(node.Pos(), "Use a logger instead of printing")
		}

		return true
	}

	for _, f := range pass.Files {
		ast.Inspect(f, inspect)
	}
	return nil, nil
}
