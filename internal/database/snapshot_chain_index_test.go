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

// buildChainWithRepeat writes A, B, C, C onto the Directory anchor root chain of
// a BVN's anchor pool with the same AddEntry and the same unique=false the
// executor uses, and returns the database plus the hashes it wrote.
func buildChainWithRepeat(t *testing.T) (*database.Database, [][]byte) {
	t.Helper()
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	pool := new(protocol.AnchorLedger)
	pool.Url = anchorPool
	require.NoError(t, batch.Account(anchorPool).Main().Put(pool))

	c, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)

	var hashes [][]byte
	for _, name := range []string{"A", "B", "C", "C"} {
		h := sha256.Sum256([]byte(fmt.Sprintf("root %s", name)))
		hashes = append(hashes, h[:])
		// unique=false is what block_begin.go, synthetic.go and block_end.go
		// pass, so the duplicate really is offered.
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db, hashes
}

func collectSnapshot(t *testing.T, db *database.Database) []byte {
	t.Helper()
	buf := new(ioutil.Buffer)
	batch := db.Begin(false)
	defer batch.Discard()
	_, err := batch.Collect(buf, nil, nil)
	require.NoError(t, err)
	return buf.Bytes()
}

func anchorRootIndexOf(t *testing.T, db *database.Database, hash []byte) (int64, error) {
	t.Helper()
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	batch := db.Begin(false)
	defer batch.Discard()
	return batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().IndexOf(hash)
}

func anchorRootHeight(t *testing.T, db *database.Database) int64 {
	t.Helper()
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	batch := db.Begin(false)
	defer batch.Discard()
	c, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	return c.Height()
}

// TestRestore_RepeatedHashIndexesFirstOccurrence is the property that separates
// the two ways of rebuilding the index. A hash at heights 2 and 3 must index to
// 2 - the position main's AddEntry gave it - and not to 3. An unconditional Put
// over 0..Count-1 writes 3, which is plausible, wrong, and enough to break
// Receipt (see TestRestore_ReceiptForRepeatedEntry in test/e2e).
//
// DI's TestSnapshot_ChainIndexOfSurvivesRestore cannot see this: its five
// hashes are all distinct.
func TestRestore_RepeatedHashIndexesFirstOccurrence(t *testing.T) {
	built, hashes := buildChainWithRepeat(t)
	C := hashes[2]
	require.Equal(t, C, hashes[3], "C must be the repeated hash")
	require.Equal(t, int64(4), anchorRootHeight(t, built), "the duplicate must have been appended")

	// What the node that built the chain has
	want, err := anchorRootIndexOf(t, built, C)
	require.NoError(t, err, "the building node indexes the repeated hash")
	require.Equal(t, int64(2), want, "main's AddEntry records the FIRST occurrence")

	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap), nil))
	require.Equal(t, int64(4), anchorRootHeight(t, restored), "the chain must survive restore")

	got, err := anchorRootIndexOf(t, restored, C)
	require.NoError(t, err, "the restored node must index the repeated hash")
	require.Equal(t, int64(2), got, "the restored node must name the FIRST occurrence, not the last")
	require.Equal(t, want, got, "the restored node must agree with the node that built the chain")

	// The distinct hashes must land where they did too
	for i, h := range hashes[:3] {
		want, err := anchorRootIndexOf(t, built, h)
		require.NoError(t, err)
		got, err := anchorRootIndexOf(t, restored, h)
		require.NoError(t, err, "entry %d must be indexed", i)
		require.Equal(t, want, got, "entry %d", i)
	}
}

// TestRestore_ChainIndexIsRebuiltAcrossBatchBoundaries drives the rebuild with a
// record limit far smaller than the number of entries, so the writer commits and
// replaces its batch several times mid-chain. Every entry must still be indexed,
// and the repeated hash must still name its first occurrence - the existence
// check has to see what earlier batches committed.
func TestRestore_ChainIndexIsRebuiltAcrossBatchBoundaries(t *testing.T) {
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	const n = 500
	const repeatAt = 7 // the hash at height 7 is written again at the end

	built := database.OpenInMemory(nil)
	batch := built.Begin(true)
	pool := new(protocol.AnchorLedger)
	pool.Url = anchorPool
	require.NoError(t, batch.Account(anchorPool).Main().Put(pool))
	c, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("root %d", i)))
		hashes = append(hashes, h[:])
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, c.AddEntry(hashes[repeatAt], false))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch.Discard()

	require.Equal(t, int64(n+1), anchorRootHeight(t, built))
	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap),
		// Small enough that the rebuild rotates its batch many times inside
		// this one chain.
		&database.RestoreOptions{BatchRecordLimit: 37}))

	require.Equal(t, int64(n+1), anchorRootHeight(t, restored))
	for i, h := range hashes {
		got, err := anchorRootIndexOf(t, restored, h)
		require.NoErrorf(t, err, "entry %d must be indexed", i)
		want, err := anchorRootIndexOf(t, built, h)
		require.NoError(t, err)
		require.Equalf(t, want, got, "entry %d", i)
	}
	got, err := anchorRootIndexOf(t, restored, hashes[repeatAt])
	require.NoError(t, err)
	require.Equal(t, int64(repeatAt), got,
		"the repeated hash must keep its first position across a batch rotation")
}

// buildManyAccountsWithFarRepeat builds several accounts, each with a main chain
// whose last entry repeats an entry from far earlier in the same chain. The gap
// is much wider than most of the batch limits used below, so the existence check
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

// TestRestore_ChunkedRebuildOnRealStore is the reviewer's test, taken from the
// review branch.
//
// It replaces an earlier on-disk test of mine that did not test what it claimed:
// that one used four entries and a limit of two, so its repeated hash was always
// in the SAME batch as its first occurrence and it never asked whether the
// existence check sees a COMMITTED write. This one spans the repeat across many
// batch boundaries, on memory, badger and leveldb, at limits either side of the
// mark frequency and of the chain length.
func TestRestore_ChunkedRebuildOnRealStore(t *testing.T) {
	const nAccounts, n, repeatAt = 4, 300, 5
	built, urls, all := buildManyAccountsWithFarRepeat(t, nAccounts, n, repeatAt)
	snap := collectSnapshot(t, built)

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
