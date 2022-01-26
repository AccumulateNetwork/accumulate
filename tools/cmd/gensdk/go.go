package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed go.tmpl
var goSrc string

var Go = mustParseTemplate("go.tmpl", goSrc, template.FuncMap{
	"ioParam": GoIOParam,
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
		return fmt.Sprintf("%s", typ)
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