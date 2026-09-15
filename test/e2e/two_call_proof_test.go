// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4272: a BPT is a tree of current state, so an account cannot be proved
// against a past BPT. The proof is built against the current one, and that root
// reaches the directory only after an anchor round trip — so it takes two
// calls, except on the directory where the root is already local.

func twoCallSim(t *testing.T) (*Sim, *url.URL, []byte) {
	t.Helper()
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(10)
	return sim, lite, liteKey
}

// A BVN account's receipt is not complete: it stops at the partition's BPT root
// and names who to bind against.
func TestTwoCallProof_BvnAccountIsNotComplete(t *testing.T) {
	sim, lite, _ := twoCallSim(t)

	r := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	require.NotNil(t, r.Receipt, "the query must return a receipt")
	require.False(t, r.Receipt.Complete,
		"a BVN account's root reaches the directory only after an anchor round trip")
	require.NotEmpty(t, r.Receipt.Partition,
		"an incomplete receipt must name what to bind against")
	require.NotEqual(t, Directory, r.Receipt.Partition)
	t.Logf("BVN account: complete=%v partition=%s", r.Receipt.Complete, r.Receipt.Partition)
}

// A directory account's receipt is complete: the BPT is already the
// directory's, so there is nothing to wait for and no second call.
func TestTwoCallProof_DirectoryAccountIsComplete(t *testing.T) {
	sim, _, _ := twoCallSim(t)

	r := sim.QueryAccount(DnUrl().JoinPath(Ledger), &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	require.NotNil(t, r.Receipt)
	require.True(t, r.Receipt.Complete,
		"a directory account needs no second call — its BPT is already the directory's")
	require.Equal(t, Directory, r.Receipt.Partition)
	t.Logf("DN account: complete=%v partition=%s", r.Receipt.Complete, r.Receipt.Partition)
}

// The flag must track the account, not the node that answered.
func TestTwoCallProof_FlagFollowsTheAccount(t *testing.T) {
	sim, lite, _ := twoCallSim(t)

	bvn := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	dn := sim.QueryAccount(DnUrl().JoinPath(Ledger), &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})

	require.NotEqual(t, bvn.Receipt.Complete, dn.Receipt.Complete,
		"the same node answers both; the flag must describe the account")
}

// Sanity: activity keeps producing receipts, and an incomplete one still
// validates on its own — it is a correct proof, just half of one.
func TestTwoCallProof_IncompleteStillValidates(t *testing.T) {
	sim, lite, liteKey := twoCallSim(t)
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("tc-other")).RootIdentity().JoinPath(ACME)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	r := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	require.NotNil(t, r.Receipt)
	require.True(t, r.Receipt.Receipt.Validate(nil), "an incomplete receipt is still a valid proof")
}

// proofServiceFor builds the service that answers call 2 for a partition. It
// needs both databases: the partition's to extend a BPT root to a root-chain
// anchor, the directory's to bind that anchor (#4274). On a real node both are
// local, because every node runs the directory alongside its own BVN.
func proofServiceFor(sim *Sim, partition string) *apiimpl.ProofService {
	return &apiimpl.ProofService{
		Database:  sim.Database(partition),
		Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
		Partition: config.NetworkUrl{URL: PartitionUrl(partition)},
	}
}

// The claim the whole design rests on: call 1 and call 2 compose into one
// receipt that validates against a directory root.
func TestTwoCallProof_ComposesEndToEnd(t *testing.T) {
	sim, lite, liteKey := twoCallSim(t)

	// Move the account, so its BPT root is fresh
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("tc-e2e-other")).RootIdentity().JoinPath(ACME)
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// CALL 1 — the account's receipt, to its partition's BPT root
	first := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	require.NotNil(t, first.Receipt)
	require.False(t, first.Receipt.Complete, "a BVN account needs the second call")
	var bptRoot [32]byte
	copy(bptRoot[:], first.Receipt.Receipt.Anchor)
	t.Logf("call 1: partition=%s root=%x", first.Receipt.Partition, bptRoot[:6])

	// Let the anchor reach the directory
	svc := proofServiceFor(sim, first.Receipt.Partition)
	var second *apiv3.AnchorReceiptRecord
	var err error
	for i := 0; i < 60 && (second == nil || !second.Anchored); i++ {
		sim.Step()
		second, err = svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{
			Partition: first.Receipt.Partition,
			BptRoot:   bptRoot,
		})
		require.NoError(t, err)
	}
	require.True(t, second.Anchored, "the anchor never reached the directory")
	require.NotNil(t, second.Receipt)
	t.Logf("call 2: anchored at directory block %d", second.DirectoryBlock)

	// COMPOSE — call 1 ends where call 2 starts
	require.Equal(t, first.Receipt.Receipt.Anchor, second.Receipt.Start,
		"the two calls must meet at the BPT root")
	joined, err := first.Receipt.Receipt.Combine(second.Receipt)
	require.NoError(t, err, "the two receipts must compose")
	require.True(t, joined.Validate(nil), "the joined receipt must validate")
	t.Logf("joined: %x .. %x", joined.Start[:6], joined.Anchor[:6])
}

