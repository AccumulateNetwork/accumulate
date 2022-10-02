// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"sort"
	"strings"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

var (
	Go = mustParseTemplate("Go", goSrc)
)

type TApi struct {
	Package string
	Methods []*TMethod
}

type TMethod struct {
	typegen.Method
	Name string
}

func convert(api typegen.API) *TApi {
	tapi := new(TApi)
	tapi.Package = flags.Package
	tapi.Methods = make([]*TMethod, 0, len(api))

	for name, method := range api {
		tm := new(TMethod)
		tm.Method = method
		tm.Name = name
		if tm.Call == "" {
			tm.Call = name
		}
		tapi.Methods = append(tapi.Methods, tm)
	}

	sort.Slice(tapi.Methods, func(i, j int) bool {
		return strings.Compare(tapi.Methods[i].Name, tapi.Methods[j].Name) < 0
	})

	return tapi
}

//go:embed go.go.tmpl
var goSrc string

func mustParseTemplate(name, src string) *template.Template {
	tmpl, err := template.New(name).Parse(src)
	checkf(err, "bad template")
	return tmpl
}
