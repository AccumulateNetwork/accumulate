package logging

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"
)

type Hex []byte

func (h Hex) MarshalJSON() ([]byte, error) {
	b := make([]byte, hex.EncodedLen(len(h)))
	hex.Encode(b, h)
	return json.Marshal(string(b))
}

//go:inline
func AsHex(v interface{}) Hex {
	switch v := v.(type) {
	case []byte:
		return Hex(v)
	case [32]byte:
		return Hex(v[:])
	case string:
		return Hex(v)
	case types.Bytes:
		return Hex(v)
	case types.Bytes32:
		return Hex(v[:])
	case types.String:
		return Hex(v)
	case *types.Bytes:
		if v == nil {
			return Hex(nil)
		}
		return Hex(*v)
	case *types.Bytes32:
		if v == nil {
			return Hex(nil)
		}
		return Hex(v[:])
	case *types.String:
		if v == nil {
			return Hex(nil)
		}
		return Hex(*v)
	case fmt.Stringer:
		return Hex(v.String())
	default:
		return Hex(fmt.Sprint(v))
	}
}
