// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"fmt"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

//go:embed types.ts.tmpl
var tsTypes string

//go:embed unions.ts.tmpl
var tsUnionsSrc string

func init() {
	Templates.Register(tsTypes, "typescript", tsFuncs, "TypeScript")
	Templates.Register(tsUnionsSrc, "typescript-union", tsFuncs)
}

var tsFuncs = template.FuncMap{
	"resolveType":      TsResolveType,
	"inputType":        TsInputType,
	"objectify":        TsObjectify,
	"unobjectify":      TsUnobjectify,
	"needsCtor":        TsNeedsCtor,
	"encodeAnnotation": TsEncodeAnnotation,
}

func TsEncodeAnnotation(field *Field) string {
	if field.MarshalAsType != typegen.TypeCodeUnknown {
		return field.MarshalAsType.String()
	}
	if field.MarshalAs != typegen.MarshalAsBasic {
		return field.MarshalAs.String()
	}
	return field.Type.Code.String()
}

func TsNeedsCtor(typ *Type) bool {
	for _, f := range typ.Fields {
		if f.Virtual || !f.IsMarshalled() {
			continue
		}
		if !f.IsEmbedded {
			return true
		}
		if TsNeedsCtor(f.TypeRef) {
			return true
		}
	}
	return false
}

func TsResolveType(field *Field) string {
	typ := field.Type.TypescriptType(false)
	if field.Repeatable {
		typ = typ + "[]"
	}
	return typ
}

func TsInputType(field *Field) string {
	var typ string
	switch field.MarshalAs {
	case Reference, Value, Union:
		var args string
		if p := field.TypeParam(); p == nil {
			args = field.Type.TypescriptType(true)
		} else {
			args = p.Type + "Args /* TODO " + p.Type + "Args is too broad */"
		}
		typ = field.Type.TypescriptType(false) + " | " + args
	case Enum:
		typ = field.Type.TypescriptType(true)
	default:
		typ = field.Type.TypescriptInputType(false)
	}
	if field.Repeatable {
		typ = "(" + typ + ")[]"
	}
	return typ
}

func TsObjectify(field *Field, varName string) string {
	// This is a hack
	if field.Type.Name == "AllowedTransactions" {
		return fmt.Sprintf("%s.map(v => TransactionType.getName(v))", varName)
	}

	switch field.Type.Code {
	case BigInt, Url, TxID:
		return fmt.Sprintf("%s.toString()", varName)
	case Bytes, Hash:
		return fmt.Sprintf("Buffer.from(%s).toString('hex')", varName)
	}

	switch field.MarshalAs {
	case Reference, Union, Value:
		return fmt.Sprintf("%s.asObject()", varName)
	case Enum:
		return fmt.Sprintf("%s.getName(%s)", field.Type.Name, varName)
	}

	return varName
}

func TsUnobjectify(field *Field, varName string) string {
	if !field.Repeatable {
		return tsUnobjectify2(field, varName)
	}

	return fmt.Sprintf("%s.map(v => %s)", varName, tsUnobjectify2(field, "v"))
}

func tsUnobjectify2(field *Field, varName string) string {
	switch field.Type.Code {
	case Time:
		return fmt.Sprintf("%s instanceof %s ? %[1]s : new %[2]s(%[1]s)", varName, field.Type.TypescriptType(false))
	case BigInt:
		return fmt.Sprintf("typeof %s === 'bigint' ? %[1]s : BigInt(%[1]s)", varName)
	case Url, TxID:
		return fmt.Sprintf("%s.parse(%s)", field.Type.TypescriptType(false), varName)
	case Bytes, Hash:
		return fmt.Sprintf("%s instanceof Uint8Array ? %[1]s : Buffer.from(%[1]s, 'hex')", varName)
	}

	switch field.MarshalAs {
	case Reference, Value:
		return fmt.Sprintf("%s instanceof %s ? %[1]s : new %[2]s(%[1]s)", varName, field.Type.TypescriptType(false))
	case Enum:
		return fmt.Sprintf("%s.fromObject(%s)", field.Type.Name, varName)
	default:
		return varName
	case Union:
	}

	p := field.TypeParam()
	if p == nil {
		return fmt.Sprintf("%s.fromObject(%s)", field.Type.Name, varName)
	}
	return fmt.Sprintf("<%s>%s.fromObject(%s)", p.Name, p.Type, varName)
}
