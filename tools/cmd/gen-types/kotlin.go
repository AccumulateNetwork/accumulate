// Copyright 2023 The Accumulate Authors
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

//go:embed types.kotlin.tmpl
var kotlinSrc string

//go:embed unions.kotlin.tmpl
var kotlinUnionsSrc string

var _ = Templates.Register(kotlinSrc, "kotlin", kotlinFuncs, "kotlin")
var _ = Templates.Register(kotlinUnionsSrc, "kotlin-unions", kotlinFuncs)

var kotlinFuncs = template.FuncMap{
	"resolveType": func(field *Field) string {
		typ := field.Type.KotlinType()
		if field.Repeatable {
			typ = "Array<" + typ + ">"
		}
		return typ
	},
	"safeClassName": func(value string) string {
		return SafeClassNameKt(value)
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
			return fmt.Sprintf("(%s).equals(kotlin.math.BigInteger.ZERO)", varName), nil
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

func SafeClassNameKt(value string) string {
	// Handle illegal keywords for kotlin class/file names
	_, className := filepath.Split(value)
	if className == "Object" {
		return "ProtocolObject"
	} else if className == "Object.kt" {
		return strings.Replace(value, "Object.kt", "ProtocolObject.kt", 1)
	}
	return value
}
