// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// tornBlock makes the stored block under prefix in db disagree with the hash
// its parent records for it, as a block record written without its parent's
// would: it copies the block's record from from, a tree that differs from db
// only under prefix, and leaves db's root record as it is.
func tornBlock(t *testing.T, db, from *database.Database, prefix byte) {
	t.Helper()
	read := db.Begin(false)
	key, err := read.BPT().StoredBlockKey([]byte{prefix})
	read.Discard()
	require.NoError(t, err)

	src, err := from.Store()
	require.NoError(t, err)
	in := src.Begin(nil, false)
	raw, err := in.Get(key)
	in.Discard()
	require.NoError(t, err)

	dst, err := db.Store()
	require.NoError(t, err)
	out := dst.Begin(nil, true)
	require.NoError(t, out.Put(key, raw))
	require.NoError(t, out.Commit())
}

// brokenBlocks is every stored block of db's own tree that cannot be read as
// the tree's.
func brokenBlocks(t *testing.T, db *database.Database) []*bpt.InconsistentBlockError {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	top, err := localTop(b.BPT())
	require.NoError(t, err)
	return top.broken
}

// requireTorn requires db's stored block under prefix to fail its own check.
func requireTorn(t *testing.T, db *database.Database, prefix byte) {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	_, err := b.BPT().GetStoredBlock([]byte{prefix})
	var bad *bpt.InconsistentBlockError
	require.ErrorAs(t, err, &bad, "precondition: the node's block under %x is torn", prefix)
}

// TestANodeWhoseOwnBlockFailsItsCheckTakesItsSectionsAgain — #4523, executor
// spec, "Sync", §3b: "A section that fails its own check is a section not
// loaded." A node restarting on a store whose stored block under one prefix
// disagrees with the hash its root records for it -- the failure that left a
// BVN1 follower retrying the top 115 times in run 20261004T195558Z -- takes
// the top, condemns the sixteen sections under that prefix, forgets them, and
// stands at the peer's root. The failure and its repair are each one entry of
// the repair log.
func TestANodeWhoseOwnBlockFailsItsCheckTakesItsSectionsAgain(t *testing.T) {
	const U = 600
	here := protocol.PartitionUrl("BVN0")
	var accounts []*url.URL
	for i := 0; i < U; i++ {
		accounts = append(accounts, protocol.AccountUrl(fmt.Sprintf("account%03d", i), "tokens"))
	}
	values, _ := genesisValues(t, 4)
	fill := func(db *database.Database, changed *url.URL) {
		putNetwork(t, db, here, values)
		putLedger(t, db, here, 10)
		for _, u := range accounts {
			bal := int64(5)
			if u == changed {
				bal = 7
			}
			putTokens(t, db, u, bal)
		}
	}

	// The account whose block is torn: one in a block of its own, that is,
	// one whose first byte at least one other account shares.
	byPrefix := map[byte]int{}
	for _, u := range accounts {
		byPrefix[byte(database.SectionOfAccount(u)>>4)]++
	}
	var victim *url.URL
	for _, u := range accounts {
		if byPrefix[byte(database.SectionOfAccount(u)>>4)] >= 3 {
			victim = u
			break
		}
	}
	require.NotNil(t, victim)
	prefix := byte(database.SectionOfAccount(victim) >> 4)

	peerDB := peerStore(t)
	fill(peerDB, nil)
	node := peerStore(t)
	fill(node, nil)
	other := peerStore(t)
	fill(other, victim)

	tornBlock(t, node, other, prefix)
	requireTorn(t, node, prefix)

	log := filepath.Join(t.TempDir(), "repairs.jsonl")
	peer := apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})
	s, err := NewState(StateOptions{Partition: here, Database: node, Sources: &onePeer{partition: here, querier: peer, block: 10}, ExecutedBlock: 1, RepairLog: log})
	require.NoError(t, err)
	require.NoError(t, s.Pull(context.Background()))

	require.True(t, s.sync.topTaken, "the node's own torn block kept it from taking the top")
	require.Empty(t, brokenBlocks(t, node), "the sections under the torn block were not taken again")
	require.Equal(t, rootIn(t, peerDB), rootIn(t, node), "the node does not stand at the peer's root")

	f, err := os.Open(log)
	require.NoError(t, err)
	defer f.Close()
	var entries []RepairEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e RepairEntry
		require.NoError(t, json.Unmarshal(sc.Bytes(), &e))
		entries = append(entries, e)
	}
	require.Len(t, entries, 2, "one entry for the failure, one for its outcome")
	require.Equal(t, "section-hash", entries[0].Kind)
	require.Equal(t, hex.EncodeToString([]byte{prefix}), entries[0].Prefix)
	require.Len(t, entries[0].Sections, 16)
	require.Equal(t, entries[0].ID, entries[1].ID)
	require.Equal(t, "repaired", entries[1].Outcome)
}

