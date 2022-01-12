package main

import (
	_ "embed"
	"strings"
	"text/template"

	"github.com/AccumulateNetwork/accumulate/tools/internal/typegen"
)

var PackagePath string

type Types struct {
	Package string
	Types   []*Type
}

type Type struct {
	Name            string
	IsChain         bool
	IsTransaction   bool
	IsVal           bool
	IsBinary        bool
	IsComparable    bool
	MakeConstructor bool
	ChainType       string
	TransactionType string
	ValType         string
	Embeddings      []string
	Fields          []*Field
}

type Field struct {
	Name            string
	AlternativeName string
	Type            string
	IsPointer       bool
	IsOptional      bool
	IsUrl           bool
	IsMarshalled    bool
	AsReference     bool
	AsValue         bool
	OmitEmpty       bool
	UnmarshalWith   string
	Slice           *Field
}

func convert(types typegen.DataTypes, pkgName, pkgPath string) *Types {
	ttypes := new(Types)
	ttypes.Package = pkgName
	PackagePath = pkgPath
	ttypes.Types = make([]*Type, len(types))

	for i, typ := range types {
		ttyp := new(Type)
		ttypes.Types[i] = ttyp
		ttyp.Name = typ.Name
		ttyp.IsChain = typ.Kind == "chain"
		ttyp.IsTransaction = typ.Kind == "tx"
		ttyp.IsVal = typ.Kind == "val"
		ttyp.IsBinary = !typ.NonBinary
		ttyp.IsComparable = !typ.Incomparable
		ttyp.ChainType = typ.ChainType
		ttyp.TransactionType = typ.TxType
		ttyp.ValType = typ.ValType
		ttyp.Embeddings = typ.Embeddings
		ttyp.Fields = make([]*Field, len(typ.Fields))
		ttyp.MakeConstructor = !typ.OmitNewFunc
		for i, field := range typ.Fields {
			ttyp.Fields[i] = convertField(field)
		}
	}

	return ttypes
}

func convertField(field *typegen.Field) *Field {
	tfield := new(Field)
	tfield.Name = field.Name
	tfield.AlternativeName = field.Alternative
	tfield.Type = field.Type
	tfield.IsPointer = field.Pointer
	tfield.IsOptional = field.Optional
	tfield.IsUrl = field.IsUrl
	tfield.IsMarshalled = field.MarshalAs != "none"
	tfield.AsReference = field.MarshalAs == "reference"
	tfield.AsValue = field.MarshalAs == "value"
	tfield.OmitEmpty = !field.KeepEmpty
	tfield.UnmarshalWith = field.UnmarshalWith
	if field.Slice != nil {
		tfield.Slice = convertField(field.Slice)
	}
	return tfield
}

func lcName(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func mustParseTemplate(name, src string, funcs template.FuncMap) *template.Template {
	tmpl := template.New(name)
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	tmpl, err := tmpl.Parse(src)
	checkf(err, "bad template")
	return tmpl
}
