package main

import (
	_ "embed"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"strings"
	"text/template"
)

var cFuncMap = template.FuncMap{
	"isPkg": func(s string) bool { return s == PackagePath },
	//	"lcName":      lcName,
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

	"init":        CInit,
	"resolveType": CResolveType,
	"typeIsKnown": func(field *Field) bool {
		return field.Type.IsKnown()
	},

	"jsonType": func(field *Field) string {
		typ := CJsonType(field)
		if typ == "" {
			typ = CResolveType(field, false, false)
		}
		return typ
	},

	"get":                     CGetField,
	"areEqual":                CAreEqual,
	"copy":                    CCopy,
	"binaryMarshalValue":      CBinaryMarshalValue,
	"binaryUnmarshalValue":    CBinaryUnmarshalValue,
	"valueToJson":             CValueToJson,
	"valueFromJson":           CValueFromJson,
	"jsonZeroValue":           CJsonZeroValue,
	"isZero":                  CIsZero,
	"errVirtualFieldNotEqual": CErrVirtualFieldNotEqual,
	"unionType": func(typ *Type) string {
		return typ.Union.Type
	},
	"isUnionFieldType": func(field *Field) bool {
		if field.TypeRef == nil {
			if field.Name == "DataEntry" {
				fmt.Printf("union typeref field type is null %v", field.Name)
			}
			return false
		}
		if field.Name == "DataEntry" {
			fmt.Printf("union typeef field type is null %v is union? %v", field.Name, field.TypeRef.IsUnion())
		}
		return field.TypeRef.IsUnion()
	},
	//"areEqual":             CAreEqual,
	"binarySize": CBinarySize,
	//"binaryMarshalValue":   CBinaryMarshalValue,
	//"binaryUnmarshalValue": CBinaryUnmarshalValue,
	//"valueToJson":          CValueToJson,
	//"valueFromJson":        CValueFromJson,
	//
	//"isUnionFieldType": func(n uint) bool {
	//	return n == 1
	//},
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
		if !f.Optional {
			flags = append(flags, "required")
		}
		if len(flags) == 0 {
			return ""
		}
		return fmt.Sprintf(` validate:"%s"`, strings.Join(flags, ","))
	},
}

//go:embed c_header.tmpl
var chSrc string

//go:embed c.tmpl
var cSrc string

//go:embed c_union_header.tmpl
var chunionSrc string

//go:embed c_union_source.tmpl
var cunionSrc string

func init() {
	_ = Templates.Register(cSrc, "c-source", cFuncMap, "")
	_ = Templates.Register(chSrc, "c-header", cFuncMap)
	_ = Templates.Register(chunionSrc, "c-header-union", cFuncMap)
	_ = Templates.Register(cunionSrc, "c-source-union", cFuncMap)
}

func CMethodName(typ, name string) string {
	return strings.Title(typ) + "_" + name
}

func CErrVirtualFieldNotEqual(field *Field, varName, valName string) (string, error) {
	return fmt.Sprintf(`return fmt.Errorf("field %s: not equal: want %%%%v, got %%%%v", %s, %s)`, field.Name, varName, valName), nil
}

func CJsonZeroValue(field *Field) (string, error) {
	if field.IsPointer() {
		return "nil", nil
	}

	switch field.Type.Code {
	case Bytes, BigInt, Duration, Any, RawJson:
		return "nil", nil
	case Bool:
		return "false", nil
	case String, Hash:
		return `""`, nil
	case Uint, Int, Float:
		return "0", nil
	}

	switch field.MarshalAs {
	case Enum:
		return "0", nil
	case Union:
		return "nil", nil
	case Reference, Value:
		if field.Pointer {
			return "nil", nil
		}
		return fmt.Sprintf("(%s{})", CResolveType(field, false, false)), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, CResolveType(field, false, false))
}

func CIsZero(field *Field, varName string) (string, error) {
	if field.Repeatable {
		return fmt.Sprintf("len(%s) == 0", varName), nil
	}
	if field.Pointer {
		return fmt.Sprintf("%s == nil", varName), nil
	}
	if field.ZeroValue != nil {
		return fmt.Sprintf("%s == (%v)", varName, field.ZeroValue), nil
	}

	switch field.Type.Code {
	case Bytes, RawJson, String:
		return fmt.Sprintf("%s.Length(&%s) == 0", varName), nil
	case Any:
		return fmt.Sprintf("%s == nil", varName), nil
	case Bool:
		return fmt.Sprintf("!%s", varName), nil
	case Uint, Int, Duration, Float:
		return fmt.Sprintf("%s == 0", varName), nil
	case BigInt:
		return fmt.Sprintf("(%s).Cmp(new(big.Int)) == 0", varName), nil
	case Url, TxID, Hash, Time:
		return fmt.Sprintf("%s == (%s{})", varName, CResolveType(field, false, false)), nil
	}

	switch field.MarshalAs {
	case Reference:
		return fmt.Sprintf("(%s).Equal(new(%s))", varName, field.Type), nil
	case Enum:
		return fmt.Sprintf("%s == 0", varName), nil
	case Union:
		return fmt.Sprintf("%s == nil", varName), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, CResolveType(field, false, false))
}

