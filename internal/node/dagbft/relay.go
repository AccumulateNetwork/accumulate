// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"sync"

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
	// Standing asks a peer which author key it holds for the partition and
	// whether it is still catching up. Together they are the question that
	// separates a node that can propose from one that would only relay
	// again.
	Standing(ctx context.Context, p peer.ID) (keyHash [32]byte, catchingUp bool, err error)

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
type Relay struct {
	logger     logging.OptionalLogger
	partition  string
	membership *Membership
	peers      RelayPeers
	rpc        RelayRPC
	submitAddr *api.ServiceAddress

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

	candidates := r.candidates(ctx)
	if len(candidates) == 0 {
		return nil, metrics.RelayUnreachable, errors.NoPeer.WithFormat(
			"cannot relay for %s: no peer is known to handle its submissions", r.partition)
	}

	var sawNotReady, sawAny bool
	for _, p := range candidates {
		if ctx.Err() != nil {
			break
		}

		// Confirm the target can propose BEFORE handing it anything, and
		// confirm it now rather than from a remembered answer: a peer whose
		// key is in no committee, or which is still catching up, would only
		// relay it again, and two nodes each holding a stale "it can
		// propose" about the other is a relay that goes round in a circle.
		hash, catchingUp, err := r.rpc.Standing(ctx, p)
		if err != nil {
			r.logger.Debug("Relay candidate did not answer", "peer", p, "partition", r.partition, "error", err)
			continue
		}
		if catchingUp || !r.membership.IsMember(hash) {
			continue
		}
		sawAny = true

		res, err := r.rpc.Submit(ctx, p, env, opts)
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
	return nil, metrics.RelayUnreachable, errors.NoPeer.WithFormat(
		"cannot relay for %s: no peer that can propose was found", r.partition)
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

// classifyRelay says what one attempt was.
//
// Three buckets are named, and the rest is the target's judgement of the
// submission, handed back to the caller unchanged:
//
//   - NotReady and TooManyRequests say "ask someone else" and "ask again".
//     They are about the TARGET, so the relay tries the next validator.
//   - the transport statuses, plus InternalError and the unclassified
//     UnknownError the dialer wraps a failed connection in, mean nothing
//     came back from a validator at all.
//   - everything else is a validator's refusal, which is final and is not
//     re-judged, queued or retried elsewhere. Retrying a refusal at the next
//     validator is how a relaying node would launder one node's refusal into
//     the network's answer.
func classifyRelay(res []*api.Submission, err error) string {
	if err != nil {
		switch {
		case errors.Is(err, errors.NotReady),
			errors.Is(err, errors.TooManyRequests):
			return metrics.RelayNotReady

		case errors.Is(err, errors.NoPeer),
			errors.Is(err, errors.NotFound),
			errors.Is(err, errors.StreamAborted),
			errors.Is(err, errors.PeerMisbehaved),
			errors.Is(err, errors.EncodingError),
			errors.Is(err, errors.InternalError),
			errors.Is(err, errors.UnknownError),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, context.Canceled):
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

func (c ClientRelayRPC) Standing(ctx context.Context, p peer.ID) ([32]byte, bool, error) {
	no := false
	st, err := c.forPeer(p, api.ServiceTypeConsensus).
		ConsensusStatus(ctx, api.ConsensusStatusOptions{
			// The key and the sync state, and nothing that costs the target
			// a database read.
			IncludeAccumulate: &no,
			IncludePeers:      &no,
			NodeID:            p.String(),
			Partition:         c.Partition,
		})
	if err != nil {
		return [32]byte{}, false, errors.UnknownError.Wrap(err)
	}
	return st.ValidatorKeyHash, st.CatchingUp, nil
}

func (c ClientRelayRPC) Submit(ctx context.Context, p peer.ID, env *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return c.forPeer(p, api.ServiceTypeSubmit).Submit(ctx, env, opts)
}
