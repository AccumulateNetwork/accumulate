// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import "strings"

func (f FieldType) JavaType() string {
	switch f.Code {
	case TypeCodeUnknown:
		split := strings.SplitAfter(f.Name, ".")
		return split[len(split)-1]
	case TypeCodeString:
		return "String"
	case TypeCodeBytes:
		return "byte[]"
	case TypeCodeRawJson:
		return "RawJson"
	case TypeCodeUrl:
		return "Url"
	case TypeCodeTxid:
		return "TxID"
	case TypeCodeBigInt:
		return "java.math.BigInteger"
	case TypeCodeUint:
		return "long"
	case TypeCodeInt:
		return "int"
	case TypeCodeHash:
		return "byte[]"
	case TypeCodeDuration:
		return "Duration"
	case TypeCodeTime:
		return "java.time.OffsetDateTime"
	case TypeCodeAny:
		return "JsonNode"
	case TypeCodeBool:
		return "boolean"
	case TypeCodeFloat:
		return "float"
	default:
		switch f.Name {
		case "AllowedTransactions":
			return "long"
		}
		return f.Code.String()
	}
}
