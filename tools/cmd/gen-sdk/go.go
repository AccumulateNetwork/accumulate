// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed go.go.tmpl
var goSrc string

var Go = mustParseTemplate("go.tmpl", goSrc, template.FuncMap{
	"ioParam":   GoIOParam,
	"outputVar": GoOutputVar,
	"outputRet": GoOutputReturn,
})

func GoIOParam(typ string) string {
	switch {
	case strings.Contains(typ, "|"):
		return "interface{}"
	case strings.Contains(typ, "."):
		return "*" + typ
	default:
		return "*api." + typ
	}
}

func GoOutputVar(typ string) string {
	switch {
	case strings.Contains(typ, "|"):
		return "interface{}"
	case strings.Contains(typ, "."):
		return typ
	default:
		return fmt.Sprintf("api.%s", typ)
	}
}

func GoOutputReturn(typ string) string {
	switch {
	case strings.Contains(typ, "|"):
		return "resp"
	default:
		return "&resp"
	}
}
