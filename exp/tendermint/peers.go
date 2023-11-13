// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cometbft/cometbft/p2p"
	rpc "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"golang.org/x/exp/slog"
)

// WalkPeers walks a Tendermint network, using the nodes' peer lists. WalkPeers
// calls visit for each (unique) discovered peer. If visit returns false,
// WalkPeers will return immediately; otherwise it will return once every peer
// has been visited.
func WalkPeers(ctx context.Context, client rpc.NetworkClient, visit func(context.Context, coretypes.Peer) (rpc.NetworkClient, bool)) {
	jobsIn := make(chan rpc.NetworkClient, 1)
	jobsOut := make(chan []coretypes.Peer)
	activeJobs := new(sync.WaitGroup)
	activeWorkers := new(sync.WaitGroup)
	workerSem := make(chan struct{}, 10)

	// Send the first client
	activeJobs.Add(1)
	jobsIn <- client

	// Close the channel once all requests are complete or once this function
	// returns
	done := new(sync.Once)
	defer done.Do(func() { close(jobsIn) })
	go func() { activeJobs.Wait(); done.Do(func() { close(jobsIn) }) }()

	go func() {
		// Close the channel once all work is done
		defer func() {
			activeWorkers.Wait()
			close(jobsOut)
		}()

		// Spawn a routine for each job
		for c := range jobsIn {
			activeWorkers.Add(1)
			go func(c rpc.NetworkClient) {
				defer activeWorkers.Done()

				// Limit the number of simultaneous requests
				workerSem <- struct{}{}
				defer func() { <-workerSem }()

				// Timeout after a second
				ctx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()

				info, err := c.NetInfo(ctx)
				if err == nil {
					// Send the response
					jobsOut <- info.Peers
					return
				}

				// There's no response, so decrement the counter
				activeJobs.Done()
				slog.InfoCtx(ctx, "Failed to query net info", "error", err)
			}(c)
		}
	}()

	seen := map[p2p.ID]bool{}
	doVisit := func(p coretypes.Peer) bool {
		// Only visit each peer once
		if seen[p.NodeInfo.ID()] {
			return true
		}

		seen[p.NodeInfo.ID()] = true
		d, ok := visit(ctx, p)

		// Stop?
		if !ok {
			return false
		}

		// Skip?
		if d == nil {
			return true
		}

		// Queue a job
		activeJobs.Add(1)
		jobsIn <- d
		return true
	}

	for peers := range jobsOut {
		// Visit all the peers
		for _, p := range peers {
			if !doVisit(p) {
				return
			}
		}

		// We got a response
		activeJobs.Done()
	}
}

// NewHTTPClient creates a new Tendermint RPC HTTP client for the given peer.
func NewHTTPClient(ctx context.Context, peer coretypes.Peer, offset config.PortOffset) (*http.HTTP, error) {
	// peer.node_info.listen_addr should include the Tendermint P2P port
	i := strings.IndexByte(peer.NodeInfo.ListenAddr, ':')
	if i < 0 {
		return nil, errors.New("invalid listen address")
	}

	// Convert the port number to a string
	port, err := strconv.ParseUint(peer.NodeInfo.ListenAddr[i+1:], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}

	// Calculate the RPC port from the P2P port
	port = port - uint64(config.PortOffsetTendermintP2P) + uint64(config.PortOffsetTendermintRpc) + uint64(offset)

	// Construct the address from peer.remote_ip and the calculated port
	addr := fmt.Sprintf("http://%s:%d", peer.RemoteIP, port)

	// Create a new client
	return http.New(addr, addr+"/ws")
}
