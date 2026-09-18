// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// buildManyAccountsWithFarRepeat builds several accounts, each with a main
// chain whose last entry repeats an entry from far earlier in the same chain.
// The gap is much wider than the batch limit used below, so the existence check
// for the repeat has to see a write that a PREVIOUS, already-committed batch
// made - not one sitting in the live batch's map.
func buildManyAccountsWithFarRepeat(t *testing.T, nAccounts, n, repeatAt int) (*database.Database, []*url.URL, [][][]byte) {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	var urls []*url.URL
	var all [][][]byte
	for a := 0; a < nAccounts; a++ {
		u := protocol.AccountUrl(fmt.Sprintf("acct%d.acme", a), "tokens")
		urls = append(urls, u)

		acct := new(protocol.TokenAccount)
		acct.Url = u
		acct.TokenUrl = protocol.AcmeUrl()
		acct.Balance = *big.NewInt(1000)
		require.NoError(t, batch.Account(u).Main().Put(acct))

		c, err := batch.Account(u).MainChain().Get()
		require.NoError(t, err)

		var hashes [][]byte
		for i := 0; i < n; i++ {
			h := sha256.Sum256([]byte(fmt.Sprintf("acct %d entry %d", a, i)))
			hashes = append(hashes, h[:])
			require.NoError(t, c.AddEntry(h[:], false))
		}
		require.NoError(t, c.AddEntry(hashes[repeatAt], false))
		hashes = append(hashes, hashes[repeatAt])
		all = append(all, hashes)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db, urls, all
}

// TestReview_ChunkedRebuildOnRealStore is the on-disk version of the branch's
// TestRestore_ChainIndexIsRebuiltAcrossBatchBoundaries. The branch's on-disk
// test uses four entries and a limit of two, so its repeated hash is always in
// the same batch as its first occurrence; it never asks whether the existence
// check sees a committed write. This one does, on badger and leveldb.
func TestReview_ChunkedRebuildOnRealStore(t *testing.T) {
	const nAccounts, n, repeatAt = 4, 300, 5
	built, urls, all := buildManyAccountsWithFarRepeat(t, nAccounts, n, repeatAt)
	snap := reviewCollect(t, built)

	for _, store := range []struct {
		name string
		open func(string) (*database.Database, error)
	}{
		{"memory", func(string) (*database.Database, error) { return database.OpenInMemory(nil), nil }},
		{"badger", func(p string) (*database.Database, error) { return database.OpenBadger(p, nil) }},
		{"leveldb", func(p string) (*database.Database, error) { return database.OpenLevelDB(p, nil) }},
	} {
		for _, limit := range []int{1, 2, 7, 13, 256, 257, 301} {
			t.Run(fmt.Sprintf("%s/limit=%d", store.name, limit), func(t *testing.T) {
				db, err := store.open(filepath.Join(t.TempDir(), "restored.db"))
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })

				require.NoError(t, database.Restore(db, ioutil.NewBuffer(snap),
					&database.RestoreOptions{BatchRecordLimit: limit}))

				batch := db.Begin(false)
				defer batch.Discard()
				for a, u := range urls {
					c, err := batch.Account(u).MainChain().Get()
					require.NoError(t, err)
					require.Equal(t, int64(n+1), c.Height(), "account %d height", a)

					seen := map[string]int64{}
					for i, h := range all[a] {
						got, err := c.HeightOf(h)
						require.NoErrorf(t, err, "account %d entry %d must be indexed", a, i)
						if first, ok := seen[string(h)]; ok {
							require.Equalf(t, first, got,
								"account %d entry %d must keep its first position", a, i)
						} else {
							seen[string(h)] = int64(i)
							require.Equalf(t, int64(i), got, "account %d entry %d", a, i)
						}
					}
				}
			})
		}
	}
}

func reviewCollect(t *testing.T, db *database.Database) []byte {
	t.Helper()
	buf := new(ioutil.Buffer)
	batch := db.Begin(false)
	defer batch.Discard()
	_, err := batch.Collect(buf, nil, nil)
	require.NoError(t, err)
	return buf.Bytes()
}
