// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import "strings"

func (f FieldType) KotlinType() string {
	switch f.Code {
	case TypeCodeUnknown:
		split := strings.SplitAfter(f.Name, ".")
		return split[len(split)-1]
	case TypeCodeString:
		return "String"
	case TypeCodeBytes:
		return "ByteArray"
	case TypeCodeRawJson:
		return "RawJson"
	case TypeCodeUrl:
		return "AccUrl"
	case TypeCodeTxid:
		return "TxID"
	case TypeCodeBigInt:
		return "ULong"
	case TypeCodeUint:
		return "Long"
	case TypeCodeInt:
		return "Int"
	case TypeCodeHash:
		return "ByteArray"
	case TypeCodeDuration:
		return "Duration"
	case TypeCodeTime:
		return "kotlinx.datetime.Instant"
	case TypeCodeAny:
		return "jsonElement"
	case TypeCodeBool:
		return "Boolean"
	case TypeCodeFloat:
		return "Float"
	default:
		switch f.Name {
		case "AllowedTransactions":
			return "Long"
		}
		return f.Code.String()
	}
}
