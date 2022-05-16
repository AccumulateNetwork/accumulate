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
func convert(types, refTypes typegen.Types, pkgName, pkgPath string) (*Types, error) {
	// Initialize
	ttypes := new(Types)
	ttypes.Package = pkgName
	PackagePath = pkgPath
	lup := map[string]*Type{}
	unions := map[string]*UnionSpec{}

	// Convert types
	for _, typ := range append(types, refTypes...) {
		// Initialize
		ttyp := new(Type)
		ttyp.Type = *typ
		ttypes.Types = append(ttypes.Types, ttyp)
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
			if typ.Union.Name == "" {
				union.Name = typ.Union.Type
			} else {
				union.Name = typ.Union.Name
			}
			union.Type = typ.Union.Type
			ttypes.Unions = append(ttypes.Unions, union)
			unions[typ.Union.Type] = union
		}

		// Set the union spec, add the type
		ttyp.UnionSpec = union
		union.Members = append(union.Members, ttyp)
	}

	for _, typ := range ttypes.Types {
		// Unions have a virtual field for the discriminator
		if typ.IsUnion() {
			field := new(Field)
			field.Name = "Type"
			field.Type.SetNamed(typ.UnionSpec.Enumeration())
			field.MarshalAs = typegen.MarshalAsEnum
			field.KeepEmpty = true
			field.Virtual = true
			typ.Fields = append(typ.Fields, field)
		}

		// Resolve embedded types
		for _, name := range typ.Type.Embeddings {
			etyp, ok := lup[name]
			if !ok {
				return nil, fmt.Errorf("unknown embedded type %s", name)
			}
			field := new(Field)
			field.Type.SetNamed(name)
			field.TypeRef = etyp
			typ.Fields = append(typ.Fields, field)
		}

		// Convert fields
		for _, field := range typ.Type.Fields {
			tfield := new(Field)
			tfield.Field = *field
			if field.MarshalAs != typegen.MarshalAsBasic {
				tfield.TypeRef = lup[tfield.Type.String()]
			}
			typ.Fields = append(typ.Fields, tfield)
		}
	}

	// Make names for embedded fields
	for _, typ := range ttypes.Types {
		for _, field := range typ.Fields {
			if field.Name != "" {
				continue
			}

			field.IsEmbedded = true
			bits := strings.Split(field.Type.String(), ".")
			if len(bits) == 1 {
				field.Name = field.Type.String()
			} else {
				field.Name = bits[1]
			}
		}
	}

	// Number the fields
	for _, typ := range ttypes.Types {
		taken := make(map[uint]bool, len(typ.Fields))
		var next uint = 1
		for _, field := range typ.Fields {
			if !field.IsMarshalled() || !field.IsBinary() {
				continue
			}

			// Use the next available number
			if field.Number == 0 {
				field.Number = next
			} else {
				next = field.Number
			}
			next++

			// Make sure we're not doubling up
			if taken[field.Number] {
				return nil, fmt.Errorf("duplicate field number %d", field.Number)
			}

			taken[field.Number] = true
		}
	}

	// Slice out reference types
	ttypes.Types = ttypes.Types[:len(types)]
	return ttypes, nil
}

type Types struct {
	Package string
	Types   []*Type
	Unions  []*UnionSpec
}

type UnionSpec struct {
	Name    string
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
	TypeRef    *Type
	IsEmbedded bool
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
	switch u.Name {
	case "transaction":
		return "TransactionBody"
	default:
		return typegen.TitleCase(u.Name)
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
	return typegen.TitleCase(u.Type) + "Type"
}

func (f *Field) IsBinary() bool          { return !f.NonBinary }
func (f *Field) AlternativeName() string { return f.Alternative }
func (f *Field) IsPointer() bool         { return f.Pointer }
func (f *Field) IsMarshalled() bool      { return f.MarshalAs != typegen.MarshalAsNone }
func (f *Field) AsReference() bool       { return f.MarshalAs == typegen.MarshalAsReference }
func (f *Field) AsValue() bool           { return f.MarshalAs == typegen.MarshalAsValue }
func (f *Field) AsEnum() bool            { return f.MarshalAs == typegen.MarshalAsEnum }
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
