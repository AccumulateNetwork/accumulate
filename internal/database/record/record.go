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

type ValueReader interface {
	GetValue() (encoding.BinaryValue, error)
}

type ValueWriter interface {
	// LoadValue stores the value of the reader into the receiver.
	LoadValue(value ValueReader, put bool) error
	// LoadBytes unmarshals a value from bytes into the receiver.
	LoadBytes(data []byte) error
}

type Store interface {
	// GetValue loads the value from the underlying store and writes it. Byte
	// stores call LoadBytes(data) and value stores call LoadValue(v, false).
	GetValue(key Key, value ValueWriter) error
	// PutValue gets the value from the reader and stores it. A byte store
	// marshals the value and stores the bytes. A value store finds the
	// appropriate value and calls LoadValue(v, true).
	PutValue(key Key, value ValueReader) error
}
