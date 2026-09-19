// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// globalsWith builds the network definition a node reads its committee from:
// each key active on the partitions named for it.
func globalsWith(t *testing.T, active map[string][]ed25519.PublicKey) *network.GlobalValues {
	t.Helper()
	g := new(network.GlobalValues)
	g.Network = new(protocol.NetworkDefinition)
	// AddValidator, so the fixture is shaped exactly as the definition on
	// chain is: sorted by key hash, with PublicKeyHash set.
	for part, keys := range active {
		for _, k := range keys {
			g.Network.AddValidator(k, part, true)
		}
	}
	return g
}

func otherKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub
}

// TestSubmitter_ANodeInNoCommitteeRefuses — #4366, executor.md Sync step 5, "A
// node does not take what it cannot propose".
//
// A submission's only road to a block is the receiving node's own batch and
// its own header, and a header whose author is in no committee is dropped
// before any vote (pkg/consensus/primary/vote_handler.go:277-284). So a node
// whose author key is not in the partition's current committee answers
// NotReady to Submit and to Validate — whatever its node state, and with no
// join anywhere in the picture.
func TestSubmitter_ANodeInNoCommitteeRefuses(t *testing.T) {
	svc, _, mine := newJoiningService(t)
	ctx := context.Background()
	env := new(messaging.Envelope)

	// The committee of bvn1 is somebody else. This node has no join state at
	// all — it is a from-genesis follower, not a joining node (#4368).
	m := NewMembership("bvn1", mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{
		"bvn1": {otherKey(t), otherKey(t), otherKey(t), otherKey(t)},
	}))
	require.False(t, m.InCommittee())

	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m})
	val := NewValidatorService(ValidatorServiceParams{Service: svc, Membership: m})

	_, err := sub.Submit(ctx, env, api.SubmitOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "Submit: got %v", err)
	require.Contains(t, err.Error(), "bvn1", "the refusal names the partition")
	require.Contains(t, err.Error(), "committee", "the refusal says why (consensus.md invariant 10)")

	_, err = val.Validate(ctx, env, api.ValidateOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "Validate: got %v", err)
	require.Contains(t, err.Error(), "committee")
}

// TestSubmitter_AnActiveValidatorIsUnchanged — the gate is membership, and a
// node whose key is active takes traffic exactly as before.
func TestSubmitter_AnActiveValidatorIsUnchanged(t *testing.T) {
	svc, _, mine := newJoiningService(t)
	ctx := context.Background()
	env := new(messaging.Envelope)

	m := NewMembership("bvn1", mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{
		"bvn1": {mine, otherKey(t), otherKey(t), otherKey(t)},
	}))
	require.True(t, m.InCommittee())

	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m})
	val := NewValidatorService(ValidatorServiceParams{Service: svc, Membership: m})

	_, err := sub.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		require.NotContains(t, err.Error(), "committee", "an active validator was refused for membership")
	}
	_, err = val.Validate(ctx, env, api.ValidateOptions{})
	if err != nil {
		require.NotContains(t, err.Error(), "committee")
	}
}

// TestSubmitter_TheTwoRefusalsCompose — a joining node that is also in no
// committee refuses for both reasons, and the reason it gives is the one that
// will still be true when the join finishes.
func TestSubmitter_TheTwoRefusalsCompose(t *testing.T) {
	svc, _, mine := newJoiningService(t)
	ctx := context.Background()
	env := new(messaging.Envelope)

	machine := nodestate.New(protocol.PartitionUrl("bvn1"))
	m := NewMembership("bvn1", mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{
		"bvn1": {otherKey(t)},
	}))

	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, NodeState: machine, Membership: m})
	_, err := sub.Submit(ctx, env, api.SubmitOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady))
	require.Contains(t, err.Error(), "committee",
		"a node that will still refuse when its join finishes says so")

	// The same node, once it is in the committee, refuses for joining alone.
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{
		"bvn1": {mine},
	}))
	_, err = sub.Submit(ctx, env, api.SubmitOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady))
	require.Contains(t, err.Error(), "is joining")
}

// TestMembership_ReadsTheCurrentCommittee — the predicate is a read of the
// globals the node already holds, kept current the way the executor bridge
// keeps its validator set current (pkg/consensus/adapter/executor_bridge.go:117-140).
// It is a read: nothing here changes a committee on chain, which is phase 2.
func TestMembership_ReadsTheCurrentCommittee(t *testing.T) {
	mine := otherKey(t)
	bus := events.NewBus(nil)
	m := NewMembership("bvn3", mine)
	m.SubscribeGlobals(bus)

	// Before any globals arrive the committee is unknown, and an unknown
	// committee is not a refusal: refusing would take every validator of a
	// starting network off the air on a startup race.
	require.True(t, m.InCommittee(), "unknown committee must not refuse")

	require.NoError(t, bus.Publish(events.WillChangeGlobals{
		New: globalsWith(t, map[string][]ed25519.PublicKey{
			"bvn3":      {otherKey(t), otherKey(t)},
			"Directory": {mine},
		}),
	}))
	require.False(t, m.InCommittee(), "our key is active on Directory, not on bvn3")

	// Partition IDs are compared case-insensitively, as IsActiveOn does.
	dn := NewMembership("directory", mine)
	dn.SubscribeGlobals(bus)
	require.NoError(t, bus.Publish(events.WillChangeGlobals{
		New: globalsWith(t, map[string][]ed25519.PublicKey{
			"Directory": {mine},
		}),
	}))
	require.True(t, dn.InCommittee())
}

// TestSubmitter_NoMembershipMeansNoGate — every existing caller that does not
// supply a membership behaves as it did.
func TestSubmitter_NoMembershipMeansNoGate(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	sub := NewSubmitterService(SubmitterServiceParams{Service: svc})
	_, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	if err != nil {
		require.NotContains(t, err.Error(), "committee")
	}
}

// TestFromGenesisFollower_AnswersReadsAndRefusesWrites — the composed #4368
// case. A node that never joined has no join state (cmd/accumulated/run/dagbft.go:620-626),
// so every state-based gate passes; its key is in no committee. It answers
// reads and refuses Submit and Validate. Whichever way #4368 resolves the
// state a from-genesis node reports, this is the answer its services give.
func TestFromGenesisFollower_AnswersReadsAndRefusesWrites(t *testing.T) {
	svc, _, mine := newJoiningService(t)
	ctx := context.Background()

	m := NewMembership("bvn1", mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{
		"bvn1": {otherKey(t), otherKey(t), otherKey(t), otherKey(t)},
	}))

	// A read: answered. Nothing about membership gates it.
	cons := NewConsensusAPIService(ConsensusAPIServiceParams{
		Service:     svc,
		PartitionID: "bvn1",
	})
	inc := false
	st, err := cons.ConsensusStatus(ctx, api.ConsensusStatusOptions{IncludeAccumulate: &inc})
	require.NoError(t, err, "a follower must answer reads")
	require.True(t, st.Ok)
	require.Equal(t, "bvn1", st.PartitionID)

	// The writes: refused, with no node state anywhere in the picture.
	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m})
	val := NewValidatorService(ValidatorServiceParams{Service: svc, Membership: m})
	require.Nil(t, sub.nodeState, "this node never joined")
	require.Nil(t, val.nodeState)

	_, err = sub.Submit(ctx, new(messaging.Envelope), api.SubmitOptions{})
	require.True(t, errors.Is(err, errors.NotReady), "Submit: got %v", err)
	_, err = val.Validate(ctx, new(messaging.Envelope), api.ValidateOptions{})
	require.True(t, errors.Is(err, errors.NotReady), "Validate: got %v", err)
}
