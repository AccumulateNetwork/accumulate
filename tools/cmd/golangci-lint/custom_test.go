// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestRangeVarRef(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), &analysis.Analyzer{
		Name: "rangevarref",
		Run:  rangevarref,
		Doc:  "x",
	}, "rangevarref.go")
}

func TestNoPrint(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), &analysis.Analyzer{
		Name: "noprint",
		Run:  noprint,
		Doc:  "x",
	}, "noprint.go")
}

func TestNoDebug(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), &analysis.Analyzer{
		Name: "nodebug",
		Run:  nodebug,
		Doc:  "x",
	}, "nodebug.go")
}
