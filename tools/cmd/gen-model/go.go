// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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
	"fullName":        fullName,
	"hasChains":       hasChains,
	"fieldType":       fieldType,
	"recordType":      recordType,
	"stateType":       stateType,
	"parameterType":   parameterType,
	"keyType":         keyType,
	"asKey":           asKey,
	"keyToString":     keyToString,
	"unionMethod":     unionMethod,
	"chainName":       chainName,
	"chainNameFormat": func(r typegen.Record) string { s, _ := chainNameFormat(r); return s },
	"parameterized":   func(r typegen.Record) bool { return len(r.GetParameters()) > 0 },
	"parameterCount":  func(r typegen.Record) int { return len(r.GetParameters()) },
	"add":             func(x, y int) int { return x + y },
	"error": func(format string, args ...any) (string, error) {
		return "", fmt.Errorf(format, args...)
	},
}

func fullName(r typegen.Record) string {
	e, ok := r.(*typegen.EntityRecord)
	if !ok || !e.Interface {
		return r.FullName()
	}
	name := e.FullName()
	return strings.ToLower(name[:1]) + name[1:]
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
		return recordType(r, true)
	}
	return "map[" + typegen.LowerFirstWord(r.FullName()) + "Key]" + recordType(r, true)
}

func recordType(r typegen.Record, noInterface bool) string {
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
		return "*database.Chain"
	case *typegen.OtherRecord:
		return "*" + r.DataType
	case *typegen.EntityRecord:
		if !noInterface && r.Interface {
			return r.FullName()
		}
		return "*" + fullName(r)
	default:
		return "*" + r.FullName()
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

func keyType(p *typegen.Field) string {
	switch p.Type.Code {
	case typegen.TypeCodeBytes,
		typegen.TypeCodeUrl:
		return "[32]byte"
	default:
		return p.Type.GoType()
	}
}

func asKey(p *typegen.Field, varName string) string {
	switch p.Type.Code {
	case typegen.TypeCodeBytes:
		return "record.MapKeyBytes(" + varName + ")"
	case typegen.TypeCodeUrl:
		return "record.MapKeyUrl(" + varName + ")"
	default:
		return varName
	}
}

func keyToString(p *typegen.Field, varName string) string {
	switch p.Type.Code {
	case typegen.TypeCodeInt:
		return "strconv.FormatInt(" + varName + ", 10)"
	case typegen.TypeCodeUint:
		return "strconv.FormatUint(" + varName + ", 10)"
	case typegen.TypeCodeBool:
		return "strconv.FormatBool(" + varName + ")"
	case typegen.TypeCodeString:
		return varName
	case typegen.TypeCodeHash:
		return "hex.EncodeToString(" + varName + "[:])"
	case typegen.TypeCodeBytes:
		return "hex.EncodeToString(" + varName + ")"
	case typegen.TypeCodeFloat:
		return "strconv.FormatFloat(" + varName + ", 'g', 3, 10)"
	case typegen.TypeCodeUrl,
		typegen.TypeCodeTxid:
		return varName + ".RawString()"
	case typegen.TypeCodeTime,
		typegen.TypeCodeDuration,
		typegen.TypeCodeBigInt:
		fallthrough
	default:
		return varName + ".String()"
	}
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

func chainName(r typegen.Record) string {
	name := r.GetName()
	name = strings.TrimSuffix(name, "Chain")
	return typegen.DashCase(name)
}

func chainNameFormat(r typegen.Record) (string, int) {
	if e, ok := r.(*typegen.EntityRecord); ok && (e == nil || e.Root) {
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
	rname = strings.TrimSuffix(rname, "Chain")
	rname = typegen.DashCase(rname)

	name += rname
	formatters := parameterFormatters(r, keyDepth+1)
	if len(formatters) == 0 {
		return name, keyDepth + 1
	}

	name += "(" + strings.Join(formatters, ",") + ")"
	return name, keyDepth + 1 + len(formatters)
}
