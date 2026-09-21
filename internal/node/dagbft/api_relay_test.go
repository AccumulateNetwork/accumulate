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
	"crypto/sha256"
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

	rpc := &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{target: val}}
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

	rpc := &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{target: val}}
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
				partition: part,
				keys:      map[peer.ID]ed25519.PublicKey{target: val},
				answers:   map[peer.ID]answer{target: c.answer},
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

	rpc := &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{target: otherKey(t)}}
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

// TestSubmitter_AnUnknownCommitteeIsNotALicenceToPropose — decision 4, for
// any node and not only a joining one (reviewer, 2026-09-19).
//
// A node that does not know whether it is a validator cannot know that the
// header carrying a submission will be voted on. The first build let it
// propose anyway, so a follower whose globals had not arrived took traffic
// and stranded it — #4366 again, through the one door left open. It now
// relays; and holding no committee it cannot name a target, so it answers
// NotReady, counts not-ready, and says so in the log once.
func TestSubmitter_AnUnknownCommitteeIsNotALicenceToPropose(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	m := NewMembership(part, mine) // no globals
	require.False(t, m.CanPropose())
	require.False(t, m.Known())

	rpc := &fakeRPC{partition: part}
	relay := NewRelay(RelayParams{Partition: part, Membership: m,
		Peers: &fakePeers{self: "self", peers: []peer.ID{"somebody"}}, RPC: rpc})
	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m, Relay: relay})

	a0, _, _, n0, _ := counted(part)
	no := false
	_, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{Verify: &no})
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Empty(t, rpc.submitted, "nothing is handed to a peer chosen by nothing")
	a1, _, _, n1, _ := counted(part)
	require.Equal(t, float64(1), n1-n0)
	require.Equal(t, float64(1), a1-a0)

	// And no Membership at all is still no gate, for every caller that
	// predates this.
	plain := NewSubmitterService(SubmitterServiceParams{Service: svc})
	_, err = plain.Submit(context.Background(),
		&messaging.Envelope{TxHash: []byte("0123456789abcdef0123456789abcdef")}, api.SubmitOptions{Verify: &no})
	require.ErrorContains(t, err, "node not started", "an ungated node proposes as it always did")
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

// TestSubmitter_ARelayDecodesButDoesNotValidate — the spec's "not validated,
// decoded only to route" and the relaying node's own admission, which are
// the same line read from two sides (executor.md "Sync" step 6; #4366 F3).
//
// Decoding is the node's own: Normalize reads no account and touches no
// store, so it is free even for a node whose pull has half filled one, and
// without it a follower turns garbage into that garbage at every validator
// of the partition. Validating is not: nothing here judges the envelope
// against state, and a caller that asks for no verification gets none —
// which is why an envelope Normalize rejects still relays under
// Verify:false.
func TestSubmitter_ARelayDecodesButDoesNotValidate(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	val := otherKey(t)
	target := peer.ID("a-validator")

	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {val}}))

	newSub := func(rpc *fakeRPC) *SubmitterService {
		return NewSubmitterService(SubmitterServiceParams{
			Service: svc, Membership: m,
			Relay: NewRelay(RelayParams{Partition: part, Membership: m,
				Peers: &fakePeers{self: "self", peers: []peer.ID{target}}, RPC: rpc}),
		})
	}

	// An envelope nothing can decode: a transaction hash that is not a
	// hash. Normalize rejects it, and it reads nothing to do so.
	junk := &messaging.Envelope{TxHash: []byte("short")}

	// The client's default is to verify, and the relay does not carry what
	// it could not decode.
	rpc := &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{target: val}}
	a0, r0, t0, _, _ := counted(part)
	_, err := newSub(rpc).Submit(context.Background(), junk, api.SubmitOptions{})
	require.True(t, errors.Is(err, errors.BadRequest), "got %v", err)
	require.Empty(t, rpc.submitted, "garbage must cost one node one decode, not the whole committee")
	a1, r1, t1, _, _ := counted(part)
	require.Equal(t, float64(0), a1-a0)
	require.Equal(t, float64(1), r1-r0, "refused here, and never relayed")
	require.Equal(t, float64(0), t1-t0)

	// And with verification off it is relayed unread: the relay is not a
	// validator, and the node that will propose it is the node that judges
	// it.
	no := false
	rpc2 := &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{target: val}}
	_, err = newSub(rpc2).Submit(context.Background(), junk, api.SubmitOptions{Verify: &no})
	require.NoError(t, err)
	require.Equal(t, []peer.ID{target}, rpc2.submitted)
}

