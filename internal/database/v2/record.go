package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

//go:generate go run ../../../tools/cmd/gen-record --package database records.yml
//-go:generate go run ../../../tools/cmd/gen-record --language yaml --out types_gen.yml records.yml -x Chain,ChangeSet,AccountData
//go:generate go run ../../../tools/cmd/gen-types --package database types.yml

type record interface {
	resolve(key recordKey) (record, recordKey, error)
	isDirty() bool
	commit() error
}

type recordKey []interface{}

func (k recordKey) Append(v ...interface{}) recordKey {
	l := make(recordKey, len(k)+len(v))
	n := copy(l, k)
	copy(l[n:], v)
	return l
}

func (k recordKey) Hash() storage.Key {
	return storage.MakeKey(k...)
}

type recordValue interface {
	get(value encoding.BinaryValue) error
	put(value encoding.BinaryValue) error
}

type recordStore interface {
	get(key recordKey, value encoding.BinaryValue) error
	put(key recordKey, value encoding.BinaryValue) error
}
