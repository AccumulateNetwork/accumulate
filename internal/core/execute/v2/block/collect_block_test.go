// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// newCollectingExecutor is a second executor over the same network and the
// same store: a joining node, whose staging is its own and empty, reading the
// state the pull has given it (#4292).
func newCollectingExecutor(s *stagingSim) *Executor {
	x := new(Executor)
	x.Describe = s.x.Describe
	x.globalsPtr.Store(s.x.globals())
	return x
}

// putSystemLedger makes the store say which block it is: what SettleStaging
// checks Q against.
func putSystemLedger(t *testing.T, batch *database.Batch, x *Executor, index uint64) {
	t.Helper()
	ledger := new(protocol.SystemLedger)
	ledger.Url = x.Describe.Ledger()
	ledger.Index = index
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
}

// stagingIsEqual compares two stagings the way the join must make them equal:
// the same streams at the same positions, the same entries held under the
// same IDs in the same form, the same validated hashes, the same proofs
// waiting on the same anchor blocks (issue #4292).
func requireStagingEqual(t *testing.T, want, got *execute.Staging) {
	t.Helper()
	a, b := want.Begin(), got.Begin()
	defer a.Discard()
	defer b.Discard()

	as, bs := a.Streams(), b.Streams()
	require.Equal(t, len(as), len(bs), "the same streams")
	for i, sa := range as {
		sb := bs[i]
		where := fmt.Sprintf("stream %v from %v", sa.ID.Ledger, sa.ID.Source)
		require.True(t, sa.ID.Ledger.Equal(sb.ID.Ledger), where)
		require.True(t, sa.ID.Source.Equal(sb.ID.Source), where)
		require.Equal(t, sa.Delivered, sb.Delivered, "%s: delivered", where)
		require.Equal(t, sa.Held, sb.Held, "%s: held", where)
		require.Equal(t, sa.Sighted, sb.Sighted, "%s: sighted", where)
		require.Equal(t, sa.Reach, sb.Reach, "%s: reach", where)
		require.Equal(t, sa.Waiting, sb.Waiting, "%s: waiting", where)

		for n := sa.Delivered + 1; n <= sa.Sighted; n++ {
			ha, oka := a.IDOf(sa.ID, n)
			hb, okb := b.IDOf(sa.ID, n)
			require.Equal(t, oka, okb, "%s: %d held", where, n)
			if !oka {
				continue
			}
			require.Equal(t, ha.ID.String(), hb.ID.String(), "%s: %d held under the same ID", where, n)
			require.Equal(t, ha.Collected, hb.Collected, "%s: %d collected", where, n)
			require.Equal(t, ha.Hash, hb.Hash, "%s: %d hash", where, n)
			require.Equal(t, ha.Message.Type(), hb.Message.Type(), "%s: %d holds the same form", where, n)
			if ha.Companion == nil {
				require.Nil(t, hb.Companion, "%s: %d companion", where, n)
			} else {
				require.NotNil(t, hb.Companion, "%s: %d companion", where, n)
			}
			va, oka := a.Validated(sa.ID, n)
			vb, okb := b.Validated(sa.ID, n)
			require.Equal(t, oka, okb, "%s: %d validated", where, n)
			require.Equal(t, va, vb, "%s: %d validated hash", where, n)
		}
	}

	sourcesA, sourcesB := a.ProofSources(), b.ProofSources()
	require.Equal(t, len(sourcesA), len(sourcesB), "the same sources have proofs waiting")
	for i, src := range sourcesA {
		require.True(t, src.Equal(sourcesB[i]), "the same source")
		blocksA, blocksB := a.ProofBlocks(src), b.ProofBlocks(src)
		require.Equal(t, blocksA, blocksB, "%v: the same anchor blocks are waited on", src)
		for _, blk := range blocksA {
			pa, pb := a.Proofs(src, blk), b.Proofs(src, blk)
			require.Equal(t, len(pa), len(pb), "%v: block %d holds the same proofs", src, blk)
			for j := range pa {
				require.Equal(t, pa[j].ReceiptList.MerkleState.Count, pb[j].ReceiptList.MerkleState.Count)
				require.Equal(t, len(pa[j].ReceiptList.Elements), len(pb[j].ReceiptList.Elements))
			}
		}
	}
}

