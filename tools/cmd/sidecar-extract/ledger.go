// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"
	"strings"

	badger "github.com/dgraph-io/badger"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	accerrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ledgers reports, for each archive, the last block its own system ledger
// records — where the database actually ends, which a validator's last
// signature does not say. Read-only, through the model's accessors and the
// verified value reads.
func ledgers(archiveArgs []string) error {
	partitions := []string{protocol.Directory, "Apollo", "Yutu", "Chandrayaan", "Cyclops"}
	for _, arg := range archiveArgs {
		name, path, ok := strings.Cut(arg, "=")
		if !ok {
			return fmt.Errorf("archive %q: want name=path", arg)
		}
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", name, err)
		}
		a := &archive{name: name, db: db, log: &vlog{dir: path}}

		batch := coredb.New(&verifiedStore{[]*archive{a}}, nil).Begin(false)
		for _, p := range partitions {
			var ledger *protocol.SystemLedger
			err := batch.Account(protocol.PartitionUrl(p).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
			switch {
			case errors.Is(err, accerrors.NotFound):
				continue
			case err != nil:
				fmt.Printf("%s\t%s\terror: %v\n", name, p, err)
			default:
				fmt.Printf("%s\t%s\tblock %d\t%v\n", name, p, ledger.Index, ledger.Timestamp.UTC())
			}
		}
		batch.Discard()
	}
	return nil
}

// verifiedStore reads one or more copies of a partition through the verified
// value path, newest first: a record comes from the first copy that holds a
// value it can verify. It cannot write.
type verifiedStore struct{ archives []*archive }

func (s *verifiedStore) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	txns := make([]*badger.Txn, len(s.archives))
	moves := make([]*badger.Iterator, len(s.archives))
	for i, a := range s.archives {
		txns[i] = a.db.NewTransaction(false)
		moves[i] = a.newMoves(txns[i])
	}
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get: func(key *record.Key) ([]byte, error) {
			h := key.Hash()
			var failed error
			for i, a := range s.archives {
				item, err := txns[i].Get(h[:])
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				if err == nil {
					var v []byte
					if v, err = a.resolve(item, moves[i]); err == nil {
						return v, nil
					}
				}
				if failed == nil {
					failed = err
				}
			}
			if failed != nil {
				return nil, failed
			}
			return nil, (*database.NotFoundError)(key)
		},
		Commit: func(map[[32]byte]memory.Entry) error { return errors.New("read-only") },
		Discard: func() {
			for i := range s.archives {
				moves[i].Close()
				txns[i].Discard()
			}
		},
	})
}
