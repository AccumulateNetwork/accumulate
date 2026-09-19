// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// submitterParams are everything the submit service needs from the daemon.
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

	Service   *dagbft.Service
	NodeState nodestate.Serving

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
func newSubmitterService(p submitterParams) *dagbft.SubmitterService {
	membership := dagbft.NewMembership(p.Partition, p.AuthorKey)
	membership.SetGlobals(p.Globals)
	membership.SubscribeGlobals(p.EventBus)

	var relay *dagbft.Relay
	if p.Node != nil {
		relay = dagbft.NewRelay(dagbft.RelayParams{
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
	}

	return dagbft.NewSubmitterService(dagbft.SubmitterServiceParams{
		Logger:     p.Logger,
		Service:    p.Service,
		NodeState:  p.NodeState,
		Membership: membership,
		Relay:      relay,
	})
}
