// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Recode is an automated refactoring/code transform tool.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:  "recode <file>",
	Args: cobra.ExactArgs(1),
	Run:  run,
}

var flag = struct {
	Write bool
}{}

func init() {
	cmd.PersistentFlags().BoolVarP(&flag.Write, "write", "w", false, "Write the changes")
}

var passes []func(*token.FileSet, *ast.File) (*ast.File, bool)

func run(_ *cobra.Command, args []string) {
	contents, err := os.ReadFile(args[0])
	check(err)

	var changed bool
	for _, pass := range passes {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, args[0], contents, parser.AllErrors|parser.ParseComments)
		check(err)

		file, ok := pass(fset, file)
		if !ok {
			continue
		}
		changed = true

		buf := new(bytes.Buffer)
		check(format.Node(buf, fset, file))
		contents = buf.Bytes()
	}
	if !changed {
		return
	}

	if flag.Write {
		f, err := os.Create(args[0])
		check(err)
		defer f.Close()
		_, err = f.Write(contents)
		check(err)
	} else {
		_, err = os.Stdout.Write(contents)
		check(err)
		return
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

func fluent(expr ast.Expr, fun string, args ...ast.Expr) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   expr,
			Sel: ast.NewIdent(fun),
		},
		Args: args,
	}
}
