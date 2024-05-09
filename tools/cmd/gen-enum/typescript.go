// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"text/template"
)

//go:embed main.ts.tmpl
var tsSrc string

var _ = Templates.Register(tsSrc, "typescript", template.FuncMap{
	"valueName": func(v *TypeValue) string { return v.Key },
}, "TypeScript")
