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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
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
