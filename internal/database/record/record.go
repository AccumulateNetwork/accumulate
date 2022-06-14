package record

import (
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

type ValueReadWriter interface {
	GetValue() (encoding.BinaryValue, error)
	ReadFrom(value ValueReadWriter) error
	ReadFromBytes(data []byte) error
}

type Store interface {
	LoadValue(key Key, value ValueReadWriter) error
	StoreValue(key Key, value ValueReadWriter) error
}
