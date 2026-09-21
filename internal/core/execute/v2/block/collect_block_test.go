// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// joiner is a second node of the same partition: its own staging, and its own
// store, which starts empty and is filled by the pull. Collecting decides
// what to hold against the store as it stands, so a test that gave it the
// peer's live store would prove only that collect matches execute given
// identical state — which is the one thing a joining node does not have
// (#4292 review).
type joiner struct {
	x     *Executor
	store *memory.Database
	db    *database.Database
	peer  *stagingSim
}

func newJoiner(t *testing.T, s *stagingSim) *joiner {
	t.Helper()
	x := new(Executor)
	x.Describe = s.x.Describe
	x.globalsPtr.Store(s.x.globals())
	store := memory.New(nil)
	db := database.New(store, nil)
	x.Database = db
	return &joiner{x: x, store: store, db: db, peer: s}
}

// collect takes one of the peer's blocks into the joining node's staging,
// against the joining node's own store.
func (j *joiner) collect(t *testing.T, index uint64, envelopes ...*messaging.Envelope) *execute.CollectedBlock {
	t.Helper()
	batch := j.db.Begin(true)
	defer batch.Discard()
	out, err := j.x.CollectBlock(batch, execute.BlockParams{Index: index}, envelopes)
	require.NoError(t, err)
	require.False(t, batch.IsDirty(), "collecting a block writes nothing")
	return out
}

// pull is what #4293 does, in one step: the joining node's store becomes the
// peer's state. The peer commits at every block, so what is exported is the
// state as of its last closed block.
func (j *joiner) pull(t *testing.T) {
	t.Helper()
	entries, err := j.peer.store.Export()
	require.NoError(t, err)
	require.NoError(t, j.store.Import(entries))
}

// settle brings the joining node's staging to the block its pulled state is.
func (j *joiner) settle(t *testing.T, q uint64) {
	t.Helper()
	batch := j.db.Begin(true)
	defer batch.Discard()
	require.NoError(t, j.x.SettleStaging(batch, q))
	require.False(t, batch.IsDirty(), "settling staging writes nothing: it is memory")
}

// newAnchorFixture is a staging fixture whose globals carry a Directory
// validator, so an anchor copy can be signed the way a real one is: an anchor
// copy that its own executor would refuse must not be held (#4292 review).
func newAnchorFixture(t *testing.T) (*stagingFixture, ed25519.PrivateKey) {
	t.Helper()
	f := newStagingFixture(t, 0)
	seed := sha256.Sum256([]byte("directory validator"))
	key := ed25519.NewKeyFromSeed(seed[:])
	f.x.globalsPtr.Store(&Globals{Active: core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionLatest,
		Globals:         &protocol.NetworkGlobals{ValidatorAcceptThreshold: protocol.Rational{Numerator: 2, Denominator: 3}},
		Network: &protocol.NetworkDefinition{
			Version: 1,
			Partitions: []*protocol.PartitionInfo{
				{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
				{ID: "BVN0", Type: protocol.PartitionTypeBlockValidator},
			},
			Validators: []*protocol.ValidatorInfo{{
				PublicKey:     key[32:],
				PublicKeyHash: sha256.Sum256(key[32:]),
				Partitions:    []*protocol.ValidatorPartitionInfo{{ID: protocol.Directory, Active: true}},
			}},
		},
	}})
	return f, key
}

