// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// DefaultStorageType is the storage backend used when a node's configuration
// does not specify one. Init flows record the storage type explicitly in the
// generated configuration, so this default only applies to hand-written
// configurations — and a mismatch with an existing database is caught by
// checkStorageDir.
const DefaultStorageType = StorageTypeLevelDB

var storageProvides = ioc.Provides[keyvalue.Beginner](func(c *StorageService) string { return c.Name })

// detectStorageDir inspects an existing database directory and reports which
// backend created it. Badger creates KEYREGISTRY (v2+) and *.vlog files;
// LevelDB creates CURRENT; BlockchainDB creates its permanent layer's segment
// manifest. None creates another's markers.
func detectStorageDir(path string) (StorageType, bool) {
	if _, err := os.Stat(filepath.Join(path, "perm", "segments.json")); err == nil {
		return StorageTypeBlockchainDB, true
	}
	if _, err := os.Stat(filepath.Join(path, "KEYREGISTRY")); err == nil {
		return StorageTypeBadger, true
	}
	if m, _ := filepath.Glob(filepath.Join(path, "*.vlog")); len(m) > 0 {
		return StorageTypeBadger, true
	}
	if _, err := os.Stat(filepath.Join(path, "CURRENT")); err == nil {
		return StorageTypeLevelDB, true
	}
	return 0, false
}

// checkStorageDir verifies that the database directory, if it exists, was
// created by the configured backend. Opening a directory with the wrong
// backend would at best fail cryptically and at worst come up as an empty
// database, so refuse loudly instead.
func checkStorageDir(path string, want StorageType) error {
	got, ok := detectStorageDir(path)
	if !ok || got == want {
		return nil
	}
	return errors.FatalError.WithFormat(
		"the database at %s was created by %v but the node is configured to use %v — set storage-type to %v (or remove the database to reinitialize)",
		path, got, want, got)
}

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

func (s *BadgerStorage) setPath(path string)       { s.Path = path }
func (s *BoltStorage) setPath(path string)         { s.Path = path }
func (s *BlockchainDBStorage) setPath(path string) { s.Path = path }
func (s *ExpBlockDBStorage) setPath(path string)   { s.Path = path }
func (s *LevelDBStorage) setPath(path string)      { s.Path = path }
func (s *MemoryStorage) setPath(path string)       {}

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
	if err := checkStorageDir(inst.path(s.Path), StorageTypeBadger); err != nil {
		return nil, err
	}

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

	inst.cleanup("storage", func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *BoltStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := bolt.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup("storage", func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *LevelDBStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	if err := checkStorageDir(inst.path(s.Path), StorageTypeLevelDB); err != nil {
		return nil, err
	}

	db, err := leveldb.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup("storage", func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *BlockchainDBStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := bcdb.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup("storage", func(context.Context) error { return db.Close() })
	return db, nil
}

func (s *ExpBlockDBStorage) open(inst *Instance) (keyvalue.Beginner, error) {
	db, err := block.Open(inst.path(s.Path))
	if err != nil {
		return nil, err
	}

	inst.cleanup("storage", func(context.Context) error { return db.Close() })
	return db, nil
}
