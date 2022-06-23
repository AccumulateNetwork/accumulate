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
//nolint:unused,deadcode
func printNode(wr io.Writer, fset *token.FileSet, node ast.Node) {
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, node)
	fmt.Fprintf(wr, "%s | %#v\n", buf.String(), node)
}
