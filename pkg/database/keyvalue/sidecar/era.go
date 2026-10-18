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
// anchor chains, the routing table -- is in no store it builds except the
// overwritten store: for every such key, each archive's value when it
// differs, under the storage key followed by the archive's index
// (sidecar-extract -keep-overwritten).
//
// A read answers from the overwritten store, the first archive in index
// order that holds the key, else from the sidecar. Nothing is written.
type Era struct {
	db   *leveldb.DB
	n    int
	side keyvalue.Beginner
}

// OpenEra opens the overwritten store in dir (overwritten.db beside its
// overwritten.json) read-only, over the sidecar.
func OpenEra(dir string, side keyvalue.Beginner) (*Era, error) {
	b, err := os.ReadFile(filepath.Join(dir, "overwritten.json"))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Archives []string `json:"archives"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	if len(meta.Archives) == 0 || len(meta.Archives) > 255 {
		return nil, accerr.BadRequest.WithFormat("overwritten.json names %d archives", len(meta.Archives))
	}
	db, err := leveldb.OpenFile(filepath.Join(dir, "overwritten.db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return nil, err
	}
	return &Era{db: db, n: len(meta.Archives), side: side}, nil
}

func (e *Era) Close() error { return e.db.Close() }

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
	for i := 0; i < e.n; i++ {
		k[32] = byte(i)
		v, err := e.db.Get(k, nil)
		switch {
		case err == nil:
			return v, nil
		case !errors.Is(err, leveldb.ErrNotFound):
			return nil, err
		}
	}
	if e.side == nil {
		return nil, (*database.NotFoundError)(key)
	}
	b := e.side.Begin(nil, false)
	defer b.Discard()
	return b.Get(key)
}
