package chain

import (
	"context"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/tendermint/tendermint/rpc/client/http"
	tm "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"
)

type txBatch []tm.Tx

// dispatcher is responsible for dispatching outgoing synthetic transactions to
// their recipients.
type dispatcher struct {
	isDirectory bool
	isTest      bool
	localIndex  int
	local       api.ABCIBroadcastClient
	bvn         []*http.HTTP
	bvnBatches  []txBatch
	dn          *http.HTTP
	dnBatch     txBatch
	errg        *errgroup.Group
}

// newDispatcher creates a new dispatcher.
func newDispatcher(opts ExecutorOptions) (*dispatcher, error) {
	d := new(dispatcher)
	d.isDirectory = opts.SubnetType == config.Directory
	d.isTest = opts.IsTest
	d.local = opts.Local
	d.localIndex = -1
	d.bvn = make([]*http.HTTP, len(opts.BlockValidators))
	d.bvnBatches = make([]txBatch, len(opts.BlockValidators))

	// If we're not a directory, make an RPC client for the DN
	if !d.isDirectory && opts.Directory != "" {
		// Parse the config entry
		addr, err := networks.GetRpcAddr(opts.Directory)
		if err != nil {
			return nil, fmt.Errorf("could not resolve directory %q: %v", opts.Directory, err)
		}

		// Make the client
		d.dn, err = http.New(addr)
		if err != nil {
			return nil, fmt.Errorf("could not create client for directory %q: %v", opts.Directory, err)
		}
	}

	// Make a client for all of the BVNs
	for i, bv := range opts.BlockValidators {
		// Use the local client for ourself
		if bv == "self" || bv == "local" {
			d.localIndex = i
			continue
		}

		// Parse the config entry
		addr, err := networks.GetRpcAddr(bv)
		if err != nil {
			return nil, fmt.Errorf("could not resolve block validator %q: %v", bv, err)
		}

		// Make the client
		d.bvn[i], err = http.New(addr)
		if err != nil {
			return nil, fmt.Errorf("could not create client for block validator %q: %v", bv, err)
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
	if dnUrl().Equal(u) {
		if d.isDirectory {
			return nil, true
		}
		if d.dn == nil && !d.isTest {
			panic("Directory was not configured")
		}
		return &d.dnBatch, false
	}

	// Is it a BVN URL?
	if isBvnUrl(u) {
		// For this we need some kind of lookup table that maps subnet to IP
		panic("Cannot route BVN ADI URLs")
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
		_, err := d.local.BroadcastTxAsync(ctx, tx)
		return err
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
			return err
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
			return err
		})
	}
}

// Send sends all of the batches.
func (d *dispatcher) Send(ctx context.Context) error {
	// Send to the DN
	if d.dn != nil || !d.isTest {
		d.send(ctx, d.dn, d.dnBatch)
	}

	// Send to the BVNs
	for i := range d.bvn {
		d.send(ctx, d.bvn[i], d.bvnBatches[i])
	}

	// Wait for everyone to finish
	return d.errg.Wait()
}
