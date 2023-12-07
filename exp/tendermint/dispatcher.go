// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/cometbft/cometbft/mempool"
	rpc "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	jrpc "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"golang.org/x/exp/slog"
)

type DispatcherClient interface {
	tm.SubmitClient
	rpc.NetworkClient
}

// dispatcher implements [block.Dispatcher].
type dispatcher struct {
	router routing.Router
	self   map[string]DispatcherClient
	queue  map[string][]*messaging.Envelope
}

var _ execute.Dispatcher = (*dispatcher)(nil)

// NewDispatcher creates a new dispatcher. NewDispatcher will panic if self does
// not include a client for the directory network.
func NewDispatcher(router routing.Router, self map[string]DispatcherClient) execute.Dispatcher {
	d := new(dispatcher)
	d.router = router
	d.queue = map[string][]*messaging.Envelope{}

	// Make sure the partition IDs are all lower-case
	d.self = make(map[string]DispatcherClient, len(self))
	for part, client := range self {
		d.self[strings.ToLower(part)] = client
	}

	// Sanity check
	if _, ok := d.self["directory"]; !ok {
		panic("dispatcher cannot function without a client for the directory")
	}
	return d
}

// Submit routes the account URL, constructs a multiaddr, and queues addressed
// submit requests.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	// If there's something wrong with the envelope, it's better for that error
	// to be logged closer to the source, at the sending side instead of the
	// receiving side
	_, err := env.Normalize()
	if err != nil {
		return err
	}

	// Route the account
	partition, err := d.router.RouteAccount(u)
	if err != nil {
		return err
	}

	// Queue the envelope (copy for insurance)
	partition = strings.ToLower(partition)
	d.queue[partition] = append(d.queue[partition], env.Copy())
	return nil
}

var errTxInCache1 = jrpc.RPCInternalError(jrpc.JSONRPCIntID(0), mempool.ErrTxInCache).Error
var errTxInCache2 = jsonrpc2.NewError(jsonrpc2.ErrorCode(errTxInCache1.Code), errTxInCache1.Message, errTxInCache1.Data)
var errTxInCacheAcc = jsonrpc2.NewError(v2.ErrCodeAccumulate, "Accumulate Error", errTxInCache1.Data)

// CheckDispatchError ignores errors we don't care about.
func CheckDispatchError(err error) error {
	if err == nil {
		return nil
	}

	// TODO This may be unnecessary once this issue is fixed:
	// https://github.com/tendermint/tendermint/issues/7185.

	// Is the error "tx already exists in cache"?
	if err.Error() == mempool.ErrTxInCache.Error() {
		return nil
	}

	// Or RPC error "tx already exists in cache"?
	var rpcErr1 *jrpc.RPCError
	if errors.As(err, &rpcErr1) && *rpcErr1 == *errTxInCache1 {
		return nil
	}

	var rpcErr2 jsonrpc2.Error
	if errors.As(err, &rpcErr2) && (rpcErr2 == errTxInCache2 || rpcErr2 == errTxInCacheAcc) {
		return nil
	}

	var errorsErr *errors.Error
	if errors.As(err, &errorsErr) {
		// This probably should not be necessary
		if errorsErr.Code == errors.Delivered {
			return nil
		}
	}

	// It's a real error
	return err
}

// Send sends all of the batches asynchronously using one connection per
// partition.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	queue := d.queue
	d.queue = make(map[string][]*messaging.Envelope, len(queue))

	// Run asynchronously
	errs := make(chan error)
	go d.send(ctx, queue, errs)

	// Let the caller wait for errors
	return errs
}

