package record

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// A Record is a component of a data model.
type Record interface {
	// Resolve resolves the record or a child record.
	Resolve(key KeyPart) (Record, KeyPart, error)
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
type Key interface {
	KeyPart
	Append(...any) Key
	Hash() storage.Key
}

// A KeyPart is part of of a Key.
type KeyPart interface {
	Len() int
	GetAll() []any
	Get(int) any
	Rest(int) KeyPart
}

type keyPart []any

func (k keyPart) Len() int      { return len(k) }
func (k keyPart) GetAll() []any { return k }

func (k keyPart) Get(i int) any {
	if i < 0 || i >= len(k) {
		return nil
	}
	return k[i]
}

func (k keyPart) Rest(i int) KeyPart {
	if i < 0 || i > len(k) {
		return nil
	}
	return k[i:]
}

type key struct {
	keyPart

	memoize struct {
		hash   storage.Key
		append map[any]Key
	}
}

func NewKey(v ...any) Key {
	madeNewKey++
	return &key{keyPart: v}
}

var madeNewKey, gotMemoKey int
var calcHash, gotMemoHash int
var calcString int

func PrintStats() {
	fmt.Printf("Key: %d new, %d memo\n", madeNewKey, gotMemoKey)
	fmt.Printf("Hash: %d new, %d memo\n", calcHash, gotMemoHash)
	fmt.Printf("String: %d\n", calcString)
}

// Append creates a child key of this key.
func (k *key) Append(v ...any) Key {
	if len(v) == 0 {
		return k
	}

	// Make the memoization memoKey hashable
	memoKey := mapKeys(v)

	if k.memoize.append == nil {
		k.memoize.append = map[any]Key{}
	}
	if k, ok := k.memoize.append[memoKey]; ok {
		gotMemoKey++
		return k
	}

	l := make(keyPart, len(k.keyPart)+len(v))
	n := copy(l, k.keyPart)
	copy(l[n:], v)
	ll := NewKey(l...)
	k.memoize.append[memoKey] = ll
	return ll
}

func mapKeys(v []any) any {
	switch len(v) {
	case 0:
		return nil
	case 1:
		return [1]any{mapKey(v[0])}
	case 2:
		return [2]any{mapKey(v[0]), mapKey(v[1])}
	case 3:
		return [3]any{mapKey(v[0]), mapKey(v[1]), mapKey(v[2])}
	case 4:
		return [4]any{mapKey(v[0]), mapKey(v[1]), mapKey(v[2]), mapKey(v[3])}
	case 5:
		return [5]any{mapKey(v[0]), mapKey(v[1]), mapKey(v[2]), mapKey(v[3]), mapKey(v[4])}
	default:
		panic("not implemented for keys longer than 5 elements")
	}
}

func mapKey(v any) any {
	switch v := v.(type) {
	case []byte:
		return MapKeyBytes(v)

	case *url.URL:
		return MapKeyUrl(v)

	default:
		// Most values we use as keys are safe for use in maps
		return v
	}
}

// Hash converts the record key to a storage key.
func (k *key) Hash() storage.Key {
	if k.memoize.hash == (storage.Key{}) {
		calcHash++
		k.memoize.hash = storage.MakeKey(k.keyPart...)
	} else {
		gotMemoHash++
	}
	return k.memoize.hash
}

// String returns a human-readable string for the key.
func (k *key) String() string {
	calcString++
	s := make([]string, len(k.keyPart))
	for i, v := range k.keyPart {
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
func (k *key) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// A ValueReader holds a readable value.
type ValueReader interface {
	// GetValue returns the value.
	GetValue() (encoding.BinaryValue, error)
}

// A ValueWriter holds a writable value.
type ValueWriter interface {
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
