// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// RelayPeers enumerates the peers that might serve a service and names this
// node, so that this node is never one of them.
type RelayPeers interface {
	// Self is this node's own peer ID.
	Self() peer.ID

	// Providers returns peers known to handle the service.
	Providers(ctx context.Context, sa *api.ServiceAddress) []peer.ID
}

// RelayRPC is the two calls a relay makes on a NAMED peer — not on "whoever
// the router picks", which is how a relay ends up talking to itself.
type RelayRPC interface {
	// Standing asks a peer which author key it holds for the partition,
	// whether it is still catching up, and for the caller's challenge
	// signed with that key. The first two separate a node that can propose
	// from one that would only relay again; the third separates the holder
	// of the key from a peer that merely names it.
	Standing(ctx context.Context, p peer.ID, challenge []byte) (keyHash [32]byte, catchingUp bool, signature []byte, err error)

	// Submit hands the submission to that one peer.
	Submit(ctx context.Context, p peer.ID, env *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error)
}

// Relay hands a submission this node cannot propose to a node that can
// (docs/spec/executor.md, "Sync" step 6: a read needs local state, a relay
// needs none).
//
// The spec's bound is three negatives — never to the relaying node itself,
// never to another node that would only relay it again, and never twice for
// one submission — and this build meets all three by CONFIRMING the target
// instead of marking the submission. A candidate is a peer that handles
// submit for the partition; the relay asks that peer for the author key it
// holds and keeps it only if that key is an active validator of the
// partition in this node's own globals. A confirmed proposer does not relay,
// so there is no second hop; this node is excluded by peer ID, so there is no
// self-relay; and one submission is handed to at most one node that takes it.
//
// The alternative — relay to any provider and carry an "already relayed" flag
// so the second node bounces — needs a new field in api.SubmitOptions, which
// is public protocol surface any client could set to switch relaying off.
//
// The relay is SYNCHRONOUS: the caller gets the target's answer. Accepting
// and forwarding would re-create accept-then-drop under another name, which
// is the failure this whole issue is (#4366, threat-reviewer note_3869812517
// D4).
//
// What a lying peer can do with this, stated rather than discovered:
//
//   - A peer's answer about its own key and its own sync state is an
//     UNAUTHENTICATED CLAIM. A peer that claims a real validator's key hash
//     is handed submissions and can drop them, and the caller is told they
//     were taken. The relay does not create that exposure — before it, the
//     same peer was a provider of submit for the partition and the
//     dispatcher and the API router dialled it anyway — but it does not
//     close it either. Closing it needs the claim signed by the key it
//     names, which is not in this build.
//   - Two colluding committee members that both claim they are not catching
//     up can pass one submission back and forth: each excludes only itself.
//     The chain ends at the caller's context deadline. An honest node cannot
//     be made to do this — a node that is catching up says so, and one that
//     is not proposes rather than relays — so this is collusion, not a
//     property of the mechanism, and the hard bound for it is a hop marker
//     on the submission, which is public protocol surface a client could
//     set (see above).
//
// relayAttemptTimeout bounds ONE call to one candidate.
//
// Neither hop has a deadline of its own: a submission that arrives over the
// p2p submit service is handled on a context built from context.Background()
// (pkg/api/v3/message/handler.go), and on the way out only the stream OPEN is
// bounded (p2p.go, getPeerService) -- the response read is not. So a
// candidate that accepts a stream and never answers pins one goroutine and
// two streams per submission, on a host the consensus engine shares, until
// the resource manager refuses every open and the node stops following
// (#4366 F2, threat-reviewer note_3869947547).
const relayAttemptTimeout = 10 * time.Second

// relayBudget bounds the WHOLE relay of one submission, every candidate
// together. "One try per committee member" still holds inside it; what it
// stops is a submission costing the follower unbounded time because each of
// ten candidates is slow rather than silent (#4366 F3).
const relayBudget = 30 * time.Second