// TestSubmitter_AValidatorsRefusalReachesTheCallerUnchanged — F4's half that
// is the node's: the harness subtracts relayed{refused} from the stranded
// figure only because a refusal is an ANSWER that went back to the caller
// (note_3869951312). If the relaying node rewrote it, the subtraction would
// be subtracting a drop.
func TestSubmitter_AValidatorsRefusalReachesTheCallerUnchanged(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	val := otherKey(t)
	target := peer.ID("a-validator")

	m := NewMembership(part, mine)
	m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {val}}))

	refusal := []*api.Submission{{
		Success: false,
		Message: "Transaction validation failed: insufficient credits",
		Status:  &protocol.TransactionStatus{Code: errors.InsufficientCredits},
	}}
	rpc := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{target: val},
		answers:   map[peer.ID]answer{target: {res: refusal}},
	}
	sub := NewSubmitterService(SubmitterServiceParams{
		Service: svc, Membership: m,
		Relay: NewRelay(RelayParams{Partition: part, Membership: m,
			Peers: &fakePeers{self: "self", peers: []peer.ID{target}}, RPC: rpc}),
	})

	no := false
	res, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{Verify: &no})
	require.NoError(t, err)
	require.Equal(t, refusal, res, "the validator's answer, byte for byte")
	require.Equal(t, "Transaction validation failed: insufficient credits", res[0].Message)
	require.Equal(t, errors.InsufficientCredits, res[0].Status.Code)
}

// TestSubmitter_AJoiningNodeWithNoCommitteeSaysNotReady — decision 4 where
// it is actually reachable.
//
// An unknown committee does not stop a node proposing (a startup race must
// not take a validator off the air), so the only node that reaches the relay
// holding no committee is one that is JOINING. It cannot name a node that
// can propose, so it says NotReady, counts not-ready, and says it in the log
// once.
func TestSubmitter_AJoiningNodeWithNoCommitteeSaysNotReady(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)

	m := NewMembership(part, mine) // no globals: no committee held
	joining := nodestate.New(protocol.PartitionUrl(part))
	relay := NewRelay(RelayParams{Partition: part, Membership: m,
		Peers: &fakePeers{self: "self", peers: []peer.ID{"somebody"}},
		RPC:   &fakeRPC{partition: part}})
	sub := NewSubmitterService(SubmitterServiceParams{
		Service: svc, NodeState: joining, Membership: m, Relay: relay,
	})

	a0, r0, _, n0, _ := counted(part)
	no := false
	for i := 0; i < 2; i++ {
		_, err := sub.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{Verify: &no})
		require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	}
	a1, r1, _, n1, _ := counted(part)
	require.Equal(t, float64(2), n1-n0, "counted not-ready, once per submission")
	require.Equal(t, float64(2), a1-a0, "and one accepted with each, so sum(relayed) <= accepted holds")
	require.Equal(t, float64(0), r1-r0)
	require.True(t, relay.saidBlind, "and it says so in the log once, not twice")
}

// TestValidator_ASyncedNodeInNoCommitteeAnswers — Validate is a READ, and
// reads divide from relays on whether local state is needed. A node that is
// synced answers it whatever its committee, because a validation judges
// against the latest committed state and promises nothing about proposal
// (executor.md, "Sync" step 6). The service takes no Membership at all, and
// this test exists to fail if one is ever added.
func TestValidator_ASyncedNodeInNoCommitteeAnswers(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	val := NewValidatorService(ValidatorServiceParams{Service: svc, NodeState: nodestate.Always{}})

	res, err := val.Validate(context.Background(), new(messaging.Envelope), api.ValidateOptions{})
	require.False(t, errors.Is(err, errors.NotReady),
		"a synced node must answer Validate whatever its committee: %v", err)
	if err == nil {
		require.NotEmpty(t, res)
	}
}