// TestATornBlockThePeerHoldsNothingUnderIsForgotten — #4523, executor spec,
// "Sync", §3b: the sections under a torn block are condemned whatever the
// peer's top says of them. A peer holding nothing under the prefix has those
// sections empty, and so, read without the torn block, has the node: they
// compare equal, and only their condemnation has the node forget what it
// holds there and stand at the peer's root.
func TestATornBlockThePeerHoldsNothingUnderIsForgotten(t *testing.T) {
	const U = 600
	here := protocol.PartitionUrl("BVN0")
	values, _ := genesisValues(t, 4)
	var accounts []*url.URL
	for i := 0; i < U; i++ {
		accounts = append(accounts, protocol.AccountUrl(fmt.Sprintf("account%03d", i), "tokens"))
	}
	byPrefix := map[byte][]*url.URL{}
	for _, u := range accounts {
		p := byte(database.SectionOfAccount(u) >> 4)
		byPrefix[p] = append(byPrefix[p], u)
	}
	var prefix byte
	for p := 0; p < 256; p++ {
		if len(byPrefix[byte(p)]) >= 3 {
			prefix = byte(p)
			break
		}
	}
	under := byPrefix[prefix]
	require.GreaterOrEqual(t, len(under), 3)

	fill := func(db *database.Database, skipPrefix bool, changed *url.URL) {
		putNetwork(t, db, here, values)
		putLedger(t, db, here, 10)
		for _, u := range accounts {
			if skipPrefix && byte(database.SectionOfAccount(u)>>4) == prefix {
				continue
			}
			bal := int64(5)
			if u == changed {
				bal = 7
			}
			putTokens(t, db, u, bal)
		}
	}
	peerDB := peerStore(t)
	fill(peerDB, true, nil)
	node := peerStore(t)
	fill(node, false, nil)
	other := peerStore(t)
	fill(other, false, under[0])

	tornBlock(t, node, other, prefix)
	requireTorn(t, node, prefix)

	peer := apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})
	s, err := NewState(StateOptions{Partition: here, Database: node, Sources: &onePeer{partition: here, querier: peer, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)
	require.NoError(t, s.Pull(context.Background()))

	require.True(t, s.sync.topTaken, "the torn block survived the top: what the node held under it was not forgotten")
	require.Empty(t, brokenBlocks(t, node))
	require.Equal(t, rootIn(t, peerDB), rootIn(t, node), "the node does not stand at the peer's root")
}

// readRepairs reads a repair log.
func readRepairs(t *testing.T, path string) []RepairEntry {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	var out []RepairEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e RepairEntry
		require.NoError(t, json.Unmarshal(sc.Bytes(), &e))
		out = append(out, e)
	}
	require.NoError(t, sc.Err())
	return out
}

// TestABlockThatLeftAccountsOtherThanItsRecordIsOneRepairEntry — #4523,
// executor spec, "Sync", §3b: a failure leaves one entry, not a dump. A block
// whose execution left three accounts and a chain other than its record is
// one account-diverged entry naming them, closed as repaired: the record's
// state stands for each.
func TestABlockThatLeftAccountsOtherThanItsRecordIsOneRepairEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repairs.jsonl")
	o := &overlap{partition: protocol.PartitionUrl("BVN0"), label: "BVN0", log: slog.Default(), repairs: newRepairs(slog.Default(), path), active: true}
	for i := 0; i < 3; i++ {
		o.said(12, protocol.AccountUrl(fmt.Sprintf("a%d", i)), [32]byte{1}, [32]byte{2}, "peer", "")
	}
	o.chainSaid(12, "acc://a0.acme#main", "the record names a change this execution did not make")
	o.done(12, true, 4)
	o.done(13, true, 0)

	got := readRepairs(t, path)
	require.Len(t, got, 2, "one entry for block 12, one for its outcome, none for the clean block 13")
	require.Equal(t, "account-diverged", got[0].Kind)
	require.Equal(t, uint64(12), got[0].Block)
	require.Equal(t, "4 differ", got[0].Held)
	require.Contains(t, got[0].Account, "acc://a2.acme")
	require.Equal(t, got[0].ID, got[1].ID)
	require.Equal(t, "repaired", got[1].Outcome)
}
