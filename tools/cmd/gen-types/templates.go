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

// convert converts typegen.Types to local Types.
func convert(types typegen.Types, pkgName, pkgPath string) (*Types, error) {
	// Initialize
	ttypes := new(Types)
	ttypes.Package = pkgName
	PackagePath = pkgPath
	ttypes.Types = make([]*Type, len(types))
	lup := map[string]*Type{}
	unions := map[string]*UnionSpec{}

	// Convert types
	for i, typ := range types {
		// Initialize
		ttyp := new(Type)
		ttyp.Type = *typ
		ttypes.Types[i] = ttyp
		lup[typ.Name] = ttyp
		ttyp.Fields = make([]*Field, 0, len(typ.Fields)+len(typ.Embeddings))

		// If the type is a union
		if typ.Union.Type == "" {
			continue
		}

		// Look up or define the union spec
		union, ok := unions[typ.Union.Type]
		if !ok {
			union = new(UnionSpec)
			union.Type = typ.Union.Type
			ttypes.Unions = append(ttypes.Unions, union)
			unions[typ.Union.Type] = union
		}

		// Set the union spec, add the type
		ttyp.UnionSpec = union
		union.Members = append(union.Members, ttyp)
	}

	for i, typ := range types {
		ttyp := ttypes.Types[i]
		var num uint = 1

		// Unions have a virtual field for the discriminator
		if ttyp.IsUnion() {
			field := new(Field)
			field.Number = num
			field.Name = "Type"
			field.Type = ttyp.UnionSpec.Enumeration()
			field.MarshalAs = "enum"
			field.KeepEmpty = true
			field.Virtual = true
			ttyp.Fields = append(ttyp.Fields, field)
			num++
		}

		// Resolve embedded types
		for _, name := range typ.Embeddings {
			etyp, ok := lup[name]
			if !ok {
				return nil, fmt.Errorf("unknown embedded type %s", name)
			}
			field := new(Field)
			field.Type = name
			field.TypeRef = etyp
			if field.IsMarshalled() {
				field.Number = num
				num++
			}
			ttyp.Fields = append(ttyp.Fields, field)
		}

		// Convert fields
		for _, field := range typ.Fields {
			tfield := new(Field)
			tfield.Field = *field
			if field.MarshalAs != "" {
				tfield.TypeRef = lup[tfield.Type]
			}
			if tfield.IsMarshalled() {
				tfield.Number = num
				num++
			}
			ttyp.Fields = append(ttyp.Fields, tfield)
		}
	}

	return ttypes, nil
}

type Types struct {
	Package string
	Types   []*Type
	Unions  []*UnionSpec
}

type UnionSpec struct {
	Type    string
	Members []*Type
}

type Type struct {
	typegen.Type
	Fields    []*Field
	UnionSpec *UnionSpec
}

type Field struct {
	typegen.Field
	TypeRef *Type
	Number  uint
}

func (t *Type) IsAccount() bool    { return t.Union.Type == "account" }
func (t *Type) IsBinary() bool     { return !t.NonBinary }
func (t *Type) IsComparable() bool { return !t.Incomparable }

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

func (f *Field) IsEmbedded() bool        { return f.Name == "" }
func (f *Field) AlternativeName() string { return f.Alternative }
func (f *Field) IsPointer() bool         { return f.Pointer }
func (f *Field) IsMarshalled() bool      { return f.MarshalAs != "none" }
func (f *Field) AsReference() bool       { return f.MarshalAs == "reference" }
func (f *Field) AsValue() bool           { return f.MarshalAs == "value" }
func (f *Field) AsEnum() bool            { return f.MarshalAs == "enum" }
func (f *Field) IsOptional() bool        { return f.Optional }
func (f *Field) IsRequired() bool        { return !f.Optional }
func (f *Field) OmitEmpty() bool         { return !f.KeepEmpty }

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
	"debug":   fmt.Printf,
})
