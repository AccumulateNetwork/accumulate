// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var ErrSkip = errors.New("skip")

var enUsTitle = cases.Title(language.AmericanEnglish)
var reUpper = regexp.MustCompile(`^\p{Lu}+`)
var reLowerUpper = regexp.MustCompile(`\p{Ll}?\p{Lu}+`)

func DashCase(s string) string {
	s = LowerFirstWord(s)
	s = reLowerUpper.ReplaceAllStringFunc(s, func(s string) string {
		return s[:1] + "-" + strings.ToLower(s[1:])
	})
	return s
}

func UnderscoreUpperCase(s string) string {
	s = LowerFirstWord(s)
	s = reLowerUpper.ReplaceAllStringFunc(s, func(s string) string {
		return s[:1] + "_" + strings.ToLower(s[1:])
	})
	return strings.ToUpper(s) // FIXME shortcut
}

func TitleCase(s string) string {
	return enUsTitle.String(s[:1]) + s[1:]
}

func LowerFirstWord(s string) string {
	return reUpper.ReplaceAllStringFunc(s, strings.ToLower)
}

func Natural(name string) string {
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
		w.WriteString(strings.ToLower(word))
		w.WriteRune(' ')
	}

	w.WriteString(strings.ToLower(name))
	return w.String()
}

func MakeMap(v ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(v)/2)
	for len(v) > 1 {
		m[fmt.Sprint(v[0])] = v[1]
		v = v[2:]
	}
	return m
}

type TemplateLibrary struct {
	functions template.FuncMap
	templates map[string]*template.Template
}

func NewTemplateLibrary(funcs template.FuncMap) *TemplateLibrary {
	return &TemplateLibrary{
		functions: funcs,
		templates: map[string]*template.Template{},
	}
}

func (lib *TemplateLibrary) Register(src, name string, funcs template.FuncMap, altNames ...string) *template.Template {
	tmpl, err := lib.Parse(src, name, funcs)
	if err != nil {
		panic(err)
	}
	lib.templates[name] = tmpl
	for _, name := range altNames {
		lib.templates[name] = tmpl
	}
	return tmpl
}

func (lib *TemplateLibrary) Parse(src, name string, funcs template.FuncMap) (*template.Template, error) {
	tmpl := template.New(name)
	tmpl = tmpl.Funcs(template.FuncMap{
		"skip": func(reason ...any) (string, error) {
			if len(reason) == 0 {
				return "", ErrSkip
			}
			return "", fmt.Errorf("%w: %s", ErrSkip, fmt.Sprint(reason...))
		},
	})
	if lib.functions != nil {
		tmpl = tmpl.Funcs(lib.functions)
	}
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	return tmpl.Parse(src)
}

func (lib *TemplateLibrary) Execute(w io.Writer, name string, data interface{}) error {
	names := strings.Split(name, ":")
	name, names = names[0], names[1:]

	tmpl, ok := lib.templates[name]
	if ok {
		return execute(tmpl, names, w, data)
	}

	b, err := ioutil.ReadFile(name)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%q is not a known template or a template file", name)
	default:
		return err
	}

	tmpl = template.New(name)
	if lib.functions != nil {
		tmpl = tmpl.Funcs(lib.functions)
	}
	tmpl, err = tmpl.Parse(string(b))
	if err != nil {
		return fmt.Errorf("error parsing %q: %v", name, err)
	}

	return execute(tmpl, names, w, data)
}

func execute(tmpl *template.Template, names []string, w io.Writer, data interface{}) error {
	for _, name := range names {
		tmpl = tmpl.Lookup(name)
		if tmpl == nil {
			return fmt.Errorf("unknown sub-template %q", name)
		}
	}
	return tmpl.Execute(w, data)
}
