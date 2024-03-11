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

//go:embed go.go.tmpl
var goSrc string

var _ = Templates.Register(goSrc, "go", template.FuncMap{
	"valueName": func(v *TypeValue) string {
		if v.Name != "" {
			return v.Name
		}
		if flags.ShortNames {
			return v.Key
		}
		return v.Type.Name + v.Key
	},
}, "Go")