type Relay struct {
	logger     logging.OptionalLogger
	partition  string
	membership *Membership
	peers      RelayPeers
	rpc        RelayRPC
	submitAddr *api.ServiceAddress

	// attemptTimeout and budget are relayAttemptTimeout and relayBudget,
	// as fields so a test can make them small.
	attemptTimeout time.Duration
	budget         time.Duration

	mu        sync.Mutex
	cursor    int
	saidBlind bool
}

// RelayParams are the parts of a relay.
type RelayParams struct {
	Logger     logging.Logger
	Partition  string
	Membership *Membership
	Peers      RelayPeers
	RPC        RelayRPC
}

// NewRelay returns a relay for one partition.
func NewRelay(p RelayParams) *Relay {
	r := new(Relay)
	r.logger.L = p.Logger
	r.partition = p.Partition
	r.membership = p.Membership
	r.peers = p.Peers
	r.rpc = p.RPC
	r.submitAddr = api.ServiceTypeSubmit.AddressFor(p.Partition)
	r.attemptTimeout = relayAttemptTimeout
	r.budget = relayBudget
	return r
}

// Submit relays one submission and answers with the outcome the harness
// counts (#4366 note_3869838619) alongside the answer for the caller.
//
// The outcome is decided ONCE, at the submission's final answer: a target
// that says NotReady is a joining node and the protocol's meaning of that is
// "ask someone else", so it is a retry and not an outcome.
func (r *Relay) Submit(ctx context.Context, env *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, string, error) {
	// A node that holds no committee cannot name a node that can propose.
	// It says so rather than guessing, and says it once (#4366, decision 4).
	if !r.membership.Known() {
		r.mu.Lock()
		first := !r.saidBlind
		r.saidBlind = true
		r.mu.Unlock()
		if first {
			r.logger.Info("Cannot relay: this node holds no committee for the partition yet",
				"partition", r.partition)
		}
		return nil, metrics.RelayNotReady, errors.NotReady.WithFormat(
			"cannot relay for %s: this node holds no committee yet", r.partition)
	}

	// The whole relay of this submission is bounded, and so is every call
	// inside it (#4366 F2, F3).
	ctx, cancel := context.WithTimeout(ctx, r.budget)
	defer cancel()

	candidates := r.candidates(ctx)
	if len(candidates) == 0 {
		return nil, metrics.RelayUnreachable, errors.NoPeer.WithFormat(
			"cannot relay for %s: no peer is known to handle its submissions", r.partition)
	}

	var sawNotReady, sawAny, sawCatchingUp bool
	for _, p := range candidates {
		if ctx.Err() != nil {
			break
		}

		// Confirm the target can propose BEFORE handing it anything, and
		// confirm it now rather than from a remembered answer: a peer whose
		// key is in no committee, or which is still catching up, would only
		// relay it again, and two nodes each holding a stale "it can
		// propose" about the other is a relay that goes round in a circle.
		//
		// And make it PROVE it holds the key it names. Everything else here
		// is checked against this node's own globals, but "the peer
		// answering is the holder of that key" terminated in the peer: a
		// validator's key hash is public, so one libp2p identity with a
		// canned answer could take a share of everything this node relays
		// and drop it, counted `taken` (#4366 F1). A fresh nonce, signed
		// with the key whose hash is claimed, ends that.
		nonce, err := newRelayChallenge()
		if err != nil {
			return nil, metrics.RelayUnreachable, errors.UnknownError.Wrap(err)
		}

		hash, catchingUp, sig, err := r.standing(ctx, p, nonce)
		if err != nil {
			r.logger.Debug("Relay candidate did not answer", "peer", p, "partition", r.partition, "error", err)
			r.skipped(metrics.SkipNoAnswer)
			continue
		}
		if catchingUp {
			// Counted, not silent. A validator that says this for ever while
			// voting normally opts out of relay duty, and a bare continue
			// left nothing to read (#4295, threat review finding 5).
			sawCatchingUp = true
			r.skipped(metrics.SkipCatchingUp)
			continue
		}
		key, ok := r.membership.ActiveKey(hash)
		if !ok {
			r.skipped(metrics.SkipNotAValidator)
			continue
		}
		if !verifyRelayChallenge(key, r.partition, hash, p, nonce, sig) {
			// Unreachable-class, not refused: nothing was said about the
			// submission. The peer named a validator and could not answer
			// as one.
			r.logger.Info("Relay candidate could not prove it is the holder of the key it claims",
				"peer", p, "partition", r.partition)
			r.skipped(metrics.SkipUnprovenKey)
			continue
		}
		sawAny = true

		res, err := r.submit(ctx, p, env, opts)
		switch classifyRelay(res, err) {
		case metrics.RelayTaken:
			r.logger.Debug("Relayed", "peer", p, "partition", r.partition)
			return res, metrics.RelayTaken, nil

		case metrics.RelayRefused:
			// Passed back unchanged. The relaying node does not re-judge a
			// validator's refusal and does not queue it: doing either would
			// launder the refusal into this node's own answer.
			return res, metrics.RelayRefused, err

		case metrics.RelayNotReady:
			sawNotReady = true
			if errors.Code(err) == errors.TooManyRequests {
				// Back-pressure is the network saying it is at capacity.
				// Shopping the envelope to every other validator multiplies
				// the load exactly when it is weakest, so this one stops
				// here (#4366 F3).
				return nil, metrics.RelayNotReady, errors.NotReady.WithFormat(
					"%s is at capacity: %w", r.partition, err)
			}

		default:
			r.logger.Debug("Relay target unreachable", "peer", p, "partition", r.partition, "error", err)
		}
	}

	if sawNotReady {
		return nil, metrics.RelayNotReady, errors.NotReady.WithFormat(
			"relayed %s to every validator it could reach and all are not ready", r.partition)
	}
	if sawAny {
		return nil, metrics.RelayUnreachable, errors.NoPeer.WithFormat(
			"relayed %s to every validator it could reach and none answered", r.partition)
	}
	if sawCatchingUp {
		// Every validator that answered is catching up. That is the NETWORK
		// saying "later", which is NotReady — the same thing the caller is
		// told when every target answers NotReady. NoPeer says no validator
		// was found, which is a different fault and sends an operator
		// looking at discovery (#4295, threat review finding 5).
		return nil, metrics.RelayNotReady, errors.NotReady.WithFormat(
			"every validator of %s that answered is still catching up", r.partition)
	}
	return nil, metrics.RelayUnreachable, errors.NoPeer.WithFormat(
		"cannot relay for %s: no peer that can propose was found", r.partition)
}

