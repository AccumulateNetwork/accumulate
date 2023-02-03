// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:generate go run ../../cmd/gen-enum --package typegen --out enums_gen.go enums.yml
//go:generate go run ../../cmd/gen-types --package typegen --out types_gen.go types.yml
//go:generate go run ../../cmd/gen-types --package typegen --language go-union --out unions_gen.go types.yml

type MarshalAs int

type TypeCode int

type FieldType struct {
	Code TypeCode
	Name string
}

func (f *FieldType) Equal(g *FieldType) bool {
	if f == g {
		return true
	}
	return f != nil && g != nil && *f == *g
}

func (f *FieldType) Title() string {
	return TitleCase(f.String())
}

func (f *FieldType) IsKnown() bool {
	return f.Code != TypeCodeUnknown
}

func (f *FieldType) SetKnown(code TypeCode) {
	*f = FieldType{Code: code}
}

func (f *FieldType) SetNamed(name string) {
	*f = FieldType{Name: name}
}

func (f FieldType) String() string {
	if f.Name != "" {
		return f.Name
	}
	return f.Code.String()
}

func (f FieldType) MarshalYAML() (interface{}, error) {
	return f.String(), nil
}

func (f *FieldType) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}

	if strings.HasSuffix(s, "!") {
		f.SetNamed(s[:len(s)-1])
		return nil
	}

	code, ok := TypeCodeByName(s)
	if ok {
		f.SetKnown(code)
	} else {
		f.SetNamed(s)
	}
	return nil
}

func (f FieldType) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

func (f *FieldType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	code, ok := TypeCodeByName(s)
	if ok {
		f.SetKnown(code)
	} else {
		f.SetNamed(s)
	}
	return nil
}

func (m *MarshalAs) MarshalYAML() (interface{}, error) {
	return m.String(), nil
}

func (m *MarshalAs) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}

	v, ok := MarshalAsByName(s)
	if !ok {
		return fmt.Errorf("cannot unmarshal %q as MarshalAs", s)
	}
	*m = v
	return nil
}

// Types is a list of struct types.
type Types []*Type

func (t Types) Sort() {
	sort.Slice(t, func(i, j int) bool {
		return t[i].Name < t[j].Name
	})
}

