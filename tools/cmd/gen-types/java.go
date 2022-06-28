package main

import (
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed java.tmpl
var javaSrc string

var _ = Templates.Register(javaSrc, "java", javaFuncs, "Java")

var javaFuncs = template.FuncMap{
	"resolveType": func(field *Field) string {
		typ := field.Type.JavaType()
		if field.Repeatable {
			typ += "[]"
		}
		return typ
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
		case Any, Url, TxID, Time:
			return fmt.Sprintf("%s == null", varName), nil
		case Bool:
			return fmt.Sprintf("!%s", varName), nil
		case Uint, Int, Duration, Float:
			return fmt.Sprintf("%s == 0", varName), nil
		case BigInt:
			return fmt.Sprintf("(%s).equals(BigInteger.ZERO)", varName), nil
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
