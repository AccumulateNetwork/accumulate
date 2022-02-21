package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

var PackagePath string

type Types struct {
	Package string
	Types   []*Type
}

type Type struct {
	typegen.DataType
	Embeddings []Embedding
	Fields     []*Field
}

func (t *Type) IsChain() bool           { return t.Kind == "chain" }
func (t *Type) IsAccount() bool         { return t.Kind == "chain" }
func (t *Type) IsTransaction() bool     { return t.Kind == "tx" }
func (t *Type) IsSignature() bool       { return t.Kind == "signature" }
func (t *Type) IsTxResult() bool        { return t.Kind == "tx-result" }
func (t *Type) IsBinary() bool          { return !t.NonBinary }
func (t *Type) IsComparable() bool      { return !t.Incomparable }
func (t *Type) MakeConstructor() bool   { return !t.OmitNewFunc }
func (t *Type) AccountType() string     { return t.ChainType }
func (t *Type) TransactionType() string { return t.TxType }

func (t *Type) IsUnion() bool {
	return t.IsChain() || t.IsAccount() || t.IsTransaction() || t.IsTxResult() || t.IsSignature()
}

func (t *Type) UnionType() string {
	switch t.Kind {
	case "chain":
		return "AccountType"
	case "tx", "tx-result":
		return "TransactionType"
	case "signature":
		return "SignatureType"
	default:
		return ""
	}
}

func (t *Type) UnionValue() string {
	switch t.Kind {
	case "chain":
		return t.ChainType
	case "tx", "tx-result":
		return t.TxType
	case "signature":
		return t.SignatureType
	default:
		return ""
	}
}

type Embedding struct {
	*Type
	Number uint
}

type Field struct {
	typegen.Field
	Number uint
}

func (f *Field) AlternativeName() string { return f.Alternative }
func (f *Field) IsPointer() bool         { return f.Pointer }
func (f *Field) IsMarshalled() bool      { return f.MarshalAs != "none" }
func (f *Field) AsReference() bool       { return f.MarshalAs == "reference" }
func (f *Field) AsValue() bool           { return f.MarshalAs == "value" }
func (f *Field) AsEnum() bool            { return f.MarshalAs == "enum" }
func (f *Field) IsOptional() bool        { return f.Optional }
func (f *Field) IsRequired() bool        { return !f.Optional }
func (f *Field) OmitEmpty() bool         { return !f.KeepEmpty }

func convert(types typegen.DataTypes, pkgName, pkgPath string) (*Types, error) {
	ttypes := new(Types)
	ttypes.Package = pkgName
	PackagePath = pkgPath
	ttypes.Types = make([]*Type, len(types))
	lup := map[string]*Type{}

	for i, typ := range types {
		ttyp := new(Type)
		ttyp.DataType = *typ
		ttypes.Types[i] = ttyp
		lup[typ.Name] = ttyp
		ttyp.Fields = make([]*Field, len(typ.Fields))
		for i, field := range typ.Fields {
			ttyp.Fields[i] = &Field{Field: *field}
		}
	}

	for i, typ := range types {
		ttyp := ttypes.Types[i]
		ttyp.Embeddings = make([]Embedding, len(typ.Embeddings))
		for i, name := range typ.Embeddings {
			etyp, ok := lup[name]
			if !ok {
				return nil, fmt.Errorf("unknown embedded type %s", name)
			}
			ttyp.Embeddings[i].Type = etyp
		}
	}

	for _, typ := range ttypes.Types {
		var num uint = 1
		if typ.IsUnion() {
			num += 1
		}
		for i := range typ.Embeddings {
			typ.Embeddings[i].Number = num
			num++
		}
		for _, field := range typ.Fields {
			if !field.IsMarshalled() {
				continue
			}
			field.Number = num
			num++
		}
	}

	return ttypes, nil
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
