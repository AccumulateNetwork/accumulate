// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// historicalFixture builds a BVN with an ADI on it, optionally retaining BPT
// history, and returns a querier for that partition.
//
// Every account anyone wants to prove lives on a BVN, so these tests run there
// and not on the directory.
type historicalFixture struct {
	sim     *Sim
	querier api.Querier2
	alice   *url.URL
	created uint64
}

func newHistoricalFixture(t *testing.T, depth uint64) *historicalFixture {
	t.Helper()
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "alice")
	keyHash := sha256.Sum256(aliceKey[32:])

	opts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	}
	if depth > 0 {
		opts = append(opts, simulator.BPTHistoryDepth(depth))
	}
	sim := NewSim(t, opts...)

	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(5)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			Body(&CreateIdentity{Url: alice, KeyHash: keyHash[:], KeyBookUrl: alice.JoinPath("book")}).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Move the chain on, so alice's creation becomes history
	for i := 0; i < 10; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 2)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	part, err := sim.Router().RouteAccount(alice)
	require.NoError(t, err)
	require.NotEqual(t, Directory, part, "alice should be on a BVN, not the directory")

	q := dut.NewQuerier(dut.QuerierParams{
		Logger:    acctesting.NewTestLogger(t),
		Database:  sim.Database(part),
		Partition: part,
	})
	return &historicalFixture{sim: sim, alice: alice, querier: api.Querier2{Querier: q}}
}

func (f *historicalFixture) query(t *testing.T, forHeight uint64) (*api.AccountRecord, error) {
	t.Helper()
	q := new(api.DefaultQuery)
	q.IncludeReceipt = &api.ReceiptOptions{ForAny: true, ForHeight: forHeight}
	r, err := f.querier.QueryAccount(context.Background(), f.alice, q)
	return r, err
}

// TestForHeight_ZeroIsUnchanged is the compatibility gate. Omitting ForHeight
// and passing it as zero must produce byte-identical records, and switching
// retention on must not perturb either.
//
// The first half is the one that tests this change directly: the historical
// branch is taken on ForHeight != 0, so a zero request must reach exactly the
// code it reached before.
func TestForHeight_ZeroIsUnchanged(t *testing.T) {
	f := newHistoricalFixture(t, 10_000)

	absent := new(api.DefaultQuery)
	absent.IncludeReceipt = &api.ReceiptOptions{ForAny: true}
	a, err := f.querier.QueryAccount(context.Background(), f.alice, absent)
	require.NoError(t, err)
	require.NotNil(t, a.Receipt, "a current-state receipt should still be produced")

	z, err := f.query(t, 0)
	require.NoError(t, err)

	ab, err := a.MarshalBinary()
	require.NoError(t, err)
	zb, err := z.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, ab, zb, "ForHeight=0 differs from omitting ForHeight")
	require.Zero(t, z.Receipt.ForHeight)
}

// Note on a test that is deliberately NOT here. "Enabling retention does not
// change a ForHeight==0 response" cannot be tested at this layer: two runs of
// the simulator do not produce the same BPT root even at the same retention
// depth, and simulator.Deterministic() does not change that — measured, twice.
// The claim is proven where it can be, against the BPT directly, by
// TestHistory_DepthZeroChangesNothing (the exported record set is byte-identical
// at depth 0) and TestHistory_RootUnchanged (roots agree block for block at
// depth 0 and depth 500).

// TestForHeight_RefusesWithoutRetention is the rule the whole AIP rests on: a
// node that cannot prove the past says so. It must never answer a historical
// query with a current-state receipt.
func TestForHeight_RefusesWithoutRetention(t *testing.T) {
	f := newHistoricalFixture(t, 0)

	current, err := f.query(t, 0)
	require.NoError(t, err)
	require.NotNil(t, current.Receipt)

	for _, h := range []uint64{1, 5, 10, 20} {
		r, err := f.query(t, h)
		require.Errorf(t, err, "height %d was answered", h)
		require.Nilf(t, r, "height %d returned a record", h)

		// And the refusal must be the one a client can branch on
		code := errors.Code(err)
		require.Containsf(t, []errors.Status{errors.IncompleteChain, errors.NotFound}, code,
			"height %d refused with %v: %v", h, code, err)
	}
}

// TestForHeight_ProvesThePast is the feature: with retention on, a historical
// request returns a receipt for the state at that block, echoing the resolved
// height, and it is not the current-state receipt.
func TestForHeight_ProvesThePast(t *testing.T) {
	f := newHistoricalFixture(t, 10_000)

	current, err := f.query(t, 0)
	require.NoError(t, err)
	require.NotNil(t, current.Receipt)
	require.Zero(t, current.Receipt.ForHeight, "a current-state receipt must not claim a height")

	answered, refused := 0, 0
	for h := uint64(1); h <= current.Receipt.LocalBlock; h++ {
		r, err := f.query(t, h)
		if err != nil {
			refused++
			continue
		}
		require.NotNilf(t, r.Receipt, "height %d returned no receipt", h)

		// The resolved height is echoed, and never exceeds what was asked
		require.NotZerof(t, r.Receipt.ForHeight, "height %d did not echo a resolved height", h)
		require.LessOrEqualf(t, r.Receipt.ForHeight, h, "height %d resolved forward", h)

		// It validates offline, and terminates where the current receipt does
		require.Truef(t, r.Receipt.Validate(nil), "height %d does not validate", h)
		require.Equalf(t, current.Receipt.Anchor, r.Receipt.Anchor,
			"height %d does not terminate at the current root", h)
		answered++
	}
	require.Greater(t, answered, 3, "only %d heights were answered", answered)
	t.Logf("answered %d heights, refused %d", answered, refused)
}

// TestForHeight_StartsAtThePastState pins what the historical receipt proves,
// and it is deliberately NOT the same thing the current-state receipt proves.
//
// The current path starts at the account's MAIN STATE hash: StateReceipt
// combines hasher.Receipt(0, len-1) with the BPT receipt, so it proves element 0
// of the account hash. The historical path cannot do that — reconstructing the
// hasher at a past block would need the account's main state at that block, and
// only BPT nodes are retained, not account state. So it starts at the account's
// full BPT entry, the merkle hash over main state, secondary state, chain
// anchors and pending.
//
// A client that assumed both starts meant the same thing would compare the wrong
// value and get a confident wrong answer, so the difference is asserted here
// rather than left to be discovered.
func TestForHeight_StartsAtThePastState(t *testing.T) {
	f := newHistoricalFixture(t, 10_000)

	current, err := f.query(t, 0)
	require.NoError(t, err)

	var past *api.Receipt
	for h := uint64(1); h <= current.Receipt.LocalBlock; h++ {
		r, err := f.query(t, h)
		if err == nil && r.Receipt != nil {
			past = r.Receipt
			break
		}
	}
	require.NotNil(t, past, "no height produced a receipt")

	require.NotEqual(t, current.Receipt.Start, past.Start,
		"the historical receipt starts at the account's BPT entry, not its main state hash")
	require.NotZero(t, past.ForHeight)
	require.Zero(t, current.Receipt.ForHeight)
	t.Logf("current start %x (main state), historical start %x (BPT entry) at height %d",
		current.Receipt.Start[:6], past.Start[:6], past.ForHeight)
}
