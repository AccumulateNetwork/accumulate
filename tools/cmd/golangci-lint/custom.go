package main

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/golangci/golangci-lint/pkg/golinters/goanalysis"
	"github.com/golangci/golangci-lint/pkg/lint/linter"
	"github.com/golangci/golangci-lint/pkg/lint/lintersdb"
	"golang.org/x/tools/go/analysis"
)

func addCustomLinters(db *lintersdb.Manager) {
	field, ok := reflect.TypeOf(db).Elem().FieldByName("nameToLCs")
	if !ok {
		panic(fmt.Errorf("Can't find linter config field"))
	}

	// This is a horrific abuse of Go. But using a plugin would be a huge PITA.
	nameToLCs := *(*map[string][]*linter.Config)(unsafe.Pointer(uintptr(unsafe.Pointer(db)) + field.Offset))

	addLinterFunc(nameToLCs, "noprint", "Checks for out of place print statements", noprint)
}

func addLinterFunc(configs map[string][]*linter.Config, name, doc string, run ...func(*analysis.Pass) (interface{}, error)) {
	var a []*analysis.Analyzer
	for _, run := range run {
		a = append(a, &analysis.Analyzer{
			Name: name,
			Doc:  doc,
			Run:  run,
		})
	}

	l := goanalysis.NewLinter(name, doc, a, nil)
	l.WithLoadMode(goanalysis.LoadModeSyntax)

	lc := linter.NewConfig(l)
	configs[lc.Name()] = append(configs[lc.Name()], lc)
}
