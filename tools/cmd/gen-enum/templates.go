package main

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/AccumulateNetwork/accumulate/tools/internal/typegen"
)

type Types struct {
	Package string
	Types   []*Type
}

type Type struct {
	Name   string
	Values []*TypeValue
}

type TypeValue struct {
	Name string
	typegen.TypeValue
}

var Templates = typegen.NewTemplateLibrary(template.FuncMap{
	"lower":   lower,
	"natural": natural,
})

func convert(types map[string]typegen.Type, pkgName string) *Types {
	ttypes := make([]*Type, 0, len(types))

	for name, typ := range types {
		ttyp := new(Type)
		ttypes = append(ttypes, ttyp)
		ttyp.Name = name
		ttyp.Values = make([]*TypeValue, 0, len(typ))
		for name, val := range typ {
			tval := new(TypeValue)
			ttyp.Values = append(ttyp.Values, tval)
			tval.Name = name
			tval.TypeValue = *val
		}
		sort.Slice(ttyp.Values, func(i, j int) bool {
			return strings.Compare(fmt.Sprint(ttyp.Values[i].Value), fmt.Sprint(ttyp.Values[j].Value)) < 0
		})
	}
	sort.Slice(ttypes, func(i, j int) bool {
		return strings.Compare(ttypes[i].Name, ttypes[j].Name) < 0
	})

	return &Types{Package: pkgName, Types: ttypes}
}

func lower(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func natural(name string) string {
	var splits []int

	var wasLower bool
	for i, r := range name {
		if wasLower && unicode.IsUpper(r) {
			splits = append(splits, i)
		}
		wasLower = unicode.IsLower(r)
	}

	w := new(strings.Builder)
	w.Grow(len(name) + len(splits))

	var word string
	var split int
	for len(splits) > 0 {
		split, splits = splits[0], splits[1:]
		word, name = name[:split], name[split:]
		w.WriteString(word)
		w.WriteRune(' ')
	}

	w.WriteString(name)
	return w.String()
}
