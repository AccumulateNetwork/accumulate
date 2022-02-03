package main

import (
	_ "embed"
	"fmt"
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
	IsTxResult      bool
	IsBinary        bool
	IsComparable    bool
	MakeConstructor bool
	ChainType       string
	TransactionType string
	Embeddings      []*Type
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

func convert(types typegen.DataTypes, pkgName, pkgPath string) (*Types, error) {
	ttypes := new(Types)
	ttypes.Package = pkgName
	PackagePath = pkgPath
	ttypes.Types = make([]*Type, len(types))
	lup := map[string]*Type{}

	for i, typ := range types {
		ttyp := new(Type)
		ttypes.Types[i] = ttyp
		lup[typ.Name] = ttyp
		ttyp.Name = typ.Name
		ttyp.IsChain = typ.Kind == "chain"
		ttyp.IsTransaction = typ.Kind == "tx"
		ttyp.IsTxResult = typ.Kind == "tx-result"
		ttyp.IsBinary = !typ.NonBinary
		ttyp.IsComparable = !typ.Incomparable
		ttyp.ChainType = typ.ChainType
		ttyp.TransactionType = typ.TxType
		ttyp.Fields = make([]*Field, len(typ.Fields))
		ttyp.MakeConstructor = !typ.OmitNewFunc
		for i, field := range typ.Fields {
			ttyp.Fields[i] = convertField(field)
		}
	}

	for i, typ := range types {
		ttyp := ttypes.Types[i]
		ttyp.Embeddings = make([]*Type, len(typ.Embeddings))
		for i, name := range typ.Embeddings {
			etyp, ok := lup[name]
			if !ok {
				return nil, fmt.Errorf("unknown embedded type %s", name)
			}
			ttyp.Embeddings[i] = etyp
		}
	}

	return ttypes, nil
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

func makeMap(v ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(v)/2)
	for len(v) > 1 {
		m[fmt.Sprint(v[0])] = v[1]
		v = v[2:]
	}
	return m
}

var Templates = typegen.NewTemplateLibrary(template.FuncMap{
	"lcName": lcName,
	"map":    makeMap,
})
