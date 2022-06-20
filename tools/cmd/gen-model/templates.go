package main

import (
	_ "embed"
	"fmt"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

//go:embed state.yml.tmpl
var ymlSrc string

func init() {
	Templates.Register(ymlSrc, "yaml", nil)
}

var Templates = typegen.NewTemplateLibrary(template.FuncMap{
	"lower":   typegen.LowerFirstWord,
	"title":   typegen.TitleCase,
	"natural": typegen.Natural,
	"debug":   fmt.Printf,
	"excluded": func(r typegen.Record) bool {
		name := r.FullName()
		for _, n := range flags.Exclude {
			if n == name {
				return true
			}
		}
		return false
	},
})
