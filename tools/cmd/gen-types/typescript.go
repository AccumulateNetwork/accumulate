// Copyright 2023 The Accumulate Authors
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
	typ := field.Type.TypescriptType()
	if field.Repeatable {
		typ = typ + "[]"
	}
	return typ
}

func TsInputType(field *Field) string {
	typ := field.Type.TypescriptInputType()
	switch field.MarshalAs {
	case Reference, Value, Union:
		typ = field.Type.Name + " | " + field.Type.Name + ".Args"
	case Enum:
		typ = field.Type.Name + ".Args"
	}
	if field.Repeatable {
		typ = "(" + typ + ")[]"
	}
	return typ
}

func TsObjectify(field *Field, varName string) string {
	// This is a hack
	if field.Type.Name == "AllowedTransactions" {
		return fmt.Sprintf("%s.map(v => v.toString())", varName)
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
		return fmt.Sprintf("%s.toString()", varName)
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
	case BigInt, Url, TxID, Time:
		return fmt.Sprintf("%s instanceof %s ? %[1]s : new %[2]s(%[1]s)", varName, field.Type.TypescriptType())
	case Bytes, Hash:
		return fmt.Sprintf("%s instanceof Uint8Array ? %[1]s : Buffer.from(%[1]s, 'hex')", varName)
	}

	switch field.MarshalAs {
	case Reference, Value:
		return fmt.Sprintf("%s instanceof %s ? %[1]s : new %[2]s(%[1]s)", varName, field.Type.Name)
	case Enum, Union:
		return fmt.Sprintf("%s.fromObject(%s)", field.Type.Name, varName)
	}

	return varName
}
