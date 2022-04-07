package typegen

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var enUsTitle = cases.Title(language.AmericanEnglish)

func TitleCase(s string) string {
	return enUsTitle.String(s[:1]) + s[1:]
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
	tmpl := template.New(name)
	if lib.functions != nil {
		tmpl = tmpl.Funcs(lib.functions)
	}
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	tmpl, err := tmpl.Parse(src)
	if err != nil {
		panic(err)
	}
	lib.templates[name] = tmpl
	for _, name := range altNames {
		lib.templates[name] = tmpl
	}
	return tmpl
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
