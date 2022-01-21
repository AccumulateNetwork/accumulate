package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed c.tmpl
var cSrc string

var C = mustParseTemplate("c.tmpl", cSrc, template.FuncMap{
	"isPkg":       func(s string) bool { return s == PackagePath },
	"lcName":      lcName,
	"resolveType": CResolveType,

	"jsonType": func(field *Field) string {
		typ := CJsonType(field)
		if typ == "" {
			typ = CResolveType(field, false)
		}
		return typ
	},

	"areEqual":             CAreEqual,
	"binarySize":           CBinarySize,
	"binaryMarshalValue":   CBinaryMarshalValue,
	"binaryUnmarshalValue": CBinaryUnmarshalValue,
	"valueToJson":          CValueToJson,
	"valueFromJson":        CValueFromJson,

	"needsCustomJSON": func(typ *Type) bool {
		for _, f := range typ.Fields {
			if CJsonType(f) != "" {
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

func CMethodName(typ, name string) string {
	return "encoding." + strings.Title(typ) + name
}

func CFieldError(op, name string, args ...string) string {
	args = append(args, "err")
	return fmt.Sprintf("fmt.Errorf(\"error %s %s: %%w\", %s)", op, name, strings.Join(args, ","))
}

func CResolveType(field *Field, forNew bool) string {
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
		return "[]" + CResolveType(field.Slice, false)
	}

	typ := field.Type
	if field.IsPointer && !forNew {
		typ = "*" + typ
	}
	return typ
}

func CJsonType(field *Field) string {
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
		jt := CJsonType(field.Slice)
		if jt != "" {
			return "[]" + jt
		}
	}

	return ""
}

func CAreEqual(field *Field, varName, otherName string) (string, error) {
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
			return "", fmt.Errorf("field %q: cannot determine how to compare %s", field.Name, CResolveType(field, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName, otherName)
	fmt.Fprintf(w, "\tif !(%s) { return false }\n\n", expr)

	switch field.Type {
	case "slice":
		fmt.Fprintf(w, "\tfor i := range %s {\n", varName)
		fmt.Fprintf(w, "\t\tv, u := %s[i], %s[i]\n", varName, otherName)
		str, err := CAreEqual(field.Slice, "v", "u")
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

func CBinarySize(field *Field, varName string) (string, error) {
	typ := field.Type

	var expr string
	switch typ {
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
		expr = CMethodName(typ, "BinarySize") + "(%s)"
	case "bigint", "chain":
		expr = CMethodName(typ, "BinarySize") + "(&%s)"
	case "slice":
		expr = "encoding.UvarintBinarySize(uint64(len(%s)))"
	default:
		switch {
		case field.AsReference, field.AsValue:
			expr = "%s.BinarySize()"
		default:
			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, CResolveType(field, false))
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
	str, err := CBinarySize(field.Slice, "v")
	if err != nil {
		return "", err
	}
	w.WriteString(str)
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func CBinaryMarshalValue(field *Field, varName, errName string, errArgs ...string) (string, error) {
	typ := field.Type

	var expr string
	var canErr bool
	switch typ {
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
		expr, canErr = CMethodName(typ, "MarshalBinary")+"(%s)", false
	case "bigint", "chain":
		expr, canErr = CMethodName(typ, "MarshalBinary")+"(&%s)", false
	case "slice":
		expr, canErr = "encoding.UvarintMarshalBinary(uint64(len(%s)))", false
	default:
		switch {
		case field.AsReference, field.AsValue:
			expr, canErr = "%s.MarshalBinary()", true
		default:
			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, CResolveType(field, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName)
	if canErr {
		err := CFieldError("encoding", errName, errArgs...)
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
	str, err := CBinaryMarshalValue(field.Slice, "v", errName+"[%d]", "i")
	if err != nil {
		return "", err
	}
	w.WriteString(str)
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func CBinaryUnmarshalValue(field *Field, varName, errName string, errArgs ...string) (string, error) {
	typ := field.Type
	w := new(strings.Builder)

	var expr, size, init, sliceName string
	var inPlace bool
	switch typ {
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
		expr, size = CMethodName(typ, "UnmarshalBinary")+"(data)", CMethodName(typ, "BinarySize")+"(%s)"
	case "bigint", "chain":
		expr, size = CMethodName(typ, "UnmarshalBinary")+"(data)", CMethodName(typ, "BinarySize")+"(&%s)"
	case "slice":
		sliceName, varName = varName, "len"+field.Name
		fmt.Fprintf(w, "var %s uint64\n", varName)
		expr, size = "encoding.UvarintUnmarshalBinary(data)", "encoding.UvarintBinarySize(%s)"
	default:
		switch {
		case field.AsReference:
			if field.IsPointer {
				init = "%s = new(" + CResolveType(field, true) + ")"
			}
			expr, size, inPlace = varName+".UnmarshalBinary(data)", "%s.BinarySize()", true
		case field.AsValue:
			expr, size, inPlace = varName+".UnmarshalBinary(data)", "%s.BinarySize()", true
		default:
			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, CResolveType(field, false))
		}

		if field.UnmarshalWith != "" {
			expr, inPlace = field.UnmarshalWith+"(data)", false
		}
	}

	if init != "" {
		fmt.Fprintf(w, "\t%s\n", fmt.Sprintf(init, varName))
	}

	size = fmt.Sprintf(size, varName)
	err := CFieldError("decoding", errName, errArgs...)
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

	fmt.Fprintf(w, "\t%s = make(%s, %s)\n", sliceName, CResolveType(field, false), varName)
	fmt.Fprintf(w, "\tfor i := range %s {\n", sliceName)
	if field.Slice.IsPointer {
		fmt.Fprintf(w, "\t\tvar x %s\n", CResolveType(field.Slice, false))
		str, err := CBinaryUnmarshalValue(field.Slice, "x", errName+"[%d]", "i")
		if err != nil {
			return "", err
		}
		w.WriteString(str)
		fmt.Fprintf(w, "\t\t%s[i] = x", sliceName)
	} else {
		str, err := CBinaryUnmarshalValue(field.Slice, sliceName+"[i]", errName+"[%d]", "i")
		if err != nil {
			return "", err
		}
		w.WriteString(str)
	}
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func CValueToJson(field *Field, tgtName, srcName string) string {
	w := new(strings.Builder)
	switch field.Type {
	case "bytes", "chain", "chainSet", "duration", "any":
		fmt.Fprintf(w, "\t%s = %s(%s)", tgtName, CMethodName(field.Type, "ToJSON"), srcName)
		return w.String()
	case "bigint":
		fmt.Fprintf(w, "\t%s = %s(&%s)", tgtName, CMethodName(field.Type, "ToJSON"), srcName)
		return w.String()
	case "slice":
		if GoJsonType(field.Slice) == "" {
			break
		}

		fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, CJsonType(field.Slice), srcName)
		fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		w.WriteString(CValueToJson(field.Slice, tgtName+"[i]", "x"))
		fmt.Fprintf(w, "\t}")
		return w.String()
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s", tgtName, srcName)
	return w.String()
}

func CValueFromJson(field *Field, tgtName, srcName, errName string, errArgs ...string) string {
	w := new(strings.Builder)
	err := CFieldError("decoding", errName, errArgs...)
	switch field.Type {
	case "any":
		fmt.Fprintf(w, "\t%s = %s(%s)\n", tgtName, CMethodName(field.Type, "FromJSON"), srcName)
		return w.String()

	case "bytes", "chain", "chainSet", "duration":
		fmt.Fprintf(w, "\tif x, err := %s(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = x\n\t}", CMethodName(field.Type, "FromJSON"), srcName, err, tgtName)
		return w.String()
	case "bigint":
		fmt.Fprintf(w, "\tif x, err := %s(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = *x\n\t}", CMethodName(field.Type, "FromJSON"), srcName, err, tgtName)
		return w.String()
	case "slice":
		if CJsonType(field.Slice) == "" {
			break
		}

		fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, CResolveType(field.Slice, false), srcName)
		fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		w.WriteString(CValueFromJson(field.Slice, tgtName+"[i]", "x", errName+"[%d]", "i"))
		fmt.Fprintf(w, "\t}")
		return w.String()
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s", tgtName, srcName)
	return w.String()
}
