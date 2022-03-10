package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed go.go.tmpl
var goSrc string

//go:embed union.go.tmpl
var goUnionSrc string

func init() {
	Templates.Register(goSrc, "go", goFuncs, "Go")
	Templates.Register(goUnionSrc, "go-union", goFuncs)
}

type goUnionTypeSpec struct {
	Kind        string
	Enumeration string
	Interface   string
	NaturalName string
}

var goFuncs = template.FuncMap{
	"_unionTypes": func() []goUnionTypeSpec {
		// TODO This should not be hard coded
		return []goUnionTypeSpec{
			{"chain", "AccountType", "Account", "account type"},
			{"tx", "TransactionType", "TransactionBody", "transaction type"},
			{"signature", "SignatureType", "Signature", "signature type"},
		}
	},
	"isPkg": func(s string) bool {
		return s == PackagePath
	},
	"pkg": func(s string) string {
		if s == PackagePath {
			return ""
		}
		i := strings.LastIndexByte(s, '/')
		if i < 0 {
			return s + "."
		}
		return s[i+1:] + "."
	},

	"resolveType": func(field *Field, forNew bool) string {
		return GoResolveType(field, forNew, false)
	},

	"jsonType": func(field *Field) string {
		typ := GoJsonType(field)
		if typ == "" {
			typ = GoResolveType(field, false, false)
		}
		return typ
	},

	"areEqual":             GoAreEqual,
	"binaryMarshalValue":   GoBinaryMarshalValue,
	"binaryUnmarshalValue": GoBinaryUnmarshalValue,
	"valueToJson":          GoValueToJson,
	"valueFromJson":        GoValueFromJson,
	"jsonZeroValue":        GoJsonZeroValue,
	"isZero":               GoIsZero,

	"needsCustomJSON": func(typ *Type) bool {
		if typ.IsUnion() {
			return true
		}

		// Add a custom un/marshaller if the type embeds another type - fields
		// of embedded types are un-embedded during JSON un/marshalling
		if len(typ.Embeddings) > 0 {
			return true
		}

		for _, f := range typ.Fields {
			// Add a custom un/marshaller if the field needs special handling
			if GoJsonType(f) != "" {
				return true
			}

			// Add a custom un/marshaller if the field has an alternate name
			if f.Alternative != "" {
				return true
			}
		}
		return false
	},

	"validateTag": func(f *Field) string {
		var flags []string
		if !f.Optional {
			flags = append(flags, "required")
		}
		if len(flags) == 0 {
			return ""
		}
		return fmt.Sprintf(` validate:"%s"`, strings.Join(flags, ","))
	},
}

func GoFieldError(op, name string, args ...string) string {
	args = append(args, "err")
	return fmt.Sprintf("fmt.Errorf(\"error %s %s: %%w\", %s)", op, name, strings.Join(args, ","))
}

func goBinaryMethod(field *Field) (methodName string, wantPtr bool) {
	switch field.Type {
	case "bool", "string", "duration", "time", "bytes", "uint", "int":
		return strings.Title(field.Type), false
	case "url", "hash":
		return strings.Title(field.Type), true
	case "rawJson":
		return "Bytes", false
	case "bigint":
		return "BigInt", true
	case "chain":
		return "Hash", true
	case "uvarint":
		return "Uint", false
	case "varint":
		return "Int", false
	}

	switch field.MarshalAs {
	case "reference":
		return "Value", true
	case "value":
		return "Value", false
	case "enum":
		return "Enum", false
	}

	return "", false
}

func goJsonMethod(field *Field) (methodName string, wantPtr bool) {
	switch field.Type {
	case "bytes", "chain", "duration", "any":
		return strings.Title(field.Type), false
	case "bigint":
		return strings.Title(field.Type), true
	}

	return "", false
}

func GoResolveType(field *Field, forNew, ignoreRepeatable bool) string {
	typ := field.Type
	switch typ {
	case "bytes":
		typ = "[]byte"
	case "rawJson":
		typ = "json.RawMessage"
	case "url":
		typ = "url.URL"
	case "bigint":
		typ = "big.Int"
	case "uvarint", "uint":
		typ = "uint64"
	case "varint", "int":
		typ = "int64"
	case "chain", "hash":
		typ = "[32]byte"
	case "duration":
		typ = "time.Duration"
	case "time":
		typ = "time.Time"
	case "any":
		typ = "interface{}"
	}

	if field.Pointer && !forNew {
		typ = "*" + typ
	}
	if field.Repeatable && !ignoreRepeatable {
		typ = "[]" + typ
	}
	return typ
}

func GoJsonType(field *Field) string {
	var jtype string
	switch field.Type {
	case "bytes":
		jtype = "*string"
	case "bigint":
		jtype = "*string"
	case "chain":
		jtype = "string"
	case "duration", "any":
		jtype = "interface{}"
	default:
		if field.UnmarshalWith != "" {
			jtype = "json.RawMessage"
		} else {
			return ""
		}
	}

	if field.Repeatable {
		jtype = "[]" + jtype
	}
	return jtype
}

