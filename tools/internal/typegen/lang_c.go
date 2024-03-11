// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import "strings"

// CType generated code requires common/encoding.h
func (f FieldType) CType() string {
	switch f.Code {
	case TypeCodeUnknown:
		split := strings.SplitAfter(f.Name, ".")
		return split[len(split)-1]
	case TypeCodeString:
		return "String"
	case TypeCodeBytes:
		return "Bytes"
	case TypeCodeRawJson:
		return "RawJson" //?
	case TypeCodeUrl:
		return "Url" //?
	case TypeCodeTxid:
		return "Bytes32" //?
	case TypeCodeBigInt:
		return "BigInt"
	case TypeCodeUint:
		return "UVarInt"
	case TypeCodeInt:
		return "VarInt"
	case TypeCodeHash:
		return "Bytes32"
	case TypeCodeDuration:
		return "Duration"
	case TypeCodeTime:
		return "Time"
	case TypeCodeAny:
		return "RawJson"
	case TypeCodeBool:
		return "Bool"
	case TypeCodeFloat:
		return "Float"
	default:
		switch f.Name {
		case "AllowedTransactions":
			return "uint64_t"
		}
		return f.Code.String()
	}
}
