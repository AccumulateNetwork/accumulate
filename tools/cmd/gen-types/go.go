// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

const (
	Int      = typegen.TypeCodeInt
	Uint     = typegen.TypeCodeUint
	Bool     = typegen.TypeCodeBool
	String   = typegen.TypeCodeString
	Hash     = typegen.TypeCodeHash
	Bytes    = typegen.TypeCodeBytes
	Url      = typegen.TypeCodeUrl
	TxID     = typegen.TypeCodeTxid
	Time     = typegen.TypeCodeTime
	Duration = typegen.TypeCodeDuration
	BigInt   = typegen.TypeCodeBigInt
	Any      = typegen.TypeCodeAny
	RawJson  = typegen.TypeCodeRawJson
	Float    = typegen.TypeCodeFloat

	// Basic     = typegen.MarshalAsBasic
	// None      = typegen.MarshalAsNone
	Enum      = typegen.MarshalAsEnum
	Value     = typegen.MarshalAsValue
	Reference = typegen.MarshalAsReference
	Union     = typegen.MarshalAsUnion
)

//go:embed types.go.tmpl
var goSrc string

//go:embed union.go.tmpl
var goUnionSrc string

//go:embed types.machine.go.tmpl
var goMachineSrc string

func init() {
	Templates.Register(goSrc, "go", goFuncs, "Go")
	Templates.Register(goUnionSrc, "go-union", goFuncs)
	Templates.Register(goMachineSrc, "go-alt", goFuncs)
}

