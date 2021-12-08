package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
)

const (
	KeyLength = 32 // Total bytes used for keys
)

const debugKeys = false

var debugKeyMap = map[Key]string{}
var debugKeyMu = new(sync.RWMutex)

type Key [KeyLength]byte

// String hex encodes the key. If debugging is enabled, String looks up the original composite key.
func (k Key) String() string {
	if len(k) == 0 {
		return "(empty)"
	}
	if !debugKeys {
		return fmt.Sprintf("%X", k[:])
	}

	debugKeyMu.RLock()
	v := debugKeyMap[k]
	debugKeyMu.RUnlock()

	if v != "" {
		return v
	}
	return fmt.Sprintf("%X", k[:])
}

// MarshalJSON is implemented for JSON-based logging
func (k Key) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func ComputeKey(keys ...interface{}) Key {
	// Allow pre-computed keys
	if len(keys) == 1 {
		k, ok := keys[0].(Key)
		if ok {
			return k
		}
	}

	n := -1
	bkeys := make([][]byte, len(keys))
	printv := make([]bool, len(keys))
	for i, key := range keys {
		switch key := key.(type) {
		case nil:
			bkeys[i] = []byte{}
		case []byte:
			bkeys[i] = key
		case [32]byte:
			bkeys[i] = key[:]
		case types.Bytes:
			bkeys[i] = key
		case types.Bytes32:
			bkeys[i] = key[:]
		case string:
			bkeys[i] = []byte(key)
			printv[i] = true
		case types.String:
			bkeys[i] = []byte(key)
			printv[i] = true
		case interface{ Bytes() []byte }:
			bkeys[i] = key.Bytes()
		case uint:
			bkeys[i] = common.Uint64Bytes(uint64(key))
			printv[i] = true
		case uint8:
			bkeys[i] = common.Uint64Bytes(uint64(key))
			printv[i] = true
		case uint16:
			bkeys[i] = common.Uint64Bytes(uint64(key))
			printv[i] = true
		case uint32:
			bkeys[i] = common.Uint64Bytes(uint64(key))
			printv[i] = true
		case uint64:
			bkeys[i] = common.Uint64Bytes(key)
			printv[i] = true
		case int:
			bkeys[i] = common.Int64Bytes(int64(key))
			printv[i] = true
		case int8:
			bkeys[i] = common.Int64Bytes(int64(key))
			printv[i] = true
		case int16:
			bkeys[i] = common.Int64Bytes(int64(key))
			printv[i] = true
		case int32:
			bkeys[i] = common.Int64Bytes(int64(key))
			printv[i] = true
		case int64:
			bkeys[i] = common.Int64Bytes(key)
			printv[i] = true
		default:
			panic(fmt.Errorf("cannot use %T as a key", key))
		}

		n += len(bkeys[i]) + 1
	}

	composite := make([]byte, 0, n)
	for i, k := range bkeys {
		if i > 0 {
			composite = append(composite, '.')
		}
		composite = append(composite, k...)
	}

	h := sha256.Sum256(composite)
	if !debugKeys {
		return h
	}

	var str string
	for i := range bkeys {
		if i > 0 {
			str += "."
		}
		if printv[i] {
			str += fmt.Sprint(keys[i])
		} else {
			str += fmt.Sprintf("%X", bkeys[i])
		}
	}

	debugKeyMu.Lock()
	debugKeyMap[h] = str
	debugKeyMu.Unlock()

	fmt.Printf("Key %s => %X\n", str, h)

	return h
}
