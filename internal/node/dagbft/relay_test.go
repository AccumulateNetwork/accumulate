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
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
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

// testPrivate remembers the private half of every key a test generates, so
// an honest fake candidate can answer the relay's challenge the way a real
// validator's ConsensusStatus does.
var testPrivate = map[string]ed25519.PrivateKey{}

func otherKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	testPrivate[string(pub)] = priv
	return pub
}

// fakePeers and fakeRPC stand in for the libp2p host and the network client.
// They are the network, not the relay: every decision under test — who is a
// candidate, who is confirmed, what an answer means, what the caller is told
// — is the production Relay's.
type fakePeers struct {
	self  peer.ID
	peers []peer.ID
}

func (f *fakePeers) Self() peer.ID { return f.self }
func (f *fakePeers) Providers(context.Context, *api.ServiceAddress) []peer.ID {
	return f.peers
}

type answer struct {
	res []*api.Submission
	err error
}

type fakeRPC struct {
	partition  string
	keys       map[peer.ID]ed25519.PublicKey // what each peer says it holds
	catchingUp map[peer.ID]bool
	keyErr     map[peer.ID]error
	answers    map[peer.ID]answer

	// cannotSign is the F1 peer: it names a real validator's key hash --
	// which is public -- and cannot answer the challenge, because it does
	// not hold the key.
	cannotSign map[peer.ID]bool

	// silent opens and never answers: the F2 peer.
	silent map[peer.ID]bool

	// forwards is the peer that holds no key and hands back an answer it
	// got from the validator whose hash it names: signed by the right key,
	// for the WRONG peer ID.
	forwards map[peer.ID]peer.ID

	probed    []peer.ID
	submitted []peer.ID
}

func (f *fakeRPC) part() string {
	if f.partition == "" {
		return "BVN3"
	}
	return f.partition
}

func (f *fakeRPC) Standing(ctx context.Context, p peer.ID, challenge []byte) ([32]byte, bool, []byte, error) {
	f.probed = append(f.probed, p)
	if f.silent[p] {
		<-ctx.Done()
		return [32]byte{}, false, nil, ctx.Err()
	}
	if err, ok := f.keyErr[p]; ok {
		return [32]byte{}, false, nil, err
	}
	k, ok := f.keys[p]
	if !ok {
		return [32]byte{}, false, nil, errors.NoPeer.With("no such peer")
	}
	var sig []byte
	switch {
	case f.cannotSign[p]:
		// names the key, cannot sign for it
	case f.forwards[p] != "":
		// forwarded the nonce to the real holder and hands its answer back
		sig = signRelayChallenge(testPrivate[string(k)], f.part(), f.forwards[p], f.forwards[p].String(), challenge)
	default:
		sig = signRelayChallenge(testPrivate[string(k)], f.part(), p, p.String(), challenge)
	}
	return sha256.Sum256(k), f.catchingUp[p], sig, nil
}

func (f *fakeRPC) Submit(ctx context.Context, p peer.ID, _ *messaging.Envelope, _ api.SubmitOptions) ([]*api.Submission, error) {
	f.submitted = append(f.submitted, p)
	if f.silent[p] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	a, ok := f.answers[p]
	if !ok {
		return []*api.Submission{{Success: true}}, nil
	}
	return a.res, a.err
}

func relayFor(t *testing.T, part string, mine ed25519.PublicKey, g *network.GlobalValues, peers *fakePeers, rpc *fakeRPC) *Relay {
	t.Helper()
	m := NewMembership(part, mine)
	m.SetGlobals(g)
	return NewRelay(RelayParams{Partition: part, Membership: m, Peers: peers, RPC: rpc})
}

