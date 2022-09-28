package main

import (
	_ "embed"
	"text/template"
)

//go:embed go.go.tmpl
var goSrc string

var _ = Templates.Register(goSrc, "go", template.FuncMap{
	"valueName": func(v *TypeValue) string {
		if flags.ShortNames {
			return v.Name
		}
		return v.Type.Name + v.Name
	},
}, "Go")
