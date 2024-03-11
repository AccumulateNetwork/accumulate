// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import "strings"

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
	if f.Code != TypeCodeUnknown {
		return f.Code.GoType()
	}
	if len(f.Parameters) == 0 {
		return f.Name
	}
	var params []string
	for _, p := range f.Parameters {
		s := p.Type.GoType()
		if p.Pointer {
			s = "*" + s
		}
		params = append(params, s)
	}
	return f.Name + "[" + strings.Join(params, ", ") + "]"
}