// TestRelay_GoesToANodeThatCanPropose — the spec's three bounds, all three of
// them, in the one decision that produces them: the relay confirms the
// candidate's author key against the committee before it hands anything over
// (executor.md, "Sync" step 6, "A relay goes to a node that can propose,
// never to the relaying node itself, never to another node that would only
// relay it again, and never twice for one submission").
func TestRelay_GoesToANodeThatCanPropose(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	val := otherKey(t)
	fol := otherKey(t)

	self := peer.ID("self")
	valPeer := peer.ID("validator")
	folPeer := peer.ID("other-follower")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {val, otherKey(t), otherKey(t)}})
	rpc := &fakeRPC{keys: map[peer.ID]ed25519.PublicKey{
		self:    mine,
		valPeer: val,
		folPeer: fol,
	}}
	// Self is offered as a provider, as the local-first dial offers it.
	r := relayFor(t, part, mine, g, &fakePeers{self: self, peers: []peer.ID{self, folPeer, valPeer}}, rpc)

	res, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome)
	require.True(t, res[0].Success)

	require.Equal(t, []peer.ID{valPeer}, rpc.submitted,
		"the submission went to the validator and to nobody else")
	require.NotContains(t, rpc.probed, self, "the relaying node must never be its own target")
	require.NotContains(t, rpc.submitted, folPeer,
		"a node in no committee would only relay it again — that is the second hop")
}

// TestRelay_ANotReadyTargetIsARetryNotAnOutcome — a target that answers
// NotReady is a joining node, and the protocol's meaning of that is "ask
// someone else" (#4307). It is counted only if every target says it.
func TestRelay_ANotReadyTargetIsARetryNotAnOutcome(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("joining"), peer.ID("ready")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b}})
	rpc := &fakeRPC{
		keys: map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers: map[peer.ID]answer{
			pa: {err: errors.NotReady.With("joining")},
		},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome, "one taken, not a not-ready and a taken")
	require.Equal(t, []peer.ID{pa, pb}, rpc.submitted)

	// Every target joining: now it is an outcome, and it is a statement
	// about the network rather than about the submission.
	rpc2 := &fakeRPC{
		keys: map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers: map[peer.ID]answer{
			pa: {err: errors.NotReady.With("joining")},
			pb: {err: errors.NotReady.With("joining")},
		},
	}
	r2 := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc2)
	_, outcome, err = r2.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayNotReady, outcome)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Equal(t, []peer.ID{pa, pb}, rpc2.submitted, "each target once, and no more")
}

// TestRelay_ARefusalIsPassedBackUnchanged — a validator's refusal is the
// answer, and the relaying node neither re-judges it, queues it, nor shops it
// to the next validator. Retrying a refusal elsewhere is how one node's
// refusal would become the network's answer (identity laundering).
func TestRelay_ARefusalIsPassedBackUnchanged(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("first"), peer.ID("second")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b}})
	refusal := []*api.Submission{{Success: false, Message: "Transaction validation failed: nope"}}
	rpc := &fakeRPC{
		keys:    map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers: map[peer.ID]answer{pa: {res: refusal}},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc)

	res, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayRefused, outcome)
	require.Equal(t, refusal, res, "the target's answer, unchanged")
	require.Equal(t, []peer.ID{pa}, rpc.submitted, "a refusal is not shopped to the next validator")

	// An error-shaped refusal travels the same way.
	bad := errors.BadRequest.With("oversized")
	rpc2 := &fakeRPC{
		keys:    map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers: map[peer.ID]answer{pa: {err: bad}},
	}
	r2 := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc2)
	_, outcome, err = r2.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayRefused, outcome)
	require.True(t, errors.Is(err, errors.BadRequest), "got %v", err)
	require.Equal(t, []peer.ID{pa}, rpc2.submitted)
}

// TestRelay_UnreachableIsNotRefused — no validator answered at all, which is
// a statement about reachability and must not be filed as a refusal.
func TestRelay_UnreachableIsNotRefused(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a := otherKey(t)
	pa := peer.ID("gone")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a}})
	rpc := &fakeRPC{
		keys:    map[peer.ID]ed25519.PublicKey{pa: a},
		answers: map[peer.ID]answer{pa: {err: errors.StreamAborted.With("reset")}},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayUnreachable, outcome)
	require.Error(t, err)

	// And nobody at all is unreachable, not not-ready.
	r2 := relayFor(t, part, mine, g, &fakePeers{self: "self"}, &fakeRPC{})
	_, outcome, err = r2.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayUnreachable, outcome)
	require.Error(t, err)
}

