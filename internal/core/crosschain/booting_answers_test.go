// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// verifiedSigners is the distinct committee members whose signature on the
// answered anchor verifies against the sequenced form they signed, as the
// collector counts them (anchorsrc.verify).
func verifiedSigners(r *api.MessageRecord[messaging.Message], committee func([]byte) bool) map[string]bool {
	signed := map[string]bool{}
	for _, set := range r.Signatures.Records {
		for _, m := range set.Signatures.Records {
			sm, ok := m.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			ks, ok := sm.Signature.(protocol.KeySignature)
			if !ok || !committee(ks.GetPublicKey()) || !ks.Verify(nil, r.Sequence) {
				continue
			}
			signed[string(ks.GetPublicKey())] = true
		}
	}
	return signed
}

// Two of four validators booting at once still let every node collect a
// quorum for its partition's anchor, and match its own root to it (executor
// spec, "Sync", §6, "Two things do not wait for ACTIVE"; #4443).
//
// Before #4443 the sequencer answered only while ACTIVE, so two BOOTING
// validators of four left two answers where three are the threshold: no node
// -- booting or not -- could prove its root, and nobody ever matched. The
// collector asks each validator's sequencer for anchor n by number; so does
// the Directory's heal requester, which is run here too, through the
// production routing (Peers.FindService, then each peer).
//
// Done by hand: the two nodes are stood in a fresh BOOTING machine while they
// keep executing (SetNodeMachine), which is where a join that hands off
// unproven leaves a node. A restarted simulator node collects rather than
// executes, so RestartNode cannot stand it there.
func TestTwoOfFourBootingStillLetEveryNodeCollectAndMatch(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 4),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	helpers.MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	helpers.CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	helpers.MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
	helpers.CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	helpers.MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	helpers.MakeAccount(t, sim.DatabaseFor(bob), &protocol.TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})

	bvn := sim.S.Partition("BVN0")
	require.Equal(t, 4, bvn.NodeCount())
	globals := bvn.NodeConductor(0).Globals.Load()
	threshold := globals.ValidatorThreshold("BVN0")
	require.Equal(t, uint64(3), threshold, "premise: three of four sign a BVN0 anchor")
	committee := func(pub []byte) bool {
		_, info, ok := globals.Network.ValidatorByKey(pub)
		return ok && info.IsActiveOn("BVN0")
	}

	// Two validators boot at once, and keep executing: handed off, not yet
	// matched.
	const booting1, booting2 = 2, 3
	for _, i := range []int{booting1, booting2} {
		bvn.SetNodeMachine(i, nodestate.New(protocol.PartitionUrl("BVN0")))
	}

	for i := uint64(1); i <= 10; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
		sim.StepN(2)
	}
	n, ok := bvn.SynthCache().LastAnchorNumber()
	require.True(t, ok, "BVN0 produced no anchor")
	sim.StepN(16) // out of every sequencer's in-flight window

	ctx := context.Background()
	src := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	for collector := 0; collector < bvn.NodeCount(); collector++ {
		// What the collector on this node gathers: every other validator's
		// answer for anchor n, each with that validator's signature.
		var anchor *api.MessageRecord[messaging.Message]
		signed := map[string]bool{}
		for v := 0; v < bvn.NodeCount(); v++ {
			if v == collector {
				continue
			}
			r, err := bvn.NodePrivate(v).Sequence(ctx, src, protocol.DnUrl(), n, private.SequenceOptions{})
			if err != nil {
				t.Logf("collector %d: validator %d refused anchor %d: %v", collector, v, n, err)
				continue
			}
			if anchor == nil {
				anchor = r
			}
			require.Equal(t, anchor.Sequence.Hash(), r.Sequence.Hash(), "two validators answered different anchors for %d", n)
			for k := range verifiedSigners(r, committee) {
				signed[k] = true
			}
		}
		require.GreaterOrEqualf(t, uint64(len(signed)), threshold,
			"node %d collected %d verified signatures on BVN0's anchor %d; %d are the threshold",
			collector, len(signed), n, threshold)

		// And it matches: the quorum's root is the root this node's own
		// execution recorded under that number.
		body := anchor.Message.(*messaging.TransactionMessage).Transaction.Body.(protocol.AnchorBody).GetPartitionAnchor()
		helpers.View(t, bvn.NodeDatabase(collector), func(batch *database.Batch) {
			chain, err := batch.Account(src).AnchorSequenceChain().Get()
			require.NoError(t, err)
			hash, err := chain.Entry(int64(n) - 1)
			require.NoError(t, err)
			var own *messaging.TransactionMessage
			require.NoError(t, batch.Message2(hash).Main().GetAs(&own))
			require.Equal(t, body.StateTreeAnchor, own.Transaction.Body.(protocol.AnchorBody).GetPartitionAnchor().StateTreeAnchor,
				"node %d: the collected root for block %d is not its own", collector, body.MinorBlockIndex)
		})
	}

	// The Directory's heal requester assembles a quorum too: its own routed
	// client first, then each of BVN0's sequencers FindService names.
	dn := sim.S.Partition(protocol.Directory)
	ranger, ok := dn.NodeConductor(0).Sequencer.(private.SequenceRanger)
	require.True(t, ok)
	var envs []*messaging.Envelope
	_, _, err := dn.NodeConductor(0).HealAnchorSpan(ctx, ranger, protocol.PartitionUrl("BVN0"), n, n, func(env *messaging.Envelope) {
		envs = append(envs, env)
	})
	require.NoError(t, err)
	require.Len(t, envs, 1)
	healers := map[string]bool{}
	for _, m := range envs[0].Messages {
		if a, ok := m.(*messaging.BlockAnchor); ok && committee(a.Signature.GetPublicKey()) && a.Signature.Verify(nil, a.Anchor) {
			healers[string(a.Signature.GetPublicKey())] = true
		}
	}
	require.GreaterOrEqual(t, uint64(len(healers)), threshold,
		"the Directory's heal of BVN0's anchor %d carries %d verified signatures", n, len(healers))
}
