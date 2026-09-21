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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
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

// markBlock is the number of chain entries in one mark block: 1 << markPower,
// and markPower is 8 (internal/database/database.go). A chain restored with
// its OPEN MARK SET holds the entries after the last mark, so a chain longer
// than this has a beginning the restoring node does not have.
const markBlock = 256

// A JOINED NODE MUST NOT CALL ITS OWN GAP AN ABSENCE (#4361 F2).
//
// A node that pulled its state holds each non-spine chain with its open mark
// set only (`pull.go`, ModeStateOnly). For a main index chain longer than one
// mark block, element 0 is simply not in its store — and
// AccountFirstIndexedBlock reads element 0 to answer "did this account exist at
// that height".
//
// Before the fix that read came back as a store NotFound wrapped in
// UnknownError, and [errors.Code] walks a wrapping UnknownError down to its
// cause, so a client saw **NotFound** for what was really "I cannot read my own
// index". That is the one status a requester on this line reads as a fact about
// the RECORD rather than about the peer: `join/sources.go` makes a NotFound
// that every peer gives the network's answer, and on a network whose reachable
// peers have all joined that is every peer. The pull would conclude the account
// did not exist and drop one that all of them hold.
//
// Measured on this branch before the fix, through the production pull:
//
//	PULLED AccountFirstIndexedBlock => ok=false
//	  err=load acc://alice/tokens main chain index entry 0: cannot locate element 0
//	  code=notFound
//
// The store here is built by `pull.Fetch` + `Pending.Keep` — the production
// restore, not a chain with a hole punched in it by hand. What the test does
// by hand is stand in for the joining node's loop: choosing the account and
// calling Fetch. The restore itself, which is what produces the gap, is
// production code.
func TestAJoinedNodeDoesNotCallItsOwnGapAnAbsence(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	tokens := alice.JoinPath("tokens")

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: tokens, TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), tokens, big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	sim.StepN(10)

	// The account has to change in more blocks than one mark block holds, or
	// the pull restores the whole index chain and there is no gap to mistake
	// for an absence. This is the precondition, not the claim.
	for i := 0; i < markBlock+44; i++ {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(uint64(i + 1)).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(5)

	var servedFirst uint64
	View(t, sim.DatabaseFor(alice), func(b *database.Batch) {
		mi, err := b.Account(tokens).MainChain().Index().Get()
		require.NoError(t, err)
		require.Greaterf(t, mi.Height(), int64(markBlock),
			"the account's main index chain is %d entries; the gap only appears past %d", mi.Height(), markBlock)

		// A node that executed everything can tell, and does.
		block, ok, err := indexing.AccountFirstIndexedBlock(b.Account(tokens))
		require.NoError(t, err)
		require.True(t, ok, "a node that executed every block should know when the account first appeared")
		servedFirst = block
	})

	// Now stand the joined node: the production restore, into an empty store.
	src := pull.Source(api.Querier2{Querier: sim.S.Services()})
	joined := emptyDb()
	batch := joined.Begin(true)
	defer batch.Discard()
	p, err := pull.Fetch(context.Background(), src, batch, tokens,
		pull.Options{Mode: pull.ModeStateOnly, Partition: PartitionUrl("BVN0")}, true)
	require.NoError(t, err)
	require.NoError(t, p.Keep())

	// The precondition again, on the joined side: the beginning of the index
	// chain really is missing, so the test is exercising the case.
	mi, err := batch.Account(tokens).MainChain().Index().Get()
	require.NoError(t, err)
	require.Greater(t, mi.Height(), int64(markBlock))
	_, err = mi.Entry(0)
	require.Error(t, err, "the joined node holds element 0; the open-mark-set restore did not leave a gap")
	require.Equal(t, errors.NotFound, errors.Code(err),
		"the store's own answer for the missing element should be NotFound — that is what must not escape")

	// THE CLAIM. The joined node says "I cannot tell", and says it as a
	// non-answer rather than as a status a requester would believe.
	block, ok, err := indexing.AccountFirstIndexedBlock(batch.Account(tokens))
	require.NoError(t, err, "a joined node turned its own gap into an error")
	require.NotEqualf(t, errors.NotFound, errors.Code(err),
		"a joined node answered NotFound for %v, which it holds; every peer having joined makes that the network's answer", tokens)
	require.False(t, ok, "a joined node claimed to know when the account first appeared")
	require.Zero(t, block)

	t.Logf("executing node: first indexed block %d; joined node: cannot tell, and does not say NotFound", servedFirst)
}
