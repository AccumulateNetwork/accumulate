// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"reflect"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"golang.org/x/exp/slog"
)

var storageProvides = provides[keyvalue.Beginner](func(c *StorageService) string { return c.Name })

func (c *StorageService) needs() []ServiceDescriptor {
	return nil
}

func (c *StorageService) provides() []ServiceDescriptor {
	return []ServiceDescriptor{
		storageProvides.with(c),
	}
}

func (s *StorageService) start(inst *Instance) error {
	store, err := s.Storage.open(inst)
	if err != nil {
		return err
	}
	return storageProvides.register(inst, s, store)
}

type Storage interface {
	Type() StorageType
	CopyAsInterface() any

	open(*Instance) (keyvalue.Beginner, error)
}

type StorageOrRef struct {
	reference *string
	storage   Storage
}

func (s *StorageOrRef) refOr(def string) string {
	if s != nil && s.reference != nil {
		return *s.reference
	}
	return def
}

func (*StorageOrRef) Type() reflect.Type {
	return reflect.TypeOf(new(keyvalue.Beginner)).Elem()
}

func (s *StorageOrRef) needs(def string) []ServiceDescriptor {
	if s != nil && s.storage != nil {
		return nil
	}
	return []ServiceDescriptor{
		&simpleDescriptor{
			name: s.refOr(def),
			typ:  s.Type(),
		},
	}
}

func (s *StorageOrRef) open(inst *Instance, def string) (keyvalue.Beginner, error) {
	if s != nil && s.storage != nil {
		return s.storage.open(inst)
	}
	return getService[keyvalue.Beginner](inst, &simpleDescriptor{
		name: s.refOr(def),
		typ:  s.Type(),
	})
}

func (s *StorageOrRef) MarshalJSON() ([]byte, error) {
	if s.storage != nil {
		return json.Marshal(s.storage)
	}
	return json.Marshal(s.reference)
}

func (s *StorageOrRef) UnmarshalJSON(b []byte) error {
	if json.Unmarshal(b, &s.reference) == nil {
		return nil
	}
	ss, err := UnmarshalStorageJSON(b)
	if err != nil {
		return err
	}
	s.storage = ss
	return nil
}

func (s *StorageOrRef) Copy() *StorageOrRef {
	if s.storage != nil {
		return &StorageOrRef{storage: s.storage.CopyAsInterface().(Storage)}
	}
	return s // Reference is immutable
}

func (s *StorageOrRef) Equal(t *StorageOrRef) bool {
	if s.reference != t.reference {
		return false
	}
	if s.storage == t.storage {
		return true
	}
	if s.storage == nil || t.storage == nil {
		return false
	}
	return EqualStorage(s.storage, t.storage)
}

func (s *MemoryStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	return memory.New(nil), nil
}

func (s *BadgerStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := badger.New(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup(func() {
		err := db.Close()
		if err != nil {
			slog.ErrorCtx(inst.context, "Error while closing Badger", "error", err)
		}
	})

	return db, nil
}
