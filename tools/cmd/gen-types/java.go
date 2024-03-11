// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed types.java.tmpl
var javaSrc string

var _ = Templates.Register(javaSrc, "java", javaFuncs, "Java", "java-union", "Java-union")

var javaFuncs = template.FuncMap{
	"resolveType": func(field *Field) string {
		typ := field.Type.JavaType()
		if field.Repeatable {
			typ += "[]"
		}
		return typ
	},
	"safeClassName": func(value string) string {
		return SafeClassName(value)
	},
	"isZero": func(field *Field, varName string) (string, error) {
		if field.Repeatable {
			return fmt.Sprintf("%s == null || %[1]s.length == 0", varName), nil
		}

		if field.IsEmbedded {
			return fmt.Sprintf("%s == null", varName), nil
		}

		switch field.Type.Code {
		case Bytes, RawJson, Hash:
			return fmt.Sprintf("%s == null || %[1]s.length == 0", varName), nil
		case String:
			return fmt.Sprintf("%s == null || %[1]s.length() == 0", varName), nil
		case Any, Url, TxID, Time, Duration:
			return fmt.Sprintf("%s == null", varName), nil
		case Bool:
			return fmt.Sprintf("!%s", varName), nil
		case Uint, Int, Float:
			return fmt.Sprintf("%s == 0", varName), nil
		case BigInt:
			return fmt.Sprintf("(%s).equals(java.math.BigInteger.ZERO)", varName), nil
		}

		switch field.MarshalAs {
		case Reference:
			return fmt.Sprintf("%s == null", varName), nil
		case Enum:
			return fmt.Sprintf("%s == %s.Unknown", varName, field.Type), nil
		case Union:
			return fmt.Sprintf("%s == nil", varName), nil
		}

		return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, GoResolveType(field, false, false))
	},
}

func SafeClassName(value string) string {
	// Handle illegal keywords for java class/file names
	_, className := filepath.Split(value)
	if className == "Object" {
		return "ProtocolObject"
	} else if className == "Object.java" {
		return strings.Replace(value, "Object.java", "ProtocolObject.java", 1)
	}
	return value
}