func (d *dispatcher) send(ctx context.Context, queue map[string][]*messaging.Envelope, errs chan<- error) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorCtx(ctx, "Panicked while dispatching", "error", r)
			mDispatchPanics.Inc()
		}
	}()

	mDispatchCalls.Inc()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer close(errs)

	// Scan Tendermint for nodes
	want := make(map[string]bool, len(queue))
	for part := range queue {
		want[part] = true
	}
	clients := d.getClients(ctx, want)

	check := func(err error, peer *coretypes.Peer) {
		err = CheckDispatchError(err)
		if err != nil {
			return
		}

		if peer == nil {
			errs <- errors.UnknownError.WithFormat("local: %w", err)
		} else {
			errs <- errors.UnknownError.WithFormat("peer %v (%v): %w", peer.NodeInfo.ID(), peer.RemoteIP, err)
		}
	}

	for part, queue := range queue {
		mDispatchEnvelopes.Add(float64(len(queue)))

		// Get a client for the partition
		client, ok := clients[part]
		if !ok {
			errs <- errors.InternalError.WithFormat("no client for %v", part)
			continue
		}

		// Submit envelopes directly via Tendermint RPC
		submitter := tm.NewSubmitter(tm.SubmitterParams{
			Local: client,
		})

		// Process the queue
		for _, env := range queue {
			subs, err := submitter.Submit(ctx, env, api.SubmitOptions{})
			if err != nil {
				mDispatchErrors.Inc()
				check(err, client.peer)
				continue
			}

			// Check for failed submissions
			for _, sub := range subs {
				if sub.Status != nil && sub.Status.Error != nil {
					check(sub.Status.AsError(), client.peer)
				}
			}
		}
	}
}

type peerClient struct {
	DispatcherClient
	peer *coretypes.Peer
}

// getClients returns a map of Tendermint RPC clients for the given partitions.
func (d *dispatcher) getClients(ctx context.Context, want map[string]bool) map[string]peerClient {
	clients := make(map[string]peerClient, len(want))

	// Prefer local clients
	for part, client := range d.self {
		clients[part] = peerClient{DispatcherClient: client}
		delete(want, part)
	}

	// Do we need any others?
	if len(want) == 0 {
		return clients
	}

	var scanned int
	start := time.Now()
	defer func() {
		mGetClientsScans.Inc()
		mGetClientsLatency.Set(time.Since(start).Seconds())
		mGetClientsWanted.Set(float64(len(want)))
		mGetClientsPeers.Set(float64(scanned))
	}()

	// Walk the directory for dual-mode nodes on the desired partitions
	WalkPeers(ctx, d.self["directory"], func(ctx context.Context, peer coretypes.Peer) (rpc.NetworkClient, bool) {
		scanned++

		// Create a client for the BVNN
		bvn, err := NewHTTPClient(ctx, peer, config.PortOffsetBlockValidator-config.PortOffsetDirectory)
		if err != nil {
			slog.ErrorCtx(ctx, "Failed to create client for peer (bvn)", "peer", peer.NodeInfo.ID(), "error", err)
			mGetClientsFailed.Inc()
			return nil, true
		}

		// Check which BVN its on
		st, err := bvn.Status(ctx)
		if err != nil {
			slog.ErrorCtx(ctx, "Failed to query BVN status", "peer", peer.NodeInfo.ID(), "error", err)
			mGetClientsFailed.Inc()
			return nil, true
		}

		// The Tendermint network ID amy be prefixed, e.g. Millau.BVN0
		part := st.NodeInfo.Network
		if i := strings.LastIndexByte(part, '.'); i >= 0 {
			part = part[i+1:]
		}

		// Do we want this partition?
		part = strings.ToLower(part)
		if want[part] {
			delete(want, part)
			clients[part] = peerClient{bvn, &peer}
		}

		// If we have everything we want, we're done
		if len(want) == 0 {
			return nil, false
		}

		// Scan the DNN's peers
		dir, err := NewHTTPClient(ctx, peer, 0)
		if err != nil {
			slog.ErrorCtx(ctx, "Failed to create client for peer (directory)", "peer", peer.NodeInfo.ID(), "error", err)
			mGetClientsFailed.Inc()
		}
		return dir, true
	})

	return clients
}
