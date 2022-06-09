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
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dispatcher is responsible for dispatching outgoing synthetic transactions to
// their recipients.
type dispatcher struct {
	ExecutorOptions
	isDirectory bool
	batches     map[string][]*protocol.Envelope
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) *dispatcher {
	d := new(dispatcher)
	d.ExecutorOptions = opts
	d.isDirectory = opts.Network.Type == config.Directory
	d.batches = map[string][]*protocol.Envelope{}
	return d
}

func (d *dispatcher) push(partition string, env *protocol.Envelope) error {
	deliveries, err := chain.NormalizeEnvelope(env)
	if err != nil {
		return err
	}

	batch := d.batches[partition]
	for _, delivery := range deliveries {
		env := new(protocol.Envelope)
		env.Signatures = append(env.Signatures, delivery.Signatures...)
		env.Transaction = append(env.Transaction, delivery.Transaction)
		batch = append(batch, env)
	}
	d.batches[partition] = batch
	return nil
}

// BroadcastTx dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTx(ctx context.Context, u *url.URL, tx *protocol.Envelope) error {
	partition, err := d.Router.RouteAccount(u)
	if err != nil {
		return err
	}

	return d.push(partition, tx)
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxLocal(ctx context.Context, tx *protocol.Envelope) error {
	return d.push(d.Network.LocalPartitionID, tx)
}

var errTxInCache1 = jrpc.RPCInternalError(jrpc.JSONRPCIntID(0), tm.ErrTxInCache).Error
var errTxInCache2 = jsonrpc2.NewError(jsonrpc2.ErrorCode(errTxInCache1.Code), errTxInCache1.Message, errTxInCache1.Data)

// Send sends all of the batches asynchronously.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	errs := make(chan error)
	wg := new(sync.WaitGroup)

	// Send transactions to each destination in parallel
	for partition, batch := range d.batches {
		if len(batch) == 0 {
			continue
		}

		wg.Add(1)
		partition, batch := partition, batch // Don't capture loop variables
		go func() {
			defer wg.Done()
			for _, tx := range batch {

				resp, err := d.Router.Submit(ctx, partition, tx, false, false)
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
					if r.Error != nil {
						errs <- r.Error
					} else if r.Code != 0 {
						errs <- protocol.NewError(protocol.ErrorCode(r.Code), errors.New(r.Message))
					}
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
	for partition := range d.batches {
		delete(d.batches, partition)
	}

	// Let the caller wait for errors
	return errs
}