// TestRelay_NoCommitteeHeldYetSaysSo — the lead's decision 4. A node that has
// no globals holds no committee, so it cannot name a node that can propose;
// it answers NotReady and counts not-ready rather than handing the
// submission to a peer chosen by nothing.
func TestRelay_NoCommitteeHeldYetSaysSo(t *testing.T) {
	mine := otherKey(t)
	rpc := &fakeRPC{keys: map[peer.ID]ed25519.PublicKey{"p": otherKey(t)}}
	r := NewRelay(RelayParams{
		Partition:  "BVN3",
		Membership: NewMembership("BVN3", mine), // no globals
		Peers:      &fakePeers{self: "self", peers: []peer.ID{"p"}},
		RPC:        rpc,
	})

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayNotReady, outcome)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Empty(t, rpc.submitted, "nothing is handed to a peer chosen by nothing")
}

// TestRelay_FollowsTheCommitteeItIsGiven — the target set is a read of the
// current globals, so a committee change moves the relay with no re-probing
// of anyone: only the KEY comes from the peer, and the judgement stays here.
func TestRelay_FollowsTheCommitteeItIsGiven(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("was-in"), peer.ID("now-in")

	bus := events.NewBus(nil)
	m := NewMembership(part, mine)
	m.SubscribeGlobals(bus)
	require.NoError(t, bus.Publish(events.WillChangeGlobals{
		New: globalsWith(t, map[string][]ed25519.PublicKey{part: {a}}),
	}))

	rpc := &fakeRPC{keys: map[peer.ID]ed25519.PublicKey{pa: a, pb: b}}
	r := NewRelay(RelayParams{Partition: part, Membership: m,
		Peers: &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, RPC: rpc})

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome)
	require.Equal(t, []peer.ID{pa}, rpc.submitted)

	require.NoError(t, bus.Publish(events.WillChangeGlobals{
		New: globalsWith(t, map[string][]ed25519.PublicKey{part: {b}}),
	}))
	_, outcome, err = r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome)
	require.Equal(t, []peer.ID{pa, pb}, rpc.submitted,
		"the relay followed the committee it holds, not an answer it remembered")
}

// TestRelay_ACommitteeMemberThatIsCatchingUpIsNotATarget — the second hop.
//
// A validator that is still joining IS in the committee and still cannot
// propose, so handing it a submission makes it relay again: two hops for one
// submission, and two nodes in that state can pass one to each other. The
// relay asks for the target's standing on every attempt, so neither a stale
// answer nor a committee key alone makes a node a target.
func TestRelay_ACommitteeMemberThatIsCatchingUpIsNotATarget(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	joining, ready := otherKey(t), otherKey(t)
	pj, pr := peer.ID("joining-validator"), peer.ID("ready-validator")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {joining, ready}})
	rpc := &fakeRPC{
		keys:       map[peer.ID]ed25519.PublicKey{pj: joining, pr: ready},
		catchingUp: map[peer.ID]bool{pj: true},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pj, pr}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome)
	require.Equal(t, []peer.ID{pr}, rpc.submitted, "the joining validator would have relayed it again")

	// And the standing is read again for the next submission, not
	// remembered: once the joining validator has caught up it is a target.
	rpc.catchingUp[pj] = false
	rpc.submitted = nil
	for i := 0; i < 4; i++ {
		_, _, err = r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
		require.NoError(t, err)
	}
	require.Contains(t, rpc.submitted, pj,
		"every attempt asks, so a node that has caught up is a target again")
}

// TestRelay_ACandidateMustProveItHoldsTheKeyItNames — F1
// (threat-reviewer note_3869947547; lead's close, note_3869952619).
//
// A validator's key hash is public: it is in the network definition and in
// every honest node's ConsensusStatus. So "I hold key H" was a claim that
// terminated in the peer, and one libp2p identity with a canned answer could
// take a share of everything a follower relays and drop it, counted `taken`.
// The relay now sends a fresh nonce and the candidate must sign it with the
// key whose hash it names.
func TestRelay_ACandidateMustProveItHoldsTheKeyItNames(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	val := otherKey(t)
	impostor, honest := peer.ID("names-the-key"), peer.ID("holds-the-key")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {val}})
	rpc := &fakeRPC{
		partition: part,
		// Both name the same real validator hash. Only one can sign for it.
		keys:       map[peer.ID]ed25519.PublicKey{impostor: val, honest: val},
		cannotSign: map[peer.ID]bool{impostor: true},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{impostor, honest}}, rpc)

	res, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome)
	require.True(t, res[0].Success)
	require.Equal(t, []peer.ID{honest}, rpc.submitted,
		"a peer that only NAMES a validator's key must never be handed a submission")

	// And a peer that cannot sign is not a refusal of the submission: it is
	// unreachable-class, so nothing is said about the envelope.
	only := &fakeRPC{
		partition:  part,
		keys:       map[peer.ID]ed25519.PublicKey{impostor: val},
		cannotSign: map[peer.ID]bool{impostor: true},
	}
	r2 := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{impostor}}, only)
	_, outcome, err = r2.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayUnreachable, outcome)
	require.Error(t, err)
	require.Empty(t, only.submitted)
}

