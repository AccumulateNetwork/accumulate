// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import "strings"

func (t TypeCode) TypescriptType() string {
	switch t {
	case TypeCodeBytes:
		return "Uint8Array"
	case TypeCodeRawJson:
		return "unknown"
	case TypeCodeUrl:
		return "URL"
	case TypeCodeTxid:
		return "TxID"
	case TypeCodeBigInt:
		return "bigint"
	case TypeCodeUint:
		return "number"
	case TypeCodeInt:
		return "number"
	case TypeCodeHash:
		return "Uint8Array"
	case TypeCodeDuration:
		return "number"
	case TypeCodeTime:
		return "Date"
	case TypeCodeBool:
		return "boolean"
	default:
		return t.String()
	}
}

func (t TypeCode) TypescriptInputType() string {
	switch t {
	case TypeCodeBytes:
		return "Uint8Array | string"
	case TypeCodeRawJson:
		return "unknown"
	case TypeCodeUrl:
		return "URLArgs"
	case TypeCodeTxid:
		return "TxIDArgs"
	case TypeCodeBigInt:
		return "bigint | string | number"
	case TypeCodeUint:
		return "number"
	case TypeCodeInt:
		return "number"
	case TypeCodeHash:
		return "Uint8Array | string"
	case TypeCodeDuration:
		return "number"
	case TypeCodeTime:
		return "Date | string"
	case TypeCodeBool:
		return "boolean"
	default:
		return t.String()
	}
}

func (f FieldType) TypescriptType(args bool) string {
	if f.Code != TypeCodeUnknown {
		return f.Code.TypescriptType()
	}
	var suffix string
	if args {
		suffix = "Args"
	}
	if len(f.Parameters) == 0 {
		return f.Name + suffix
	}
	var params []string
	for _, p := range f.Parameters {
		params = append(params, p.Type.TypescriptType(false))
	}
	return f.Name + suffix + "<" + strings.Join(params, ", ") + ">"
}

func (f FieldType) TypescriptInputType(args bool) string {
	if f.Code != TypeCodeUnknown {
		return f.Code.TypescriptInputType()
	}
	return f.TypescriptType(args)
}

func (t *Type) TypescriptFullName(args, withType, paramName, paramType bool) string {
	var suffix string
	if args {
		suffix = "Args"
		if withType {
			suffix += "WithType"
		}
	}
	if len(t.Params) == 0 {
		return t.Name + suffix
	}
	var params []string
	for _, p := range t.Params {
		switch {
		case paramName && paramType && p.Type != "any":
			params = append(params, p.Name+" extends "+p.Type+" = "+p.Type)
		case paramName:
			params = append(params, p.Name)
		case paramType:
			params = append(params, p.Type)
		}
	}
	return t.Name + suffix + "<" + strings.Join(params, ", ") + ">"
}
