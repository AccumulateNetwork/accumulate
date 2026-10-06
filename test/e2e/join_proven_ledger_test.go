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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
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

// roundBendingSources serves the system ledger with a leader round that is
// not the one the block was committed at, and the true receipt, from every
// peer.
type roundBendingSources struct {
	join.Sources
	bend bool
}

func (b *roundBendingSources) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	srcs, partition, err := b.Sources.For(ctx, account)
	if err != nil || !b.bend {
		return srcs, partition, err
	}
	bent := make([]pull.Source, len(srcs))
	for i, s := range srcs {
		bent[i] = roundBendingSource{s}
	}
	return bent, partition, nil
}

func (b *roundBendingSources) ValidatorsOf(ctx context.Context, partition *url.URL) ([]anchorsrc.Validator, error) {
	return b.Sources.ValidatorsOf(ctx, partition)
}

type roundBendingSource struct{ pull.Source }

func (s roundBendingSource) QueryAccount(ctx context.Context, scope *url.URL, query *api.DefaultQuery) (*api.AccountRecord, error) {
	rec, err := s.Source.QueryAccount(ctx, scope, query)
	if err != nil || rec == nil {
		return rec, err
	}
	if ledger, ok := rec.Account.(*SystemLedger); ok {
		bent := ledger.Copy()
		bent.LeaderRound += 1000
		rec.Account = bent
	}
	return rec, nil
}

// A joining node proves its partition's system ledger as of the newest block
// a signed anchor has been collected for before it pulls anything: that
// ledger's leader round is what a follower's consensus is seeded at when its
// join starts (consensus spec, "Restart"; #4530). The proof is the production
// one end to end -- the anchors collected from the validators and checked
// against the node's own genesis keys, the ledger served as of that block by
// the validators' querier with its receipt (#4361) -- and it writes nothing.
//
// A peer that serves another round with the true receipt is refused: the
// round is a peer's word until the body hashes to the receipt's start.
func TestAJoiningNodeProvesTheLedgerAtTheNewestSignedAnchorBeforeItPulls(t *testing.T) {
	for _, bend := range []bool{false, true} {
		name := "every peer honest"
		if bend {
			name = "every peer serves another round"
		}
		t.Run(name, func(t *testing.T) { proveTheLedgerBeforePulling(t, bend) })
	}
}

func proveTheLedgerBeforePulling(t *testing.T, bend bool) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		// The daemon's default (cmd/accumulated/run/dagbft.go): a node
		// serves an account as of any of its last 1,024 blocks.
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	for ts := uint64(1); ts <= 20; ts++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.Step()
	}

	part := PartitionUrl("BVN0")
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources: &roundBendingSources{bend: bend, Sources: &join.QueryPeers{
			Client:  sim.S.Services(),
			Network: t.Name(),
			Router:  sim.S.Router(),
		}},
	})
	require.NoError(t, err)

	ledger, err := state.ProvenLedger(context.Background())
	if bend {
		require.Error(t, err, "a round no receipt proves was taken: %v", ledger)
		require.ErrorIs(t, err, errors.NotReady)
		return
	}
	require.NoError(t, err)

	// The block is a recent one of the partition's, and the ledger is that
	// block's.
	var now *SystemLedger
	View(t, sim.S.Database("BVN0"), func(batch *database.Batch) {
		require.NoError(t, batch.Account(part.JoinPath(Ledger)).Main().GetAs(&now))
	})
	require.NotZero(t, ledger.Index)
	require.LessOrEqual(t, ledger.Index, now.Index)
	require.Less(t, now.Index-ledger.Index, uint64(join.SeedWithin), "the newest signed anchor is not a recent block's")

	// Nothing was pulled: the node holds no ledger.
	View(t, local, func(batch *database.Batch) {
		_, err := batch.Account(part.JoinPath(Ledger)).Main().Get()
		require.ErrorIs(t, err, errors.NotFound, "proving the round wrote the ledger")
	})
	// A pull round asks again for the round between its steps, for a node
	// that could prove none when its join started.
	var asked int
	state.SeedBetweenSteps(func(context.Context) { asked++ })
	require.NoError(t, state.Pull(context.Background()))
	require.GreaterOrEqual(t, asked, 3, "the pull round did not ask between its steps")
}
