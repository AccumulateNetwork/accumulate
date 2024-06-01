// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"golang.org/x/exp/slog"
)

var _ client.NetworkClient

// WalkClient is the subset of [client.NetworkClient] needed for [WalkPeers].
type WalkClient interface {
	NetInfo(context.Context) (*coretypes.ResultNetInfo, error)
}

// WalkPeers walks a Tendermint network, using the nodes' peer lists. WalkPeers
// calls visit for each (unique) discovered peer. If visit returns false,
// WalkPeers will return immediately; otherwise it will return once every peer
// has been visited.
func WalkPeers(ctx context.Context, client WalkClient, visit func(context.Context, coretypes.Peer) (WalkClient, bool)) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Query the initial peer
	var pendingQueries = 1
	results := make(chan []coretypes.Peer)
	workerSem := make(chan struct{}, 10)
	go getPeers(ctx, client, workerSem, results)

	// Process results
	seen := map[p2p.ID]bool{}
	for {
		var r []coretypes.Peer
		select {
		case r = <-results:
		case <-ctx.Done():
			return
		}

		// We received a response which means a query is complete
		pendingQueries--

		for _, p := range r {
			// Only visit each peer once
			if seen[p.NodeInfo.ID()] {
				continue
			}

			seen[p.NodeInfo.ID()] = true
			d, ok := visit(ctx, p)

			// Stop?
			if !ok {
				return
			}

			// Skip?
			if d == nil {
				continue
			}

			// Queue a job
			pendingQueries++
			go getPeers(ctx, d, workerSem, results)
		}

		// If there are no pending queries, we're done
		if pendingQueries == 0 {
			return
		}
	}
}

func getPeers(ctx context.Context, client WalkClient, sem chan struct{}, results chan<- []coretypes.Peer) {
	// Capture the done channel in a variable. Otherwise the reassignment of the
	// context below will cause problems when that new context is canceled.
	done := ctx.Done()

	// Send the peer list regardless of what happens so that pending
	// queries are counted correctly
	var peers []coretypes.Peer
	defer func() {
		select {
		case results <- peers:
		case <-done:
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			slog.ErrorCtx(ctx, "Panicked while querying peers", "error", r)
		}
	}()

	// Limit the number of simultaneous requests
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-done:
		return
	}

	// Timeout after a second
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Query the node
	info, err := client.NetInfo(ctx)
	if err != nil {
		slog.InfoCtx(ctx, "Failed to query net info", "error", err)
	} else {
		peers = info.Peers
	}
}

// NewHTTPClientForPeer creates a new Tendermint RPC HTTP client for the given peer.
func NewHTTPClientForPeer(peer coretypes.Peer, offset config.PortOffset) (*HTTPClient, error) {
	// peer.node_info.listen_addr should include the Tendermint P2P port
	s := peer.NodeInfo.ListenAddr
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return nil, errors.New("invalid listen address")
	}

	// Convert the port number to a string
	port, err := strconv.ParseUint(s[i+1:], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}

	// Calculate the RPC port from the P2P port
	port = port - uint64(config.PortOffsetTendermintP2P) + uint64(config.PortOffsetTendermintRpc) + uint64(offset)

	host := peer.RemoteIP
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		// If remote_ip is localhost, use listen_addr instead
		host = s[:i]
	}

	// Construct the address from peer.remote_ip and the calculated port
	addr := fmt.Sprintf("http://%s:%d", host, port)

	// Create a new client
	return NewHTTPClient(addr)
}
