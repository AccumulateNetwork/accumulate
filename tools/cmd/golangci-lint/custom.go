// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"golang.org/x/tools/go/analysis"
)

type customLinter struct {
	LoadMode string
	Analyzer *analysis.Analyzer
}

func (c *customLinter) GetLoadMode() string {
	return c.LoadMode
}

func (c *customLinter) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{c.Analyzer}, nil
}
