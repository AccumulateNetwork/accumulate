// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
)

const KeyHashLength = 32

const debugPrintKeys = false

var debugKeyMap = map[KeyHash]string{}
var debugKeyMu = new(sync.RWMutex)

type KeyHash [KeyHashLength]byte

// String hex encodes the key. If debugging is enabled, String looks up the original composite key.
func (k KeyHash) String() string {
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
func (k KeyHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func (k KeyHash) Append(key ...interface{}) KeyHash {
	// If k is the zero value, don't stringify it
	var s string
	if debugKeys && k != (KeyHash{}) {
		s = k.String()
	}

	for _, key := range key {
		bytes, printv := convert(key)
		b := make([]byte, KeyHashLength+len(bytes))
		copy(b, k[:])
		copy(b[KeyHashLength:], bytes)
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
	bytes = keyBytes(key)

	switch key.(type) {
	case nil, []byte, [32]byte, *[32]byte, interface{ Bytes() []byte }:
		return bytes, false
	default:
		return bytes, true
	}
}

func keyBytes(v interface{}) []byte {
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
	case interface{ Bytes() []byte }:
		return v.Bytes()
	case interface{ AccountID() []byte }:
		return v.AccountID()
	case uint:
		return encodeUint(uint64(v))
	case uint8:
		return encodeUint(uint64(v))
	case uint16:
		return encodeUint(uint64(v))
	case uint32:
		return encodeUint(uint64(v))
	case uint64:
		return encodeUint(v)
	case int:
		return encodeInt(int64(v))
	case int8:
		return encodeInt(int64(v))
	case int16:
		return encodeInt(int64(v))
	case int32:
		return encodeInt(int64(v))
	case int64:
		return encodeInt(v)
	default:
		panic(fmt.Errorf("cannot use %T as a v", v))
	}
}

func encodeUint(v uint64) []byte {
	var buf [16]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

func encodeInt(v int64) []byte {
	var buf [16]byte
	n := binary.PutVarint(buf[:], v)
	return buf[:n]
}