var goFuncs = template.FuncMap{
	"unionMethod": GoTmplUnionMethod,
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
	"dec": func(v uint) uint {
		return v - 1
	},

	"resolveType": func(field *Field, forNew bool) string {
		return GoResolveType(field, forNew, false)
	},
	"resolveEip712Type": func(field *Field, forNew bool) string {
		return GoResolveEip712Type(field, forNew, false)
	},
	"resolveElemType": func(field *Field, forNew bool) string {
		return GoResolveType(field, forNew, true)
	},

	"jsonType": func(field *Field) string {
		typ := GoJsonType(field, "*")
		if typ == "" {
			typ = GoResolveType(field, false, false)
		}
		return typ
	},

	"get":                     GoGetField,
	"areEqual":                GoAreEqual,
	"copy":                    GoCopy,
	"binaryMarshalValue":      GoBinaryMarshalValue,
	"binaryUnmarshalValue":    GoBinaryUnmarshalValue,
	"valueToJson":             GoValueToJson,
	"valueFromJson":           GoValueFromJson,
	"jsonZeroValue":           GoJsonZeroValue,
	"isZero":                  GoIsZero,
	"errVirtualFieldNotEqual": GoErrVirtualFieldNotEqual,

	"needsCustomJSON": func(typ *Type) bool {
		if typ.NonJSON {
			return false
		}
		if typ.IsUnion() {
			return true
		}

		// Add a custom un/marshaller if the type embeds another type - fields
		// of embedded types are un-embedded during JSON un/marshalling
		if len(typ.Embeddings) > 0 {
			return true
		}

		for _, f := range typ.Fields {
			// Add a custom un/marshaller if the type embeds another type
			if f.IsEmbedded {
				return true
			}

			// Add a custom un/marshaller if the field needs special handling
			if GoJsonType(f, "") != "" {
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

	"accessor": goFieldAccessor,
}

func GoTmplUnionMethod(u *UnionSpec, method string) string {
	if u.PrivateUnion {
		method = typegen.LowerFirstWord(method)
	}
	if flags.ElidePackageType && strings.EqualFold(u.Name, u.Package) {
		return method
	}
	return method + u.interfaceName(true)
}

func GoGetField(field *Field) string {
	if field.Virtual {
		return field.Name + "()"
	}
	return field.Name
}

func GoFieldError(op, name string, args ...string) string {
	args = append(args, "err")
	return fmt.Sprintf("fmt.Errorf(\"error %s %s: %%w\", %s)", op, name, strings.Join(args, ","))
}

func goFieldAccessor(field *Field) (string, error) {
	var ptr string
	if field.Pointer {
		ptr = "Ptr"
	}

	var typ string
	switch {
	case field.Repeatable:
		var ptr string
		if field.Pointer {
			ptr = "*"
		}
		typ = "encoding.SliceIndex[" + ptr + field.Type.GoType() + "]"
	case field.Virtual:
		typ = field.Type.GoType()
	default:
		typ = "*" + field.ParentType.Name
	}

	switch field.Type.Code {
	case Hash, Int, Uint, Float, Bool, Time, Bytes, String, Duration, BigInt, Url:
		return fmt.Sprintf("encoding.%s%sField[%s]", typegen.TitleCase(field.Type.Code.String()), ptr, typ), nil
	case TxID:
		return fmt.Sprintf("encoding.TxID%sField[%s]", ptr, typ), nil
	}

	switch field.MarshalAs {
	case Reference:
		return fmt.Sprintf("encoding.Struct%sField[%s, *%s, %[3]s]", ptr, typ, GoResolveType(field, true, true)), nil
	case Enum:
		return fmt.Sprintf("encoding.Enum%sField[%s, *%s, %[3]s]", ptr, typ, GoResolveType(field, true, true)), nil
	}

	return "nil", nil
}

func goUnionMethod(field *Field, name string) string {
	var typ string
	param, ok := field.ParentType.ResolveTypeParam(&field.Field)
	if ok {
		typ = param.Type
	} else {
		typ = field.Type.String()
	}

	parts := strings.SplitN(typ, ".", 2)
	if field.ElideUnionMethods {
		if len(parts) == 1 {
			return name
		}
		return fmt.Sprintf("%s.%s", parts[0], name)
	}
	if len(parts) > 1 {
		return fmt.Sprintf("%s.%s%s", parts[0], name, parts[1])
	}

	// Is the union private?
	if !unicode.IsLower(rune(parts[0][0])) {
		return name + parts[0]
	}

	return strings.ToLower(name[:1]) + name[1:] + strings.ToUpper(parts[0][:1]) + parts[0][1:]
}

func goBinaryMethod(field *Field) (methodName string, cast, wantPtr bool) {
	cast = field.MarshalAsType != typegen.TypeCodeUnknown
	switch code := field.EffectiveMarshalType(); code {
	case Bool, String, Duration, Time, Bytes, Uint, Int, Float:
		return typegen.TitleCase(code.String()), cast, false
	case Url, TxID, Hash:
		return typegen.TitleCase(code.String()), cast, true
	case RawJson:
		return "Bytes", cast, false
	case BigInt:
		return "BigInt", cast, true
	}

	switch field.MarshalAs {
	case Reference:
		return "Value", cast, true
	case Value, Union:
		return "Value", cast, false
	case Enum:
		return "Enum", cast, false
	}

	return "", cast, false
}

func goJsonMethod(field *Field) (methodName string, wantPtr bool) {
	switch code := field.EffectiveMarshalType(); code {
	case Bytes, Duration, Any:
		return code.Title(), false
	case Hash:
		return "Chain", true
	case BigInt:
		return "Bigint", true
	}

	return "", false
}

func GoResolveType(field *Field, forNew, ignoreRepeatable bool) string {
	typ := field.Type.GoType()
	if field.Pointer && !forNew {
		typ = "*" + typ
	}
	if field.Repeatable && !ignoreRepeatable {
		typ = "[]" + typ
	}
	return typ
}

func GoResolveEip712Type(field *Field, forNew, ignoreRepeatable bool) string {
	typ := field.Type.GoType()
	switch code := field.EffectiveMarshalType(); code {
	case Int, Uint, Bool, String:
		typ = field.Type.GoType()
	case Hash:
		typ = "bytes32"
	case Bytes:
		typ = "bytes"
	case Url, TxID:
		typ = "string" // reduces the Url and transaction id to a string
	case Time, Duration:
		typ = "string" //???? reduces time.Time and time.Duration to a string
	case BigInt:
		typ = "uint256"
	case RawJson:
		typ = "string"
	case Float:
		typ = "float"
	case Any:
		typ = code.Title()
	}

	field.Type.GoType()
	if field.AsEnum() {
		typ = "string" //interpret as string types
	}

	ret := ""
	lcName := typegen.LowerFirstWord(field.Name)
	if field.Repeatable && !ignoreRepeatable {
		ret = "encoding.NewTypeField(\"" + lcName + "\",\"" + typ + "[]\")"
	} else {
		ret = "encoding.NewTypeField(\"" + lcName + "\",\"" + typ + "\")"
	}
	return ret
}

func goJsonTypeSingle(field *Field, pointer string) string {
	switch field.EffectiveMarshalType() {
	case Bytes:
		return "*string"
	case BigInt:
		return "*string"
	case Hash:
		return "*string"
	case Duration, Any:
		return "interface{}"
	}

	if field.MarshalAs != Union {
		return ""
	}

	return pointer + "encoding.JsonUnmarshalWith[" + GoResolveType(field, false, true) + "]"
}

func GoJsonType(field *Field, pointer string) string {
	if field.Repeatable && field.MarshalAs == Union {
		return pointer + "encoding.JsonUnmarshalListWith[" + GoResolveType(field, false, true) + "]"
	}

	typ := goJsonTypeSingle(field, pointer)
	switch {
	case !field.Repeatable:
		return typ
	case typ != "":
		return "encoding.JsonList[" + typ + "]"
	default:
		return "encoding.JsonList[" + GoResolveType(field, false, true) + "]"
	}
}

func GoErrVirtualFieldNotEqual(field *Field, varName, valName string) (string, error) {
	return fmt.Sprintf(`return fmt.Errorf("field %s: not equal: want %%%%v, got %%%%v", %s, %s)`, field.Name, varName, valName), nil
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

	switch field.EffectiveMarshalType() {
	case Bytes, RawJson, String:
		return fmt.Sprintf("len(%s) == 0", varName), nil
	case Any:
		return fmt.Sprintf("%s == nil", varName), nil
	case Bool:
		return fmt.Sprintf("!%s", varName), nil
	case Uint, Int, Duration, Float:
		return fmt.Sprintf("%s == 0", varName), nil
	case BigInt:
		return fmt.Sprintf("(%s).Cmp(new(big.Int)) == 0", varName), nil
	case Url, TxID, Hash, Time:
		return fmt.Sprintf("%s == (%s{})", varName, GoResolveType(field, false, false)), nil
	}

	switch field.MarshalAs {
	case Reference:
		return fmt.Sprintf("(%s).Equal(new(%s))", varName, field.Type), nil
	case Enum:
		return fmt.Sprintf("%s == 0", varName), nil
	case Union:
		return fmt.Sprintf("%s(%s, nil)", goUnionMethod(field, "Equal"), varName), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, GoResolveType(field, false, false))
}

func GoJsonZeroValue(field *Field) (string, error) {
	if field.ZeroValue != nil {
		return fmt.Sprint(field.ZeroValue), nil
	}
	if field.Pointer {
		return "nil", nil
	}

	switch field.EffectiveMarshalType() {
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
		return fmt.Sprintf("(%s{})", GoResolveType(field, false, false)), nil
	}

	return "", fmt.Errorf("field %q: cannot determine zero value for %s", field.Name, GoResolveType(field, false, false))
}

func GoAreEqual(field *Field, varName, otherName, whenNotEqual string) (string, error) {
	var expr string
	var wantPtr bool
	switch field.EffectiveMarshalType() {
	case Bool, String, Hash, Uint, Int, Float, Duration, Any:
		expr, wantPtr = "%[1]s%[2]s == %[1]s%[3]s", false
	case Bytes, RawJson:
		expr, wantPtr = "bytes.Equal(%[1]s%[2]s, %[1]s%[3]s)", false
	case BigInt:
		expr, wantPtr = "(%[1]s%[2]s).Cmp(%[1]s%[3]s) == 0", true
	case Url, TxID:
		expr, wantPtr = "(%[1]s%[2]s).Equal(%[1]s%[3]s)", true
	case Time:
		expr, wantPtr = "(%[1]s%[2]s).Equal(%[1]s%[3]s)", false
	default:
		switch field.MarshalAs {
		case Union:
			expr, wantPtr = goUnionMethod(field, "Equal")+"(%[1]s%[2]s,%[1]s%[3]s)", false
		case Reference:
			expr, wantPtr = "(%[1]s%[2]s).Equal(%[1]s%[3]s)", true
		case Value, Enum:
			expr, wantPtr = "%[1]s%[2]s == %[1]s%[3]s", false
		default:
			return "", fmt.Errorf("field %q: %s cannot determine how to compare %s", field.Name, field.Type.Name, GoResolveType(field, false, false))
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

	if field.Repeatable {
		expr = fmt.Sprintf(expr, ptrPrefix, "%[2]s[i]", "%[3]s[i]")
		return fmt.Sprintf(
			"	if len(%[2]s) != len(%[3]s) { "+whenNotEqual+" }\n"+
				"	for i := range %[2]s {\n"+
				"		if !("+expr+") { "+whenNotEqual+" }\n"+
				"	}",
			ptrPrefix, varName, otherName), nil
	}

	if !field.Pointer {
		return fmt.Sprintf("\tif !("+expr+") { "+whenNotEqual+" }", ptrPrefix, varName, otherName), nil
	}

	return fmt.Sprintf(
		"	switch {\n"+
			"	case %[2]s == %[3]s:\n"+
			"		// equal\n"+
			"	case %[2]s == nil || %[3]s == nil:\n"+
			"		return false\n"+
			"	case !("+expr+"):\n"+
			"		return false\n"+
			"	}",
		ptrPrefix, varName, otherName), nil
}

func GoCopy(field *Field, dstName, srcName string) (string, error) {
	if !field.Repeatable {
		return goCopy(field, dstName, srcName)
	}

	expr, err := goCopy(field, dstName+"[i]", "v")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"\t%[1]s = make(%[2]s, len(%[3]s))\n"+
			"\tfor i, v := range %[3]s { v := v; %s }",
		dstName, GoResolveType(field, false, false), srcName, expr), nil
}

func goCopy(field *Field, dstName, srcName string) (string, error) {
	switch field.EffectiveMarshalType() {
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
		var nilCheck string
		_, ok := field.ParentType.ResolveTypeParam(&field.Field)
		if ok {
			nilCheck = "if !" + goUnionMethod(field, "Equal") + "(%[1]s, %[5]s)"
		} else {
			nilCheck = "if %[1]s != %[5]s"
		}
		var assert string
		for _, param := range field.ParentType.Params {
			if field.Type.Name == param.Name {
				assert = ".(" + GoResolveType(field, false, true) + ")"
				break
			}
		}
		zeroValue := "nil"
		if field.ZeroValue != nil {
			zeroValue = fmt.Sprint(field.ZeroValue)
		}
		format := nilCheck + " { %[2]s = %[3]s(%[1]s)%[4]s }"
		return goCopyNonPointer(field, format, srcName, dstName, goUnionMethod(field, "Copy"), assert, zeroValue), nil
	case Reference:
		return goCopyPointer(field, "(%s).Copy()", dstName, srcName), nil
	case Value, Enum:
		return goCopyNonPointer(field, "%s = %s", dstName, srcName), nil
	default:
		return "", fmt.Errorf("field %q: cannot determine how to copy %s", field.Name, GoResolveType(field, false, false))
	}
}

func goCopyNonPointer(field *Field, expr, dstName, srcName string, exprArgs ...interface{}) string {
	if !field.Pointer {
		exprArgs = append([]interface{}{dstName, srcName}, exprArgs...)
		return fmt.Sprintf(expr, exprArgs...)
	}

	exprArgs = append([]interface{}{"*" + dstName, "*" + srcName}, exprArgs...)
	expr = fmt.Sprintf(expr, exprArgs...)
	return fmt.Sprintf("if %s != nil { %s = new(%s); %s }", srcName, dstName, GoResolveType(field, true, true), expr)
}

func goCopyPointer(field *Field, expr, dstName, srcName string) string {
	if field.Pointer {
		expr = fmt.Sprintf(expr, srcName)
		return fmt.Sprintf("if %s != nil { %s = %s }", srcName, dstName, expr)
	}

	expr = fmt.Sprintf(expr, "&"+srcName)
	return fmt.Sprintf("%s = *%s", dstName, expr)
}

func GoBinaryMarshalValue(field *Field, writerName, varName string) (string, error) {
	method, cast, wantPtr := goBinaryMethod(field)
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

	var suffix string
	if method == "Value" {
		ptrPrefix = ""
		suffix = ".MarshalBinary"
	}

	var castPrefix, castSuffix string
	if cast {
		castPrefix, castSuffix = "("+field.MarshalAsType.GoType()+")(", ")"
	}

	if !field.Repeatable {
		return fmt.Sprintf("\t%s.Write%s(%d, %s%s%s%s%s)", writerName, method, field.Number, castPrefix, ptrPrefix, varName, suffix, castSuffix), nil
	}

	return fmt.Sprintf("\tfor _, v := range %s { %s.Write%s(%d, %s%sv%s%s) }", varName, writerName, method, field.Number, castPrefix, ptrPrefix, suffix, castSuffix), nil
}

func GoBinaryUnmarshalValue(field *Field, readerName, varName string) (string, error) {
	method, cast, wantPtr := goBinaryMethod(field)
	if method == "" {
		return "", fmt.Errorf("field %q: cannot determine how to marshal %s", field.Name, GoResolveType(field, false, false))
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

	var castPrefix, castSuffix string
	if cast {
		castPrefix, castSuffix = "("+GoResolveType(field, false, true)+")(", ")"
	}

	var set string
	if field.Repeatable {
		set = fmt.Sprintf("%s = append(%[1]s, %s%sx%s)", varName, castPrefix, ptrPrefix, castSuffix)
	} else {
		set = fmt.Sprintf("%s = %s%sx%s", varName, castPrefix, ptrPrefix, castSuffix)
	}

	var expr string
	var hasIf bool
	switch {
	case field.MarshalAs == Union:
		unmarshal := goUnionMethod(field, "Unmarshal") + "From"
		param, ok := field.ParentType.ResolveTypeParam(&field.Field)
		if ok {
			unmarshal = fmt.Sprintf("encoding.Cast[%s](%s(r))", param.Name, unmarshal)
		} else {
			unmarshal += "(r)"
		}
		expr, hasIf = fmt.Sprintf("%s.ReadValue(%d, func(r io.Reader) error { x, err := %s; if err == nil { %s }; return err })", readerName, field.Number, unmarshal, set), false
	case method == "Value":
		expr, hasIf = fmt.Sprintf("if x := new(%s); %s.ReadValue(%d, x.UnmarshalBinaryFrom) { %s }", GoResolveType(field, true, true), readerName, field.Number, set), true
	case method == "Enum":
		var x = "x"
		if isGenericParameter(field.ParentType, field.Type.String()) {
			x = "any(x).(encoding.EnumValueSetter)" // This is a hack to avoid requiring more complex generic type parameterization
		}
		expr, hasIf = fmt.Sprintf("if x := new(%s); %s.ReadEnum(%d, %s) { %s }", GoResolveType(field, true, true), readerName, field.Number, x, set), true
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

func isGenericParameter(parent *Type, typ string) bool {
	for _, param := range parent.Params {
		if param.Name == typ {
			return true
		}
	}
	return false
}

func GoValueToJson(field *Field, tgtName, srcName string) (string, error) {
	if field.MarshalAs == Union {
		unmarshal := goUnionMethod(field, "Unmarshal") + "JSON"
		param, ok := field.ParentType.ResolveTypeParam(&field.Field)
		if ok {
			unmarshal = fmt.Sprintf("func(b []byte) (%s, error) { return encoding.Cast[%[1]s](%s(b)) }", param.Name, unmarshal)
		}
		s := fmt.Sprintf("\t%s = %s{Value: %s, Func: %s}", tgtName, GoJsonType(field, "&"), srcName, unmarshal)
		return s, nil
	}

	method, wantPtr := goJsonMethod(field)
	var ptrPrefix string
	var checkNil bool
	switch {
	case method == "":
		return fmt.Sprintf("\t%s = %s", tgtName, srcName), nil
	case wantPtr && !field.Pointer:
		ptrPrefix = "&"
	case !wantPtr && field.Pointer:
		ptrPrefix, checkNil = "*", true
	}

	if !field.Repeatable {
		format := "\t%s = encoding.%sToJSON(%s%s)"
		if checkNil {
			format = "\tif %[4]s != nil { %[1]s = encoding.%[2]sToJSON(%[3]s%[4]s) }"
		}
		return fmt.Sprintf(format, tgtName, method, ptrPrefix, srcName), nil
	}

	format := "\t%s = make(%s, len(%s)); for i, x := range %[3]s { %[1]s[i] = encoding.%[4]sToJSON(%sx) }"
	if checkNil {
		format = "\t%s = make(%s, len(%s)); for i, x := range %[3]s { if x != nil { %[1]s[i] = encoding.%[4]sToJSON(%sx) } }"
	}
	return fmt.Sprintf(format, tgtName, GoJsonType(field, "&"), srcName, method, ptrPrefix), nil
}

func GoValueFromJson(field *Field, tgtName, srcName, errName string, errArgs ...string) (string, error) {
	err := GoFieldError("decoding", errName, errArgs...)
	if field.MarshalAs == Union {
		if !field.Repeatable {
			return fmt.Sprintf("\tif %s != nil { %s = %[1]s.Value }\n", srcName, tgtName), nil
		}
		return fmt.Sprintf(
			"	if %[3]s != nil && %[3]s.Value != nil {\n"+
				"		%[1]s = make(%s, len(%s.Value));\n"+
				"		for i, x := range %[3]s.Value {\n"+
				"			%[1]s[i] = x\n"+
				"		}\n"+
				"	}",
			tgtName, GoResolveType(field, false, false), srcName), nil
	}

	method, wantPtr := goJsonMethod(field)
	var ptrPrefix string
	switch {
	case method == "":
		return fmt.Sprintf("\t%s = %s", tgtName, srcName), nil
	case wantPtr && !field.Pointer:
		ptrPrefix = "*"
	case !wantPtr && field.Pointer:
		ptrPrefix = "&"
	}

	if field.Repeatable {
		return fmt.Sprintf("\t%s = make(%s, len(%s)); for i, x := range %[3]s { if x, err := encoding.%sFromJSON(x); err != nil { return %s } else { %[1]s[i] = %[6]sx } }", tgtName, GoResolveType(field, false, false), srcName, method, err, ptrPrefix), nil
	}

	expr := fmt.Sprintf("if x, err := encoding.%sFromJSON(%s); err != nil { return %s } else { %s = %sx }", method, srcName, err, tgtName, ptrPrefix)
	if field.Pointer {
		expr = fmt.Sprintf("if %s != nil { %s }", srcName, expr)
	}

	return "\t" + expr, nil
}
