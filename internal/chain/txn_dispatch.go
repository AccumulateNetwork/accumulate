package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/rpc/client/http"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"
)

type txBatch []tm.Tx

// dispatcher is responsible for dispatching outgoing synthetic transactions to
// their recipients.
type dispatcher struct {
	ExecutorOptions
	localIndex  int
	isDirectory bool
	bvn         []*http.HTTP
	bvnBatches  []txBatch
	dn          *http.HTTP
	dnBatch     txBatch
	errg        *errgroup.Group
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) (*dispatcher, error) {
	d := new(dispatcher)
	d.ExecutorOptions = opts
	d.isDirectory = opts.Network.Type == config.Directory
	d.localIndex = -1
	d.bvn = make([]*http.HTTP, len(opts.Network.BvnNames))
	d.bvnBatches = make([]txBatch, len(opts.Network.BvnNames))

	// If we're not a directory, make an RPC client for the DN
	if !d.isDirectory && !d.IsTest {
		// Get the address of a directory node
		addr := opts.Network.AddressWithPortOffset(protocol.Directory, networks.TmRpcPortOffset)

		// Make the client
		var err error
		d.dn, err = http.New(addr)
		if err != nil {
			return nil, fmt.Errorf("could not create client for directory %q: %v", addr, err)
		}
	}

	// Make a client for all of the BVNs
	for i, id := range opts.Network.BvnNames {
		// Use the local client for ourself
		if id == opts.Network.ID {
			d.localIndex = i
			continue
		}

		// Get the BVN address
		addr := opts.Network.AddressWithPortOffset(id, networks.TmRpcPortOffset)

		// Make the client
		var err error
		d.bvn[i], err = http.New(addr)
		if err != nil {
			return nil, fmt.Errorf("could not create client for block validator %q: %v", id, err)
		}
	}

	return d, nil
}

// Reset creates new RPC client batches.
func (d *dispatcher) Reset(ctx context.Context) {
	d.errg = new(errgroup.Group)

	d.dnBatch = d.dnBatch[:0]
	for i, bv := range d.bvnBatches {
		d.bvnBatches[i] = bv[:0]
	}
}

// route gets the client for the URL
func (d *dispatcher) route(u *url.URL) (batch *txBatch, local bool) {
	// Is it a DN URL?
	if protocol.IsDnUrl(u) {
		if d.isDirectory {
			return nil, true
		}
		if d.dn == nil && !d.IsTest {
			panic("Directory was not configured")
		}
		return &d.dnBatch, false
	}

	// Is it a BVN URL?
	if bvn, ok := protocol.ParseBvnUrl(u); ok {
		for i, id := range d.Network.BvnNames {
			if strings.EqualFold(bvn, id) {
				if i == d.localIndex {
					// Use the local client for local requests
					return nil, true
				}

				return &d.bvnBatches[i], false
			}
		}

		// Is it OK to just route unknown BVNs normally?
	}

	// Modulo routing
	i := u.Routing() % uint64(len(d.bvn))
	if i == uint64(d.localIndex) {
		// Use the local client for local requests
		return nil, true
	}
	return &d.bvnBatches[i], false
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsync(ctx context.Context, u *url.URL, tx []byte) {
	batch, local := d.route(u)
	if local {
		d.BroadcastTxAsyncLocal(ctx, tx)
		return
	}

	*batch = append(*batch, tx)
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsyncLocal(ctx context.Context, tx []byte) {
	d.errg.Go(func() error {
		_, err := d.Local.BroadcastTxAsync(ctx, tx)
		return d.checkError(err)
	})
}

func (d *dispatcher) send(ctx context.Context, client *http.HTTP, batch txBatch) {
	switch len(batch) {
	case 0:
		// Nothing to do

	case 1:
		// Send single. Tendermint's batch RPC client is buggy - it breaks if
		// you don't give it more than one request.
		d.errg.Go(func() error {
			_, err := client.BroadcastTxAsync(ctx, batch[0])
			return d.checkError(err)
		})

	default:
		// Send batch
		d.errg.Go(func() error {
			b := client.NewBatch()
			for _, tx := range batch {
				_, err := b.BroadcastTxAsync(ctx, tx)
				if err != nil {
					return err
				}
			}
			_, err := b.Send(ctx)
			return d.checkError(err)
		})
	}
}

var errTxInCache = jrpc.RPCInternalError(jrpc.JSONRPCIntID(0), tm.ErrTxInCache).Error

// checkError returns nil if the error can be ignored.
func (*dispatcher) checkError(err error) error {
	if err == nil {
		return nil
	}

	// TODO This may be unnecessary once this issue is fixed:
	// https://github.com/tendermint/tendermint/issues/7185.

	// Is the error "tx already exists in cache"?
	if err.Error() == tm.ErrTxInCache.Error() {
		return nil
	}

	// Or RPC error "tx already exists in cache"?
	var rpcErr *jrpc.RPCError
	if errors.As(err, &rpcErr) && *rpcErr == *errTxInCache {
		return nil
	}

	// It's a real error
	return err
}

// Send sends all of the batches.
func (d *dispatcher) Send(ctx context.Context) error {
	// Send to the DN
	if d.dn != nil || !d.IsTest {
		d.send(ctx, d.dn, d.dnBatch)
	}

	// Send to the BVNs
	for i := range d.bvn {
		d.send(ctx, d.bvn[i], d.bvnBatches[i])
	}

	// Wait for everyone to finish
	return d.errg.Wait()
}
