package logging

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

type Hex []byte

func (h Hex) MarshalJSON() ([]byte, error) {
	b := make([]byte, hex.EncodedLen(len(h)))
	hex.Encode(b, h)
	return json.Marshal(strings.ToUpper(string(b)))
}

//go:inline
func AsHex(v interface{}) interface{} {
	switch v := v.(type) {
	case []byte:
		u := make(Hex, len(v))
		copy(u, v)
		return u
	case [32]byte:
		return Hex(v[:])
	case *[32]byte:
		return Hex(v[:])
	case string:
		return Hex(v)
	case encoding.Byter:
		return Hex(v.Bytes())
	case fmt.Stringer:
		return Hex(v.String())
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		v := make([]interface{}, rv.Len())
		for i := range v {
			v[i] = AsHex(rv.Index(i).Interface())
		}
		return v
	}

	return Hex(fmt.Sprint(v))
}
