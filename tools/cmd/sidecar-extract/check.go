// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// check reports, for each account, how much of its main chain can be read —
// entries and the messages they name — from the live database alone and with
// the sidecar behind it. Both are opened read-only.
func check(currentPaths []string, sidecarPath string, accounts []string) error {
	var live []*leveldb.DB
	for _, p := range currentPaths {
		db, err := leveldb.OpenFile(p, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return fmt.Errorf("current %s: %w", p, err)
		}
		defer db.Close()
		live = append(live, db)
	}
	side, err := leveldb.OpenFile(sidecarPath, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return fmt.Errorf("sidecar: %w", err)
	}
	defer side.Close()

	for _, s := range accounts {
		u, err := url.Parse(s)
		if err != nil {
			return err
		}
		for _, layers := range []struct {
			name string
			dbs  []*leveldb.DB
		}{{"live", live}, {"live+sidecar", append(live[:len(live):len(live)], side)}} {
			batch := coredb.New(layered(layers.dbs), nil).Begin(false)
			chain := batch.Account(u).MainChain()
			head, err := chain.Head().Get()
			if err != nil {
				fmt.Printf("%s\t%s\thead: %v\n", u, layers.name, err)
				batch.Discard()
				continue
			}
			var entries, messages int64
			var firstErr error
			for i := int64(0); i < head.Count; i++ {
				h, err := chain.Entry(i)
				if err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("entry %d: %w", i, err)
					}
					continue
				}
				entries++
				if _, err := batch.Message([32]byte(h)).Main().Get(); err == nil {
					messages++
				} else if firstErr == nil {
					firstErr = fmt.Errorf("message of entry %d: %w", i, err)
				}
			}
			fmt.Printf("%s\t%s\tcount %d\tentries %d\tmessages %d", u, layers.name, head.Count, entries, messages)
			if firstErr != nil {
				fmt.Printf("\tfirst failure: %v", firstErr)
			}
			fmt.Println()
			batch.Discard()
		}
	}
	return nil
}

// layered reads each key from the first database that holds it. It cannot
// write.
type layered []*leveldb.DB

func (l layered) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get: func(key *record.Key) ([]byte, error) {
			h := key.Hash()
			for _, db := range l {
				v, err := db.Get(h[:], nil)
				if err == nil {
					return v, nil
				}
				if !errors.Is(err, leveldb.ErrNotFound) {
					return nil, err
				}
			}
			return nil, (*database.NotFoundError)(key)
		},
		Commit:  func(map[[32]byte]memory.Entry) error { return errors.New("read-only") },
		Discard: func() {},
	})
}
