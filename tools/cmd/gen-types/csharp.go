// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"strings"
	"text/template"
)

//go:embed types.csharp.tmpl
var csSrc string

var csFuncs = template.FuncMap{
	"resolveType": func(field *Field) string {
		// For now, simply return the field's type name.
		// You can expand this function to perform a proper conversion to C# types.
		return field.Type.Name
	},
	"safeClassName": func(name string) string {
		// For example, if the name is "Object", return "ProtocolObject"
		if name == "Object" {
			return "ProtocolObject"
		}
		// Otherwise return the name unchanged
		return name
	},
	"lowerCamel": func(s string) string {
		if s == "" {
			return ""
		}
		return strings.ToLower(s[:1]) + s[1:]
	},
	"parentTypeName": func(f *Field) string {
		if f.ParentType == nil {
			return ""
		}
		return f.ParentType.Name
	},
	// Add additional helper functions as needed.
}

func init() {
	// Register the C# templates for types and unions with the C# function map.
	Templates.Register(csSrc, "csharp", csFuncs, "CSharp", "csharp-union", "CSharp-union")
}
