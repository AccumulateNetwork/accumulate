package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

const (
	KeyLength = 32 // Total bytes used for keys
)

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
		fmt.Printf("Key %s => %X\n", s, k[:]) //nolint:noprint
	}
	return k
}

func convert(key interface{}) (bytes []byte, printVal bool) {
	bytes = encoding.AsBytes(key)

	switch key.(type) {
	case nil, []byte, [32]byte, *[32]byte, encoding.Byter:
		return bytes, false
	default:
		return bytes, true
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