// signedAnchor is a Directory anchor copy as a validator of the Directory
// sends it.
func signedAnchor(key ed25519.PrivateKey, n uint64) *messaging.BlockAnchor {
	dn := protocol.DnUrl()
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: dn, MinorBlockIndex: n}}
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      dn,
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      n,
	}
	h := seq.Hash()
	sig := &protocol.ED25519Signature{PublicKey: key[32:], Signer: dn.JoinPath(protocol.Network), SignerVersion: 1, TransactionHash: h}
	protocol.SignED25519(sig, key, nil, h[:])
	return &messaging.BlockAnchor{Anchor: seq, Signature: sig}
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
	j := newJoiner(t, s)

	// Block 1: a package covering entries 1..3 arrives ahead of the anchor
	// that proves it. One node executes the block; the other collects it,
	// against its own store, which the pull has not filled yet.
	env1 := s.packageEnvelope(0, 2, 3)
	s.packageArrives(0, 2, 3)
	require.Equal(t, 3, j.collect(t, 1, env1).Held, "three entries held, none executed")

	// Block 2: the anchor lands. The executing node validates the proof and
	// runs the entries; the collecting node executes nothing.
	s.newBlock()
	s.anchorExecutes(3, s.rootAt(3))
	require.Equal(t, []uint64{1, 2, 3}, s.run())

	// Block 3: a second package, whose anchor is not here. Both hold it.
	s.newBlock()
	env2 := s.packageEnvelope(3, 5, 6)
	s.packageArrives(3, 5, 6)
	require.Equal(t, 3, j.collect(t, 3, env2).Held)
	s.newBlock() // close block 3: Delivered is written back to the ledger

	// Before the settle the collecting node holds everything it collected —
	// including what the executing node has already run, because its own
	// store says nothing has been delivered — and waits on both anchors.
	{
		tx := j.x.staging().Begin()
		require.Equal(t, []uint64{3, 6}, tx.ProofBlocks(s.str.source), "both packages' proofs wait")
		st := tx.Status(s.str.id())
		require.Equal(t, uint64(0), st.Delivered, "its own store has delivered nothing")
		require.Equal(t, 6, st.Held)
		tx.Discard()
	}

	// The pull brings its store to the peer's state at block 3.
	j.pull(t)

	// Staging is settled against the state it pulled.
	j.settle(t, 3)

	requireStagingEqual(t, s.x.staging(), j.x.staging())
}

// TEST (b) at the executor: a peer holds an entry from before the joining node
// started listening, so the block after the state has a GAP and must not be
// executed (executor spec, "Sync", §4; #4290, #4362).
//
// The whole of the join's decision is here. Block B+1's transactions are, by
// definition, ones not executed as of B. If every stream can deliver a
// contiguous run from Delivered + 1 through what B+1 carries, this node
// executes B+1 exactly as its peers do. A hole in that run is an entry that
// arrived before this node was listening -- the peers have held it since a
// block before B -- and executing without it delivers a SHORTER run than they
// do, after which the root chain never matches again.
//
// Nothing here is hand-fed. The block goes through CollectBlock, which is what
// a collecting node does with every block consensus commits, and the reach it
// reports is what that intake saw; Delivered comes from the pulled ledger, as
// it does in a block.
func TestStagingGaps_APreListenEntryIsAGapAndClearsWhenThePeersRunIt(t *testing.T) {
	s := newStagingSim(t, 6)
	j := newJoiner(t, s)

	// Block 1 on the peer: entries 1..3 arrive with a proof whose anchor is
	// not here yet. The joining node is not listening, so it has none of them.
	s.packageArrives(0, 2, 3)
	s.newBlock()

	// Block 2: the joining node starts listening and collects a block that
	// carries 4..6. Its own store says nothing has been delivered.
	env := s.packageEnvelope(3, 5, 6)
	s.packageArrives(3, 5, 6)
	out := j.collect(t, 2, env)
	s.newBlock()

	require.NotEmpty(t, out.Reach, "the block reports what it carried on each stream")
	var reach uint64
	for _, r := range out.Reach {
		if r.ID.Ledger.Equal(s.str.id().Ledger) && r.ID.Source.Equal(s.str.id().Source) {
			reach = r.High
		}
	}
	require.Equal(t, uint64(6), reach, "the block carried through 6 on the stream")

	// The state this node pulled says the peers have delivered NOTHING, and
	// the block it collected starts at 4. Entries 1 to 3 arrived before it was
	// listening: a gap, and it must not execute.
	gaps, err := j.x.StagingGaps(out.Reach)
	require.NoError(t, err)
	require.Len(t, gaps, 1, "the stream whose run is not contiguous is named")
	require.Equal(t, s.str.id().Ledger.String(), gaps[0].ID.Ledger.String())
	require.Equal(t, s.str.id().Source.String(), gaps[0].ID.Source.String())
	require.Equal(t, uint64(0), gaps[0].Delivered)
	require.Equal(t, uint64(6), gaps[0].Through)
	require.Equal(t, [][2]uint64{{1, 3}}, gaps[0].Missing,
		"and the entries it cannot deliver are named, not just the fact of them")

	// The peers' anchor lands and they run 1..3. The joining node pulls the
	// state again: Delivered is now 3, which is what the peers actually ran.
	s.anchorExecutes(3, s.rootAt(3))
	require.Equal(t, []uint64{1, 2, 3}, s.run())
	s.newBlock()
	j.pull(t)

	// Everything it lacks is now at or under Delivered, so the same block has
	// no gap and it executes.
	gaps, err = j.x.StagingGaps(out.Reach)
	require.NoError(t, err)
	require.Empty(t, gaps,
		"once the peers have executed the pre-listen entries, the run from Delivered+1 is contiguous")
}

