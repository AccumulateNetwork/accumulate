// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"fmt"
	"path"
	"regexp"
	"strings"
	"text/template"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

var PackagePath string

var reVersion = regexp.MustCompile(`^v\d+$`)

// convert converts typegen.Types to local Types.
func convert(types, refTypes typegen.Types, pkgName, subPkgName string) (*Types, error) {
	// Initialize
	ttypes := new(Types)
	ttypes.Package = pkgName
	ttypes.LongUnionDiscriminator = flags.LongUnionDiscriminator
	lup := map[string]*Type{}
	unions := map[string]*UnionSpec{}

	// Fixup reference names
	for _, typ := range refTypes {
		if typ.Package == PackagePath {
			continue
		}
		for _, field := range typ.Fields {
			if field.Type.Code != typegen.TypeCodeUnknown {
				continue
			}
			if !strings.ContainsRune(field.Type.Name, '.') {
				base := path.Base(typ.Package)
				// If the package is foo/v3, use 'foo' instead of 'v3'
				if reVersion.MatchString(base) {
					base = path.Base(typ.Package[:len(typ.Package)-len(base)-1])
				}
				field.Type.Name = base + "." + field.Type.Name
			}
		}
	}

	// Convert types
	for _, typ := range append(types, refTypes...) {
		// Initialize
		ttyp := new(Type)
		ttyp.Type = *typ
		ttyp.SubPackage = subPkgName
		ttypes.Types = append(ttypes.Types, ttyp)
		lup[typ.Name] = ttyp
		lup[path.Base(typ.Package)+"."+typ.Name] = ttyp
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
			union.Package = pkgName
			union.SubPackage = subPkgName
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
			if flags.LongUnionDiscriminator {
				field.Name = typ.UnionType()
			} else {
				field.Name = "Type"
			}
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
			if flags.ExpandEmbedded {
				for _, embField := range etyp.Type.Fields {
					field := new(Field)
					field.Name = embField.Name
					field.Type.SetNamed(embField.Type.Name)
					field.Type = embField.Type
					field.Repeatable = embField.Repeatable
					typ.Fields = append(typ.Fields, field)
				}
			} else {
				field := new(Field)
				field.Type.SetNamed(name)
				field.TypeRef = etyp
				typ.Fields = append(typ.Fields, field)
			}
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

	for _, typ := range ttypes.Types {
		for _, field := range typ.Fields {
			field.ParentType = typ
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
	Package                string
	LongUnionDiscriminator bool
	Types                  []*Type
	Unions                 []*UnionSpec
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
	Package    string
	Name       string
	Type       string
	Members    []*Type
	SubPackage string
}

type Type struct {
	typegen.Type
	Fields     []*Field
	SubPackage string
	UnionSpec  *UnionSpec
}

type Field struct {
	typegen.Field
	TypeRef    *Type
	ParentType *Type
	IsEmbedded bool
}

func (t *Type) IsAccount() bool    { return t.Union.Type == "account" }
func (t *Type) IsBinary() bool     { return !t.NonBinary }
func (t *Type) IsComparable() bool { return !t.Incomparable }
func (t *Type) IsCopyable() bool   { return !(t.Incomparable || t.NoCopy) }

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
	}
	return typegen.TitleCase(u.Name)
}

func (u *UnionSpec) Enumeration() string {
	if u.Type == "" {
		return ""
	}
	switch u.Type {
	case "result":
		return "TransactionType"
	}
	if flags.ElidePackageType && strings.EqualFold(u.Name, u.Package) {
		return "Type"
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
	"hasSuffix":           strings.HasSuffix,
	"debug":               fmt.Printf,
})
