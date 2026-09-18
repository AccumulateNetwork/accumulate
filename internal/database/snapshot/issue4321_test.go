// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Evidence for issue #4321 on main (f9635bf0b).
//
// RestoreElementIndexFromMarkPoints (merkle_snapshot.go:139) writes
// state.Count+i where the hash actually sits at state.Count-markFreq+i, so
// every element index rebuilt from a mark point names a position one mark set
// (256 entries) too high. These tests measure that, and then ask whether a
// database carrying those wrong indices can skip an append and diverge.

// sysAccount is a system account: its root identity parses as a partition URL,
// which is the gate on the element-index rebuild in restore.go:137.
func sysAccount() *url.URL { return protocol.DnUrl().JoinPath("issue4321") }

// buildV1Snapshot builds a chain of n entries on a system account, collects a
// version 1 snapshot with full history, and returns the snapshot bytes and the
// hashes in the order they were added.
func buildV1Snapshot(t *testing.T, u *url.URL, n int) (*ioutil2.Buffer, [][]byte) {
	t.Helper()

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	require.NoError(t, batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u}))
	chain, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)

	var rh common.RandHash
	for i := 0; i < n; i++ {
		require.NoError(t, chain.AddEntry(rh.NextList(), false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	buf := new(ioutil2.Buffer)
	w, err := snapshot.Create(buf, new(snapshot.Header))
	require.NoError(t, err)

	batch = db.Begin(true)
	defer batch.Discard()
	require.NoError(t, w.CollectAccounts(batch, snapshot.CollectOptions{}))
	batch.Discard()
	require.NoError(t, db.Close())

	return buf, rh.List
}

// restoreV1 restores a version 1 snapshot into a fresh database.
func restoreV1(t *testing.T, buf *ioutil2.Buffer) (*database.Database, *memory.Database) {
	t.Helper()
	store := memory.New(nil)
	db := database.New(store, nil)
	require.NoError(t, snapshot.Restore(db, buf, nil))
	return db, store
}

func restoredChain(t *testing.T, store *memory.Database, u *url.URL) *merkle.Chain {
	t.Helper()
	key := record.NewKey("Account", u, "MainChain")
	tx := store.Begin(nil, false)
	t.Cleanup(tx.Discard)
	return merkle.NewChain(nil, keyvalue.RecordStore{Store: tx}, key, 8, merkle.ChainTypeTransaction, "main")
}

// TestIssue4321_WrongElementIndex measures how many element index records a
// version 1 restore gets wrong on a system account.
func TestIssue4321_WrongElementIndex(t *testing.T) {
	const n = 600
	u := sysAccount()
	buf, hashes := buildV1Snapshot(t, u, n)
	_, store := restoreV1(t, buf)
	c := restoredChain(t, store, u)

	head, err := c.Head().Get()
	require.NoError(t, err)
	require.Equal(t, int64(n), head.Count)

	var wrong, atOrPastHead, missing int
	for i, h := range hashes {
		got, err := c.IndexOf(h)
		if err != nil {
			missing++
			continue
		}
		if got != int64(i) {
			wrong++
			if got >= head.Count {
				atOrPastHead++
			}
			if wrong <= 3 {
				t.Logf("entry %d: index says %d (off by %d)", i, got, got-int64(i))
			}
		}
	}
	t.Logf("n=%d markFreq=%d: %d/%d element index records wrong, %d of those name a position at or past the head (%d), %d missing",
		n, c.MarkFreq(), wrong, n, atOrPastHead, head.Count, missing)

	// Every entry is present in the index (presence is complete), but the
	// mark-point-derived ones carry the wrong value.
	require.Zero(t, missing, "every entry must be present in the element index")
	require.Equal(t, 512, wrong)
	require.Equal(t, 168, atOrPastHead)
}

// TestIssue4321_SkippedAppend is the question that decides severity: does a
// database carrying #4321's wrong element indices go on to skip a real append,
// and so execute a different chain than its peers?
//
// Two databases, identical inputs:
//   - clean:    600 entries appended directly, then the post-snapshot work.
//   - restored: the same 600 entries restored from a version 1 snapshot (and
//     therefore carrying 512 wrong element index records), then the same
//     post-snapshot work.
//
// The post-snapshot work deliberately includes replays of hashes whose index
// records are wrong, since the dedup is the only thing that can act on them.
func TestIssue4321_SkippedAppend(t *testing.T) {
	const n = 600
	const extra = 300
	u := sysAccount()

	buf, hashes := buildV1Snapshot(t, u, n)

	// Fresh hashes, from a different seed, so they are genuinely new
	var rh common.RandHash
	rh.SetSeed([]byte("issue-4321-second-half"))
	more := make([][]byte, 0, extra)
	for i := 0; i < extra; i++ {
		more = append(more, rh.NextList())
	}

	// Replays: one whose index came from a mark point and is wrong (300 -> 556),
	// one from the head whose index is right (599), one at the very start
	// (0 -> 256).
	replays := [][]byte{hashes[300], hashes[599], hashes[0]}

	// after applies the identical post-snapshot work to a chain
	after := func(c *database.Chain) {
		t.Helper()
		for _, h := range more {
			require.NoError(t, c.AddEntry(h, true))
		}
		for _, h := range replays {
			require.NoError(t, c.AddEntry(h, true)) // must be skipped
		}
	}

	// Clean database
	cleanDb := database.OpenInMemory(nil)
	cb := cleanDb.Begin(true)
	require.NoError(t, cb.Account(u).Main().Put(&protocol.UnknownAccount{Url: u}))
	cc, err := cb.Account(u).MainChain().Get()
	require.NoError(t, err)
	for _, h := range hashes {
		require.NoError(t, cc.AddEntry(h, false))
	}
	after(cc)
	require.NoError(t, cb.UpdateBPT())
	require.NoError(t, cb.Commit())

	// Restored database
	db, store := restoreV1(t, buf)
	rb := db.Begin(true)
	rc, err := rb.Account(u).MainChain().Get()
	require.NoError(t, err)
	after(rc)
	require.NoError(t, rb.UpdateBPT())
	require.NoError(t, rb.Commit())

	// Read both back
	cb2 := cleanDb.Begin(false)
	defer cb2.Discard()
	cc2, err := cb2.Account(u).MainChain().Get()
	require.NoError(t, err)
	rb2 := db.Begin(false)
	defer rb2.Discard()
	rc2, err := rb2.Account(u).MainChain().Get()
	require.NoError(t, err)

	t.Logf("clean height=%d restored height=%d", cc2.Height(), rc2.Height())
	t.Logf("clean anchor=%x", cc2.Anchor())
	t.Logf("restored anchor=%x", rc2.Anchor())

	// The dedup must have skipped the three replays on both databases
	require.Equal(t, int64(n+extra), cc2.Height(), "clean chain: replays must be skipped")

	// And the two databases must agree
	require.Equal(t, cc2.Height(), rc2.Height(), "restored chain skipped or added an append the clean chain did not")
	require.Equal(t, cc2.Anchor(), rc2.Anchor(), "restored chain diverged from the clean chain")

	// ... entry for entry
	for i := int64(0); i < cc2.Height(); i++ {
		ce, err := cc2.Entry(i)
		require.NoError(t, err)
		re, err := rc2.Entry(i)
		require.NoError(t, err)
		require.Truef(t, bytes.Equal(ce, re), "entry %d differs", i)
	}

	_ = store
}

// TestIssue4321_HeightOfLeaksTheWrongValue records what the wrong indices do
// reach: Chain.HeightOf, which is what AddChainEntry2 returns when the dedup
// skips an append (internal/core/execute/v2/chain/state_state.go:150), and what
// indexing.getIndexedChainReceipt uses to build a receipt.
func TestIssue4321_HeightOfLeaksTheWrongValue(t *testing.T) {
	const n = 600
	u := sysAccount()
	buf, hashes := buildV1Snapshot(t, u, n)

	// Clean
	cleanDb := database.OpenInMemory(nil)
	cb := cleanDb.Begin(true)
	defer cb.Discard()
	require.NoError(t, cb.Account(u).Main().Put(&protocol.UnknownAccount{Url: u}))
	cc, err := cb.Account(u).MainChain().Get()
	require.NoError(t, err)
	for _, h := range hashes {
		require.NoError(t, cc.AddEntry(h, false))
	}

	// Restored
	db, _ := restoreV1(t, buf)
	rb := db.Begin(false)
	defer rb.Discard()
	rc, err := rb.Account(u).MainChain().Get()
	require.NoError(t, err)

	for _, i := range []int{0, 300, 511, 512, 599} {
		ch, err := cc.HeightOf(hashes[i])
		require.NoError(t, err)
		rh, err := rc.HeightOf(hashes[i])
		require.NoError(t, err)
		t.Logf("entry %3d: clean HeightOf=%d restored HeightOf=%d", i, ch, rh)
	}

	ch, err := cc.HeightOf(hashes[300])
	require.NoError(t, err)
	rh, err := rc.HeightOf(hashes[300])
	require.NoError(t, err)
	require.Equal(t, int64(300), ch)
	require.Equal(t, int64(556), rh, "restored node reports a different height for the same entry")
}

// TestIssue4321_HeadAtMarkPoint covers the second instance of the same
// arithmetic: RestoreElementIndexFromHead (merkle_snapshot.go:120) writes
// lastMark+i, and when the head's count is an exact multiple of the mark
// frequency the head's hash list still holds the *previous* mark set, so
// lastMark == Count and those records are wrong by one mark set too.
func TestIssue4321_HeadAtMarkPoint(t *testing.T) {
	for _, n := range []int{512, 513} {
		u := sysAccount()
		buf, hashes := buildV1Snapshot(t, u, n)
		_, store := restoreV1(t, buf)
		c := restoredChain(t, store, u)

		var wrong int
		for i, h := range hashes {
			got, err := c.IndexOf(h)
			require.NoError(t, err)
			if got != int64(i) {
				wrong++
			}
		}
		// The tail (entries at or after the last mark point) is restored from
		// the head.
		var tailWrong int
		for i := n - n%256; i < n; i++ {
			got, err := c.IndexOf(hashes[i])
			require.NoError(t, err)
			if got != int64(i) {
				tailWrong++
			}
		}
		t.Logf("n=%d: %d/%d wrong overall, %d wrong in the tail restored from the head", n, wrong, n, tailWrong)
	}
}

// TestV1RestoreDropsUserAccountIndex is a different mechanism that shares the
// same face, recorded here because it is the one that *does* change an append
// decision: restore.go only rebuilds the element index for system accounts
// (restore.go:137), so after a version 1 restore a user account has no element
// index at all for its pre-snapshot entries. The dedup cannot fire, and a
// replay that a clean node skips is appended by the restored node.
func TestV1RestoreDropsUserAccountIndex(t *testing.T) {
	const n = 600
	u := protocol.AccountUrl("foo") // NOT a system account
	buf, hashes := buildV1Snapshot(t, u, n)

	// Clean
	cleanDb := database.OpenInMemory(nil)
	cb := cleanDb.Begin(true)
	defer cb.Discard()
	require.NoError(t, cb.Account(u).Main().Put(&protocol.UnknownAccount{Url: u}))
	cc, err := cb.Account(u).MainChain().Get()
	require.NoError(t, err)
	for _, h := range hashes {
		require.NoError(t, cc.AddEntry(h, false))
	}
	require.NoError(t, cc.AddEntry(hashes[300], true)) // replay

	// Restored
	db, store := restoreV1(t, buf)
	rb := db.Begin(true)
	defer rb.Discard()
	rc, err := rb.Account(u).MainChain().Get()
	require.NoError(t, err)

	// The element index is simply not there
	mc := restoredChain(t, store, u)
	for _, i := range []int{0, 300, 599} {
		_, err := mc.IndexOf(hashes[i])
		require.Error(t, err, "entry %d should have no element index record", i)
	}

	require.NoError(t, rc.AddEntry(hashes[300], true)) // the same replay

	_, errClean := cc.HeightOf(hashes[300])
	_, errRestored := rc.HeightOf(hashes[300])
	t.Logf("HeightOf on clean: %v; on restored: %v", errClean, errRestored)
	t.Logf("clean height=%d restored height=%d", cc.Height(), rc.Height())
	t.Logf("clean anchor=%x", cc.Anchor())
	t.Logf("restored anchor=%x", rc.Anchor())

	require.Equal(t, int64(600), cc.Height(), "clean node skips the replay")
	require.Equal(t, int64(601), rc.Height(), "restored node appends the replay")
	require.NotEqual(t, cc.Anchor(), rc.Anchor(), "and the two nodes diverge")
}

// TestV2RestoreDropsElementIndex asks the same question of the version 2 path,
// which is the one a node actually uses (state sync, restore-snapshot, and
// genesis built by internal/node/genesis all speak version 2): the v2 collect
// walks with IgnoreIndices (internal/database/snapshot.go:955) and the v2
// restore has no index rebuild, so a v2-restored chain has no element index
// either.
func TestV2RestoreDropsElementIndex(t *testing.T) {
	const n = 600
	u := protocol.AccountUrl("foo")

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	require.NoError(t, batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u}))

	// The v2 collect writes a header from the system ledger
	ledgerUrl := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(ledgerUrl).Main().Put(&protocol.SystemLedger{Url: ledgerUrl, Index: 1}))

	c, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)
	var rh common.RandHash
	for i := 0; i < n; i++ {
		require.NoError(t, c.AddEntry(rh.NextList(), false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	hashes := rh.List

	buf := new(ioutil2.Buffer)
	_, err = db.Collect(buf, protocol.PartitionUrl("BVN0"), &database.CollectOptions{})
	require.NoError(t, err)

	store := memory.New(nil)
	db2 := database.New(store, nil)
	require.NoError(t, database.Restore(db2, buf, &database.RestoreOptions{SkipHashCheck: true}))

	b2 := db2.Begin(true)
	defer b2.Discard()
	c2, err := b2.Account(u).MainChain().Get()
	require.NoError(t, err)
	t.Logf("restored height=%d", c2.Height())
	require.Equal(t, int64(n), c2.Height(), "the chain itself is restored")

	_, err = c2.HeightOf(hashes[300])
	t.Logf("HeightOf on a v2-restored chain: %v", err)
	require.Error(t, err, "v2 restore leaves no element index")

	// Therefore the dedup cannot fire: a replay is appended
	require.NoError(t, c2.AddEntry(hashes[300], true))
	require.Equal(t, int64(n+1), c2.Height(), "v2-restored node appends a replay a clean node skips")
}
