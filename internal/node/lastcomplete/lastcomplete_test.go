// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lastcomplete

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestStateAsOf(t *testing.T) {
	none := indexing.BlockRange{Earliest: 1, Latest: 0}
	kept := indexing.BlockRange{Earliest: 20, Latest: 40}
	for name, c := range map[string]struct {
		executed, block uint64
		rng             indexing.BlockRange
		rngErr          error
		want            State
		wantErr         string
	}{
		"at N":                       {33, 33, none, nil, AtHand, ""},
		"at N, history unreadable":   {33, 33, none, errors.New("x"), AtHand, ""},
		"before N":                   {30, 33, kept, nil, NotObtainable, ""},
		"past N, none kept":          {34, 33, none, nil, NotObtainable, ""},
		"past N, N retained":         {34, 33, kept, nil, Retained, ""},
		"past N, N before the kept":  {50, 10, kept, nil, NotObtainable, ""},
		"past N, history unreadable": {34, 33, none, errors.New("bpt chain: closed"), Unreadable, "bpt chain: closed"},
	} {
		t.Run(name, func(t *testing.T) {
			got, why := StateAsOf(c.executed, c.block, c.rng, c.rngErr)
			require.Equal(t, c.want, got)
			require.Equal(t, c.wantErr, why)
		})
	}
}

func TestAnUnreadableHistoryIsNotSaidToBeAbsent(t *testing.T) {
	a := Answer{Known: true, Block: 33, Executed: 34, State: Unreadable, StateErr: "bpt chain: closed"}
	require.Contains(t, a.String(), "unreadable")
	require.Contains(t, a.String(), "bpt chain: closed")
	require.NotContains(t, a.String(), "keeps no history")
}

// A node's store with a ledger at height executed and anchors 1..len(blocks),
// anchor i carrying blocks[i-1].
func nodeStore(t *testing.T, part string, executed uint64, blocks ...uint64) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	here := protocol.PartitionUrl(part)
	batch := db.Begin(true)
	require.NoError(t, batch.Account(here.JoinPath(protocol.Ledger)).Main().Put(&protocol.SystemLedger{Url: here.JoinPath(protocol.Ledger), Index: executed}))
	pool := here.JoinPath(protocol.AnchorPool)
	for _, b := range blocks {
		txn := new(protocol.Transaction)
		txn.Header.Principal = pool
		txn.Body = &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: here, MinorBlockIndex: b, StateTreeAnchor: [32]byte{byte(b)}}}
		require.NoError(t, batch.Message(txn.ID().Hash()).Main().Put(&messaging.TransactionMessage{Transaction: txn}))
		h := txn.ID().Hash()
		require.NoError(t, batch.Account(pool).AnchorSequenceChain().Inner().AddEntry(h[:], false))
	}
	require.NoError(t, batch.Commit())
	return db
}

func directoryDelivered(part string, n uint64) *protocol.AnchorLedger {
	return &protocol.AnchorLedger{Sequence: []*protocol.PartitionSyntheticLedger{
		{Url: protocol.PartitionUrl("BVN9"), Delivered: 99},
		{Url: protocol.PartitionUrl(part), Delivered: n},
	}}
}

func TestFindNamesTheBlockOfTheAnchorTheDirectoryDelivered(t *testing.T) {
	db := nodeStore(t, "BVN0", 34, 10, 20, 33)

	// The Directory delivered anchor 2: N is anchor 2's block, whatever this
	// node produced after it.
	got := Find(protocol.PartitionUrl("BVN0"), db, directoryDelivered("BVN0", 2), nil)
	require.True(t, got.Known, got.Why)
	require.EqualValues(t, 20, got.Block)
	require.Equal(t, [32]byte{20}, got.Root)
	require.EqualValues(t, 2, got.Anchor)
	require.EqualValues(t, 34, got.Executed)
	require.EqualValues(t, 14, got.Ahead)
	require.Equal(t, NotObtainable, got.State)

	// All three delivered, node at 33: state at hand.
	db = nodeStore(t, "BVN0", 33, 10, 20, 33)
	got = Find(protocol.PartitionUrl("BVN0"), db, directoryDelivered("BVN0", 3), nil)
	require.True(t, got.Known, got.Why)
	require.EqualValues(t, 33, got.Block)
	require.Zero(t, got.Ahead)
	require.Equal(t, AtHand, got.State)
}

func TestFindSaysWhyWhenItCannotName(t *testing.T) {
	db := nodeStore(t, "BVN0", 34, 10, 20)
	part := protocol.PartitionUrl("BVN0")

	got := Find(part, db, directoryDelivered("BVN0", 0), nil)
	require.False(t, got.Known)
	require.Contains(t, got.Why, "none of this partition's anchors")

	// The Directory delivered an anchor this node does not hold: unknown,
	// with the number the Directory named.
	got = Find(part, db, directoryDelivered("BVN0", 5), nil)
	require.False(t, got.Known)
	require.EqualValues(t, 5, got.Anchor)
	require.Contains(t, got.Why, "cannot read the anchor")

	got = Find(part, db, nil, errors.New("no peer serves the Directory"))
	require.False(t, got.Known)
	require.Contains(t, got.Why, "no peer serves the Directory")
	require.EqualValues(t, 34, got.Executed)
}

func TestNoSourceForTheDirectory(t *testing.T) {
	require.Nil(t, Source(nil, nil, protocol.Directory))
	require.Nil(t, Source(nil, nil, "directory"))
	require.NotNil(t, Source(nil, nil, "BVN0"))
}