// TestRelay_AForwardedAnswerIsNotProof — the forwarding half of F1
// (note_3869991754).
//
// ConsensusStatus signs any caller's nonce, so a peer with no key at all
// can take the relay's nonce, ask the validator whose hash it names, and
// hand that answer back as its own: one extra round trip, well inside the
// deadline. The signature is real, the key is right, and the peer is a
// sink. It is refused because the signature is for the validator's peer ID
// and the relay verifies with the ID it dialled.
func TestRelay_AForwardedAnswerIsNotProof(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	val := otherKey(t)
	sink, honest := peer.ID("forwards-the-nonce"), peer.ID("holds-the-key")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {val}})
	rpc := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{sink: val, honest: val},
		// The sink's answer is signed by the real key, for the real
		// holder's peer ID.
		forwards: map[peer.ID]peer.ID{sink: honest},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{sink, honest}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome)
	require.Equal(t, []peer.ID{honest}, rpc.submitted,
		"a peer that forwarded somebody else's proof must never be handed a submission")

	// And alone, it is unreachable-class: nothing was said about the
	// envelope.
	only := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{sink: val},
		forwards:  map[peer.ID]peer.ID{sink: honest},
	}
	r2 := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{sink}}, only)
	_, outcome, err = r2.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayUnreachable, outcome)
	require.Error(t, err)
	require.Empty(t, only.submitted)
}

// TestRelayChallenge_BindsTagPartitionKeyPeerAndNonce — what the signature
// is over, what it cannot be mistaken for, and what a node refuses to sign.
func TestRelayChallenge_BindsTagPartitionKeyPeerAndNonce(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hash := sha256.Sum256(pub)
	const self, other = peer.ID("me"), peer.ID("somebody-else")

	nonce, err := newRelayChallenge()
	require.NoError(t, err)
	require.Len(t, nonce, relayChallengeSize)
	spare, err := newRelayChallenge()
	require.NoError(t, err)
	require.NotEqual(t, nonce, spare, "a nonce is fresh, never reused")

	sig := signRelayChallenge(priv, "BVN3", self, self.String(), nonce)
	require.True(t, verifyRelayChallenge(pub, "BVN3", hash, self, nonce, sig))

	require.False(t, verifyRelayChallenge(pub, "BVN2", hash, self, nonce, sig), "bound to the partition")
	require.False(t, verifyRelayChallenge(pub, "BVN3", sha256.Sum256(spare), self, nonce, sig), "bound to the claimed key")
	require.False(t, verifyRelayChallenge(pub, "BVN3", hash, other, nonce, sig), "bound to the peer that answered")
	require.False(t, verifyRelayChallenge(pub, "BVN3", hash, self, spare, sig), "bound to the nonce")
	require.False(t, verifyRelayChallenge(pub, "BVN3", hash, self, nonce, nil), "no signature is no proof")

	// What a node refuses to sign: for another peer, nothing, or a payload
	// of the caller's own length.
	require.Nil(t, signRelayChallenge(priv, "BVN3", self, other.String(), nonce), "never for another peer ID")
	require.Nil(t, signRelayChallenge(priv, "BVN3", self, "", nonce))
	require.Nil(t, signRelayChallenge(priv, "BVN3", "", "", nonce))
	require.Nil(t, signRelayChallenge(priv, "BVN3", self, self.String(), nil))
	require.Nil(t, signRelayChallenge(priv, "BVN3", self, self.String(), make([]byte, relayChallengeMaxNonce+1)))
	require.NotNil(t, signRelayChallenge(priv, "BVN3", self, self.String(), make([]byte, relayChallengeMaxNonce)))

	// And what it signs cannot be a consensus message. The separation is
	// the tag, not the length: a vote signs 48 bytes and a state hash 56,
	// so length proves nothing and the prefix proves everything.
	msg := relayChallengeMessage("BVN3", hash, self, nonce)
	require.True(t, strings.HasPrefix(string(msg), relayChallengeTag))
}

