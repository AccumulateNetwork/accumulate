package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed go.tmpl
var goSrc string

var Go = mustParseTemplate("go.tmpl", goSrc, template.FuncMap{
	"isPkg":       func(s string) bool { return s == PackagePath },
	"lcName":      lcName,
	"resolveType": GoResolveType,

	"jsonType": func(field *Field) string {
		typ := GoJsonType(field)
		if typ == "" {
			typ = GoResolveType(field, false)
		}
		return typ
	},

	"areEqual":             GoAreEqual,
	"binarySize":           GoBinarySize,
	"binaryMarshalValue":   GoBinaryMarshalValue,
	"binaryUnmarshalValue": GoBinaryUnmarshalValue,
	"valueToJson":          GoValueToJson,
	"valueFromJson":        GoValueFromJson,
	"jsonZeroValue":        GoJsonZeroValue,

	"needsCustomJSON": func(typ *Type) bool {
		if typ.IsTxResult {
			return true
		}
		for _, f := range typ.Fields {
			if GoJsonType(f) != "" {
				return true
			}
		}
		return false
	},

	"validateTag": func(f *Field) string {
		var flags []string
		if !f.IsOptional {
			flags = append(flags, "required")
		}
		if f.IsUrl {
			flags = append(flags, "acc-url")
		}
		if len(flags) == 0 {
			return ""
		}
		return fmt.Sprintf(` validate:"%s"`, strings.Join(flags, ","))
	},
})

func GoMethodName(typ, name string) string {
	return "encoding." + strings.Title(typ) + name
}

func GoFieldError(op, name string, args ...string) string {
	args = append(args, "err")
	return fmt.Sprintf("fmt.Errorf(\"error %s %s: %%w\", %s)", op, name, strings.Join(args, ","))
}

func GoResolveType(field *Field, forNew bool) string {
	switch field.Type {
	case "bytes":
		return "[]byte"
	case "rawJson":
		return "json.RawMessage"
	case "bigint":
		return "big.Int"
	case "uvarint":
		return "uint64"
	case "varint":
		return "int64"
	case "chain":
		return "[32]byte"
	case "chainSet":
		return "[][32]byte"
	case "duration":
		return "time.Duration"
	case "time":
		return "time.Time"
	case "any":
		return "interface{}"
	case "slice":
		return "[]" + GoResolveType(field.Slice, false)
	}

	typ := field.Type
	if field.IsPointer && !forNew {
		typ = "*" + typ
	}
	return typ
}

func GoJsonType(field *Field) string {
	switch field.Type {
	case "bytes":
		return "*string"
	case "bigint":
		return "*string"
	case "chain":
		return "string"
	case "chainSet":
		return "[]string"
	case "duration", "any":
		return "interface{}"
	case "slice":
		jt := GoJsonType(field.Slice)
		if jt != "" {
			return "[]" + jt
		}
	}

	if field.UnmarshalWith != "" {
		return "json.RawMessage"
	}

	return ""
}

func GoJsonZeroValue(field *Field) (string, error) {
	switch field.Type {
	case "bytes", "bigint", "chainSet", "duration", "any", "slice", "rawJson":
		return "nil", nil
	case "bool":
		return "false", nil
	case "string", "chain":
		return `""`, nil
	case "uvarint", "varint":
		return "0", nil
	}

	switch {
	case field.AsReference, field.AsValue:
		if field.IsPointer {
			return "nil", nil
		}
		return fmt.Sprintf("(%s{})", GoResolveType(field, false)), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, GoResolveType(field, false))
}

