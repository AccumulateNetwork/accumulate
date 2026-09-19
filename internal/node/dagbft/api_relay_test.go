// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// counted reads the three families the harness joins, for one partition.
func counted(part string) (accepted, rejected, taken, notReady, unreachable float64) {
	return testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "accepted")),
		testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "rejected")),
		testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, metrics.RelayTaken)),
		testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, metrics.RelayNotReady)),
		testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, metrics.RelayUnreachable))
}

// TestSubmitter_ANodeInNoCommitteeRelays — #4366, and the reversal of the
// refusal that was built first. Paul, 2026-09-19: "Followers can relay txs.
// And should."
//
// A node whose author key is in no committee of the partition cannot get a
// submission into a block by proposing it — its header is dropped before any
// vote — so it hands it to a node that can, and answers its caller with that
// node's answer. It does not refuse, and it does not keep a copy: relay or
// propose, never both.
func TestSubmitter_ANodeInNoCommitteeRelays(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	val := otherKey(t)
	target := peer.ID("a-validator")

	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {val, otherKey(t)}}))
	require.False(t, m.CanPropose(), "this node is in no committee")

	rpc := &fakeRPC{keys: map[peer.ID]ed25519.PublicKey{target: val}}
	sub := NewSubmitterService(SubmitterServiceParams{
		Service:    svc,
		Membership: m,
		Relay: NewRelay(RelayParams{Partition: part, Membership: m,
			Peers: &fakePeers{self: "self", peers: []peer.ID{"self", target}}, RPC: rpc}),
	})

	a0, r0, t0, _, _ := counted(part)
	res, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err, "a node that cannot propose must not refuse")
	require.True(t, res[0].Success)
	require.Equal(t, []peer.ID{target}, rpc.submitted)

	a1, r1, t1, _, _ := counted(part)
	require.Equal(t, float64(1), a1-a0, "accepted is Submit returning success TO THE CALLER")
	require.Equal(t, float64(0), r1-r0)
	require.Equal(t, float64(1), t1-t0, "one relay, counted once, at its final answer")
}

// TestSubmitter_ASyncingNodeRelays — the half of the rule that is not about
// committees at all (Paul, 2026-09-19: "relaying txs if following or
// syncing"). A node whose key IS in the committee but which has not caught
// up cannot propose either: it would validate against a store its pull has
// half filled. A relay reads no account and verifies no signature, so the
// sync rule does not reach it (executor.md, "Sync" step 6).
func TestSubmitter_ASyncingNodeRelays(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	val := otherKey(t)
	target := peer.ID("a-validator")

	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {mine, val}}))
	require.True(t, m.CanPropose(), "its key IS in the committee")

	joining := nodestate.New(protocol.PartitionUrl(part))
	require.False(t, joining.CanServeCurrent(), "and it is still joining")

	rpc := &fakeRPC{keys: map[peer.ID]ed25519.PublicKey{target: val}}
	sub := NewSubmitterService(SubmitterServiceParams{
		Service:    svc,
		NodeState:  joining,
		Membership: m,
		Relay: NewRelay(RelayParams{Partition: part, Membership: m,
			Peers: &fakePeers{self: "self", peers: []peer.ID{target}}, RPC: rpc}),
	})

	res, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err, "a syncing node relays; it does not answer NotReady to a transaction")
	require.True(t, res[0].Success)
	require.Equal(t, []peer.ID{target}, rpc.submitted)
}

// TestSubmitter_TheRelayAnswerIsTheCallersAnswer — every outcome the harness
// counts, and what the caller is told for each. The submitter never invents
// an answer of its own.
func TestSubmitter_TheRelayAnswerIsTheCallersAnswer(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	val := otherKey(t)
	target := peer.ID("a-validator")

	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {val}}))

	cases := []struct {
		name    string
		answer  answer
		outcome string
		check   func(t *testing.T, res []*api.Submission, err error)
	}{
		{"refused", answer{res: []*api.Submission{{Success: false, Message: "no"}}},
			metrics.RelayRefused, func(t *testing.T, res []*api.Submission, err error) {
				require.NoError(t, err)
				require.False(t, res[0].Success)
				require.Equal(t, "no", res[0].Message, "the target's words, not ours")
			}},
		{"not-ready", answer{err: errors.NotReady.With("joining")},
			metrics.RelayNotReady, func(t *testing.T, _ []*api.Submission, err error) {
				require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
			}},
		{"unreachable", answer{err: errors.StreamAborted.With("reset")},
			metrics.RelayUnreachable, func(t *testing.T, _ []*api.Submission, err error) {
				require.Error(t, err)
			}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rpc := &fakeRPC{
				keys:    map[peer.ID]ed25519.PublicKey{target: val},
				answers: map[peer.ID]answer{target: c.answer},
			}
			sub := NewSubmitterService(SubmitterServiceParams{
				Service: svc, Membership: m,
				Relay: NewRelay(RelayParams{Partition: part, Membership: m,
					Peers: &fakePeers{self: "self", peers: []peer.ID{target}}, RPC: rpc}),
			})

			before := testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, c.outcome))
			a0, r0, _, _, _ := counted(part)
			res, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
			c.check(t, res, err)

			require.Equal(t, float64(1),
				testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, c.outcome))-before,
				"the outcome is counted once, under its own name")
			a1, r1, _, _, _ := counted(part)
			require.Equal(t, float64(1), a1-a0,
				"every relayed submission is one accepted, or sum(relayed) > accepted fires on a correct build")
			require.Equal(t, float64(0), r1-r0,
				"a relay is not a rejection: the outcome family says what became of it")
		})
	}
}