func (t *Types) Unmap(types map[string]*Type, files map[*Type]string, packages map[string]string) error {
	seen := map[string]bool{}
	for _, t := range *t {
		seen[t.Name] = true
	}

	if *t == nil {
		*t = make(Types, 0, len(types))
	} else {
		*t = append(*t, make(Types, 0, len(types))...)
	}
	for name, typ := range types {
		err := typ.parseName(name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		name := typ.Name
		if seen[name] {
			return fmt.Errorf("duplicate entries for %q", name)
		}
		seen[name] = true

		typ.File = files[typ]
		typ.Package = packages[filepath.Dir(typ.File)]
		*t = append(*t, typ)

		if typ.Union.Name == "" {
			typ.Union.Name = typ.Union.Type
		}

		if typ.Union.Name != "" && typ.Union.Value == "" {
			typ.Union.Value = strings.TrimSuffix(name, TitleCase(typ.Union.Name))
		}

		for _, field := range typ.Fields {
			if field.Type.Code == TypeCodeUnknown && field.Type.Name == "" {
				if field.Name == "" {
					return fmt.Errorf("an unnamed field of %s does not have a type", typ.Name)
				} else {
					return fmt.Errorf("%s.%s does not have a type", typ.Name, field.Name)
				}
			}
		}
	}
	return nil
}

// Type is a struct type.
type Type struct {
	// Name is the name of the type.
	Name string `yaml:"-"`
	// Params are the type parameters of a generic type.
	Params []*TypeParam `yaml:"-"`
	// File is the file the type was loaded from.
	File string `yaml:"-"`
	// Package is the package the type was loaded from.
	Package string `yaml:"-"`
	// Description is the description of the type.
	Description string
	// Union is the tagged union specifier for the type.
	Union Union
	// NonBinary specifies whether the type is binary marshallable.
	NonBinary bool `yaml:"non-binary"`
	// Incomparable specifies whether two values of the type can be checked for equality.
	Incomparable    bool `yaml:"incomparable"`
	NoCopy          bool `yaml:"no-copy"`
	IgnoreSizeLimit bool `yaml:"ignore-size-limit"`
	// CustomIsValid specifies the type defines a custom IsValid() error method.
	CustomIsValid bool `yaml:"custom-is-valid"`
	// CustomEqual specifies the type defines a custom Equal() error method.
	CustomEqual bool `yaml:"custom-equal"`
	// Fields is a list of fields.
	Fields []*Field
	// Embeddings is a list of embedded types.
	Embeddings []string `yaml:"embeddings"`
}

func (t *Type) parseName(s string) error {
	i := strings.IndexByte(s, '[')
	if i < 0 {
		t.Name = s
		return nil
	}

	if s[len(s)-1] != ']' {
		return errors.New("invalid generic type")
	}

	t.Name = s[:i]
	for _, s := range strings.Split(s[i+1:len(s)-1], ",") {
		s := strings.SplitN(s, " ", 2)
		if len(s) != 2 {
			return errors.New("invalid generic type")
		}
		t.Params = append(t.Params, &TypeParam{Name: s[0], Type: s[1]})
	}
	return nil
}

func (t *Type) FullName(paramName, paramType bool) string {
	if len(t.Params) == 0 {
		return t.Name
	}
	var params []string
	for _, p := range t.Params {
		switch {
		case paramName && paramType:
			params = append(params, p.Name+" "+p.Type)
		case paramName:
			params = append(params, p.Name)
		case paramType:
			params = append(params, p.Type)
		}
	}
	return t.Name + "[" + strings.Join(params, ", ") + "]"
}

func (t *Type) ResolveTypeParam(f *Field) (*TypeParam, bool) {
	s := f.Type.String()
	for _, p := range t.Params {
		if p.Name == s {
			return p, true
		}
	}
	return nil, false
}

// TypeParam is a type parameter of a generic type.
type TypeParam struct {
	Name string
	Type string
}

// Union specifies that a type is part of a tagged union.
type Union struct {
	// Name is the name of the union type.
	Name string
	// Type is the name of the tag type.
	Type string
	// Value is the name of the corresponding enumeration value.
	Value string
}

// Field is a field of a type.
type Field struct {
	// Name is the name of the field.
	Name string
	// Number overrides the default field number.
	Number uint `yaml:"field-number"`
	// Description is the description of the field.
	Description string
	// Type is the type of the field.
	Type FieldType
	// MarshalAs specifies how to marshal the field.
	MarshalAs MarshalAs `yaml:"marshal-as"`
	// Repeatable specifies whether the the field is repeatable (represented as a slice).
	Repeatable bool
	// Pointer specifies whether the field is a pointer.
	Pointer bool
	// Optional specifies whether the field can be omitted.
	Optional bool
	// KeepEmpty specifies whether the field should be marshalled even when set to its zero value.
	KeepEmpty bool `yaml:"keep-empty"`
	// Alternative specifies an alternative name used for text marshalling.
	Alternative string
	// ZeroValue specifies the zero value of the field.
	ZeroValue interface{} `yaml:"zero-value"`
	// ElideUnionMethods specifies whether the union type name should be elided from method names.
	ElideUnionMethods bool `yaml:"elide-union-methods"`
	// Virtual specifies whether the field is implemented as a method instead of an actual field.
	Virtual bool
	// NonBinary specifies whether the field is binary marshallable.
	NonBinary bool `yaml:"non-binary"`
	// Toml specifies the name that should be used for TOML marshalling.
	Toml string
}

// API is an API with a set of methods.
type API map[string]Method

// Method is a method of an API.
type Method struct {
	Kind         string
	Description  string
	Deprecated   string
	Experimental bool
	RPC          string
	Input        string
	Output       string
	Call         string
	RouteParam   string   `yaml:"route-param"`
	CallParams   []string `yaml:"call-params"`
	Validate     []string `yaml:"validate"`
}

// Enum is an enumeration with a set of values.
type Enum map[string]*EnumValue

// EnumValue is a particular value of an enumeration.
type EnumValue struct {
	Value       interface{}
	Description string
	Label       string
	Aliases     []string
}
