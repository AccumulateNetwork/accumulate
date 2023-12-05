// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type NetworkScan = healing.NetworkInfo

func LoadNetworkScan(file string) (*NetworkScan, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var net *NetworkScan
	err = json.NewDecoder(f).Decode(&net)
	if err != nil {
		return nil, err
	}
	return net, nil
}

func NewMessageRouter(scan *NetworkScan) (message.Router, error) {
	var err error
	router := new(routing.MessageRouter)
	router.Router, err = routing.NewStaticRouter(scan.Status.Routing, nil)
	return router, err
}

type StaticDialer struct {
	Scan   *healing.NetworkInfo
	Dialer message.Dialer
	Nodes  api.NodeService

	mu   sync.RWMutex
	good map[string]peer.ID
}

func (h *StaticDialer) BadDial(ctx context.Context, addr multiaddr.Multiaddr, stream message.Stream, err error) bool {
	return true
}

func (h *StaticDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	// Have we found a good peer?
	s := h.dialKnownGood(ctx, addr)
	if s != nil {
		return s, nil
	}
	// Unpack the service address
	network, peerID, service, _, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, err
	}

	// Check for a recorded address
	if h.Scan != nil {
		if peerID != "" {
			info := h.Scan.PeerByID(peerID)
			if info != nil {
				addr := addr
				if peerID == "" {
					c, err := multiaddr.NewComponent("p2p", info.ID.String())
					if err != nil {
						panic(err)
					}
					addr = c.Encapsulate(addr)
				}
				for _, paddr := range info.Addresses {
					s, err := h.Dialer.Dial(ctx, paddr.Encapsulate(addr))
					if err == nil {
						h.markGood(addr, info.ID)
						return s, nil
					}
					slog.Error("Failed to connect", "peer", info.ID, "address", paddr, "service", addr, "error", err)
				}
			}
		} else if service.Argument != "" {
			// In the future not all peers will have all services
			part, ok := h.Scan.Peers[strings.ToLower(service.Argument)]
			if ok {
				tried := map[string]bool{}
				pick := func() (*healing.PeerInfo, multiaddr.Multiaddr) {
					for _, p := range part {
						for _, addr := range p.Addresses {
							if tried[addr.String()] {
								continue
							}
							tried[addr.String()] = true
							c, err := multiaddr.NewComponent("p2p", p.ID.String())
							if err != nil {
								panic(err)
							}
							addr = c.Encapsulate(addr)
							return p, addr
						}
					}
					return nil, nil
				}

				for {
					info, paddr := pick()
					if paddr == nil {
						break
					}
					s, err := h.Dialer.Dial(ctx, paddr.Encapsulate(addr))
					if err == nil {
						h.markGood(addr, info.ID)
						return s, nil
					}
					slog.Error("Failed to connect", "peer", info.ID, "address", paddr, "service", addr, "error", err)
				}
			}
		}
	}

	// If it specifies a node, do nothing
	if h.Nodes == nil || peerID != "" {
		return h.Dialer.Dial(ctx, addr)
	}

	// Use the API to find a node
	nodes, err := h.Nodes.FindService(ctx, api.FindServiceOptions{Network: network, Service: service})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate nodes for %v: %w", addr, err)
	}
	if len(nodes) == 0 {
		return nil, errors.NoPeer.WithFormat("cannot locate a peer for %v", addr)
	}

	// Try all the nodes
	for _, n := range nodes {
		s, err := h.dial(ctx, addr, n.PeerID)
		if err == nil {
			h.markGood(addr, n.PeerID)
			return s, nil
		}
		slog.ErrorCtx(ctx, "Failed to dial", "peer", n.PeerID, "error", err)
	}
	return nil, errors.NoPeer.WithFormat("no peers are responding for %v", addr)
}

func (h *StaticDialer) dial(ctx context.Context, addr multiaddr.Multiaddr, peer peer.ID) (message.Stream, error) {
	c, err := multiaddr.NewComponent("p2p", peer.String())
	if err != nil {
		return nil, err
	}
	addr = addr.Encapsulate(c)
	return h.Dialer.Dial(ctx, addr)
}

func (h *StaticDialer) dialKnownGood(ctx context.Context, addr multiaddr.Multiaddr) message.Stream {
	h.mu.RLock()
	id, ok := h.good[addr.String()]
	h.mu.RUnlock()
	if !ok {
		return nil
	}

	s, err := h.dial(ctx, addr, id)
	if err == nil {
		return s
	}

	slog.Info("Failed to dial previously good node", "id", id, "error", err)

	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.good, addr.String())

	return nil
}

func (h *StaticDialer) markGood(addr multiaddr.Multiaddr, id peer.ID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.good == nil {
		h.good = map[string]peer.ID{}
	}
	h.good[addr.String()] = id
}