func GoIsZero(field *Field, varName string) (string, error) {
	if field.Repeatable {
		return fmt.Sprintf("len(%s) == 0", varName), nil
	}
	if field.Pointer {
		return fmt.Sprintf("%s == nil", varName), nil
	}
	if field.ZeroValue != nil {
		return fmt.Sprintf("%s == (%v)", varName, field.ZeroValue), nil
	}

	switch field.Type {
	case "bytes", "rawJson", "string":
		return fmt.Sprintf("len(%s) == 0", varName), nil
	case "any":
		return fmt.Sprintf("%s == nil", varName), nil
	case "bool":
		return fmt.Sprintf("!%s", varName), nil
	case "uvarint", "varint", "uint", "int", "duration":
		return fmt.Sprintf("%s == 0", varName), nil
	case "bigint":
		return fmt.Sprintf("(%s).Cmp(new(big.Int)) == 0", varName), nil
	case "url", "chain", "time":
		return fmt.Sprintf("%s == (%s{})", varName, GoResolveType(field, false, false)), nil
	}

	switch field.MarshalAs {
	case "reference":
		return fmt.Sprintf("(%s).Equal(new(%s))", varName, field.Type), nil
	case "enum":
		return fmt.Sprintf("%s == 0", varName), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, GoResolveType(field, false, false))
}

func GoJsonZeroValue(field *Field) (string, error) {
	if field.IsPointer() {
		return "nil", nil
	}

	switch field.Type {
	case "bytes", "bigint", "duration", "any", "slice", "rawJson":
		return "nil", nil
	case "bool":
		return "false", nil
	case "string", "chain":
		return `""`, nil
	case "uvarint", "varint", "uint", "int":
		return "0", nil
	}

	switch field.MarshalAs {
	case "enum":
		return "0", nil
	case "reference", "value":
		if field.Pointer {
			return "nil", nil
		}
		return fmt.Sprintf("(%s{})", GoResolveType(field, false, false)), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, GoResolveType(field, false, false))
}

func GoAreEqual(field *Field, varName, otherName string) (string, error) {
	var expr string
	var wantPtr bool
	switch field.Type {
	case "bool", "string", "chain", "uvarint", "varint", "uint", "int", "duration", "time":
		expr, wantPtr = "%[1]s%[2]s == %[1]s%[3]s", false
	case "bytes", "rawJson":
		expr, wantPtr = "bytes.Equal(%[1]s%[2]s, %[1]s%[3]s)", false
	case "bigint":
		expr, wantPtr = "(%[1]s%[2]s).Cmp(%[1]s%[3]s) == 0", true
	case "url":
		expr, wantPtr = "(%[1]s%[2]s).Equal(%[1]s%[3]s)", true
	default:
		switch field.MarshalAs {
		case "reference":
			expr, wantPtr = "(%[1]s%[2]s).Equal(%[1]s%[3]s)", true
		case "value", "enum":
			expr, wantPtr = "%[1]s%[2]s == %[1]s%[3]s", false
		default:
			return "", fmt.Errorf("field %q: cannot determine how to compare %s", field.Name, GoResolveType(field, false, false))
		}
	}

	var ptrPrefix string
	switch {
	case wantPtr && !field.Pointer:
		// If we want a pointer and have a value, take the address of the value
		ptrPrefix = "&"
	case !wantPtr && field.Pointer:
		// If we want a value and have a pointer, dereference the pointer
		ptrPrefix = "*"
	}

	if !field.Repeatable {
		return fmt.Sprintf("\tif !("+expr+") { return false }", ptrPrefix, varName, otherName), nil
	}

	expr = fmt.Sprintf(expr, ptrPrefix, "%[2]s[i]", "%[3]s[i]")
	return fmt.Sprintf(
		"	if len(%[2]s) != len(%[3]s) { return false }\n"+
			"	for i := range %[2]s {\n"+
			"		if !("+expr+") { return false }\n"+
			"	}",
		ptrPrefix, varName, otherName), nil
}

func GoBinaryMarshalValue(field *Field, writerName, varName string) (string, error) {
	method, wantPtr := goBinaryMethod(field)
	if method == "" {
		return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, GoResolveType(field, false, false))
	}

	var ptrPrefix string
	switch {
	case wantPtr && !field.Pointer:
		ptrPrefix = "&"
	case !wantPtr && field.Pointer:
		ptrPrefix = "*"
	}

	if !field.Repeatable {
		return fmt.Sprintf("\t%s.Write%s(%d, %s%s)", writerName, method, field.Number, ptrPrefix, varName), nil
	}

	return fmt.Sprintf("\tfor _, v := range %s { %s.Write%s(%d, %sv) }", varName, writerName, method, field.Number, ptrPrefix), nil
}