// TestSubmitter_AnActiveValidatorTakesItAndRelaysNothing — the path that
// carries the whole network is byte for byte what it was: a validator in the
// committee and not joining proposes, counts one accepted, and touches no
// relay counter at all. Relay or propose, never both (#4364).
func TestSubmitter_AnActiveValidatorTakesItAndRelaysNothing(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	target := peer.ID("somebody-else")

	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {mine, otherKey(t)}}))

	rpc := &fakeRPC{keys: map[peer.ID]ed25519.PublicKey{target: otherKey(t)}}
	sub := NewSubmitterService(SubmitterServiceParams{
		Service: svc, Membership: m,
		Relay: NewRelay(RelayParams{Partition: part, Membership: m,
			Peers: &fakePeers{self: "self", peers: []peer.ID{target}}, RPC: rpc}),
	})

	a0, _, t0, n0, u0 := counted(part)
	verify := false
	env := &messaging.Envelope{TxHash: []byte("0123456789abcdef0123456789abcdef")}
	// The consensus node of this fixture is not started, so the local path
	// ends at the worker with "node not started" -- which is the point: the
	// submission went to THIS node's worker and to no peer.
	_, err := sub.Submit(context.Background(), env, api.SubmitOptions{Verify: &verify})
	require.ErrorContains(t, err, "node not started",
		"an active validator's submission goes to its own worker, unchanged")

	a1, _, t1, n1, u1 := counted(part)
	require.Equal(t, float64(0), a1-a0)
	require.Equal(t, float64(0), t1-t0+n1-n0+u1-u0, "a validator relays nothing")
	require.Empty(t, rpc.submitted, "and hands nothing to a peer")
	require.Empty(t, rpc.probed)
}

// TestSubmitter_AnUnknownCommitteeStillProposes — the submit path's half of
// the three-valued answer (#4366, #4367).
//
// While the node holds no network definition the answer is CommitteeUnknown,
// and here that is not "you cannot propose": the daemon waits five seconds
// for globals and then carries on with an empty definition
// (cmd/accumulated/run/dagbft.go:383-390), so treating the race as "relay
// everything" would send every validator of a starting network hunting for a
// target it cannot name. The conductor decides the opposite for the same
// value, because it must not sign what it cannot justify.
func TestSubmitter_AnUnknownCommitteeStillProposes(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	m := NewMembership(part, mine) // no globals
	require.True(t, m.CanPropose())

	rpc := &fakeRPC{}
	sub := NewSubmitterService(SubmitterServiceParams{
		Service: svc, Membership: m,
		Relay: NewRelay(RelayParams{Partition: part, Membership: m,
			Peers: &fakePeers{self: "self"}, RPC: rpc}),
	})

	verify := false
	env := &messaging.Envelope{TxHash: []byte("fedcba9876543210fedcba9876543210")}
	_, err := sub.Submit(context.Background(), env, api.SubmitOptions{Verify: &verify})
	require.ErrorContains(t, err, "node not started", "it proposed rather than relayed")
	require.Empty(t, rpc.submitted)
}

// TestSubmitter_NoRelayIsARefusalNotADrop — a node that cannot propose and
// has nowhere to hand it to says so. The one thing it must never do is take
// it: what it takes for a partition it cannot propose for never reaches a
// block (the 1,249-of-1,249 reproduction, note_3869694995).
func TestSubmitter_NoRelayIsARefusalNotADrop(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {otherKey(t)}}))

	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m})
	_, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
}
