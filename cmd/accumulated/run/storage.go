// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
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
		storageProvides.describe(c),
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
