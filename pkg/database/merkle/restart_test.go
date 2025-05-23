// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func TestRestart(t *testing.T) {
	store := begin()

	MarkPower := int64(2)

	for i := 0; i < 20; i++ { //                                      Run 100 tests
		MM1 := testChain(store, MarkPower) //                  Create a MerkleManager

		for j := 0; j < 100; j++ { //                                   Add 100 hashes
			if j == i {
				MM2 := testChain(store, MarkPower) // Then get the highest state stored
				h1, err := MM1.Head().Get()        //
				require.NoError(t, err)            //
				h2, err := MM2.Head().Get()        //
				require.NoError(t, err)            //
				if !h1.Equal(h2) {                 // MM2 should be the same as MM1
					t.Fatalf("could not restore MM1 in MM2.  index: %d", j) // are going to be messed up.
				}
			}
			for rand.Int()%3 > 0 { //                                       Add 0 or more random hashes
				require.NoError(t, MM1.AddEntry(doSha(common.Int64Bytes(rand.Int63())), false)) // Generate and add one random hash
			}
			require.NoError(t, MM1.Commit())
		}
	}
}

func TestRestartCache(t *testing.T) {
	rand.Seed(12344) //nolint:staticcheck // It's a test, who cares
	var rh common.RandHash
	store := new(memdb)
	txn := store.Begin(nil, true)

	MarkPower := int64(2)

	for i := uint(0); i < 50; i += uint(rand.Int()) % 10 { //
		MM1 := testChain(keyvalue.RecordStore{Store: txn}, MarkPower) //                  Create a MerkleManager

		var cached [][]byte // Using this slice to track the hashes that have been written to MM1
		//                       but not yet written to disk by MM1.  Calling EndBatch on MM1 will need to
		//                       clear this cache.
	TestLoop:
		for j := uint(0); j < 50; j++ { //             Add 100 hashes
			for k := uint(0); k < uint(rand.Int())%4; k++ { //  Add 0 or more random hashes
				h := rh.Next()
				cached = append(cached, h)
				require.NoError(t, MM1.AddEntry(h, false)) // Generate and add one random hash
			}

			ended := rand.Int()%30 > 0 && len(cached) > 0 // Every so often we are going to write to disk
			if ended {                                    //   so what is cached is going away too.
				require.NoError(t, txn.Commit())
				txn = store.Begin(nil, true)
				cached = cached[:0] //  Clear the cache
				MM1 = testChain(keyvalue.RecordStore{Store: txn}, MarkPower)
			}

			if j == i {
				ndb := store.Copy()
				nbatch := ndb.Begin(nil, true)                                   // This simulates opening a new database later (MM2)
				MM2 := testChain(keyvalue.RecordStore{Store: nbatch}, MarkPower) // Then get the highest state stored
				h1, err := MM1.Head().Get()                                      //
				require.NoError(t, err)                                          //
				h2, err := MM2.Head().Get()                                      //
				require.NoError(t, err)                                          //

				if h1.Equal(h2) && len(cached) > 0 {
					t.Error("MM2 should not be the same as MM1 if some hashes are in the cache still")
				}

				for _, h := range cached {
					require.NoError(t, MM2.AddEntry(h, false))
				}

				if !h1.Equal(h2) { // MM2 should be the same as MM1
					h1.Equal(h2)
					t.Errorf("MM2 isn't tracking MM1 at index: %d %d", i, j) // are going to be messed up.
				}
				break TestLoop
			}
		}

	}
}

type memdb map[[32]byte]memory.Entry

func (d *memdb) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	var commit memory.CommitFunc
	if writable {
		commit = func(m map[[32]byte]memory.Entry) error {
			if *d == nil {
				*d = m
				return nil
			}
			for k, e := range m {
				(*d)[k] = e
			}
			return nil
		}
	}
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Commit: commit,
		Get: func(k *record.Key) ([]byte, error) {
			e, ok := (*d)[k.Hash()]
			if ok {
				return e.Value, nil
			}
			return nil, errors.NotFound
		},
	})
}

func (d memdb) Copy() memdb {
	e := make(memdb, len(d))
	for k, v := range d {
		e[k] = v
	}
	return e
}
