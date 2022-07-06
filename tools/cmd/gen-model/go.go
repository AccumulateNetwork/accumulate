package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

//go:embed go.go.tmpl
var goSrc string

func init() {
	Templates.Register(goSrc, "go", goFuncs, "Go")
}

var goFuncs = template.FuncMap{
	"hasChains":       hasChains,
	"fieldType":       fieldType,
	"recordType":      recordType,
	"stateType":       stateType,
	"parameterType":   parameterType,
	"unionMethod":     unionMethod,
	"chainName":       chainName,
	"valueNameFormat": func(r typegen.Record) string { s, _ := valueNameFormat(r); return s },
	"chainNameFormat": func(r typegen.Record) string { s, _ := chainNameFormat(r); return s },
	"parameterized":   func(r typegen.Record) bool { return len(r.GetParameters()) > 0 },
	"parameterCount":  func(r typegen.Record) int { return len(r.GetParameters()) },
	"add":             func(x, y int) int { return x + y },
	"isBaseType":      func(r *typegen.EntityRecord) bool { return r.Parent == nil && !r.Root },
}

func hasChains(r typegen.Record) bool {
	switch r := r.(type) {
	case *typegen.ChainRecord:
		return true
	case *typegen.OtherRecord:
		return r.HasChains
	case *typegen.EntityRecord:
		for _, p := range r.Attributes {
			if hasChains(p) {
				return true
			}
		}
	}
	return false
}

func fieldType(r typegen.Record) string {
	if len(r.GetParameters()) == 0 {
		return "*" + recordType(r)
	}
	return "map[storage.Key]*" + recordType(r)
}

func recordType(r typegen.Record) string {
	switch r := r.(type) {
	case typegen.ValueRecord:
		var typ string
		switch r.CollectionType() {
		case typegen.CollectionTypeSet:
			typ = "record.Set"
		case typegen.CollectionTypeList:
			typ = "record.List"
		case typegen.CollectionTypeCounted:
			typ = "record.Counted"
		default:
			typ = "record.Value"
		}
		return fmt.Sprintf("%s[%s]", typ, stateType(r, false))
	case *typegen.ChainRecord:
		return "managed.Chain"
	case *typegen.OtherRecord:
		return r.DataType
	default:
		return r.FullName()
	}
}

func unionMethod(r typegen.ValueRecord, name string) string {
	parts := strings.SplitN(r.GetDataType().String(), ".", 2)
	if len(parts) == 1 {
		return name + parts[0]
	}
	return fmt.Sprintf("%s.%s%s", parts[0], name, parts[1])
}

func stateType(r typegen.ValueRecord, forNew bool) string {
	typ := r.GetDataType().GoType()
	if !forNew && r.IsPointer() {
		typ = "*" + typ
	}
	return typ
}

func parameterType(p *typegen.Field) string {
	typ := p.Type.GoType()
	if p.Pointer {
		typ = "*" + typ
	}
	return typ
}

func parameterFormatters(r typegen.Record, keyDepth int) []string {
	var formatters []string
	for i, p := range r.GetParameters() {
		r := 'v'
		switch p.Type.Code {
		case typegen.TypeCodeHash, typegen.TypeCodeBytes:
			r = 'x'
		}
		formatters = append(formatters, fmt.Sprintf("%%[%d]%c", keyDepth+i+1, r))
	}
	return formatters
}

func valueNameFormat(r typegen.Record) (string, int) {
	if r.GetParent() == nil {
		return "", 0
	}

	name, keyDepth := valueNameFormat(r.GetParent())
	if name != "" {
		name += " "
	}

	name += typegen.Natural(r.GetName())
	formatters := parameterFormatters(r, keyDepth+1)
	if len(formatters) == 0 {
		return name, keyDepth + 1
	}

	name += " " + strings.Join(formatters, " ")
	return name, keyDepth + 1 + len(formatters)
}

func chainName(r typegen.Record) string {
	name := r.GetName()
	if strings.HasSuffix(name, "Chain") {
		name = name[:len(name)-len("Chain")]
	}
	return typegen.DashCase(name)
}

func chainNameFormat(r typegen.Record) (string, int) {
	if r.GetParent() == nil {
		return "", 0
	}

	if r.FullName() == "Account" {
		return "", 2
	}

	name, keyDepth := chainNameFormat(r.GetParent())
	if name != "" {
		name += "-"
	}

	rname := r.GetName()
	if strings.HasSuffix(rname, "Chain") {
		rname = rname[:len(rname)-len("Chain")]
	}
	rname = typegen.DashCase(rname)

	name += rname
	formatters := parameterFormatters(r, keyDepth+1)
	if len(formatters) == 0 {
		return name, keyDepth + 1
	}

	name += "(" + strings.Join(formatters, ",") + ")"
	return name, keyDepth + 1 + len(formatters)
}
