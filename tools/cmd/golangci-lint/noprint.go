// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"go/ast"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	const name = "noprint"
	const doc = "Checks for out of place print statements"

	register.Plugin(name, func(any) (register.LinterPlugin, error) {
		return &customLinter{
			LoadMode: register.LoadModeSyntax,
			Analyzer: &analysis.Analyzer{
				Name: name,
				Doc:  doc,
				Run:  noprint,
			},
		}, nil
	})
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
