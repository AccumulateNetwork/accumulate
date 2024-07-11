// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
)

var storageProvides = ioc.Provides[keyvalue.Beginner](func(c *StorageService) string { return c.Name })

func (c *StorageService) Requires() []ioc.Requirement {
	return nil
}

func (c *StorageService) Provides() []ioc.Provided {
	return []ioc.Provided{
		storageProvides.Provided(c),
	}
}

func (s *StorageService) start(inst *Instance) error {
	store, err := s.Storage.open(inst)
	if err != nil {
		return err
	}
	return storageProvides.Register(inst.services, s, store)
}

func (s *BadgerStorage) setPath(path string)     { s.Path = path }
func (s *BoltStorage) setPath(path string)       { s.Path = path }
func (s *ExpBlockDBStorage) setPath(path string) { s.Path = path }
func (s *LevelDBStorage) setPath(path string)    { s.Path = path }
func (s *MemoryStorage) setPath(path string)     {}

type StorageOrRef baseRef[Storage]

func (s *StorageOrRef) base() *baseRef[Storage] {
	return (*baseRef[Storage])(s)
}

func (s *StorageOrRef) Required(def string) []ioc.Requirement {
	if s.base().hasValue() {
		return nil
	}
	return []ioc.Requirement{
		{Descriptor: ioc.NewDescriptorOf[keyvalue.Beginner](s.base().refOr(def))},
	}
}

func (s *StorageOrRef) open(inst *Instance, def string) (keyvalue.Beginner, error) {
	if s != nil && s.value != nil {
		return s.value.open(inst)
	}
	return ioc.Get[keyvalue.Beginner](inst.services, s.base().refOr(def))
}

func (s *StorageOrRef) Copy() *StorageOrRef {
	return (*StorageOrRef)(s.base().copyWith(CopyStorage))
}

func (s *StorageOrRef) Equal(t *StorageOrRef) bool {
	return s.base().equalWith(t.base(), EqualStorage)
}

func (s *StorageOrRef) MarshalJSON() ([]byte, error) {
	return s.base().marshal()
}

func (s *StorageOrRef) UnmarshalJSON(b []byte) error {
	return s.base().unmarshalWith(b, UnmarshalStorageJSON)
}

func (s *MemoryStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	return memory.New(nil), nil
}

func (s *BadgerStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	var db interface {
		keyvalue.Beginner
		io.Closer
	}
	var err error
	switch s.Version {
	case 0, 1:
		db, err = badger.OpenV1(inst.path(s.Path))
	case 2:
		db, err = badger.OpenV2(inst.path(s.Path))
	case 3:
		db, err = badger.OpenV3(inst.path(s.Path))
	case 4:
		db, err = badger.OpenV4(inst.path(s.Path))
	}
	if err != nil {
		return nil, err
	}

	inst.cleanup(func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *BoltStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := bolt.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup(func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *LevelDBStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := leveldb.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup(func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *ExpBlockDBStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := block.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup(func(context.Context) error { return db.Close() })
	return db, nil
}
