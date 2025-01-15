// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"io"
)

// retain this for debugging purposes
//
//nolint:unused,deadcode
//lint:ignore U1000 Don't care
func printNode(wr io.Writer, fset *token.FileSet, node ast.Node) {
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, node)
	fmt.Fprintf(wr, "%s | %#v\n", buf.String(), node)
}
