// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

func (t TypeCode) GoType() string {
	switch t {
	case TypeCodeBytes:
		return "[]byte"
	case TypeCodeRawJson:
		return "json.RawMessage"
	case TypeCodeUrl:
		return "url.URL"
	case TypeCodeTxid:
		return "url.TxID"
	case TypeCodeBigInt:
		return "big.Int"
	case TypeCodeUint:
		return "uint64"
	case TypeCodeInt:
		return "int64"
	case TypeCodeHash:
		return "[32]byte"
	case TypeCodeDuration:
		return "time.Duration"
	case TypeCodeTime:
		return "time.Time"
	case TypeCodeAny:
		return "interface{}"
	case TypeCodeFloat:
		return "float64"
	default:
		return t.String()
	}
}

func (f FieldType) GoType() string {
	if f.Code == TypeCodeUnknown {
		return f.Name
	}
	return f.Code.GoType()
}
