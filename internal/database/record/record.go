package record

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type Record interface {
	Resolve(key Key) (Record, Key, error)
	IsDirty() bool
	Commit() error
}

type RecordPtr[T any] interface {
	*T
	Record
}

type Key []interface{}

func (k Key) Append(v ...interface{}) Key {
	l := make(Key, len(k)+len(v))
	n := copy(l, k)
	copy(l[n:], v)
	return l
}

func (k Key) Hash() storage.Key {
	return storage.MakeKey(k...)
}

func (k Key) String() string {
	s := make([]string, len(k))
	for i, v := range k {
		switch v := v.(type) {
		case []byte:
			s[i] = hex.EncodeToString(v)
		case [32]byte:
			s[i] = hex.EncodeToString(v[:])
		default:
			s[i] = fmt.Sprint(v)
		}
	}
	return strings.Join(s, ".")
}

func (k Key) MarshalJSON() ([]byte, error) {
	// This is implemented purely for logging
	return json.Marshal(k.String())
}

type ValueReadWriter interface {
	GetValue() (encoding.BinaryValue, error)
	GetFrom(value ValueReadWriter) error
	PutFrom(value ValueReadWriter) error
	LoadFrom(data []byte) error
}

type Store interface {
	LoadValue(key Key, value ValueReadWriter) error
	StoreValue(key Key, value ValueReadWriter) error
}