func GoAreEqual(field *Field, varName, otherName string) (string, error) {
	var expr string
	switch field.Type {
	case "bool", "string", "chain", "uvarint", "varint", "duration", "time":
		expr = "%s == %s"
	case "bytes", "rawJson":
		expr = "bytes.Equal(%s, %s)"
	case "bigint":
		if field.IsPointer {
			expr = "%s.Cmp(%s) == 0"
		} else {
			expr = "%s.Cmp(&%s) == 0"
		}
	case "slice", "chainSet":
		expr = "len(%s) == len(%s)"
	default:
		switch {
		case field.AsReference:
			if field.IsPointer {
				expr = "%s.Equal(%s)"
			} else {
				expr = "%s.Equal(&%s)"
			}
		case field.AsValue:
			if field.IsPointer {
				expr = "*%s == *%s"
			} else {
				expr = "%s == %s"
			}
		default:
			return "", fmt.Errorf("field %q: cannot determine how to compare %s", field.Name, GoResolveType(field, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName, otherName)
	fmt.Fprintf(w, "\tif !(%s) { return false }\n\n", expr)

	switch field.Type {
	case "slice":
		fmt.Fprintf(w, "\tfor i := range %s {\n", varName)
		fmt.Fprintf(w, "\t\tv, u := %s[i], %s[i]\n", varName, otherName)
		str, err := GoAreEqual(field.Slice, "v", "u")
		if err != nil {
			return "", err
		}
		w.WriteString(str)
		fmt.Fprintf(w, "\t}\n\n")

	case "chainSet":
		fmt.Fprintf(w, "\tfor i := range %s {\n", varName)
		fmt.Fprintf(w, "\t\tif %s[i] != %s[i] { return false }\n", varName, otherName)
		fmt.Fprintf(w, "\t}\n\n")

	default:
		fmt.Fprintf(w, "\n")
	}
	return w.String(), nil
}

func GoBinarySize(field *Field, varName string) (string, error) {
	typ := field.Type

	var expr string
	switch typ {
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
		expr = GoMethodName(typ, "BinarySize") + "(%s)"
	case "bigint", "chain":
		expr = GoMethodName(typ, "BinarySize") + "(&%s)"
	case "slice":
		expr = "encoding.UvarintBinarySize(uint64(len(%s)))"
	default:
		switch {
		case field.AsReference, field.AsValue:
			expr = "%s.BinarySize()"
		default:
			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, GoResolveType(field, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName)
	fmt.Fprintf(w, "\tn += %s\n\n", expr)

	if typ != "slice" {
		fmt.Fprintf(w, "\n")
		return w.String(), nil
	}

	fmt.Fprintf(w, "\tfor _, v := range %s {\n", varName)
	str, err := GoBinarySize(field.Slice, "v")
	if err != nil {
		return "", err
	}
	w.WriteString(str)
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func GoBinaryMarshalValue(field *Field, varName, errName string, errArgs ...string) (string, error) {
	typ := field.Type

	var expr string
	var canErr bool
	switch typ {
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
		expr, canErr = GoMethodName(typ, "MarshalBinary")+"(%s)", false
	case "bigint", "chain":
		expr, canErr = GoMethodName(typ, "MarshalBinary")+"(&%s)", false
	case "slice":
		expr, canErr = "encoding.UvarintMarshalBinary(uint64(len(%s)))", false
	default:
		switch {
		case field.AsReference, field.AsValue:
			expr, canErr = "%s.MarshalBinary()", true
		default:
			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, GoResolveType(field, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName)
	if canErr {
		err := GoFieldError("encoding", errName, errArgs...)
		fmt.Fprintf(w, "\tif b, err := %s; err != nil { return nil, %s } else { buffer.Write(b) }\n", expr, err)
	} else {
		fmt.Fprintf(w, "\tbuffer.Write(%s)\n", expr)
	}

	if typ != "slice" {
		fmt.Fprintf(w, "\n")
		return w.String(), nil
	}

	fmt.Fprintf(w, "\tfor i, v := range %s {\n", varName)
	fmt.Fprintf(w, "\t\t_ = i\n")
	str, err := GoBinaryMarshalValue(field.Slice, "v", errName+"[%d]", "i")
	if err != nil {
		return "", err
	}
	w.WriteString(str)
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func GoBinaryUnmarshalValue(field *Field, varName, errName string, errArgs ...string) (string, error) {
	typ := field.Type
	w := new(strings.Builder)

	var expr, size, init, sliceName string
	var inPlace bool
	switch typ {
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
		expr, size = GoMethodName(typ, "UnmarshalBinary")+"(data)", GoMethodName(typ, "BinarySize")+"(%s)"
	case "bigint", "chain":
		expr, size = GoMethodName(typ, "UnmarshalBinary")+"(data)", GoMethodName(typ, "BinarySize")+"(&%s)"
	case "slice":
		sliceName, varName = varName, "len"+field.Name
		fmt.Fprintf(w, "var %s uint64\n", varName)
		expr, size = "encoding.UvarintUnmarshalBinary(data)", "encoding.UvarintBinarySize(%s)"
	default:
		switch {
		case field.AsReference:
			if field.IsPointer {
				init = "%s = new(" + GoResolveType(field, true) + ")"
			}
			expr, size, inPlace = varName+".UnmarshalBinary(data)", "%s.BinarySize()", true
		case field.AsValue:
			expr, size, inPlace = varName+".UnmarshalBinary(data)", "%s.BinarySize()", true
		default:
			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, GoResolveType(field, false))
		}

		if field.UnmarshalWith != "" {
			expr, inPlace = field.UnmarshalWith+"(data)", false
		}
	}

	if init != "" {
		fmt.Fprintf(w, "\t%s\n", fmt.Sprintf(init, varName))
	}

	size = fmt.Sprintf(size, varName)
	err := GoFieldError("decoding", errName, errArgs...)
	if inPlace {
		fmt.Fprintf(w, "\tif err := %s; err != nil { return %s }\n", expr, err)
	} else if typ == "bigint" {
		fmt.Fprintf(w, "\tif x, err := %s; err != nil { return %s } else { %s.Set(x) }\n", expr, err, varName)
	} else {
		fmt.Fprintf(w, "\tif x, err := %s; err != nil { return %s } else { %s = x }\n", expr, err, varName)
	}
	fmt.Fprintf(w, "\tdata = data[%s:]\n\n", size)

	if typ != "slice" {
		return w.String(), nil
	}

	fmt.Fprintf(w, "\t%s = make(%s, %s)\n", sliceName, GoResolveType(field, false), varName)
	fmt.Fprintf(w, "\tfor i := range %s {\n", sliceName)
	if field.Slice.IsPointer {
		fmt.Fprintf(w, "\t\tvar x %s\n", GoResolveType(field.Slice, false))
		str, err := GoBinaryUnmarshalValue(field.Slice, "x", errName+"[%d]", "i")
		if err != nil {
			return "", err
		}
		w.WriteString(str)
		fmt.Fprintf(w, "\t\t%s[i] = x", sliceName)
	} else {
		str, err := GoBinaryUnmarshalValue(field.Slice, sliceName+"[i]", errName+"[%d]", "i")
		if err != nil {
			return "", err
		}
		w.WriteString(str)
	}
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func GoValueToJson(field *Field, tgtName, srcName string, forUnmarshal bool, errName string, errArgs ...string) string {
	w := new(strings.Builder)
	switch field.Type {
	case "bytes", "chain", "chainSet", "duration", "any":
		fmt.Fprintf(w, "\t%s = %s(%s)", tgtName, GoMethodName(field.Type, "ToJSON"), srcName)
		return w.String()
	case "bigint":
		fmt.Fprintf(w, "\t%s = %s(&%s)", tgtName, GoMethodName(field.Type, "ToJSON"), srcName)
		return w.String()
	case "slice":
		if GoJsonType(field.Slice) == "" {
			break
		}

		fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, GoJsonType(field.Slice), srcName)
		fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		w.WriteString(GoValueToJson(field.Slice, tgtName+"[i]", "x", forUnmarshal, errName+"[%d]", "i"))
		fmt.Fprintf(w, "\t}")
		return w.String()
	}

	if field.UnmarshalWith != "" {
		err := GoFieldError("encoding", errName, errArgs...)
		if !forUnmarshal {
			err = "nil, " + err
		}
		fmt.Fprintf(w, "\tif x, err := json.Marshal(%s); err != nil { return %s } else { %s = x }\n", srcName, err, tgtName)
		return w.String()
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s", tgtName, srcName)
	return w.String()
}

func GoValueFromJson(field *Field, tgtName, srcName, errName string, errArgs ...string) string {
	w := new(strings.Builder)
	err := GoFieldError("decoding", errName, errArgs...)
	switch field.Type {
	case "any":
		fmt.Fprintf(w, "\t%s = %s(%s)\n", tgtName, GoMethodName(field.Type, "FromJSON"), srcName)
		return w.String()

	case "bytes", "chain", "chainSet", "duration":
		fmt.Fprintf(w, "\tif x, err := %s(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = x\n\t}", GoMethodName(field.Type, "FromJSON"), srcName, err, tgtName)
		return w.String()
	case "bigint":
		fmt.Fprintf(w, "\tif x, err := %s(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = *x\n\t}", GoMethodName(field.Type, "FromJSON"), srcName, err, tgtName)
		return w.String()
	case "slice":
		if GoJsonType(field.Slice) == "" {
			break
		}

		fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, GoResolveType(field.Slice, false), srcName)
		fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		w.WriteString(GoValueFromJson(field.Slice, tgtName+"[i]", "x", errName+"[%d]", "i"))
		fmt.Fprintf(w, "\t}")
		return w.String()
	}

	if field.UnmarshalWith != "" {
		fmt.Fprintf(w, "\tif x, err := %sJSON(%s); err != nil { return %s } else { %s = x }\n", field.UnmarshalWith, srcName, err, tgtName)
		return w.String()
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s", tgtName, srcName)
	return w.String()
}