func CGetField(field *Field) string {
	if field.Virtual {
		return field.Name + "()"
	}
	return field.Name
}

func CFieldError(op, name string, args ...string) string {
	args = append(args, "err")
	return fmt.Sprintf("fmt.Errorf(\"error %s %s: %%w\", %s)", op, name, strings.Join(args, ","))
}

func CResolveType(field *Field, forNew, ignoreRepeatable bool) string {
	typ := field.Type.CType()
	if field.Pointer && !forNew {
		typ = typ + "*"
	}
	if field.Repeatable && !ignoreRepeatable {
		typ = typ + "*"
	}
	return typ
}

func CJsonType(field *Field) string {
	switch field.Type.Name {
	case "bytes":
		return "String*"
	case "bigint":
		return "BigInt*"
	case "chain":
		return "String"
	case "chainSet":
		return "String[]"
	case "duration", "any":
		return "interface{}"
		//case "slice":
		//	jt := CJsonType(field.Slice)
		//	if jt != "" {
		//		return "[]" + jt
		//	}
	}

	return ""
}
func CInit(field *Field, varName string) (string, error) {
	typ := CResolveType(field, false, false)

	var expr string
	switch typ {
	case "bool":
		expr = "%s"
		varName = "false"
	case "rawJson":
		typ = "bytes"
		fallthrough
	case "bytes", "string", "chainSet", "duration", "time":
		typ = CResolveType(field, false, false)
		expr = CMethodName(typ, "init") + "(%s, buffer->ptr, buffer->size)"
		varName = "0"
	case "bigint", "chain", "varint", "uvarint":
		typ = CResolveType(field, false, false)
		expr = CMethodName(typ, "init") + "(%s)"
		varName = "0"
	case "slice":
		expr = "your guess is as good as mine(uint64(len(%s)))"
	default:
		switch {
		case field.AsReference(), field.AsValue():
			expr = fmt.Sprintf("init.%s_init(%%s)", typ)
		default:
			return "", fmt.Errorf("field %q: %s, cannot determine how to marshal %s", field.Name, field.Type.Name, CResolveType(field, false, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName)
	fmt.Fprintf(w, "\t%s;\n\n", expr)

	if typ != "slice" {
		fmt.Fprintf(w, "\n")
		return w.String(), nil
	}

	fmt.Fprintf(w, "\tfor _, v := range %s {\n", varName)
	str, err := CBinarySize(field, "vFixMe")
	if err != nil {
		return "", err
	}
	w.WriteString(str)
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func CAreEqual(field *Field, varName, otherName string) (string, error) {
	var expr string
	switch field.Type.Code {
	case typegen.TypeCodeUint, typegen.TypeCodeInt, typegen.TypeCodeBigInt, typegen.TypeCodeString, typegen.TypeCodeRawJson,
		typegen.TypeCodeBytes, typegen.TypeCodeTxid, typegen.TypeCodeHash, typegen.TypeCodeUrl:
		expr = "%s.Equal(%s, %s)"
	case typegen.TypeCodeFloat, typegen.TypeCodeBool, typegen.TypeCodeDuration, typegen.TypeCodeTime:
		expr = "%s /*%s*/ == %s"
	//case "bytes", "rawJson":
	//	expr = "memcmp(%s, %s)==0"
	//case "bigint":
	//case "slice", "chainSet":
	//	expr = "%s.data.len == %s.data.len"
	default:
		switch {
		case field.AsReference():
			if field.IsPointer() {
				expr = "v->%s.Equal(v->%s,v->%s)"
			} else {
				expr = "v->%s.Equal(&v->%s,&u->%s)"
			}
		case field.AsValue():
			if field.IsPointer() {
				expr = "*%s /*%s*/ == *%s"
			} else {
				expr = "%s /*%s*/ == %s"
			}
		default:
			return "", fmt.Errorf("field %q: %s cannot determine how to compare %s", field.Name, field.Type.Name, CResolveType(field, false, false))
		}
	}

	w := new(strings.Builder)

	expr = fmt.Sprintf(expr, varName, varName, otherName)
	fmt.Fprintf(w, "\tif (!%s) { return false; }\n\n", expr)
	fmt.Fprintf(w, "\n")

	return w.String(), nil
}

func CBinarySize(field *Field, varName string) (string, error) {
	typ := field.Type.CType()

	var expr string
	switch field.Type.Code {
	case typegen.TypeCodeRawJson:
		typ = "bytes"
		fallthrough
	case typegen.TypeCodeBool, typegen.TypeCodeBytes, typegen.TypeCodeString, typegen.TypeCodeUint, typegen.TypeCodeInt,
		typegen.TypeCodeDuration, typegen.TypeCodeTime, typegen.TypeCodeUrl:
		expr = CMethodName(typ, "BinarySize") + "(%s)"
	case typegen.TypeCodeBigInt, typegen.TypeCodeHash, typegen.TypeCodeTxid:
		expr = CMethodName(typ, "BinarySize") + "(&%s)"
	//case "slice":
	//	expr = "varint_size((uint64)(%s->buffer.size))"
	default:
		switch {
		case field.AsReference(), field.AsValue():
			expr = "%s.BinarySize()"
		default:
			return "", fmt.Errorf("field %q: %s cannot determine how to marshal %s", field.Name, field.Type.Name, CResolveType(field, false, false))
		}
	}

	w := new(strings.Builder)
	expr = fmt.Sprintf(expr, varName)
	if field.Pointer {
		fmt.Fprintf(w, "\tn += %s;\n\n", expr)
	} else {
		fmt.Fprintf(w, "\tn += %s;\n\n", expr)
	}
	if typ != "slice" {
		fmt.Fprintf(w, "\n")
		return w.String(), nil
	}

	//fmt.Fprintf(w, "\tfor _, v := range %s {\n", varName)
	//str, err := CBinarySize(field.Slice, "v")
	//if err != nil {
	//	return "", err
	//}
	//w.WriteString(str)
	//fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func cUnionMethod(field *Field, name string) string {
	parts := strings.SplitN(field.Type.String(), ".", 2)
	if len(parts) == 1 {
		return name + parts[0]
	}
	return fmt.Sprintf("%s.%s%s", parts[0], name, parts[1])
}

//	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
//		expr, size = CMethodName(typ, "UnmarshalBinary")+"(data)", CMethodName(typ, "BinarySize")+"(%s)"
func cBinaryMethod(field *Field) (methodName string, wantPtr bool) {
	switch field.Type.Code {
	case Bool, String, Duration, Time, Bytes, Uint, Int, Float:
		return typegen.TitleCase(field.Type.String()), false
	case Url, TxID, Hash:
		return typegen.TitleCase(field.Type.String()), true
	case RawJson:
		return "Bytes", false
	case BigInt:
		return "BigInt", true
	}

	switch field.MarshalAs {
	case Reference:
		return "Value", true
	case Value, Union:
		return "Value", false
	case Enum:
		return "Enum", false
	}

	return "", false
}

func cJsonMethod(field *Field) (methodName string, wantPtr bool) {
	switch field.Type.Code {
	case Bytes, Duration, Any:
		return field.Type.Title(), false
	case Hash:
		return "Chain", false
	case BigInt:
		return "Bigint", true
	}

	return "", false
}

func CBinaryMarshalValue(field *Field, varName, errName string, errArgs ...string) (string, error) {
	typ := field.Type.Name

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
		case field.AsReference(), field.AsValue():
			expr, canErr = "%s.MarshalBinary()", true
		default:
			return "", fmt.Errorf("field %q: %s cannot determine how to marshal %s", field.Name, field.Type.Name, CResolveType(field, false, false))
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
	str, err := CBinaryMarshalValue(field, "v", errName+"[%d]", "i")
	if err != nil {
		return "", err
	}
	w.WriteString(str)
	fmt.Fprintf(w, "\t}\n\n")
	return w.String(), nil
}

func CBinaryUnmarshalValue(field *Field, readerName, varName string) (string, error) {
	method, wantPtr := cBinaryMethod(field)
	if method == "" {
		return "", fmt.Errorf("field %q: %s cannot determine how to marshal %s", field.Name, field.Type.Name, CResolveType(field, false, false))
	}

	// Unmarshal uses new(...) for values and enums, so wantPtr is true
	wantPtr = wantPtr || method == "Value" || method == "Enum"

	var ptrPrefix string
	switch {
	case field.MarshalAs == Union:
		// OK
	case wantPtr && !field.Pointer:
		ptrPrefix = "*"
	case !wantPtr && field.Pointer:
		ptrPrefix = "&"
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
	case field.MarshalAs == Union:
		expr, hasIf = fmt.Sprintf("%s.ReadValue(%d, func(b []byte) error { x, err := %s(b); if err == nil { %s }; return err })", readerName, field.Number, goUnionMethod(field, "Unmarshal"), set), false
	case method == "Value":
		expr, hasIf = fmt.Sprintf("if x := new(%s); %s.ReadValue(%d, x.UnmarshalBinary) { %s }", CResolveType(field, true, true), readerName, field.Number, set), true
	case method == "Enum":
		expr, hasIf = fmt.Sprintf("if x := new(%s); %s.ReadEnum(%d, x) { %s }", CResolveType(field, true, true), readerName, field.Number, set), true
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

//
//func CBinaryUnmarshalValue(field *Field, varName, errName string, errArgs ...string) (string, error) {
//	typ := field.Type
//	w := new(strings.Builder)
//
//	var expr, size, init, sliceName string
//	var inPlace bool
//	switch typ.Name {
//	case "rawJson":
//		typ = typegen."bytes"
//		fallthrough
//	case "bool", "bytes", "string", "chainSet", "uvarint", "varint", "duration", "time":
//		expr, size = CMethodName(typ, "UnmarshalBinary")+"(data)", CMethodName(typ, "BinarySize")+"(%s)"
//	case "bigint", "chain":
//		expr, size = CMethodName(typ, "UnmarshalBinary")+"(data)", CMethodName(typ, "BinarySize")+"(&%s)"
//	case "slice":
//		sliceName, varName = varName, "len"+field.Name
//		fmt.Fprintf(w, "var %s uint64\n", varName)
//		expr, size = "encoding.UvarintUnmarshalBinary(data)", "encoding.UvarintBinarySize(%s)"
//	default:
//		switch {
//		case field.AsReference:
//			if field.IsPointer {
//				init = "%s = new(" + CResolveType(field, true) + ")"
//			}
//			expr, size, inPlace = varName+".UnmarshalBinary(data)", "%s.BinarySize()", true
//		case field.AsValue:
//			expr, size, inPlace = varName+".UnmarshalBinary(data)", "%s.BinarySize()", true
//		default:
//			return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, CResolveType(field, false))
//		}
//
//		if field.UnmarshalWith != "" {
//			expr, inPlace = field.UnmarshalWith+"(data)", false
//		}
//	}
//
//	if init != "" {
//		fmt.Fprintf(w, "\t%s\n", fmt.Sprintf(init, varName))
//	}
//
//	size = fmt.Sprintf(size, varName)
//	err := CFieldError("decoding", errName, errArgs...)
//	if inPlace {
//		fmt.Fprintf(w, "\tif err := %s; err != nil { return %s }\n", expr, err)
//	} else if typ == "bigint" {
//		fmt.Fprintf(w, "\tif x, err := %s; err != nil { return %s } else { %s.Set(x) }\n", expr, err, varName)
//	} else {
//		fmt.Fprintf(w, "\tif x, err := %s; err != nil { return %s } else { %s = x }\n", expr, err, varName)
//	}
//	fmt.Fprintf(w, "\tdata = data[%s:]\n\n", size)
//
//	if typ != "slice" {
//		return w.String(), nil
//	}
//
//	fmt.Fprintf(w, "\t%s = make(%s, %s)\n", sliceName, CResolveType(field, false), varName)
//	fmt.Fprintf(w, "\tfor i := range %s {\n", sliceName)
//	if field.Slice.IsPointer {
//		fmt.Fprintf(w, "\t\tvar x %s\n", CResolveType(field.Slice, false))
//		str, err := CBinaryUnmarshalValue(field.Slice, "x", errName+"[%d]", "i")
//		if err != nil {
//			return "", err
//		}
//		w.WriteString(str)
//		fmt.Fprintf(w, "\t\t%s[i] = x", sliceName)
//	} else {
//		str, err := CBinaryUnmarshalValue(field.Slice, sliceName+"[i]", errName+"[%d]", "i")
//		if err != nil {
//			return "", err
//		}
//		w.WriteString(str)
//	}
//	fmt.Fprintf(w, "\t}\n\n")
//	return w.String(), nil
//}

func CValueToJson(field *Field, tgtName, srcName string) string {
	w := new(strings.Builder)
	switch field.Type.Name {
	case "bytes", "chain", "chainSet", "duration", "any":
		fmt.Fprintf(w, "\t%s = %s(%s)", tgtName, CMethodName(field.Type.Name, "ToJSON"), srcName)
		return w.String()
	case "bigint":
		fmt.Fprintf(w, "\t%s = %s(&%s)", tgtName, CMethodName(field.Type.Name, "ToJSON"), srcName)
		return w.String()
	case "slice":
		//if GoJsonType(field.Slice) == "" {
		//	break
		//}
		fmt.Fprintf(w, "\t\"FIXME\"\n")
		//
		//fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, CJsonType(field.Slice), srcName)
		//fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		//w.WriteString(CValueToJson(field.Slice, tgtName+"[i]", "x"))
		//fmt.Fprintf(w, "\t}")
		return w.String()
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s", tgtName, srcName)
	return w.String()
}

func CValueFromJson(field *Field, tgtName, srcName, errName string, errArgs ...string) string {
	w := new(strings.Builder)
	err := CFieldError("decoding", errName, errArgs...)
	switch field.Type.Name {
	case "any":
		fmt.Fprintf(w, "\t%s = %s(%s)\n", tgtName, CMethodName(field.Type.Name, "FromJSON"), srcName)
		return w.String()

	case "bytes", "chain", "chainSet", "duration":
		fmt.Fprintf(w, "\tif x, err := %s(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = x\n\t}", CMethodName(field.Type.Name, "FromJSON"), srcName, err, tgtName)
		return w.String()
	case "bigint":
		fmt.Fprintf(w, "\tif x, err := %s(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = *x\n\t}", CMethodName(field.Type.Name, "FromJSON"), srcName, err, tgtName)
		return w.String()
	case "slice":
		//if CJsonType(field.Slice) == "" {
		//	break
		//}

		//fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, CResolveType(field.Slice, false), srcName)
		//fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		//w.WriteString(CValueFromJson(field.Slice, tgtName+"[i]", "x", errName+"[%d]", "i"))
		//fmt.Fprintf(w, "\t}")
		fmt.Fprintf(w, "\t\"FIXME\"\n")

		return w.String()
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s", tgtName, srcName)
	return w.String()
}

func CCopy(field *Field, dstName, srcName string) (string, error) {
	if !field.Repeatable {
		return goCopy(field, dstName, srcName)
	}

	expr, err := goCopy(field, dstName+"[i]", "v")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"\t%[1]s = make(%[2]s, len(%[3]s))\n"+
			"\tfor i, v := range %[3]s { %s }",
		dstName, CResolveType(field, false, false), srcName, expr), nil
}

func cCopy(field *Field, dstName, srcName string) (string, error) {
	switch field.Type.Code {
	case Bool, String, Duration, Time, Uint, Float, Int, Hash:
		return goCopyNonPointer(field, "%s = %s", dstName, srcName), nil

	case Bytes, RawJson:
		return goCopyNonPointer(field, "%s = encoding.BytesCopy(%s)", dstName, srcName), nil

	case Url, TxID:
		// URLs and TxIDs should be immutable and thus do not need to be copied
		return goCopyPointer(field, "%s", dstName, srcName), nil

	case BigInt:
		return goCopyPointer(field, "encoding.BigintCopy(%s)", dstName, srcName), nil
	}

	switch field.MarshalAs {
	case Union:
		return goCopyNonPointer(field, "if %[1]s != nil { %[2]s = (%[1]s).CopyAsInterface().(%[3]s) }", srcName, dstName, CResolveType(field, false, true)), nil
	case Reference:
		return goCopyPointer(field, "(%s).Copy()", dstName, srcName), nil
	case Value, Enum:
		return goCopyNonPointer(field, "%s = %s", dstName, srcName), nil
	default:
		return "", fmt.Errorf("field %q: cannot determine how to copy %s", field.Name, CResolveType(field, false, false))
	}
}

func cCopyNonPointer(field *Field, expr, dstName, srcName string, exprArgs ...interface{}) string {
	if !field.Pointer {
		exprArgs = append([]interface{}{dstName, srcName}, exprArgs...)
		return fmt.Sprintf(expr, exprArgs...)
	}

	exprArgs = append([]interface{}{"*" + dstName, "*" + srcName}, exprArgs...)
	expr = fmt.Sprintf(expr, exprArgs...)
	return fmt.Sprintf("if %s != nil { %s = new(%s); %s }", srcName, dstName, CResolveType(field, true, true), expr)
}

func cCopyPointer(field *Field, expr, dstName, srcName string) string {
	if field.Pointer {
		expr = fmt.Sprintf(expr, srcName)
		return fmt.Sprintf("if %s != nil { %s = %s }", srcName, dstName, expr)
	}

	expr = fmt.Sprintf(expr, "&"+srcName)
	return fmt.Sprintf("%s = *%s", dstName, expr)
}
