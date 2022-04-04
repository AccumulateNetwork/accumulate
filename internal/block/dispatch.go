package block

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type txBatch []byte

func (b *txBatch) Append(tx []byte) {
	*b = append(*b, tx...)
}

// dispatcher is responsible for dispatching outgoing synthetic transactions to
// their recipients.
type dispatcher struct {
	ExecutorOptions
	isDirectory bool
	batches     map[string]txBatch
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) *dispatcher {
	d := new(dispatcher)
	d.ExecutorOptions = opts
	d.isDirectory = opts.Network.Type == config.Directory
	d.batches = map[string]txBatch{}
	return d
}

// BroadcastTx dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTx(ctx context.Context, u *url.URL, tx []byte) error {
	subnet, err := d.Router.RouteAccount(u)
	if err != nil {
		return err
	}

	d.batches[subnet] = append(d.batches[subnet], tx...)
	return nil
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxLocal(ctx context.Context, tx []byte) {
	subnet := d.Network.LocalSubnetID
	d.batches[subnet] = append(d.batches[subnet], tx...)
}

var errTxInCache1 = jrpc.RPCInternalError(jrpc.JSONRPCIntID(0), tm.ErrTxInCache).Error
var errTxInCache2 = jsonrpc2.NewError(jsonrpc2.ErrorCode(errTxInCache1.Code), errTxInCache1.Message, errTxInCache1.Data)

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
	var rpcErr1 *jrpc.RPCError
	if errors.As(err, &rpcErr1) && *rpcErr1 == *errTxInCache1 {
		return nil
	}

	var rpcErr2 jsonrpc2.Error
	if errors.As(err, &rpcErr2) && rpcErr2 == errTxInCache2 {
		return nil
	}

	// It's a real error
	return err
}

// Send sends all of the batches asynchronously.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	errs := make(chan error)
	wg := new(sync.WaitGroup)

	// Send transactions to each destination in parallel
	for subnet, batch := range d.batches {
		if len(batch) == 0 {
			continue
		}

		wg.Add(1)
		subnet, batch := subnet, batch // Don't capture loop variables
		go func() {
			defer wg.Done()
			resp, err := d.Router.Submit(ctx, subnet, batch, false, false)
			err = d.checkError(err)
			if err != nil {
				errs <- err
				return
			}

			if resp == nil {
				//Guard put here to prevent nil responses.
				errs <- fmt.Errorf("nil response returned from router in transaction dispatcher")
				return
			}

			// Parse the results
			rset := new(protocol.TransactionResultSet)
			err = rset.UnmarshalBinary(resp.Data)
			if err != nil {
				errs <- err
				return
			}

			for _, r := range rset.Results {
				if r.Code != 0 {
					errs <- errors.New(r.Message)
				}
			}
		}()
	}

	// Once all transactions are sent, close the error channel
	go func() {
		wg.Wait()
		close(errs)
	}()

	// Reset the batches, optimized by the compiler
	for subnet := range d.batches {
		delete(d.batches, subnet)
	}

	// Let the caller wait for errors
	return errs
}
