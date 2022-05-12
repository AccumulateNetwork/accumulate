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
	if !ok || stmt.Value == nil {
		return true
	}

	if isRefType(pass.TypesInfo.TypeOf(stmt.Value)) {
		return true
	}

	loopVar := pass.TypesInfo.ObjectOf(stmt.Value.(*ast.Ident))
	ast.Inspect(stmt.Body, func(n ast.Node) bool {
		return findTakeRefOfLoopVar(pass, n, loopVar)
	})
	return true
}

func isRefType(typ types.Type) bool {
	switch typ.(type) {
	case *types.Pointer,
		*types.Chan,
		*types.Interface,
		*types.Map,
		*types.Slice:
		return true
	}
	u := typ.Underlying()
	if u == typ {
		return false
	}

	return isRefType(u)
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

	case *ast.IndexExpr:
		operand = expr.X

	case *ast.SliceExpr:
		operand = expr.X

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

	pass.Reportf(node.Pos(), "Taking the address of a value-type range variable is unsafe")
	return true
}