// TestRelay_ASilentCandidateDoesNotPinTheRelay — F2, which blocked the merge.
//
// A submission that arrives over the p2p submit service is handled on a
// context built from context.Background(), and on the way out only the
// stream OPEN is bounded. A candidate that accepts a stream and answers
// nothing therefore pinned one goroutine and two streams per submission,
// forever, on the host the consensus engine shares.
func TestRelay_ASilentCandidateDoesNotPinTheRelay(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	quiet, ready := otherKey(t), otherKey(t)
	pq, pr := peer.ID("silent"), peer.ID("answers")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {quiet, ready}})

	// Alone, with no caller deadline of any kind.
	only := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{pq: quiet},
		silent:    map[peer.ID]bool{pq: true},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pq}}, only)
	r.attemptTimeout = 200 * time.Millisecond
	r.budget = 2 * time.Second

	done := make(chan string, 1)
	go func() {
		_, outcome, _ := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
		done <- outcome
	}()
	select {
	case outcome := <-done:
		require.Equal(t, metrics.RelayUnreachable, outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("a silent candidate pinned the relay: there is no deadline on an attempt (#4366 F2)")
	}

	// And a silent candidate does not stop the submission reaching one that
	// answers.
	both := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{pq: quiet, pr: ready},
		silent:    map[peer.ID]bool{pq: true},
	}
	r2 := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pq, pr}}, both)
	r2.attemptTimeout = 200 * time.Millisecond
	r2.budget = 2 * time.Second

	done2 := make(chan string, 1)
	go func() {
		_, outcome, _ := r2.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
		done2 <- outcome
	}()
	select {
	case outcome := <-done2:
		require.Equal(t, metrics.RelayTaken, outcome)
		require.Equal(t, []peer.ID{pr}, both.submitted)
	case <-time.After(5 * time.Second):
		t.Fatal("one silent candidate stopped the relay reaching a live one")
	}
}

// TestRelay_BackPressureIsNotShopped — F3. A validator answering
// TooManyRequests is the network saying it is at capacity; handing the same
// envelope to every other validator multiplies the load exactly when it is
// weakest.
func TestRelay_BackPressureIsNotShopped(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("busy"), peer.ID("also-there")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b}})
	rpc := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers:   map[peer.ID]answer{pa: {err: errors.TooManyRequests.With("worker backpressure")}},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayNotReady, outcome)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Equal(t, []peer.ID{pa}, rpc.submitted,
		"back-pressure must not be shopped to the rest of the committee")
}

// TestRelay_UnreachableTriesEachMemberOnce — the other half of decision 2b.
func TestRelay_UnreachableTriesEachMemberOnce(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("gone-a"), peer.ID("gone-b")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b}})
	rpc := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers: map[peer.ID]answer{
			pa: {err: errors.StreamAborted.With("reset")},
			pb: {err: errors.StreamAborted.With("reset")},
		},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayUnreachable, outcome)
	require.Error(t, err)
	require.Equal(t, []peer.ID{pa, pb}, rpc.submitted, "each committee member once, and no more")
}

// TestRelay_ConsecutiveSubmissionsRotate — the relay's own cursor (decision
// 2c as the lead let it stand). Without it every submission starts at the
// same validator, and one validator carries a follower's whole share.
func TestRelay_ConsecutiveSubmissionsRotate(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("aaa"), peer.ID("bbb")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b}})
	rpc := &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{pa: a, pb: b}}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc)

	for i := 0; i < 3; i++ {
		_, _, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
		require.NoError(t, err)
	}
	require.Equal(t, []peer.ID{pa, pb, pa}, rpc.submitted,
		"consecutive submissions must not all start at the same validator")
}

