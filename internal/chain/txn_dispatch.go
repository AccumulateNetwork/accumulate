package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"
)

type txBatch []byte

func (b *txBatch) Append(tx []byte) {
	*b = append(*b, tx...)
}

// dispatcher is responsible for dispatching outgoing synthetic transactions to
// their recipients.
type dispatcher struct {
	ExecutorOptions
	localIndex  int
	isDirectory bool
	localBatch  txBatch
	bvn         []string
	bvnBatches  []txBatch
	dn          string
	dnBatch     txBatch
	errg        *errgroup.Group
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) *dispatcher {
	d := new(dispatcher)
	d.ExecutorOptions = opts
	d.isDirectory = opts.Network.Type == config.Directory
	d.localIndex = -1
	d.bvn = make([]string, len(opts.Network.BvnNames))
	d.bvnBatches = make([]txBatch, len(opts.Network.BvnNames))

	// If we're not a directory, make an RPC client for the DN
	if !d.isDirectory && !d.IsTest {
		// Get the address of a directory node
		d.dn = opts.Network.AddressWithPortOffset(protocol.Directory, networks.TmRpcPortOffset)
	}

	// Make a client for all of the BVNs
	for i, id := range opts.Network.BvnNames {
		// Use the local client for ourself
		if id == opts.Network.ID {
			d.localIndex = i
			continue
		}

		// Get the BVN address
		d.bvn[i] = opts.Network.AddressWithPortOffset(id, networks.TmRpcPortOffset)
	}

	return d
}

// Reset creates new RPC client batches.
func (d *dispatcher) Reset(ctx context.Context) {
	d.errg = new(errgroup.Group)

	d.localBatch = d.localBatch[:0]
	d.dnBatch = d.dnBatch[:0]
	for i, bv := range d.bvnBatches {
		d.bvnBatches[i] = bv[:0]
	}
}

// route gets the client for the URL
func (d *dispatcher) route(u *url.URL) *txBatch {
	// Is it a DN URL?
	if protocol.IsDnUrl(u) {
		if d.isDirectory {
			return &d.localBatch
		}
		if d.dn == "" && !d.IsTest {
			panic("Directory was not configured")
		}
		return &d.dnBatch
	}

	// Is it a BVN URL?
	if bvn, ok := protocol.ParseBvnUrl(u); ok {
		for i, id := range d.Network.BvnNames {
			if strings.EqualFold(bvn, id) {
				if i == d.localIndex {
					// Use the local client for local requests
					return nil
				}

				return &d.bvnBatches[i]
			}
		}

		// Is it OK to just route unknown BVNs normally?
	}

	// Modulo routing
	i := u.Routing() % uint64(len(d.bvn))
	if i == uint64(d.localIndex) {
		// Use the local client for local requests
		return &d.localBatch
	}
	return &d.bvnBatches[i]
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsync(ctx context.Context, u *url.URL, tx []byte) {
	d.route(u).Append(tx)
}

// BroadcastTxAsync dispatches the txn to the appropriate client.
func (d *dispatcher) BroadcastTxAsyncLocal(ctx context.Context, tx []byte) {
	d.localBatch.Append(tx)
}

func (d *dispatcher) send(ctx context.Context, server string, batch txBatch) {
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

		httpReq, err := http.NewRequest(http.MethodPost, server, bytes.NewBuffer(data))
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
func (d *dispatcher) Send(ctx context.Context) error {
	// Send local
	if d.localIndex >= 0 && len(d.localBatch) > 0 {
		d.errg.Go(func() error {
			_, err := d.Local.BroadcastTxAsync(ctx, tm.Tx(d.localBatch))
			return d.checkError(err)
		})
	}

	// Send to the DN
	if d.dn != "" || !d.IsTest {
		d.send(ctx, d.dn, d.dnBatch)
	}

	// Send to the BVNs
	for i := range d.bvn {
		d.send(ctx, d.bvn[i], d.bvnBatches[i])
	}

	// Wait for everyone to finish
	return d.errg.Wait()
}