// skipped counts a candidate the relay passed over before handing it
// anything.
func (r *Relay) skipped(reason string) {
	metrics.RelaySkippedTotal.WithLabelValues(r.partition, reason).Inc()
}

// standing and submit are the two calls a relay makes, each under its own
// deadline. Without one, a candidate that opens a stream and answers nothing
// parks this goroutine forever: the inbound request has no deadline and the
// outbound read has none either (#4366 F2).
func (r *Relay) standing(ctx context.Context, p peer.ID, nonce []byte) ([32]byte, bool, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.attemptTimeout)
	defer cancel()
	return r.rpc.Standing(ctx, p, nonce)
}

func (r *Relay) submit(ctx context.Context, p peer.ID, env *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	ctx, cancel := context.WithTimeout(ctx, r.attemptTimeout)
	defer cancel()
	return r.rpc.Submit(ctx, p, env, opts)
}

// candidates are the peers that handle submissions for this partition, this
// node excluded, rotated so that consecutive submissions do not all start at
// the same validator.
func (r *Relay) candidates(ctx context.Context) []peer.ID {
	all := r.peers.Providers(ctx, r.submitAddr)
	self := r.peers.Self()

	out := make([]peer.ID, 0, len(all))
	for _, p := range all {
		if p == self {
			// Never itself. Dialling is local-first, so a relay that did not
			// say this would hand the submission straight back to the
			// service that could not propose it (#4366).
			continue
		}
		out = append(out, p)
	}
	if len(out) < 2 {
		return out
	}

	r.mu.Lock()
	i := r.cursor % len(out)
	r.cursor++
	r.mu.Unlock()
	return append(append(make([]peer.ID, 0, len(out)), out[i:]...), out[:i]...)
}

