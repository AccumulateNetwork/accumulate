// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"go/ast"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	const name = "nodebug"
	const doc = "Checks for enabled debug flags"

	register.Plugin(name, func(any) (register.LinterPlugin, error) {
		return &customLinter{
			LoadMode: register.LoadModeSyntax,
			Analyzer: &analysis.Analyzer{
				Name: name,
				Doc:  doc,
				Run:  nodebug,
			},
		}, nil
	})
}

func nodebug(pass *analysis.Pass) (interface{}, error) {
	inspect := func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.File, *ast.GenDecl:
			// Look for top-level const and var declarations
			return true

		case *ast.ValueSpec:
			// Check for debugFoo = true
			for i, value := range node.Values {
				if i >= len(node.Names) {
					continue
				}

				value, ok := value.(*ast.Ident)
				if !ok {
					continue
				}

				name := node.Names[i]
				if !strings.HasPrefix(name.Name, "debug") {
					continue
				}

				if value.Name == "true" {
					pass.Reportf(node.Pos(), "Flag %s is enabled", name.Name)
				}
			}
		}

		// Don't look inside functions or types or anything else
		return false
	}

	for _, f := range pass.Files {
		ast.Inspect(f, inspect)
	}
	return nil, nil
}