// A node that collects a block holds what the node that executed it holds.
// This is the proof that the join can be exact: the collecting node takes the
// same blocks, holds the same entries under the same rules, and once the
// pulled state says where each stream stands, the two stagings are the same
// thing (executor spec, "Sync"; #4292).
func TestCollectBlock_HoldsWhatTheExecutingNodeHolds(t *testing.T) {
	s := newStagingSim(t, 6)
	col := newCollectingExecutor(s)

	// Block 1: a package covering entries 1..3 arrives ahead of the anchor
	// that proves it. One node executes the block; the other collects it.
	env1 := s.packageEnvelope(0, 2, 3)
	s.packageArrives(0, 2, 3)
	out, err := col.CollectBlock(s.batch, execute.BlockParams{Index: 1}, []*messaging.Envelope{env1})
	require.NoError(t, err)
	require.Equal(t, 3, out.Held, "three entries held, none executed")

	// Block 2: the anchor lands. The executing node validates the proof and
	// runs the entries; the collecting node executes nothing.
	s.newBlock()
	s.anchorExecutes(3, s.rootAt(3))
	require.Equal(t, []uint64{1, 2, 3}, s.run())

	// Block 3: a second package, whose anchor is not here. Both hold it.
	s.newBlock()
	env2 := s.packageEnvelope(3, 5, 6)
	s.packageArrives(3, 5, 6)
	out, err = col.CollectBlock(s.batch, execute.BlockParams{Index: 3}, []*messaging.Envelope{env2})
	require.NoError(t, err)
	require.Equal(t, 3, out.Held)
	s.newBlock() // close block 3: Delivered is written back to the ledger

	// Before the settle the collecting node still holds what the executing
	// node released, and still waits on the anchor the executing node has
	// already seen.
	{
		tx := col.staging().Begin()
		require.Equal(t, []uint64{3, 6}, tx.ProofBlocks(s.str.source), "both packages' proofs wait")
		st := tx.Status(s.str.id())
		require.Equal(t, uint64(0), st.Delivered, "a collecting node delivers nothing")
		require.Equal(t, 6, st.Held)
		tx.Discard()
	}

	// The state pull has brought the store to block 3; staging is settled
	// against it.
	putSystemLedger(t, s.batch, col, 3)
	require.NoError(t, col.SettleStaging(s.batch, 3))

	requireStagingEqual(t, s.x.staging(), col.staging())
}

// Collecting writes nothing: not the message, not a signature, not a ledger.
// The state is the pull's to provide; a collecting node that wrote anything
// would be executing (executor spec, "Sync").
func TestCollectBlock_WritesNothing(t *testing.T) {
	f := newStagingFixture(t, 0)
	dn := protocol.DnUrl()
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	// A settled store: the anchor ledger is written and committed, so
	// anything dirty afterwards is this block's doing.
	var ledger protocol.AnchorLedger
	ledger.Url = pool
	ledger.Partition(dn).Delivered = 2
	require.NoError(t, f.batch.Account(pool).Main().Put(&ledger))
	require.NoError(t, f.batch.Commit())

	txn := new(protocol.Transaction)
	txn.Header.Principal = pool
	txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: dn, MinorBlockIndex: 3}}
	seq := &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: dn, Destination: protocol.PartitionUrl("BVN0"), Number: 3}
	env := &messaging.Envelope{Messages: []messaging.Message{&messaging.BlockAnchor{Anchor: seq}}}

	batch := f.db.Begin(true)
	defer batch.Discard()
	out, err := f.x.CollectBlock(batch, execute.BlockParams{Index: 1}, []*messaging.Envelope{env})
	require.NoError(t, err)
	require.Equal(t, 1, out.Held)
	require.False(t, batch.IsDirty(), "collecting a block writes nothing — not a signature, not a message, not a ledger")

	h := seq.Hash()
	_, err = batch.Message(h).Main().Get()
	require.Error(t, err, "a collected message is not stored")
	require.True(t, errors.Is(err, errors.NotFound), "got %v", err)
}

