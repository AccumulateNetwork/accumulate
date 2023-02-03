// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/mempool"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dispatcher implements [block.Dispatcher].
type dispatcher struct {
	router   routing.Router
	dialer   message.Dialer
	messages []message.Message
}

var _ execute.Dispatcher = (*dispatcher)(nil)

// newDispatcher creates a new dispatcher.
func newDispatcher(router routing.Router, dialer message.Dialer) *dispatcher {
	d := new(dispatcher)
	d.router = router
	d.dialer = dialer
	return d
}

// Submit routes the account URL, constructs a multiaddr, and queues addressed
// submit requests.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *protocol.Envelope) error {
	// Route the account
	partition, err := d.router.RouteAccount(u)
	if err != nil {
		return err
	}

	// Construct the multiaddr, /acc/submit:{partition}
	addr, err := multiaddr.NewComponent(api.N_ACC, (&api.ServiceAddress{Type: api.ServiceTypeSubmit, Partition: partition}).String())
	if err != nil {
		return err
	}

	// Convert the envelope into deliveries
	deliveries, err := messaging.NormalizeLegacy(env)
	if err != nil {
		return err
	}

	// Queue a pre-addressed message for each delivery
	for _, delivery := range deliveries {
		delivery := delivery.(*messaging.LegacyMessage)
		env := new(protocol.Envelope)
		env.Signatures = append(env.Signatures, delivery.Signatures...)
		env.Transaction = append(env.Transaction, delivery.Transaction)
		d.messages = append(d.messages, &message.Addressed{
			Address: addr,
			Message: &message.SubmitRequest{
				Envelope: env,
			},
		})
	}
	return nil
}

var errTxInCache1 = jrpc.RPCInternalError(jrpc.JSONRPCIntID(0), mempool.ErrTxInCache).Error
var errTxInCache2 = jsonrpc2.NewError(jsonrpc2.ErrorCode(errTxInCache1.Code), errTxInCache1.Message, errTxInCache1.Data)
var errTxInCacheAcc = jsonrpc2.NewError(v2.ErrCodeAccumulate, "Accumulate Error", errTxInCache1.Data)

// checkDispatchError ignores errors we don't care about.
func checkDispatchError(err error, errs chan<- error) {
	if err == nil {
		return
	}

	// TODO This may be unnecessary once this issue is fixed:
	// https://github.com/tendermint/tendermint/issues/7185.

	// Is the error "tx already exists in cache"?
	if err.Error() == mempool.ErrTxInCache.Error() {
		return
	}

	// Or RPC error "tx already exists in cache"?
	var rpcErr1 *jrpc.RPCError
	if errors.As(err, &rpcErr1) && *rpcErr1 == *errTxInCache1 {
		return
	}

	var rpcErr2 jsonrpc2.Error
	if errors.As(err, &rpcErr2) && (rpcErr2 == errTxInCache2 || rpcErr2 == errTxInCacheAcc) {
		return
	}

	var errorsErr *errors.Error
	if errors.As(err, &errorsErr) {
		// This probably should not be necessary
		if errorsErr.Code == errors.Delivered {
			return
		}
	}

	// It's a real error
	errs <- err
}

// Send sends all of the batches asynchronously using one connection per
// partition.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	messages := d.messages
	d.messages = nil

	// Run asynchronously
	errs := make(chan error)
	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer close(errs)

		// Create a client using a batch dialer, but DO NOT set the router - all
		// the messages are already addressed
		client := new(message.Client)
		client.Dialer = message.BatchDialer(ctx, d.dialer)

		// Submit all messages over a single stream
		err := client.RoundTrip(ctx, messages, func(res, req message.Message) error {
			_ = req // Ignore unused warning

			switch res := res.(type) {
			case *message.ErrorResponse:
				// Handle error
				checkDispatchError(res.Error, errs)
				return nil

			case *message.SubmitResponse:
				// Check for failed submissions
				for _, sub := range res.Value {
					if sub.Status != nil {
						checkDispatchError(sub.Status.AsError(), errs)
					}
				}
				return nil

			default:
				return errors.Conflict.WithFormat("invalid response: want %T, got %T", (*message.SubmitResponse)(nil), res)
			}
		})
		if err != nil {
			errs <- errors.UnknownError.WithFormat("send requests: %w", err)
		}
	}()

	// Let the caller wait for errors
	return errs
}

// dialer is a wrapper around the p2p node's dialer to account for the order of
// initialization.
type dialer struct {
	ready  chan struct{}
	dialer message.MultiDialer
}

var _ message.MultiDialer = (*dialer)(nil)

func (d *dialer) d() message.MultiDialer {
	<-d.ready // wait until ready
	return d.dialer
}

func (d *dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return d.d().Dial(ctx, addr)
}

func (d *dialer) BadDial(ctx context.Context, addr multiaddr.Multiaddr, stream message.Stream, err error) bool {
	return d.d().BadDial(ctx, addr, stream, err)
}
