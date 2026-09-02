// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The cadence is a function of the block index, so every node activates on the
// same blocks. That is what makes "two of us send" mean two NODES rather than
// two per-node timers that happen to overlap — which is what the jitter and
// back-off it replaces were trying to approximate.
func TestHealActivates_IsTheSameOnEveryNode(t *testing.T) {
	var on, off int
	for i := uint64(0); i < 1000; i++ {
		if healActivates(i) {
			on++
		} else {
			off++
		}
	}
	assert.Equal(t, 1000/healCadence, on, "one activation every healCadence blocks")
	assert.Greater(t, off, on, "and not every block: a request's answer takes blocks to come back")
}

// healers builds N conductors sharing one validator set, as N validators of a
// partition would.
func healers(t *testing.T, n int) []*Conductor {
	t.Helper()

	var validators []*protocol.ValidatorInfo
	var keys []ed25519.PrivateKey
	for i := 0; i < n; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys = append(keys, priv)
		validators = append(validators, &protocol.ValidatorInfo{
			PublicKey:  pub,
			Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}},
		})
	}

	globals := &core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionV2Kourou,
		Network:         &protocol.NetworkDefinition{Validators: validators},
	}

	var out []*Conductor
	for i := 0; i < n; i++ {
		c := &Conductor{
			Partition:    &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
			ValidatorKey: keys[i],
		}
		c.Globals.Store(globals)
		out = append(out, c)
	}
	return out
}

// Exactly two validators send, and every node reaches that conclusion from the
// same agreed input without asking anyone.
func TestSelectedToSend_ExactlyTwo(t *testing.T) {
	cs := healers(t, 7)
	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	selected := 0
	for _, c := range cs {
		ok, err := c.selectedToSend(batch)
		require.NoError(t, err)
		if ok {
			selected++
		}
	}
	assert.Equal(t, sendersPerActivation, selected,
		"two rather than one, which is a single point of failure; two rather than N, which is N-2 wasted round trips")
}

// The pair rotates, because the hash it is drawn from changes every block. A
// validator that cannot reach a source stops being asked at the next
// activation, and the load spreads instead of settling on whoever was picked
// first.
func TestSelectedToSend_RotatesWithTheBlockHash(t *testing.T) {
	cs := healers(t, 7)

	// Distinct states give distinct hashes, which is all the selection needs.
	seen := map[int]bool{}
	for i := 0; i < 40; i++ {
		db := database.OpenInMemory(nil)
		batch := db.Begin(true)
		require.NoError(t, batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).
			Main().Put(&protocol.SystemLedger{Url: protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger), Index: uint64(i)}))
		require.NoError(t, batch.UpdateBPT())

		for j, c := range cs {
			ok, err := c.selectedToSend(batch)
			require.NoError(t, err)
			if ok {
				seen[j] = true
			}
		}
		batch.Discard()
	}

	assert.Greater(t, len(seen), 2,
		"the pair must rotate — a fixed pair means a stream waits on nodes that are not going to answer")
}

// Every node must agree on WHO was selected, or the pair is not a pair.
func TestSelectedToSend_EveryNodeAgrees(t *testing.T) {
	cs := healers(t, 5)
	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	// Ask each conductor about itself, twice: the answer is a function of state
	// alone, so it cannot depend on when it is asked.
	for _, c := range cs {
		first, err := c.selectedToSend(batch)
		require.NoError(t, err)
		second, err := c.selectedToSend(batch)
		require.NoError(t, err)
		assert.Equal(t, first, second, "selection is a function of agreed state, not of when it is asked")
	}
}

// A network too small to choose between has everyone send. Selecting one of two
// would leave a single point of failure, which is the thing two senders exist
// to avoid.
func TestSelectedToSend_SmallNetworksAllSend(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	for n := 1; n <= sendersPerActivation; n++ {
		for _, c := range healers(t, n) {
			ok, err := c.selectedToSend(batch)
			require.NoError(t, err)
			assert.Truef(t, ok, "with %d validators every one of them sends", n)
		}
	}
}

// Healing is a validator's job: the answer re-enters through consensus, so a
// node that cannot participate has nothing to contribute by asking.
func TestSelectedToSend_NonValidatorNeverSends(t *testing.T) {
	cs := healers(t, 4)
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	outsider := &Conductor{
		Partition:    cs[0].Partition,
		ValidatorKey: priv,
	}
	outsider.Globals.Store(cs[0].Globals.Load())

	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	ok, err := outsider.selectedToSend(batch)
	require.NoError(t, err)
	assert.False(t, ok)
}

// Selection applies to a pull, never to a push — the distinction that
// TestDropInitialAnchor found the hard way.
//
// A request is fungible: whoever asks, the answer returns through consensus and
// heals every validator at once, so the other N−2 askers are pure load. A
// signature is not: only validator N can produce validator N's signature, so
// selecting a pair there does not save duplicate work, it withholds the rest of
// the quorum, and a destination that lost an anchor waits for signatures that
// are never coming.
//
// This pins the arithmetic that makes that fatal: a pair is below the threshold
// as soon as a partition has more than three validators.
func TestSelection_WouldStarveAnAnchorQuorum(t *testing.T) {
	for _, n := range []int{4, 6, 10, 16} {
		threshold := n*2/3 + 1 // what an anchor needs
		require.Greater(t, threshold, sendersPerActivation,
			"with %d validators an anchor needs %d signatures; %d senders can never reach it, "+
				"which is why the anchor push is not selected", n, threshold, sendersPerActivation)
	}
}