// The second call must still work long after the fact. A caller may come back
// much later, and the directory's anchor chain keeps the entry.
func TestTwoCallProof_StillWorksManyBlocksLater(t *testing.T) {
	sim, lite, liteKey := twoCallSim(t)
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("tc-late-other")).RootIdentity().JoinPath(ACME)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	first := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	var bptRoot [32]byte
	copy(bptRoot[:], first.Receipt.Receipt.Anchor)

	svc := proofServiceFor(sim, first.Receipt.Partition)
	// Wait for it to be anchored at all. The nudge is not ceremony: a network
	// with nothing to do stops, and a root captured as it stops is the last one
	// -- it never becomes a bpt chain entry, because that happens in the
	// FOLLOWING block, and no following block exists. Any transaction restarts
	// it. See TestTwoCallProof_QuiescentTailNeedsANudge.
	nudge(t, sim, lite, liteKey, 2)
	var rec *apiv3.AnchorReceiptRecord
	var err error
	for i := 0; i < 60 && (rec == nil || !rec.Anchored); i++ {
		sim.Step()
		rec, err = svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{Partition: first.Receipt.Partition, BptRoot: bptRoot})
		require.NoError(t, err)
	}
	require.True(t, rec.Anchored)
	firstBlock := rec.DirectoryBlock

	// Now let a great many blocks pass, with other activity
	for i := 0; i < 40; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				SendTokens(1, 0).To(other).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 3)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	// The SAME root must still produce a receipt, now terminating at a later
	// directory root
	late, err := svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{Partition: first.Receipt.Partition, BptRoot: bptRoot})
	require.NoError(t, err)
	require.True(t, late.Anchored, "the entry must still be found long afterwards")
	require.NotNil(t, late.Receipt)
	require.Equal(t, first.Receipt.Receipt.Anchor, late.Receipt.Start, "still starts at the same root")

	joined, err := first.Receipt.Receipt.Combine(late.Receipt)
	require.NoError(t, err)
	require.True(t, joined.Validate(nil), "the late receipt must still compose and validate")
	t.Logf("anchored at DN block %d (anchor %x), still provable at DN block %d (anchor %x)",
		firstBlock, rec.Receipt.Anchor[:6], late.DirectoryBlock, late.Receipt.Anchor[:6])

	// Does the late receipt terminate at a LATER directory root, or the same
	// one? The field says "the directory block the receipt terminates at", so
	// this is checking the contract rather than the plumbing.
	// The receipt terminates at the directory root AS OF the anchoring block,
	// not the current one -- the same terminus both times, 120 blocks apart.
	// That is the contract: the second call is stable, and a caller that
	// records it does not have to re-fetch as the chain moves on. Reaching the
	// current root instead would be a different proof, and a moving one.
	require.Equal(t, rec.Receipt.Anchor, late.Receipt.Anchor,
		"the terminus must be stable across time")
	require.Equal(t, firstBlock, late.DirectoryBlock,
		"DirectoryBlock names where the receipt ends, and that does not move")

	View(t, sim.Database(Directory), func(batch *database.Batch) {
		root, err := batch.Account(DnUrl().JoinPath(Ledger)).RootChain().Get()
		require.NoError(t, err)
		cur := root.Anchor()
		require.NotEqual(t, cur, late.Receipt.Anchor,
			"and it is deliberately not the current root")
		t.Logf("current DN root chain anchor %x (height %d)", cur[:6], root.Height())
	})
}

// The default is the oldest receipt that works, and it is stable. A caller that
// already trusts a later directory root can ask for one reaching that instead —
// both are valid proofs of the same BPT root.
func TestTwoCallProof_OldestByDefaultLaterOnRequest(t *testing.T) {
	sim, lite, liteKey := twoCallSim(t)
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("tc-ata-other")).RootIdentity().JoinPath(ACME)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	first := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	var bptRoot [32]byte
	copy(bptRoot[:], first.Receipt.Receipt.Anchor)

	svc := proofServiceFor(sim, first.Receipt.Partition)
	ask := func(atOrAfter uint64) *apiv3.AnchorReceiptRecord {
		r, err := svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{
			Partition: first.Receipt.Partition, BptRoot: bptRoot, AtOrAfter: atOrAfter,
		})
		require.NoError(t, err)
		return r
	}

	var oldest *apiv3.AnchorReceiptRecord
	for i := 0; i < 60 && (oldest == nil || !oldest.Anchored); i++ {
		sim.Step()
		oldest = ask(0)
	}
	require.True(t, oldest.Anchored)
	t.Logf("default: terminates at DN block %d", oldest.DirectoryBlock)

	// Move the chain well past it
	for i := 0; i < 20; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				SendTokens(1, 0).To(other).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 2)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	// Default is unchanged — that is the stability the contract promises
	again := ask(0)
	require.True(t, again.Anchored)
	require.Equal(t, oldest.DirectoryBlock, again.DirectoryBlock, "the default must not drift")
	require.Equal(t, oldest.Receipt.Anchor, again.Receipt.Anchor)

	// Asking for a later terminus gives one, and it still proves the same root
	later := ask(oldest.DirectoryBlock + 10)
	require.True(t, later.Anchored)
	require.NotNil(t, later.Receipt)
	require.GreaterOrEqual(t, later.DirectoryBlock, oldest.DirectoryBlock+10,
		"AtOrAfter must be honoured")

	// AtOrAfter names a DIRECTORY block. Asking for one far beyond any
	// partition block number is the check that it is not being applied to the
	// partition's own numbering, which is a different sequence that happens to
	// run close by in a simulator.
	far := ask(oldest.DirectoryBlock + 60)
	if far.Anchored {
		require.GreaterOrEqual(t, far.DirectoryBlock, oldest.DirectoryBlock+60,
			"AtOrAfter must be read against directory blocks, not the partition's")
		require.Equal(t, first.Receipt.Receipt.Anchor, far.Receipt.Start)
		j, err := first.Receipt.Receipt.Combine(far.Receipt)
		require.NoError(t, err)
		require.True(t, j.Validate(nil))
		t.Logf("AtOrAfter %d: terminates at DN block %d", oldest.DirectoryBlock+60, far.DirectoryBlock)
	}
	require.Equal(t, first.Receipt.Receipt.Anchor, later.Receipt.Start,
		"a later terminus still proves the same BPT root")

	joined, err := first.Receipt.Receipt.Combine(later.Receipt)
	require.NoError(t, err)
	require.True(t, joined.Validate(nil), "the later receipt must compose and validate too")
	t.Logf("AtOrAfter %d: terminates at DN block %d", oldest.DirectoryBlock+10, later.DirectoryBlock)
}

