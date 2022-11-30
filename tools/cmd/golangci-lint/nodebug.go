// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"go/ast"
	"strings"

	"github.com/golangci/golangci-lint/pkg/golinters/goanalysis"
	"github.com/golangci/golangci-lint/pkg/lint/linter"
	"golang.org/x/tools/go/analysis"
)

func init() {
	const name = "nodebug"
	const doc = "Checks for enabled debug flags"

	customLinters = append(customLinters, linter.NewConfig(
		goanalysis.NewLinter(name, doc, []*analysis.Analyzer{{
			Name: name,
			Doc:  doc,
			Run:  nodebug,
		}}, nil).
			WithLoadMode(goanalysis.LoadModeSyntax),
	))
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
