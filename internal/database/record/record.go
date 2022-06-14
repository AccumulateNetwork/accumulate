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

type RawValue interface {
	GetRaw(value encoding.BinaryValue) error
	PutRaw(value encoding.BinaryValue) error
}

type Store interface {
	GetRaw(key Key, value encoding.BinaryValue) error
	PutRaw(key Key, value encoding.BinaryValue) error
}