// TestClassifyRelay_ARefusalWithACauseIsStillARefusal — the reviewer's H1.
//
// Every refusal a validator's Submit returns carries a cause: "verify: %w"
// around the normalizer's error, "submit: %w" around the worker's. Matching
// errors.UnknownError with errors.Is matched all of them, because
// ErrorBase.Is walks the cause chain and a wrapped cause ends in an
// UnknownError — so a refusal was filed unreachable, shopped to every other
// validator, and answered NoPeer, which is the laundering this design
// exists to prevent. The unit test that passed built a BadRequest with no
// cause.
func TestClassifyRelay_ARefusalWithACauseIsStillARefusal(t *testing.T) {
	// The shapes internal/node/dagbft/api.go actually returns.
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"verify, wrapping the normalizer",
			errors.BadRequest.WithFormat("verify: %w", fmt.Errorf("invalid hash length: want 32, got 5")),
			metrics.RelayRefused},
		{"oversized, wrapping the worker",
			errors.BadRequest.WithFormat("submit: %w", worker.ErrTransactionTooLarge),
			metrics.RelayRefused},
		{"store full",
			errors.NotReady.WithFormat("submit: %w", worker.ErrStoreFull),
			metrics.RelayNotReady},
		{"back-pressure",
			errors.TooManyRequests.WithFormat("submit: %w", worker.ErrBackpressure),
			metrics.RelayNotReady},
		{"the node's own fault",
			errors.InternalError.WithFormat("submit: %w", fmt.Errorf("boom")),
			metrics.RelayUnreachable},
		{"no peer",
			errors.NoPeer.With("nobody serves that"),
			metrics.RelayUnreachable},
		{"not one of ours",
			context.DeadlineExceeded,
			metrics.RelayUnreachable},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, classifyRelay(nil, c.err), "in memory")

			// And after the round trip the message layer makes of it: the
			// relay classifies what comes back over the wire, not what the
			// target constructed.
			e, ok := c.err.(*errors.Error)
			if !ok {
				return
			}
			b, err := json.Marshal(e)
			require.NoError(t, err)
			wire := new(errors.Error)
			require.NoError(t, json.Unmarshal(b, wire))
			require.Equal(t, c.want, classifyRelay(nil, wire), "after the JSON round trip")
		})
	}
}

// TestRelay_ARefusalIsNotShoppedWhateverItsCause — H1 through the relay, not
// the classifier: a refusal that carries a cause stops at the first target.
func TestRelay_ARefusalIsNotShoppedWhateverItsCause(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b := otherKey(t), otherKey(t)
	pa, pb := peer.ID("first"), peer.ID("second")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b}})
	rpc := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{pa: a, pb: b},
		answers: map[peer.ID]answer{pa: {
			err: errors.BadRequest.WithFormat("verify: %w", fmt.Errorf("invalid hash length: want 32, got 5")),
		}},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.Equal(t, metrics.RelayRefused, outcome)
	require.Equal(t, errors.BadRequest, errors.Code(err), "got %v", err)
	require.Equal(t, []peer.ID{pa}, rpc.submitted,
		"a refusal with a cause must not be shopped to the rest of the committee")
}

// TestRelay_ManyAttemptsAreOneSubmission — the counting rule (#4366
// note_3869978257): accepted is counted once, at a submission's first entry
// into this node's worker or its relay, never per attempt.
func TestRelay_ManyAttemptsAreOneSubmission(t *testing.T) {
	const part = "BVN3"
	mine := otherKey(t)
	a, b, c := otherKey(t), otherKey(t), otherKey(t)
	pa, pb, pc := peer.ID("aa-not-ready"), peer.ID("bb-unreachable"), peer.ID("cc-takes-it")

	g := globalsWith(t, map[string][]ed25519.PublicKey{part: {a, b, c}})
	rpc := &fakeRPC{
		partition: part,
		keys:      map[peer.ID]ed25519.PublicKey{pa: a, pb: b, pc: c},
		answers: map[peer.ID]answer{
			pa: {err: errors.NotReady.With("joining")},
			pb: {err: errors.StreamAborted.With("reset")},
		},
	}
	r := relayFor(t, part, mine, g, &fakePeers{self: "self", peers: []peer.ID{pa, pb, pc}}, rpc)

	_, outcome, err := r.Submit(context.Background(), new(messaging.Envelope), api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, metrics.RelayTaken, outcome, "one outcome, at the submission's final answer")
	require.Equal(t, []peer.ID{pa, pb, pc}, rpc.submitted, "three attempts")
}
