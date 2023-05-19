// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Recode is an automated refactoring/code transform tool.
package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"strings"

	"github.com/fatih/astrewrite"
)

func init() {
	passes = append(passes, recodeBuild)
}

func recodeBuild(fset *token.FileSet, file *ast.File) (*ast.File, bool) {
	var changed bool
	file = astrewrite.Walk(file, func(n ast.Node) (ast.Node, bool) {
		if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.IMPORT {
			decl.Specs = append(decl.Specs,
				&ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `"gitlab.com/accumulatenetwork/accumulate/pkg/build"`}},
				&ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `"gitlab.com/accumulatenetwork/accumulate/test/helpers"`}, Name: ast.NewIdent(".")},
			)
			return decl, false
		}

		b := new(BuildCall)
		if !b.parse(n) {
			return n, true
		}

		changed = true
		return b.generate(fset), false
	}).(*ast.File)

	return file, changed
}

type BuildCall struct {
	Calls     []*MethodCall
	Unknown   []int
	WrongArgc []int

	didBuild bool
}

type MethodCall struct {
	Method string
	*ast.CallExpr
}

var build_ExpectedParamCount = map[string]int{
	"WithTransaction":      1,
	"WithPrincipal":        1,
	"WithDelegator":        1,
	"WithSigner":           2,
	"WithTimestamp":        1,
	"WithTimestampVar":     1,
	"WithCurrentTimestamp": 0,
	"WithBody":             1,
	"Sign":                 2,
	"Initiate":             2,
	"Build":                0,
	"BuildDelivery":        0,
	"Faucet":               0,
	"UseSimpleHash":        0,
	"Unsafe":               0,
}

