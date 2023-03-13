// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

func (f FieldType) TypescriptType() string {
	switch f.Code {
	case TypeCodeUnknown:
		return f.Name
	case TypeCodeBytes:
		return "Uint8Array"
	case TypeCodeRawJson:
		return "unknown"
	case TypeCodeUrl:
		return "URL"
	case TypeCodeTxid:
		return "TxID"
	case TypeCodeBigInt:
		return "BN"
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
		return f.Code.String()
	}
}

func (f FieldType) TypescriptInputType() string {
	switch f.Code {
	case TypeCodeUnknown:
		return f.Name
	case TypeCodeBytes:
		return "Uint8Array | string"
	case TypeCodeRawJson:
		return "unknown"
	case TypeCodeUrl:
		return "URL | string"
	case TypeCodeTxid:
		return "TxID | string"
	case TypeCodeBigInt:
		return "BN | string"
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
		return f.Code.String()
	}
}