// classifyRelay says what one attempt was, by the error's CODE.
//
// By code, not by errors.Is: ErrorBase.Is walks the cause chain, and every
// real refusal from a validator's Submit carries a cause -- "verify: %w",
// "submit: %w" -- whose chain ends in an UnknownError. Matching
// errors.UnknownError therefore matched every BadRequest a validator ever
// returned, so a refusal was filed unreachable and shopped to every other
// validator until the caller was told NoPeer: the laundering this design
// exists to prevent, invisible because the unit test built a BadRequest
// with no cause (#4366, reviewer H1). errors.Code walks past UnknownError
// to the status that was meant.
//
// Three buckets are named, and the rest is the target's judgement of the
// submission, handed back to the caller unchanged:
//
//   - NotReady and TooManyRequests say "ask someone else" and "ask again".
//     They are about the TARGET, so the relay tries the next validator.
//   - the transport statuses, plus InternalError and the unclassified
//     UnknownError a failed dial ends in, mean nothing came back from a
//     validator at all.
//   - everything else is a validator's refusal, which is final and is not
//     re-judged, queued or retried elsewhere.
func classifyRelay(res []*api.Submission, err error) string {
	if err != nil {
		switch errors.Code(err) {
		case errors.NotReady, errors.TooManyRequests:
			return metrics.RelayNotReady

		case errors.NoPeer, errors.NotFound, errors.StreamAborted,
			errors.PeerMisbehaved, errors.EncodingError,
			errors.InternalError, errors.UnknownError:
			return metrics.RelayUnreachable

		case 0:
			// Not one of ours at all: a context deadline, a net error, a
			// stream the transport gave up on.
			return metrics.RelayUnreachable
		}
		return metrics.RelayRefused
	}

	for _, s := range res {
		if s != nil && !s.Success {
			return metrics.RelayRefused
		}
	}
	return metrics.RelayTaken
}

// NodeRelayPeers is the production [RelayPeers]: the node's own libp2p host
// and the discovery the dialer itself uses — connected peers by libp2p
// identify first, the DHT behind it.
type NodeRelayPeers struct{ Node *p2p.Node }

func (n NodeRelayPeers) Self() peer.ID { return n.Node.ID() }

func (n NodeRelayPeers) Providers(ctx context.Context, sa *api.ServiceAddress) []peer.ID {
	return n.Node.Providers(ctx, sa, 10)
}

// ClientRelayRPC is the production [RelayRPC]: the node's own network client,
// addressed to one peer and one service, so no router chooses the target.
type ClientRelayRPC struct {
	Client    *message.Client
	Partition string
}

func (c ClientRelayRPC) forPeer(p peer.ID, typ api.ServiceType) message.AddressedClient {
	return c.Client.ForPeer(p).ForAddress(typ.AddressFor(c.Partition).Multiaddr())
}

func (c ClientRelayRPC) Standing(ctx context.Context, p peer.ID, challenge []byte) ([32]byte, bool, []byte, error) {
	no := false
	st, err := c.forPeer(p, api.ServiceTypeConsensus).
		ConsensusStatus(ctx, api.ConsensusStatusOptions{
			// The key, the sync state and the answer to the challenge, and
			// nothing that costs the target a database read.
			IncludeAccumulate: &no,
			IncludePeers:      &no,
			NodeID:            p.String(),
			Partition:         c.Partition,
			Challenge:         challenge,
		})
	if err != nil {
		return [32]byte{}, false, nil, errors.UnknownError.Wrap(err)
	}
	return st.ValidatorKeyHash, st.CatchingUp, st.ChallengeSignature, nil
}

func (c ClientRelayRPC) Submit(ctx context.Context, p peer.ID, env *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return c.forPeer(p, api.ServiceTypeSubmit).Submit(ctx, env, opts)
}
