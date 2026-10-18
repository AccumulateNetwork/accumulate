// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sidecar

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	accerr "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Era is the databases as they were before the reorg (#4567). The extractor
// keeps a key the live database holds as the live database's, so a chain the
// reorg restarted under its old keys -- the Directory's root chain, its
// anchor chains, the routing table -- and a data index it restarted are in
// no store it builds except the overwritten stores: for every such key, each
// archive's value when it differs, under the storage key followed by the
// archive's index (sidecar-extract -keep-overwritten, -keep-overwritten-data).
//
// A read answers from the overwritten stores in order, each from the first
// archive in index order that holds the key, else from the sidecar. Nothing
// is written.
type Era struct {
	stores []eraStore
	side   keyvalue.Beginner
}

type eraStore struct {
	db *leveldb.DB
	n  int
}

// OpenEra opens the overwritten stores in dirs (overwritten.db beside its
// overwritten.json, each) read-only, over the sidecar.
func OpenEra(dirs []string, side keyvalue.Beginner) (*Era, error) {
	e := &Era{side: side}
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(dir, "overwritten.json"))
		if err != nil {
			_ = e.Close()
			return nil, err
		}
		var meta struct {
			Archives []string `json:"archives"`
		}
		if err := json.Unmarshal(b, &meta); err != nil {
			_ = e.Close()
			return nil, err
		}
		if len(meta.Archives) == 0 || len(meta.Archives) > 255 {
			_ = e.Close()
			return nil, accerr.BadRequest.WithFormat("%s: overwritten.json names %d archives", dir, len(meta.Archives))
		}
		db, err := leveldb.OpenFile(filepath.Join(dir, "overwritten.db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			_ = e.Close()
			return nil, err
		}
		e.stores = append(e.stores, eraStore{db, len(meta.Archives)})
	}
	return e, nil
}

func (e *Era) Close() error {
	var err error
	for _, s := range e.stores {
		err = errors.Join(err, s.db.Close())
	}
	return err
}

// Begin implements [keyvalue.Beginner]. The change set only reads.
func (e *Era) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get:    e.get,
		ForEach: func(func(*record.Key, []byte) error) error {
			return accerr.NotAllowed.With("the era cannot be walked")
		},
		Commit: func(map[[32]byte]memory.Entry) error {
			return accerr.NotAllowed.With("the era is read-only")
		},
	})
}

func (e *Era) get(key *record.Key) ([]byte, error) {
	h := key.Hash()
	k := append(h[:], 0)
	for _, s := range e.stores {
		for i := 0; i < s.n; i++ {
			k[32] = byte(i)
			v, err := s.db.Get(k, nil)
			switch {
			case err == nil:
				return v, nil
			case !errors.Is(err, leveldb.ErrNotFound):
				return nil, err
			}
		}
	}
	if e.side == nil {
		return nil, (*database.NotFoundError)(key)
	}
	b := e.side.Begin(nil, false)
	defer b.Discard()
	return b.Get(key)
}
