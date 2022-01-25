package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"sync"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"golang.org/x/exp/rand"
	"golang.org/x/sync/errgroup"
	"log"
	"net/http"
)

type txBatch []byte

func (b *txBatch) Append(tx []byte) {
	*b = append(*b, tx...)
}

// dispatcher is responsible for dispatching outgoing synthetic transactions to
// their recipients.
type dispatcher struct {
	ExecutorOptions
	batches     map[connections.Route]txBatch
	dnBatch     txBatch
	errg        *errgroup.Group
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) *dispatcher {
	d := new(dispatcher)
	d.ExecutorOptions = opts
	d.batches = make(map[connections.Route]txBatch)
	return d
}

// Reset creates new RPC client batches.
func (d *dispatcher) Reset(ctx context.Context) {
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
	default:
		log.Println("*** Batch size before", len(*batch))
		batch.Append(tx)
		d.batches[route] = *batch
		log.Println("*** Appended tx to batch ", batch, " for subnet ", route.GetSubnetName(), "batch size", len(*batch))
		log.Println("*** Batch size before", len(d.batches[route]))
	}
	return nil
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsyncLocal(ctx context.Context, tx []byte) {
	d.errg.Go(func() error {
		_, err := d.Local.BroadcastTxAsync(ctx, tx)
		return d.checkError(err)
	})
}

func (d *dispatcher) send(ctx context.Context, route connections.Route, batch txBatch) {
	if len(batch) == 0 {
		return
	}

	// Tendermint's JSON RPC batch client is utter trash, so we're rolling our
	// own

	request := jsonrpc2.Request{
		Method: "broadcast_tx_async",
		Params: map[string]interface{}{"tx": batch},
		ID:     rand.Int()%5000 + 1,
	}

	d.errg.Go(func() error {
		data, err := json.Marshal(request)
		if err != nil {
			return err
		}

		httpReq, err := http.NewRequest(http.MethodPost, route.GetNodeUrl(), bytes.NewBuffer(data))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header.Add("Content-Type", "application/json")

		httpRes, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRes.Body.Close()

		response := new(jsonrpc2.Response)
		err = json.NewDecoder(httpRes.Body).Decode(&response)
		if err != nil {
			return err
		}

		if !response.HasError() {
			return nil
		}

		err = d.checkError(response.Error)
		if err != nil {
			return err
		}

		return nil
	})
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

// Send sends all of the batches.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	errs := make(chan error)
	wg := new(sync.WaitGroup)
	for route, batch := range d.batches {
		if len(batch) == 0 {
			continue
		}

		wg.Add(1)
		batch := batch // Don't capture loop variables
		go func() {
			defer wg.Done()
			err := d.send(ctx, route, batch)
			err = d.checkError(err)
			if err != nil {
				errs <- err
			}
		}()
	}

	go func() {
		wg.Wait()
		close(errs)
	}()

	d.Reset(ctx)
	return errs
}

func (d *dispatcher) getRouteAndBatch(u *url.URL) (connections.Route, *txBatch, error) {
	route, err := d.ConnectionRouter.SelectRoute(u, false)
	if err != nil {
		return nil, nil, err
	}

	if route.GetNetworkGroup() == connections.Local {
		return route, nil, nil
	}

	batch := d.batches[route]
	if batch == nil {
		batch = make(txBatch, 0)
		d.batches[route] = batch
	}
	return route, &batch, nil
}
