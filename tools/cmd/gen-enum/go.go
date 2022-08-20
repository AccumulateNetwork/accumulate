package main

import (
	_ "embed"
	"text/template"
)

//go:embed go.go.tmpl
var goSrc string

var _ = Templates.Register(goSrc, "go", template.FuncMap{
	"const": func(v *TypeValue) string {
		if flags.OmitPrefix {
			return v.Name
		}
		return v.Type.Name + v.Name
	},
}, "Go")
