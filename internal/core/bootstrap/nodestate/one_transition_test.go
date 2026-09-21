// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// THERE IS ONE TRANSITION, AND ONE VALUE THAT SERVES (#4295, test audit gap
// 6).
//
// Test4368WhatProductionCalls pins NAMES: it fails if `PromoteToComplete` or
// `StateWaiting` appears in production again. Names are not the property. A
// second promotion added under another name — `Finish`, `Backfilled`,
// anything — or a state constant reached through an aliased import passes
// that scan and reintroduces exactly what the spec retired.
//
// This pins the property instead, in two ways a rename cannot evade:
//
//  1. Whatever a State's numeric value, only ACTIVE serves and only ACTIVE
//     is reported as 2. A new constant is DEAD on arrival: it cannot serve
//     and it cannot move the gauge off 0.
//  2. Exactly one function in this package assigns `m.state`, and it is
//     `PromoteToActive`. Asserted from the package's own syntax tree, so a
//     second writer fails whatever it is called.
func TestOnlyOneTransitionExists(t *testing.T) {
	// (1) The value space. Sixteen is well past any value that has ever
	// existed; every one of them but ACTIVE must be inert.
	for i := 0; i < 16; i++ {
		s := State(i)
		if s == StateActive {
			require.True(t, s.Serves(), "ACTIVE does not serve")
			require.Equal(t, float64(2), Number(s), "ACTIVE is not 2 on the gauge")
			require.Equal(t, "ACTIVE", s.String())
			continue
		}
		require.False(t, s.Serves(), "State(%d) serves; only ACTIVE may", i)
		require.Equal(t, float64(0), Number(s), "State(%d) is not 0 on the gauge", i)
		if s != StateBooting {
			require.Equal(t, "UNKNOWN", s.String(), "State(%d) has a name", i)
		}
	}

	// A Serving, whatever it is, maps onto exactly those two.
	require.Equal(t, StateActive, StateOf(Always{}))
	require.Equal(t, StateBooting, StateOf(Undecided{}))
	require.Equal(t, StateBooting, StateOf(New(bvn0())))
	require.Equal(t, StateActive, StateOf(nil))

	// (2) The only writer of m.state, from the package's own syntax.
	fset := token.NewFileSet()
	dir, err := os.Getwd()
	require.NoError(t, err)
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)

	var writers []string
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					return true
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					as, ok := n.(*ast.AssignStmt)
					if !ok {
						return true
					}
					for _, lhs := range as.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if ok && sel.Sel.Name == "state" {
							writers = append(writers, fn.Name.Name+" ("+filepath.Base(name)+")")
						}
					}
					return true
				})
				return false // do not descend into nested funcs twice
			})
		}
	}

	require.Len(t, writers, 1,
		"the node state is written in more than one place: %v. The spec has one transition, BOOTING → ACTIVE (#4368); a second writer is a second state whatever it is called",
		writers)
	require.Contains(t, writers[0], "PromoteToActive",
		"the node state is written by %s, not by PromoteToActive", writers[0])
}