// nudge submits a transaction, which is all it takes to restart a network that
// has gone quiet. Any account update makes the block anchor, so the root that
// was captured last becomes a bpt chain entry and the anchor covering it goes.
func nudge(t *testing.T, sim *Sim, lite *url.URL, liteKey []byte, ts uint64) {
	t.Helper()
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("nudge")).RootIdentity().JoinPath(ACME)
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(ts).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
}

// The residual, stated as a test rather than left to be discovered. On a
// network that has genuinely stopped, the most recent root is not yet provable:
// the bpt chain records a block's root in the following block, and there is no
// following block. It is not permanent -- anything at all fixes it, which is
// what the transaction is for (#4276).
func TestTwoCallProof_QuiescentTailNeedsANudge(t *testing.T) {
	sim, lite, liteKey := twoCallSim(t)
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("tc-tail-other")).RootIdentity().JoinPath(ACME)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(20) // Let it settle to a stop

	first := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	var bptRoot [32]byte
	copy(bptRoot[:], first.Receipt.Receipt.Anchor)
	svc := proofServiceFor(sim, first.Receipt.Partition)
	ask := func() *apiv3.AnchorReceiptRecord {
		r, err := svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{
			Partition: first.Receipt.Partition, BptRoot: bptRoot})
		require.NoError(t, err)
		return r
	}

	// Stepping a stopped network changes nothing, however long you wait
	for i := 0; i < 100; i++ {
		sim.Step()
	}
	require.False(t, ask().Anchored, "the last root of a stopped network is not yet provable")

	// One transaction, and it is
	nudge(t, sim, lite, liteKey, 500)
	var rec *apiv3.AnchorReceiptRecord
	for i := 0; i < 60 && (rec == nil || !rec.Anchored); i++ {
		sim.Step()
		rec = ask()
	}
	require.True(t, rec.Anchored, "a nudge must make the tail provable")
	joined, err := first.Receipt.Receipt.Combine(rec.Receipt)
	require.NoError(t, err)
	require.True(t, joined.Validate(nil))
	t.Logf("tail bound after a nudge, at DN block %d", rec.DirectoryBlock)
}

// Below Kourou the bpt chain is written but never anchored, so there is no
// answer to wait for. Reporting "not anchored yet" there would be the same
// conflation the two-call design exists to remove: a capability limit dressed
// up as a temporary state (#4276). It must refuse, and name the gate.
func TestTwoCallProof_RefusesBelowTheGate(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Jiuquan),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(20)

	first := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	require.NotNil(t, first.Receipt)
	var bptRoot [32]byte
	copy(bptRoot[:], first.Receipt.Receipt.Anchor)

	// The chain is there; the index chain is not, because nothing anchors it
	View(t, sim.Database(first.Receipt.Partition), func(batch *database.Batch) {
		c := batch.Account(PartitionUrl(first.Receipt.Partition).JoinPath(Ledger)).BptChain()
		head, err := c.Inner().Head().Get()
		require.NoError(t, err)
		require.NotZero(t, head.Count, "the bpt chain is written below the gate")
		idx, err := c.Index().Head().Get()
		require.NoError(t, err)
		require.Zero(t, idx.Count, "and not anchored below the gate")
	})

	svc := proofServiceFor(sim, first.Receipt.Partition)
	rec, err := svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{
		Partition: first.Receipt.Partition, BptRoot: bptRoot})
	require.Error(t, err, "it must refuse, not report a wait that never ends")
	require.Nil(t, rec)
	require.Contains(t, err.Error(), "Kourou", "the refusal must name what is missing")
	t.Logf("below the gate: %v", err)
}