// TestConsensusStatus_ReportsCatchingUpFromTheJoinState — the source of the
// one fact that bounds a relay to a single hop.
func TestConsensusStatus_ReportsCatchingUpFromTheJoinState(t *testing.T) {
	const part = "bvn1"
	svc, _, mine := newJoiningService(t)
	machine := nodestate.New(protocol.PartitionUrl(part))

	cons := NewConsensusAPIService(ConsensusAPIServiceParams{
		Service: svc, PartitionID: part, NodeState: machine,
		ValidatorKeyHash: sha256.Sum256(mine),
	})
	inc := false
	opts := api.ConsensusStatusOptions{IncludeAccumulate: &inc, IncludePeers: &inc, Partition: part}

	st, err := cons.ConsensusStatus(context.Background(), opts)
	require.NoError(t, err)
	require.True(t, st.CatchingUp, "a BOOTING node is catching up, and is no relay's target")

	require.True(t, machine.PromoteToActive([32]byte{1}, 7))
	st, err = cons.ConsensusStatus(context.Background(), opts)
	require.NoError(t, err)
	require.False(t, st.CatchingUp, "and once it is ACTIVE it is a target again")

	// A node that never joined has no state machine and is not catching up.
	plain := NewConsensusAPIService(ConsensusAPIServiceParams{Service: svc, PartitionID: part})
	st, err = plain.ConsensusStatus(context.Background(), opts)
	require.NoError(t, err)
	require.False(t, st.CatchingUp)
}

// TestConsensusStatus_AnswersARelaysChallengeAsItselfAndForNobodyElse — the
// service side of F1 and of its forwarding half (note_3869991754).
//
// ConsensusStatus signs any caller's nonce, over the p2p consensus service
// and the public HTTP status alike. So a signature that says only "a
// validator holds this key" is obtainable by anyone: forward the relay's
// nonce to a real validator and hand back its answer. The signature is
// therefore over this node's OWN peer ID, and this node refuses to sign for
// any other.
func TestConsensusStatus_AnswersARelaysChallengeAsItselfAndForNobodyElse(t *testing.T) {
	const part = "bvn1"
	svc, _, _ := newJoiningService(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	const self, other = peer.ID("me"), peer.ID("somebody-else")

	cons := NewConsensusAPIService(ConsensusAPIServiceParams{
		Service: svc, PartitionID: part,
		ValidatorKeyHash: sha256.Sum256(pub), ValidatorKey: priv, PeerID: self,
	})
	inc := false
	nonce, err := newRelayChallenge()
	require.NoError(t, err)

	ask := func(id peer.ID, challenge []byte) *api.ConsensusStatus {
		t.Helper()
		st, err := cons.ConsensusStatus(context.Background(), api.ConsensusStatusOptions{
			IncludeAccumulate: &inc, IncludePeers: &inc, Partition: part,
			NodeID: id.String(), Challenge: challenge,
		})
		require.NoError(t, err)
		return st
	}

	st := ask(self, nonce)
	require.True(t, verifyRelayChallenge(pub, part, st.ValidatorKeyHash, self, nonce, st.ChallengeSignature),
		"the node must prove it holds the key whose hash it reports")
	require.False(t, verifyRelayChallenge(pub, part, st.ValidatorKeyHash, other, nonce, st.ChallengeSignature),
		"and the proof must not stand for another peer")

	// Asked to answer as somebody else — which is what a forwarding sink
	// does with the relay's nonce — it signs nothing.
	require.Empty(t, ask(other, nonce).ChallengeSignature,
		"a node must not mint an identity proof for another peer")
	require.Empty(t, ask("", nonce).ChallengeSignature)

	// No challenge, no signature: a node signs nothing it was not asked to.
	require.Empty(t, ask(self, nil).ChallengeSignature)

	// And not an arbitrarily long payload of the caller's choosing.
	require.Empty(t, ask(self, make([]byte, relayChallengeMaxNonce+1)).ChallengeSignature)
	require.NotEmpty(t, ask(self, make([]byte, relayChallengeMaxNonce)).ChallengeSignature)

	// A node with no key cannot answer, and is therefore no relay's target.
	none := NewConsensusAPIService(ConsensusAPIServiceParams{
		Service: svc, PartitionID: part, ValidatorKeyHash: sha256.Sum256(pub), PeerID: self,
	})
	st, err = none.ConsensusStatus(context.Background(), api.ConsensusStatusOptions{
		IncludeAccumulate: &inc, IncludePeers: &inc, Partition: part,
		NodeID: self.String(), Challenge: nonce,
	})
	require.NoError(t, err)
	require.Empty(t, st.ChallengeSignature)
}
