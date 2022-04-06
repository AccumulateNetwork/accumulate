package typegen

import (
	"fmt"
	"sort"
	"strings"
)

//go:generate go run ../../cmd/gen-enum --package typegen --out enums_gen.go enums.yml

type MarshalAs int

// Types is a list of struct types.
type Types []*Type

func (t Types) Sort() {
	sort.Slice(t, func(i, j int) bool {
		return t[i].Name < t[j].Name
	})
}

func (t *Types) DecodeFromFile(file string, dec Decoder) error {
	var m map[string]*Type
	err := dec.Decode(&m)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	for _, t := range *t {
		seen[t.Name] = true
	}

	if *t == nil {
		*t = make(Types, 0, len(m))
	} else {
		*t = append(*t, make(Types, 0, len(m))...)
	}
	for name, typ := range m {
		if seen[name] {
			return fmt.Errorf("duplicate entries for %q", name)
		}
		seen[name] = true

		typ.Name = name
		typ.File = file
		*t = append(*t, typ)

		if typ.Union.Type != "" && typ.Union.Value == "" {
			typ.Union.Value = strings.TrimSuffix(name, strings.Title(typ.Union.Type))
		}
	}
	return nil
}

// Type is a struct type.
type Type struct {
	// Name is the name of the type.
	Name string `yaml:"-"`
	// File is the file the type was loaded from.
	File string `yaml:"-"`
	// Description is the description of the type.
	Description string
	// Union is the tagged union specifier for the type.
	Union Union
	// NonBinary specifies whether the type is binary marshallable.
	NonBinary bool `yaml:"non-binary"`
	// Incomparable specifies whether two values of the type can be checked for equality.
	Incomparable bool `yaml:"incomparable"`
	// Fields is a list of fields.
	Fields []*Field
	// Embeddings is a list of embedded types.
	Embeddings []string `yaml:"embeddings"`
}

// Union specifies that a type is part of a tagged union.
type Union struct {
	// Type is the name of the union type.
	Type string
	// Value is the name of the corresponding enumeration value.
	Value string
}

// Field is a field of a type.
type Field struct {
	// Name is the name of the field.
	Name string
	// Description is the description of the field.
	Description string
	// Type is the type of the field.
	Type string
	// MarshalAs specifies how to marshal the field.
	MarshalAs string `yaml:"marshal-as"`
	// UnmarshalWith specifies a function to use to unmarshal the field.
	UnmarshalWith string `yaml:"unmarshal-with"`
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
	// Virtual specifies whether the field is implemented as a method instead of an actual field.
	Virtual bool
	// NonBinary specifies whether the field is binary marshallable.
	NonBinary bool `yaml:"non-binary"`
}

// API is an API with a set of methods.
type API map[string]Method

// Method is a method of an API.
type Method struct {
	Kind        string
	Description string
	Deprecated  string
	RPC         string
	Input       string
	Output      string
	Call        string
	CallParams  []string `yaml:"call-params"`
	Validate    []string `yaml:"validate"`
}

// Enum is an enumeration with a set of values.
type Enum map[string]*EnumValue

// EnumValue is a particular value of an enumeration.
type EnumValue struct {
	Value       interface{}
	Description string
	Aliases     []string
}
