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
	}, "rangevarref.go")
}

func TestNoPrint(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), &analysis.Analyzer{
		Name: "noprint",
		Run:  noprint,
	}, "noprint.go")
}

func TestNoDebug(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), &analysis.Analyzer{
		Name: "nodebug",
		Run:  nodebug,
	}, "nodebug.go")
}
