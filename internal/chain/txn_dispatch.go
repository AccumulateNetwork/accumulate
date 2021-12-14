package chain

import (
	"context"
	"errors"
	"github.com/AccumulateNetwork/accumulate/networks/connections"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/url"
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
	batches     map[connections.Route]txBatch
	errg        *errgroup.Group
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) (*dispatcher, error) {
	d := new(dispatcher)
	d.ExecutorOptions = opts
	d.isDirectory = opts.Network.Type == config.Directory
	d.localIndex = -1
	d.batches = make(map[connections.Route]txBatch)
	return d, nil
}

// Reset creates new RPC client batches.
func (d *dispatcher) Reset() {
	d.errg = new(errgroup.Group)
	for key := range d.batches {
		delete(d.batches, key)
	}
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsync(ctx context.Context, u *url.URL, tx []byte) error {
	route, batch, err := d.getRouteAndBatch(u)
	if err != nil {
		return err
	}
	switch route.GetNetworkGroup() {
	case connections.Local:
		d.BroadcastTxAsyncLocal(ctx, tx)

	}
	*batch = append(*batch, tx)
	return nil
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsyncLocal(ctx context.Context, tx []byte) {
	d.errg.Go(func() error {
		_, err := d.Local.BroadcastTxAsync(ctx, tx)
		return err
	})
}

func (d *dispatcher) send(ctx context.Context, client connections.BatchABCIBroadcastClient, batch txBatch) {
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
	for route, batch := range d.batches {
		if !route.IsDirectoryNode() || !d.IsTest { // TODO is this correct? The old condition "if d.dn != nil || !d.IsTest " seems wrong
			d.send(ctx, route.GetBatchBroadcastClient(), batch)
		}
	}

	// Wait for everyone to finish
	return d.errg.Wait()
}

func (d *dispatcher) getRouteAndBatch(u *url.URL) (connections.Route, *txBatch, error) {
	route, err := d.ConnectionRouter.SelectRoute(u.String(), false)
	if err != nil {
		return nil, nil, err
	}
	if route.GetNetworkGroup() == connections.Local {
		return route, nil, nil
	}

	var batch txBatch
	batch = d.batches[route]
	if batch == nil {
		batch = batch[:0]
		d.batches[route] = batch
	}
	return route, &batch, nil
}
