package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

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
			union.Name = typ.Union.Name
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
			field.ParentTypeName = typ.Name
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
			tfield.ParentTypeName = typ.Name
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

type SingleTypeFile struct {
	Package string
	*Type
}

func (f *SingleTypeFile) IsUnion() bool { return false }

type SingleUnionFile struct {
	Package string
	*UnionSpec
}

func (f *SingleUnionFile) IsUnion() bool { return true }

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
	TypeRef        *Type
	IsEmbedded     bool
	ParentTypeName string
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

var Templates = typegen.NewTemplateLibrary(template.FuncMap{
	"lcName":              typegen.LowerFirstWord,
	"upper":               strings.ToUpper,
	"underscoreUpperCase": typegen.UnderscoreUpperCase,
	"title":               typegen.TitleCase,
	"map":                 typegen.MakeMap,
	"natural":             typegen.Natural,
	"afterDot":            typegen.AfterDot,
	"debug":               fmt.Printf,
})
