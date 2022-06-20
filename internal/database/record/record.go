package record

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// A Record is a component of a data model.
type Record interface {
	// Resolve resolves the record or a child record.
	Resolve(key Key) (Record, Key, error)
	// IsDirty returns true if the record has been modified.
	IsDirty() bool
	// Commit writes any modifications to the store.
	Commit() error
}

// RecordPtr is satisfied by a type T where *T implements Record.
type RecordPtr[T any] interface {
	*T
	Record
}

// A Key is the key for a record.
type Key []interface{}

// Append creates a child key of this key.
func (k Key) Append(v ...interface{}) Key {
	l := make(Key, len(k)+len(v))
	n := copy(l, k)
	copy(l[n:], v)
	return l
}

// Hash converts the record key to a storage key.
func (k Key) Hash() storage.Key {
	return storage.MakeKey(k...)
}

// String returns a human-readable string for the key.
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

// MarshalJSON is implemented so keys are formatted nicely by zerolog.
func (k Key) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// A ValueReader holds a readable value.
type ValueReader interface {
	// GetValue returns the value.
	GetValue() (encoding.BinaryValue, error)
}

// A ValueWriter holds a writable value.
type ValueWriter interface {
	ValueReader // TODO AC-1761 remove

	// LoadValue stores the value of the reader into the receiver.
	LoadValue(value ValueReader, put bool) error
	// LoadBytes unmarshals a value from bytes into the receiver.
	LoadBytes(data []byte) error
}

// A Store loads and stores values.
type Store interface {
	// GetValue loads the value from the underlying store and writes it. Byte
	// stores call LoadBytes(data) and value stores call LoadValue(v, false).
	GetValue(key Key, value ValueWriter) error
	// PutValue gets the value from the reader and stores it. A byte store
	// marshals the value and stores the bytes. A value store finds the
	// appropriate value and calls LoadValue(v, true).
	PutValue(key Key, value ValueReader) error
}
