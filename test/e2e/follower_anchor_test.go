// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	nodeconfig "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// networkWithAFollower is the network gate 0 runs, in miniature: a partition
// of validators plus one node with a validator's wiring whose key is in the
// NetworkDefinition and active on nothing.
//
// The follower is made the way the soak makes `acc-bvn3-fol1` and the way an
// operator makes one — a NodeInit whose DnnType and BvnnType are Follower.
// BuildGenesisDocs turns exactly that into AddValidator(key, partition,
// false) (internal/node/daemon/init.go:213-214). Nothing here writes the
// definition by hand.
func networkWithAFollower(name string, bvns, validators int) (*accumulated.NetworkInit, ed25519.PrivateKey) {
	net := simulator.NewSimpleNetwork(name, bvns, validators)
	key := acctesting.GenerateKey(name, "follower")
	net.Bvns[0].Nodes = append(net.Bvns[0].Nodes, &accumulated.NodeInit{
		DnnType:    nodeconfig.Follower,
		BvnnType:   nodeconfig.Follower,
		PrivValKey: key,
		DnNodeKey:  acctesting.GenerateKey(name, "follower", "dn"),
		BvnNodeKey: acctesting.GenerateKey(name, "follower", "bvn"),
	})
	return net, key
}

// anchorSigners reports, per signing key hash, how many block anchors that
// key signed among the envelopes handed to it.
func anchorSigners(env *messaging.Envelope) [][32]byte {
	var signers [][32]byte
	msgs, err := env.Normalize()
	if err != nil {
		return nil
	}
	for _, m := range msgs {
		if a, ok := m.(*messaging.BlockAnchor); ok && a.Signature != nil {
			var h [32]byte
			copy(h[:], a.Signature.GetPublicKeyHash())
			signers = append(signers, h)
		}
	}
	return signers
}

// A node in no committee dispatches no anchor, and no peer is ever handed
// one it must refuse — through the production wiring: a real simulator
// network, the real conductor on every node, the real dispatcher, and a
// follower made from the definition rather than from a hand-built conductor.
//
// On run 20260919T191634Z the follower dispatched 1,997 anchors and every one
// was refused at msg_block_anchor.go:285, "key is not an active validator"
// (#4367). Both of its nodes did it: its Directory node to every partition,
// its BVN node to the Directory.
func TestAFollowerDispatchesNoAnchor(t *testing.T) {
	net, key := networkWithAFollower(t.Name(), 1, 3)
	followerHash := sha256.Sum256(key[32:])

	// Every envelope any node dispatches, and every anchor signature in it.
	var mu sync.Mutex
	signed := map[[32]byte]int{}
	capture := func(_ context.Context, env *messaging.Envelope) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		for _, h := range anchorSigners(env) {
			signed[h]++
		}
		return true, nil
	}

	// What the destination is asked to execute, which is the other end of
	// the same question: a refused anchor is one that got this far.
	refusable := map[[32]byte]int{}
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.CaptureDispatchedMessages(capture),
	)
	for _, id := range sim.S.Partitions() {
		sim.S.Partition(id.ID).SetSubmitHook(func(msgs []messaging.Message) (bool, bool) {
			mu.Lock()
			defer mu.Unlock()
			for _, m := range msgs {
				if a, ok := m.(*messaging.BlockAnchor); ok && a.Signature != nil {
					var h [32]byte
					copy(h[:], a.Signature.GetPublicKeyHash())
					refusable[h]++
				}
			}
			return false, true
		})
	}

	// The premise: the follower's key is in the definition and active on
	// nothing. Read from the network's own globals, because a test whose
	// follower is secretly a validator measures nothing (M5).
	def := sim.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory}).Network
	_, info, ok := def.ValidatorByKey(key[32:])
	require.True(t, ok, "the follower's key is not in the definition")
	for _, part := range def.Partitions {
		require.Falsef(t, info.IsActiveOn(part.ID),
			"the follower is active on %s — it is a validator, not a follower", part.ID)
	}

	sim.StepN(50)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, signed, "no node dispatched an anchor — this test measures nothing")
	require.Zero(t, signed[followerHash],
		"the follower dispatched %d anchors", signed[followerHash])
	require.Zero(t, refusable[followerHash],
		"a peer was handed %d anchors signed by a key it must refuse", refusable[followerHash])

	// And the validators are unaffected: the gate is membership, not a
	// switch that turns anchoring off.
	var others int
	for h, n := range signed {
		if !bytes.Equal(h[:], followerHash[:]) {
			others += n
		}
	}
	require.NotZero(t, others, "the validators stopped anchoring")
}

// A node in no committee still asks its sources for its own gaps, and the
// entries come back — through the production wiring: the real conductor's
// requester on the follower itself, the real private sequencer on the
// source, and the real dispatcher carrying the bundle back.
//
// The follower on run 20260919T191634Z asked for nothing at all, ever, while
// its four peers asked 351 times, because the pull is selected over the
// ACTIVE validator set and the selection cannot name a node outside it
// (requester.go, selectedToPull → cadence.go, partitionValidators). A hole on
// such a node is permanent and reads exactly like a calm stream.
func TestAFollowerAsksForItsOwnGaps(t *testing.T) {
	var timestamp uint64
	const validators = 3
	net, _ := networkWithAFollower(t.Name(), 2, validators)

	// Drop the first synthetic deposit, so the destination partition — the
	// one the follower is on — has a gap that only a request can fill.
	var dropped int
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.SkipProposalCheck(), // FIXME should not be necessary
		simulator.CaptureDispatchedMessages(func(_ context.Context, env *messaging.Envelope) (bool, error) {
			if dropped > 0 {
				return true, nil
			}
			msgs, err := env.Normalize()
			if err != nil {
				return false, err
			}
			for _, msg := range msgs {
			again:
				switch m := msg.(type) {
				case interface{ Unwrap() messaging.Message }:
					msg = m.Unwrap()
					goto again
				case messaging.MessageWithTransaction:
					if m.GetTransaction().Body.Type() == protocol.TransactionTypeSyntheticDepositTokens {
						dropped++
						return false, nil
					}
				}
			}
			return true, nil
		}),
	)

	// The follower is the last node of BVN0, and of the Directory, because
	// every node of a BVN runs a Directory node too (factory.go,
	// getNetworkFactories).
	const follower = validators
	require.Equal(t, validators+1, sim.S.Partition("BVN0").NodeCount())
	require.Zero(t, sim.S.Partition("BVN0").NodeHeals(follower).Requests.Load(),
		"nothing has happened yet")

	// Alice on BVN1 pays Bob on BVN0, so the synthetic that is lost is bound
	// for the partition the follower is on.
	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	sim.SetRoute(aliceUrl, "BVN1")
	sim.SetRoute(bobUrl, "BVN0")
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], protocol.AcmeUrl())

	var st []*protocol.TransactionStatus
	for i := 0; i < 4; i++ {
		st = append(st, sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice))))
		sim.StepN(2)
	}
	sim.StepUntil(True(func(*Harness) bool { return dropped > 0 }))

	// The hole closes, and the follower asked for it.
	for _, st := range st {
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}
	require.NotZero(t, sim.S.Partition("BVN0").NodeHeals(follower).Requests.Load(),
		"the follower never asked its sources for anything")
}