func (b *BuildCall) generate(fset *token.FileSet) ast.Node {
	// Scanning the AST visits the calls in reverse order, so reverse them
	var withTxn, withPrincipal, withBody *MethodCall
	calls := make([]*MethodCall, 0, len(b.Calls))
	for i := len(b.Calls) - 1; i >= 0; i-- {
		switch c := b.Calls[i]; c.Method {
		case "WithPrincipal":
			if withPrincipal != nil {
				fatalf("multiple calls to %v (%v)", c.Method, fset.Position(c.Pos()))
			}
			withPrincipal = c
		case "WithTransaction":
			if withTxn != nil {
				fatalf("multiple calls to %v (%v)", c.Method, fset.Position(c.Pos()))
			}
			withTxn = c
		case "WithBody":
			if withBody != nil {
				fatalf("multiple calls to %v (%v)", c.Method, fset.Position(c.Pos()))
			}
			withBody = c
		default:
			calls = append(calls, c)
		}
	}

	var expr ast.Expr = ast.NewIdent("build")
	var isSignerBuilder bool
	switch {
	case withTxn != nil && withPrincipal != nil && withBody != nil:
		fatalf("expected either WithTransaction or WithPrincipal+WithBody (%v)", fset.Position(b.Calls[0].Pos()))

	case withTxn != nil:
		expr = &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   expr,
				Sel: ast.NewIdent("SignatureForTransaction"),
			},
			Args: withTxn.Args,
		}
		isSignerBuilder = true

	case withPrincipal != nil && withBody != nil:
		expr = &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   expr,
				Sel: ast.NewIdent("Transaction"),
			},
		}
		expr = fluent(expr, "\nFor", withPrincipal.Args...)
		expr = fluent(expr, "\nBody", withBody.Args...)

	default:
		fatalf("expected WithTransaction or WithPrincipal+WithBody (%v)", fset.Position(b.Calls[0].Pos()))
	}

	var withSigner, withDelegator, withTimestamp *MethodCall
	mustBuild := "MustBuild"
	for _, c := range calls {
		pos := fset.Position(c.Pos())
		switch c.Method {
		case "WithSigner":
			withSigner = c
		case "WithDelegator":
			withDelegator = c
		case "WithTimestamp", "WithTimestampVar":
			withTimestamp = c
		case "WithCurrentTimestamp":
			// Timestamp(time.Now())
			withTimestamp = newCallArgs(&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("time"),
					Sel: ast.NewIdent("Now"),
				},
			})

		case "Build":
		case "BuildDelivery":
			mustBuild = "MustBuildDeliveryV1"

		case "Unsafe", "UseSimpleHash":
			// Ignore

		default:
			fatalf("unhandled method %v (%v)", c.Method, pos)

		case "Sign", "Initiate":
			if withSigner == nil {
				fatalf("missing signer (%v)", pos)
			}
			if isSignerBuilder {
				expr = fluent(expr, "\nUrl", withSigner.Args[0])
			} else {
				expr = fluent(expr, "\nSignWith", withSigner.Args[0])
			}
			expr = fluent(expr, "Version", withSigner.Args[1])

			if withDelegator != nil {
				expr = fluent(expr, "Delegator", withDelegator.Args...)
			}

			if withTimestamp != nil {
				expr = fluent(expr, "Timestamp", withTimestamp.Args...)
			}

			expr = fluent(expr, "PrivateKey", c.Args[1])
			if ident, ok := c.Args[0].(*ast.Ident); !ok || ident.Name != "SignatureTypeED25519" {
				expr = fluent(expr, "Type", c.Args[0])
			}

		case "Faucet":
			// SignWith(protocol.FaucetUrl)
			// Version(1)
			// Timestamp(time.Now().UnixNano())
			// Signer(protocol.Faucet.Signer())

			url := &ast.SelectorExpr{
				X:   ast.NewIdent("protocol"),
				Sel: ast.NewIdent("FaucetUrl"),
			}
			if isSignerBuilder {
				expr = fluent(expr, "\nUrl", url)
			} else {
				expr = fluent(expr, "\nSignWith", url)
			}

			expr = fluent(expr, "Version", &ast.BasicLit{
				Kind:  token.INT,
				Value: "1",
			})

			expr = fluent(expr, "Timestamp", &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("time"),
							Sel: ast.NewIdent("Now"),
						},
					},
					Sel: ast.NewIdent("UnixNano"),
				},
			})

			expr = fluent(expr, "Signer", &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.SelectorExpr{
						X:   ast.NewIdent("protocol"),
						Sel: ast.NewIdent("Faucet"),
					},
					Sel: ast.NewIdent("Signer"),
				},
			})
		}
	}

	return &ast.CallExpr{
		Fun: ast.NewIdent("\n" + mustBuild),
		Args: []ast.Expr{
			ast.NewIdent("t"),
			expr,
		},
	}
}

func (b *BuildCall) parse(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// The chain must end (from the AST's perspective, start) with
	// Build/BuildDelivery
	switch sel.Sel.Name {
	case "Build", "BuildDelivery", "Faucet":
		b.didBuild = true
	}

	if sel.Sel.Name != "NewTransaction" {
		// Record the call
		i := len(b.Calls)
		b.Calls = append(b.Calls, &MethodCall{sel.Sel.Name, call})

		if want, ok := build_ExpectedParamCount[sel.Sel.Name]; !ok {
			// Is the call a known method?
			b.Unknown = append(b.Unknown, i)
		} else if len(call.Args) != want {
			// Does it have the expected number of arguments?
			b.WrongArgc = append(b.WrongArgc, i)
		}

		return b.parse(sel.X)
	}

	// This is the end
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "acctesting" {
		return false
	}

	// Any calls with the wrong arg count?
	if len(b.WrongArgc) != 0 {
		return false
	}

	// Any calls to unknown methods?
	if len(b.Unknown) > 0 {
		var s []string
		for _, i := range b.Unknown {
			s = append(s, b.Calls[i].Method)
		}
		fmt.Fprintf(os.Stderr, "Unknown builder method(s) %s\n", strings.Join(s, ", "))
		return false
	}

	return b.didBuild
}

func newCallArgs(args ...ast.Expr) *MethodCall {
	return &MethodCall{
		CallExpr: &ast.CallExpr{
			Args: args,
		},
	}
}