// A hole is a hole wherever the number above it came from: the run is walked
// through everything the node holds, not only through what the block carried.
//
// A check made only against the block's own numbers finds nothing when the
// block happens to carry none of the stream's entries -- and the node then
// executes with a stream stuck below a hole it can never fill by listening,
// while its peers run those entries in the blocks where they belong.
// Measured on TestOneValidatorRestartDoesNotDiverge: BVN1's anchor stream held
// five entries for thirty-eight blocks after a join that passed a per-block
// check, and the node's root chain ended 34 entries short of its peers'.
func TestStagingGaps_AHoleIsAHoleWhereverTheNumberAboveItCameFrom(t *testing.T) {
	s := newStagingSim(t, 9)
	j := newJoiner(t, s)

	// The peer runs 1..3 and the joining node pulls that state.
	s.packageArrives(0, 2, 3)
	s.anchorExecutes(3, s.rootAt(3))
	require.Equal(t, []uint64{1, 2, 3}, s.run())
	s.newBlock()
	j.pull(t)

	// It collects a block carrying 4..6, and then one carrying 8..9 -- 7 is
	// still on its way.
	first := j.collect(t, 2, s.packageEnvelope(3, 5, 6))
	second := j.collect(t, 3, s.packageEnvelope(7, 8, 9))

	gaps, err := j.x.StagingGaps(first.Reach)
	require.NoError(t, err)
	require.Len(t, gaps, 1,
		"the first block's own numbers are contiguous, but the node holds 8 and 9 "+
			"above a hole at 7, and that hole is a gap whichever block would have "+
			"delivered it")
	require.Equal(t, [][2]uint64{{7, 7}}, gaps[0].Missing)

	gaps, err = j.x.StagingGaps(second.Reach)
	require.NoError(t, err)
	require.Len(t, gaps, 1, "and so it is when asked about the block that carried 8 and 9")
	require.Equal(t, [][2]uint64{{7, 7}}, gaps[0].Missing)
}

// Collecting writes nothing: not the message, not a signature, not a ledger.
// The state is the pull's to provide; a collecting node that wrote anything
// would be executing (executor spec, "Sync").
func TestCollectBlock_WritesNothing(t *testing.T) {
	f, key := newAnchorFixture(t)
	dn := protocol.DnUrl()
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	// A settled store: the anchor ledger is written and committed, so
	// anything dirty afterwards is this block's doing.
	var ledger protocol.AnchorLedger
	ledger.Url = pool
	ledger.Partition(dn).Delivered = 2
	require.NoError(t, f.batch.Account(pool).Main().Put(&ledger))
	require.NoError(t, f.batch.Commit())

	anchor := signedAnchor(key, 3)
	seq := anchor.Anchor.(*messaging.SequencedMessage)
	env := &messaging.Envelope{Messages: []messaging.Message{anchor}}

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
	f, key := newAnchorFixture(t)
	dn := protocol.DnUrl()
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	var ledger protocol.AnchorLedger
	ledger.Url = pool
	ledger.Partition(dn).Delivered = 2
	require.NoError(t, f.batch.Account(pool).Main().Put(&ledger))

	// One copy signed by nobody: a block refuses it, so collecting must too,
	// or its number is taken by an entry the peers never held and the real
	// one can never take it (first sighting wins).
	unsigned := signedAnchor(key, 5)
	unsigned.Signature = nil

	out, err := f.x.CollectBlock(f.batch, execute.BlockParams{Index: 9},
		[]*messaging.Envelope{{Messages: []messaging.Message{
			signedAnchor(key, 2), signedAnchor(key, 3), signedAnchor(key, 4), signedAnchor(key, 3), unsigned,
		}}})
	require.NoError(t, err)
	require.Equal(t, 2, out.Held, "#2 is at Delivered, #3 twice is one entry, #5 is not signed")

	tx := f.x.staging().Begin()
	defer tx.Discard()
	id := execute.StreamID{Ledger: pool, Source: dn}
	_, ok := tx.IDOf(id, 2)
	require.False(t, ok, "at or below Delivered is not held")
	h, ok := tx.IDOf(id, 3)
	require.True(t, ok)
	require.True(t, h.Collected, "an anchor copy below its quorum is held collected")
	require.Equal(t, messaging.MessageTypeSequenced, h.Message.Type())
	_, ok = tx.IDOf(id, 4)
	require.True(t, ok)
	_, ok = tx.IDOf(id, 5)
	require.False(t, ok, "an anchor copy its own executor refuses is not held")
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
