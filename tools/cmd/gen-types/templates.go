package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

var PackagePath string

type Types struct {
	Package string
	Types   []*Type
	Unions  []*UnionSpec
}

type Type struct {
	typegen.DataType
	Embeddings []Embedding
	Fields     []*Field
	UnionSpec  *UnionSpec
}

func (t *Type) IsAccount() bool       { return t.Union.Type == "account" }
func (t *Type) IsBinary() bool        { return !t.NonBinary }
func (t *Type) IsComparable() bool    { return !t.Incomparable }
func (t *Type) MakeConstructor() bool { return !t.OmitNewFunc }

// Deprecated: use Union
func (t *Type) Kind() string {
	switch t.Union.Type {
	case "account":
		return "chain"
	case "transaction":
		return "tx"
	case "result":
		return "tx-result"
	default:
		return t.Union.Type
	}
}

func (t *Type) IsUnion() bool {
	return t.Union.Type != ""
}

func (t *Type) UnionType() string {
	if t.UnionSpec == nil {
		return ""
	}
	return t.UnionSpec.Enumeration()
}

func (t *Type) UnionValue() string {
	return t.Union.Value
}

type UnionSpec struct {
	Type    string
	Members []*Type
}

func (u *UnionSpec) Interface() string {
	switch u.Type {
	case "transaction":
		return "TransactionBody"
	default:
		return strings.Title(u.Type)
	}
}

func (u *UnionSpec) Enumeration() string {
	if u.Type == "" {
		return ""
	}
	switch u.Type {
	case "result":
		return "TransactionType"
	}
	return strings.Title(u.Type) + "Type"
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
	unions := map[string]*UnionSpec{}

	for i, typ := range types {
		ttyp := new(Type)
		ttyp.DataType = *typ
		ttypes.Types[i] = ttyp
		lup[typ.Name] = ttyp
		ttyp.Fields = make([]*Field, len(typ.Fields))
		for i, field := range typ.Fields {
			ttyp.Fields[i] = &Field{Field: *field}
		}

		if typ.Union.Type == "" {
			continue
		}

		union, ok := unions[typ.Union.Type]
		if !ok {
			union = new(UnionSpec)
			union.Type = typ.Union.Type
			ttypes.Unions = append(ttypes.Unions, union)
			unions[typ.Union.Type] = union
		}

		ttyp.UnionSpec = union
		union.Members = append(union.Members, ttyp)
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

func natural(name string) string {
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

func makeMap(v ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(v)/2)
	for len(v) > 1 {
		m[fmt.Sprint(v[0])] = v[1]
		v = v[2:]
	}
	return m
}

var Templates = typegen.NewTemplateLibrary(template.FuncMap{
	"lcName":  lcName,
	"map":     makeMap,
	"natural": natural,
})
