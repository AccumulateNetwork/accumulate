package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/golangci/golangci-lint/pkg/golinters/goanalysis"
	"github.com/golangci/golangci-lint/pkg/lint/linter"
	"golang.org/x/tools/go/analysis"
)

func init() {
	const name = "rangevarref"
	const doc = "Checks for loops that capture a pointer to a value-type range variable"

	customLinters = append(customLinters, linter.NewConfig(
		goanalysis.NewLinter(name, doc, []*analysis.Analyzer{{
			Name: name,
			Doc:  doc,
			Run:  rangevarref,
		}}, nil).
			WithLoadMode(goanalysis.LoadModeTypesInfo),
	))
}

func rangevarref(pass *analysis.Pass) (interface{}, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			return findLoopWithValueTypeVar(pass, n)
		})
	}
	return nil, nil
}

func findLoopWithValueTypeVar(pass *analysis.Pass, node ast.Node) bool {
	stmt, ok := node.(*ast.RangeStmt)
	if !ok {
		return true
	}

	inspectRangeVar(pass, stmt.Body, stmt.Key)
	inspectRangeVar(pass, stmt.Body, stmt.Value)

	return true
}

func inspectRangeVar(pass *analysis.Pass, body ast.Node, rvar ast.Expr) {
	if rvar == nil {
		return
	}

	ident, ok := rvar.(*ast.Ident)
	if !ok {
		return
	}

	loopVar := pass.TypesInfo.ObjectOf(ident)
	ast.Inspect(body, func(n ast.Node) bool {
		return findTakeRefOfLoopVar(pass, n, loopVar)
	})
}

func findTakeRefOfLoopVar(pass *analysis.Pass, node ast.Node, loopVar types.Object) bool {
	if node == nil {
		return true
	}

	var operand ast.Expr
	switch expr := node.(type) {
	case *ast.UnaryExpr:
		if expr.Op != token.AND {
			return true
		}
		operand = expr.X

		// It is never safe to take the address of a range variable

	// case *ast.IndexExpr:
	// 	operand = expr.X

	case *ast.SliceExpr:
		operand = expr.X

		switch typ := loopVar.Type().(type) {
		case *types.Basic:
			if typ.Kind() == types.String {
				// It's safe to slice a string range variable
				return true
			}

		case *types.Slice:
			// It's safe to slice a slice range variable
			return true
		}

		// It is not safe to slice array or value-type range variables

	default:
		return true
	}

	ident, ok := operand.(*ast.Ident)
	if !ok {
		return true
	}

	if loopVar != pass.TypesInfo.ObjectOf(ident) {
		return true
	}

	pass.Reportf(node.Pos(), "Taking the address of a range variable is unsafe")
	return true
}

// func isRefType(typ types.Type) bool {
// 	switch typ.(type) {
// 	case *types.Pointer,
// 		*types.Chan,
// 		*types.Interface,
// 		*types.Map,
// 		*types.Slice:
// 		return true
// 	}
// 	u := typ.Underlying()
// 	if u == typ {
// 		return false
// 	}

// 	return isRefType(u)
// }
