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
const debugPrintKeys = false

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

func (k Key) Append(key ...interface{}) Key {
	// If k is the zero value, don't stringify it
	var s string
	if debugKeys && k != (Key{}) {
		s = k.String()
	}

	for _, key := range key {
		bytes, printv := convert(key)
		b := make([]byte, KeyLength+len(bytes))
		copy(b, k[:])
		copy(b[KeyLength:], bytes)
		k = sha256.Sum256(b)

		if debugKeys {
			if printv {
				s += fmt.Sprintf(".%v", key)
			} else {
				s += fmt.Sprintf(".%X", bytes)
			}
		}
	}

	if !debugKeys {
		return k
	}

	// If k was originally the zero value, remove the leading dot
	if len(s) > 0 && s[0] == '.' {
		s = s[1:]
	}

	debugKeyMu.Lock()
	debugKeyMap[k] = s
	debugKeyMu.Unlock()

	if debugPrintKeys {
		fmt.Printf("Key %s => %X\n", s, k[:])
	}
	return k
}

func convert(key interface{}) (bytes []byte, printVal bool) {
	switch key := key.(type) {
	case nil:
		return []byte{}, false
	case []byte:
		return key, false
	case [32]byte:
		return key[:], false
	case types.Bytes:
		return key, false
	case types.Bytes32:
		return key[:], false
	case string:
		return []byte(key), true
	case types.String:
		return []byte(key), true
	case interface{ Bytes() []byte }:
		return key.Bytes(), false
	case interface{ AccountID() []byte }:
		return key.AccountID(), true
	case uint:
		return common.Uint64Bytes(uint64(key)), true
	case uint8:
		return common.Uint64Bytes(uint64(key)), true
	case uint16:
		return common.Uint64Bytes(uint64(key)), true
	case uint32:
		return common.Uint64Bytes(uint64(key)), true
	case uint64:
		return common.Uint64Bytes(key), true
	case int:
		return common.Int64Bytes(int64(key)), true
	case int8:
		return common.Int64Bytes(int64(key)), true
	case int16:
		return common.Int64Bytes(int64(key)), true
	case int32:
		return common.Int64Bytes(int64(key)), true
	case int64:
		return common.Int64Bytes(key), true
	default:
		panic(fmt.Errorf("cannot use %T as a key", key))
	}
}

func MakeKey(keys ...interface{}) Key {
	if len(keys) == 0 {
		return Key{}
	}

	// If the first value is a Key, append to that
	k, ok := keys[0].(Key)
	if ok {
		return k.Append(keys[1:]...)
	}
	return (Key{}).Append(keys...)
}
