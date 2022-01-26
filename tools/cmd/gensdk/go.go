package main

import (
	_ "embed"
	"strings"
	"text/template"
)

//go:embed go.tmpl
var goSrc string

var Go = mustParseTemplate("go.tmpl", goSrc, template.FuncMap{
	"resolveType": GoResolveType,
})

func GoResolveType(typ string) string {
	if strings.Contains(typ, "|") {
		return "interface{}"
	}

	if strings.Contains(typ, ".") {
		return "*" + typ
	}

	return "*api." + typ
}