func GoBinaryUnmarshalValue(field *Field, readerName, varName string) (string, error) {
	method, wantPtr := goBinaryMethod(field)
	if method == "" {
		return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, GoResolveType(field, false, false))
	}

	var ptrPrefix string
	switch {
	case wantPtr && !field.Pointer:
		ptrPrefix = "*"
	case !wantPtr && field.Pointer:
		ptrPrefix = "&"
	case field.UnmarshalWith != "",
		field.Pointer:
		// OK
	case method == "Value" || method == "Enum":
		ptrPrefix = "*"
	}

	var set string
	if field.Repeatable {
		set = fmt.Sprintf("%s = append(%[1]s, %sx)", varName, ptrPrefix)
	} else {
		set = fmt.Sprintf("%s = %sx", varName, ptrPrefix)
	}

	var expr string
	var hasIf bool
	switch {
	case field.UnmarshalWith != "":
		expr, hasIf = fmt.Sprintf("%s.ReadValue(%d, func(b []byte) error { x, err := %s(b); if err == nil { %s }; return err })", readerName, field.Number, field.UnmarshalWith, set), false
	case method == "Value":
		expr, hasIf = fmt.Sprintf("if x := new(%s); %s.ReadValue(%d, x.UnmarshalBinary) { %s }", GoResolveType(field, true, true), readerName, field.Number, set), true
	case method == "Enum":
		expr, hasIf = fmt.Sprintf("if x := new(%s); %s.ReadEnum(%d, x) { %s }", GoResolveType(field, true, true), readerName, field.Number, set), true
	default:
		expr, hasIf = fmt.Sprintf("if x, ok := %s.Read%s(%d); ok { %s }", readerName, method, field.Number, set), true
	}

	if !field.Repeatable {
		return "\t" + expr, nil
	}

	if hasIf {
		return "\tfor { " + expr + " else { break } }", nil
	}

	return "\tfor { ok := " + expr + "; if !ok { break } }", nil
}

func GoValueToJson(field *Field, tgtName, srcName string, forUnmarshal bool, errName string, errArgs ...string) (string, error) {
	if field.UnmarshalWith != "" {
		err := GoFieldError("encoding", errName, errArgs...)
		if !forUnmarshal {
			err = "nil, " + err
		}
		if !field.Repeatable {
			return fmt.Sprintf("\tif x, err := json.Marshal(%s); err != nil { return %s } else { %s = x }", srcName, err, tgtName), nil
		}
		return fmt.Sprintf("\t%s = make([]json.RawMessage, len(%s)); for i, x := range %[2]s { if y, err := json.Marshal(x); err != nil { return %s } else { %[1]s[i] = y } }", tgtName, srcName, err), nil
	}

	method, wantPtr := goJsonMethod(field)
	var ptrPrefix string
	switch {
	case method == "":
		return fmt.Sprintf("\t%s = %s", tgtName, srcName), nil
	case wantPtr:
		ptrPrefix = "&"
	}

	if !field.Repeatable {
		return fmt.Sprintf("\t%s = encoding.%sToJSON(%s%s)", tgtName, method, ptrPrefix, srcName), nil
	}

	return fmt.Sprintf("\t%s = make(%s, len(%s)); for i, x := range %[3]s { %[1]s[i] = encoding.%[4]sToJSON(%sx) }", tgtName, GoJsonType(field), srcName, method, ptrPrefix), nil
}

func GoValueFromJson(field *Field, tgtName, srcName, errName string, errArgs ...string) (string, error) {
	err := GoFieldError("decoding", errName, errArgs...)
	if field.UnmarshalWith != "" {
		if !field.Repeatable {
			return fmt.Sprintf("\tif x, err := %sJSON(%s); err != nil { return %s } else { %s = x }\n", field.UnmarshalWith, srcName, err, tgtName), nil
		}
		return fmt.Sprintf("\t%s = make(%s, len(%s)); for i, x := range %[3]s { if y, err := %sJSON(x); err != nil { return %s } else { %[1]s[i] = y } }", tgtName, GoResolveType(field, false, false), srcName, field.UnmarshalWith, err), nil
	}

	method, wantPtr := goJsonMethod(field)
	var ptrPrefix string
	switch {
	case method == "":
		return fmt.Sprintf("\t%s = %s", tgtName, srcName), nil
	case wantPtr:
		ptrPrefix = "*"
	}

	if !field.Repeatable {
		return fmt.Sprintf("\tif x, err := encoding.%sFromJSON(%s); err != nil { return %s } else { %s = %sx }", method, srcName, err, tgtName, ptrPrefix), nil
	}

	return fmt.Sprintf("\t%s = make(%s, len(%s)); for i, x := range %[3]s { if x, err := encoding.%sFromJSON(x); err != nil { return %s } else { %[1]s[i] = x } }", tgtName, GoResolveType(field, false, false), srcName, method, err), nil
}
