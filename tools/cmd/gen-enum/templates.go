// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

type Types struct {
	Package string
	Year    int
	Types   []*Type
}

type SingleTypeFile struct {
	Package string
	*Type
}

type Type struct {
	Name       string
	SubPackage string
	Values     []*TypeValue
}

type TypeValue struct {
	Name string
	Type *Type
	typegen.EnumValue
}

var reCamel = regexp.MustCompile(`^\p{Lu}+`)

var Templates = typegen.NewTemplateLibrary(template.FuncMap{
	"lower":               strings.ToLower,
	"upper":               strings.ToUpper,
	"underscoreUpperCase": typegen.UnderscoreUpperCase,
	"lowerCamel":          func(s string) string { return reCamel.ReplaceAllStringFunc(s, strings.ToLower) },
	"natural":             natural,
})

func convert(types map[string]typegen.Enum, pkgName, subPkgName string) *Types {
	ttypes := make([]*Type, 0, len(types))

	for name, typ := range types {
		ttyp := new(Type)
		ttypes = append(ttypes, ttyp)
		ttyp.Name = name
		ttyp.SubPackage = subPkgName
		ttyp.Values = make([]*TypeValue, 0, len(typ))
		for name, val := range typ {
			tval := new(TypeValue)
			tval.Type = ttyp
			ttyp.Values = append(ttyp.Values, tval)
			tval.Name = name
			tval.EnumValue = *val
		}
		sort.Slice(ttyp.Values, func(i, j int) bool {
			v1, ok1 := ttyp.Values[i].Value.(int)
			v2, ok2 := ttyp.Values[j].Value.(int)
			if ok1 && ok2 {
				return v1 < v2
			}
			return strings.Compare(fmt.Sprint(ttyp.Values[i].Value), fmt.Sprint(ttyp.Values[j].Value)) < 0
		})
	}
	sort.Slice(ttypes, func(i, j int) bool {
		return strings.Compare(ttypes[i].Name, ttypes[j].Name) < 0
	})

	return &Types{Package: pkgName, Types: ttypes}
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
	var offset int
	for len(splits) > 0 {
		split, splits = splits[0], splits[1:]
		split -= offset
		offset += split
		word, name = name[:split], name[split:]
		w.WriteString(word)
		w.WriteRune(' ')
	}

	w.WriteString(name)
	return w.String()
}
