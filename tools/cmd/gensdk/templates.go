package main

import (
	_ "embed"
	"sort"
	"strings"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
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

func mustParseTemplate(name, src string, funcs template.FuncMap) *template.Template {
	tmpl := template.New(name)
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	tmpl, err := tmpl.Parse(src)
	checkf(err, "bad template")
	return tmpl
}
