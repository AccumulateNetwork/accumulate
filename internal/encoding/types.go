package encoding

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

// Byter is implemented by any value that has a Bytes method.
type Byter interface {
	Bytes() []byte
}

func AsBytes(v interface{}) []byte {
	switch v := v.(type) {
	case nil:
		return []byte{}
	case []byte:
		return v
	case [32]byte:
		return v[:]
	case *[32]byte:
		return v[:]
	case string:
		return []byte(v)
	case Byter:
		return v.Bytes()
	case interface{ AccountID() []byte }:
		return v.AccountID()
	case uint:
		return common.Uint64Bytes(uint64(v))
	case uint8:
		return common.Uint64Bytes(uint64(v))
	case uint16:
		return common.Uint64Bytes(uint64(v))
	case uint32:
		return common.Uint64Bytes(uint64(v))
	case uint64:
		return common.Uint64Bytes(v)
	case int:
		return common.Int64Bytes(int64(v))
	case int8:
		return common.Int64Bytes(int64(v))
	case int16:
		return common.Int64Bytes(int64(v))
	case int32:
		return common.Int64Bytes(int64(v))
	case int64:
		return common.Int64Bytes(v)
	default:
		panic(fmt.Errorf("cannot use %T as a v", v))
	}
}
