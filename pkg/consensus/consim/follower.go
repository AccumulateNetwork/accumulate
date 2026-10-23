// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// JoinFollower adds a node that is in no committee to a running network, as
// a follower joins the soak: its own host and pubsub connected to every
// validator, one consensus node per named partition, started with ordering
// held. After pull -- the time the join takes to pull and prove a state --
// each is seeded by Rejoin at seed(part), as internal/node/dagbft seeds it
// with the leader round its pulled state records (#4405). Until then the
// node receives certificates it cannot place, as the real one does. Its
// committed groups are drained and reported executed; it takes no part in
// stall detection.
func (s *Sim) JoinFollower(ctx context.Context, seed func(part string) types.Round, pull time.Duration, parts ...string) ([]*consensus.Node, error) {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		return nil, fmt.Errorf("follower host: %w", err)
	}
	s.hosts = append(s.hosts, h) // closed with the others
	// Pubsub before the connections, as a node's exists before its peers
	// dial it: a peer whose pubsub dials a host with none yet takes it for a
	// host that does not speak pubsub.
	// The node's pubsub options, as the validators' (New).
	ps, err := pubsub.NewGossipSub(ctx, h, pubsub.WithPeerExchange(true), pubsub.WithFloodPublish(true))
	if err != nil {
		return nil, fmt.Errorf("follower pubsub: %w", err)
	}
	for _, v := range s.hosts[:len(s.hosts)-1] {
		if err := h.Connect(ctx, peer.AddrInfo{ID: v.ID(), Addrs: v.Addrs()}); err != nil {
			return nil, fmt.Errorf("follower connect: %w", err)
		}
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	var nodes []*consensus.Node
	for _, part := range parts {
		val := s.byPart[part][0]
		cfg := val.cfg
		cfg.KeyPair = key
		cfg.StateDir = ""
		n, err := consensus.NewNode(cfg, val.committee, h, ps)
		if err != nil {
			return nil, fmt.Errorf("follower %s: %w", part, err)
		}
		n.HoldOrdering()
		if err := n.Start(s.runCtx); err != nil {
			return nil, fmt.Errorf("follower %s start: %w", part, err)
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-s.runCtx.Done():
					return
				case _, ok := <-n.Committed():
					if !ok {
						return
					}
					n.ReportExecuted()
				}
			}
		}()
		nodes = append(nodes, n)
	}

	time.Sleep(pull)
	for i, part := range parts {
		nodes[i].Rejoin(seed(part))
	}
	return nodes, nil
}

// Validators returns the running consensus nodes of a partition.
func (s *Sim) Validators(part string) []*consensus.Node {
	var out []*consensus.Node
	for _, sn := range s.byPart[part] {
		out = append(out, sn.n())
	}
	return out
}