// An anchor copy is held as a copy below its quorum is held, and its
// signature is NOT recorded: the store's signatures come from the pull, which
// has what the peers recorded through Q (#4292).
func TestCollectBlock_HoldsAnchorCopiesWithoutRecordingTheirSignatures(t *testing.T) {
	f := newStagingFixture(t, 0)
	dn := protocol.DnUrl()
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	var ledger protocol.AnchorLedger
	ledger.Url = pool
	ledger.Partition(dn).Delivered = 2
	require.NoError(t, f.batch.Account(pool).Main().Put(&ledger))

	anchor := func(n uint64) *messaging.BlockAnchor {
		txn := new(protocol.Transaction)
		txn.Header.Principal = pool
		txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: dn, MinorBlockIndex: n}}
		seq := &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: dn, Destination: protocol.PartitionUrl("BVN0"), Number: n}
		return &messaging.BlockAnchor{Anchor: seq}
	}

	out, err := f.x.CollectBlock(f.batch, execute.BlockParams{Index: 9},
		[]*messaging.Envelope{{Messages: []messaging.Message{anchor(2), anchor(3), anchor(4), anchor(3)}}})
	require.NoError(t, err)
	require.Equal(t, 2, out.Held, "#2 is at Delivered, #3 twice is one entry")

	tx := f.x.staging().Begin()
	defer tx.Discard()
	id := execute.StreamID{Ledger: pool, Source: dn}
	_, ok := tx.IDOf(id, 2)
	require.False(t, ok, "at or below Delivered is not held")
	h, ok := tx.IDOf(id, 3)
	require.True(t, ok)
	require.True(t, h.Collected, "an anchor copy is held collected: its quorum decides it")
	require.Equal(t, messaging.MessageTypeSequenced, h.Message.Type())
	_, ok = tx.IDOf(id, 4)
	require.True(t, ok)
}

// SettleStaging releases every stream through the Delivered the PULLED ledger
// names -- the peers' word on what has executed -- not through anything
// staging remembers, which on a joining node is zero (#4291's trap).
func TestSettleStaging_ReleasesThroughThePulledDelivered(t *testing.T) {
	f := newStagingFixture(t, 0)
	id := f.stream()

	tx := f.x.staging().Begin()
	for n := uint64(1); n <= 5; n++ {
		seq := &messaging.SequencedMessage{Source: f.source, Destination: protocol.PartitionUrl("BVN0"), Number: n}
		tx.Hold(id, n, &execute.Held{ID: seq.ID(), Message: seq})
	}
	tx.Commit()

	// The pulled state at block 7: the peers had delivered 3 of this stream.
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = id.Ledger
	ledger.Partition(f.source).Delivered = 3
	require.NoError(t, f.batch.Account(id.Ledger).Main().Put(ledger))
	putSystemLedger(t, f.batch, f.x, 7)

	require.NoError(t, f.batch.Commit())
	batch := f.db.Begin(true)
	defer batch.Discard()
	require.NoError(t, f.x.SettleStaging(batch, 7))
	require.False(t, batch.IsDirty(), "settling staging writes nothing: it is memory")

	tx = f.x.staging().Begin()
	defer tx.Discard()
	st := tx.Status(id)
	require.Equal(t, uint64(3), st.Delivered)
	require.Equal(t, 2, st.Held, "everything at or below the pulled Delivered is dropped")
	for n := uint64(1); n <= 3; n++ {
		_, ok := tx.IDOf(id, n)
		require.False(t, ok, "%d is at or below Delivered", n)
	}
	for n := uint64(4); n <= 5; n++ {
		_, ok := tx.IDOf(id, n)
		require.True(t, ok, "%d is still held", n)
	}
}

// A staging settled against the state of a different block would execute a
// different block than its peers; the pairing is checked, not trusted
// (#4290).
func TestSettleStaging_RefusesAStateThatIsNotTheBlock(t *testing.T) {
	f := newStagingFixture(t, 0)
	putSystemLedger(t, f.batch, f.x, 7)

	err := f.x.SettleStaging(f.batch, 6)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.Contains(t, err.Error(), "BVN0", "the partition is named: block numbers collide across partitions")
}

// The accounts a block names are what the state pull must fetch for it
// (#4293): every principal, every signer.
func TestCollectBlock_NamesTheAccountsTheBlockTouches(t *testing.T) {
	f := newStagingFixture(t, 0)

	alice := protocol.AccountUrl("alice", "tokens")
	txn := new(protocol.Transaction)
	txn.Header.Principal = alice
	txn.Body = &protocol.SendTokens{}
	env := &messaging.Envelope{Messages: []messaging.Message{
		&messaging.TransactionMessage{Transaction: txn},
		&messaging.SignatureMessage{
			Signature: &protocol.ED25519Signature{Signer: protocol.AccountUrl("alice", "book", "1"), TransactionHash: txn.ID().Hash()},
			TxID:      txn.ID(),
		},
	}}

	out, err := f.x.CollectBlock(f.batch, execute.BlockParams{Index: 4}, []*messaging.Envelope{env})
	require.NoError(t, err)
	require.Equal(t, 0, out.Held, "a user envelope holds nothing in staging")
	require.Equal(t, []string{
		protocol.AccountUrl("alice", "book", "1").String(),
		alice.String(),
	}, urlStrings(out.Accounts))
}

func urlStrings(urls []*url.URL) []string {
	out := make([]string, len(urls))
	for i, u := range urls {
		out[i] = u.String()
	}
	return out
}
