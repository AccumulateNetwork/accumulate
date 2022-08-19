package block

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
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
	d.isDirectory = opts.Describe.NetworkType == config.Directory
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
	return d.push(d.Describe.PartitionId, tx)
}

var errTxInCache1 = jrpc.RPCInternalError(jrpc.JSONRPCIntID(0), tm.ErrTxInCache).Error
var errTxInCache2 = jsonrpc2.NewError(jsonrpc2.ErrorCode(errTxInCache1.Code), errTxInCache1.Message, errTxInCache1.Data)

type txnDispatchError struct {
	typ    protocol.TransactionType
	status *protocol.TransactionStatus
}

func (e *txnDispatchError) Error() string {
	return e.status.Error.Error()
}

func (e *txnDispatchError) Unwrap() error {
	return e.status.Error
}

func (d *dispatcher) Reset() {
	for key := range d.batches {
		d.batches[key] = d.batches[key][:0]
	}
}

// Send sends all of the batches asynchronously.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	errs := make(chan error)
	wg := new(sync.WaitGroup)

	// Send transactions to each destination in parallel
	for partition, envelopes := range d.batches {
		if len(envelopes) == 0 {
			continue
		}

		batch := make(jsonrpc2.BatchRequest, 0, len(envelopes))
		types := make(map[[32]byte]protocol.TransactionType, len(envelopes))
		for i, envelope := range envelopes {
			for _, tx := range envelope.Transaction {
				types[tx.ID().Hash()] = tx.Body.Type()
			}

			batch = append(batch, jsonrpc2.Request{
				ID:     i + 1,
				Method: "execute-local",
				Params: &api.ExecuteRequest{Envelope: envelope},
			})
		}

		wg.Add(1)
		partition := partition // Don't capture loop variables
		go func() {
			defer wg.Done()

			var resp []*api.TxResponse
			err := d.Router.RequestAPIv2(ctx, partition, "", batch, &resp)
			if err != nil {
				errs <- err
				return
			}

			for _, resp := range resp {
				if resp == nil {
					//Guard put here to prevent nil responses.
					errs <- fmt.Errorf("nil response returned from router in transaction dispatcher")
					return
				}

				// Parse the results
				data, err := json.Marshal(resp.Result)
				if err != nil {
					errs <- err
					return
				}

				var results []*protocol.TransactionStatus
				if json.Unmarshal(data, &results) != nil {
					results = []*protocol.TransactionStatus{new(protocol.TransactionStatus)}
					if json.Unmarshal(data, results[0]) != nil {
						errs <- fmt.Errorf("failed to unmarshal response for %v", resp.Txid)
						return
					}
				}

				for _, r := range results {
					if r.Error != nil {
						errs <- &txnDispatchError{types[r.TxID.Hash()], r}
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
