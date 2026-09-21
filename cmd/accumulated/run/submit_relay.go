// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// submitterParams are everything the two services the relay touches need
// from the daemon.
type submitterParams struct {
	Logger    logging.Logger
	Partition string

	// AuthorKey is this node's validator PUBLIC key -- the key consensus
	// signs headers with, which is what a committee is a set of.
	AuthorKey []byte

	// Globals is the network definition the daemon has already waited for.
	// It is handed over directly AS WELL AS subscribed to, because whether a
	// subscriber sees the initial WillChangeGlobals is a startup ordering
	// race -- the same race the daemon already works around for the adapter
	// and the conductor.
	Globals  *network.GlobalValues
	EventBus *events.Bus

	Service *dagbft.Service

	// NodeState is this node's join state, and it is REQUIRED and never
	// nil: a node that never joined is nodestate.Always{}, not absence. Nil
	// used to mean "never joined", which made "the daemon forgot to pass
	// it" indistinguishable from the common case -- and forgetting it is
	// two live regressions at once, a joining node that proposes instead of
	// relaying (#4307) and a node that never reports CatchingUp, which is
	// the one fact bounding a relay to a single hop (#4366).
	NodeState nodestate.Serving

	// ValidatorKey is the full private key. Its public half must be
	// AuthorKey; it answers a relay's challenge (#4366 F1).
	ValidatorKey ed25519.PrivateKey

	// Node and Network are how the relay reaches another node: its own
	// libp2p host for discovery, and a client that addresses ONE named peer.
	Node    *p2p.Node
	Network string
}

// newSubmitterService builds the submit service for a partition, with the
// committee predicate and the relay it needs to hand on what it cannot
// propose (#4366; docs/spec/executor.md, "Sync" step 6).
//
// All of it is here, in one function the daemon calls once, because each step
// is silently survivable on its own: a nil predicate makes every node think
// it is a proposer, an unseeded predicate makes it think so for the first
// five seconds, an unsubscribed one makes it think so forever after the
// committee changes, and a nil relay turns "relay it" back into "refuse it".
// None of the four failed a test when they lived inline (#4366
// note_3869830125, M1-M4).
func newSubmitterService(p submitterParams) (*dagbft.SubmitterService, error) {
	// Every piece is required, and saying so here is what pins the daemon's
	// call site. Six of eight mutations of registerAPIServices used to stay
	// green -- no globals, no event bus, no p2p node, no network name, and
	// either node state -- because each one only changes what a node does
	// on a network the in-process tests do not build (#4366 note_3869956734,
	// C1, C2, C3, C5, C6, C8). Refused here, every one of them stops the
	// daemon at startup, where every netsim test sees it.
	switch {
	case p.Partition == "":
		return nil, errors.BadRequest.With("submit service: no partition")
	case len(p.AuthorKey) != ed25519.PublicKeySize:
		return nil, errors.BadRequest.With("submit service: no author key")
	case len(p.ValidatorKey) != ed25519.PrivateKeySize:
		return nil, errors.BadRequest.With("submit service: no validator key")
	case p.Globals == nil:
		return nil, errors.BadRequest.With("submit service: no globals, so no committee to read")
	case p.EventBus == nil:
		return nil, errors.BadRequest.With("submit service: no event bus, so a committee change would never be seen")
	case p.NodeState == nil:
		return nil, errors.BadRequest.With("submit service: no node state; a node that never joined is nodestate.Always{}")
	case p.Node == nil:
		return nil, errors.BadRequest.With("submit service: no p2p node, so nothing could be relayed")
	case p.Network == "":
		return nil, errors.BadRequest.With("submit service: no network name to dial on")
	}

	membership := dagbft.NewMembership(p.Partition, p.AuthorKey)
	membership.SetGlobals(p.Globals)
	membership.SubscribeGlobals(p.EventBus)

	relay := dagbft.NewRelay(dagbft.RelayParams{
		Logger:     p.Logger,
		Partition:  p.Partition,
		Membership: membership,
		Peers:      dagbft.NodeRelayPeers{Node: p.Node},
		RPC: dagbft.ClientRelayRPC{
			Partition: p.Partition,
			Client: &message.Client{Transport: &message.RoutedTransport{
				Network: p.Network,
				Dialer:  p.Node.DialNetwork(),
			}},
		},
	})

	return dagbft.NewSubmitterService(dagbft.SubmitterServiceParams{
		Logger:     p.Logger,
		Service:    p.Service,
		NodeState:  p.NodeState,
		Membership: membership,
		Relay:      relay,
	}), nil
}

// newConsensusAPIService builds the consensus read service, and exists for
// the same reason the submitter's factory does: the two fields #4366 added
// to it are the ones a daemon can silently omit.
//
// NodeState is what this node reports as CatchingUp, which is how another
// node's relay knows this one cannot propose yet -- omit it and a joining
// validator becomes a relay target again and the second hop is back.
// ValidatorKey is what answers a relay's challenge -- omit it and this node
// is no relay's target at all, which is quiet and wrong. PeerID is what the
// answer is bound to -- omit it and the answer proves only that SOME
// validator signed the nonce, which any peer can obtain by forwarding it.
// All three are required, so each stops the daemon at startup rather than a
// soak six hours in.
func newConsensusAPIService(p dagbft.ConsensusAPIServiceParams) (*dagbft.ConsensusAPIService, error) {
	switch {
	case p.NodeState == nil:
		return nil, errors.BadRequest.With("consensus service: no node state; a node that never joined is nodestate.Always{}")
	case len(p.ValidatorKey) != ed25519.PrivateKeySize:
		return nil, errors.BadRequest.With("consensus service: no validator key to answer a relay's challenge with")
	case p.PeerID == "":
		return nil, errors.BadRequest.With("consensus service: no peer ID, so a challenge answer would prove only that SOME validator signed it")
	}
	return dagbft.NewConsensusAPIService(p), nil
}
