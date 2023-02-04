// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"context"

	"github.com/tendermint/tendermint/libs/log"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/connections"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Router defines a basic interface for routing and sending transactions.
//
// TODO Route and Send should probably be handled separately.
type Router interface {
	RouteAccount(*url.URL) (string, error)
	Route(...*messaging.Envelope) (string, error)
	RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error
	Submit(ctx context.Context, partition string, tx *messaging.Envelope, pretend, async bool) (*ResponseSubmit, error)
}

// ResponseSubmit is the response from a call to Submit.
type ResponseSubmit struct {
	Code         uint32
	Data         []byte
	Log          string
	Info         string
	Codespace    string
	MempoolError string
}

// submit calls the appropriate client method to submit a transaction.
func submit(ctx context.Context, logger log.Logger, connMgr connections.ConnectionManager, partitionId string, tx []byte, async bool) (*ResponseSubmit, error) {
	var r1 *core.ResultBroadcastTx
	errorCnt := 0
	for {
		connCtx, err := connMgr.SelectConnection(partitionId, false)
		if err != nil {
			return nil, err
		}

		logger.Debug("Broadcasting a transaction", "partition", partitionId, "async", async, "address", connCtx.GetAddress(), "base-port", connCtx.GetBasePort(), "envelope", logging.AsHex(tx))
		if async {
			r1, err = connCtx.GetABCIClient().BroadcastTxAsync(ctx, tx)
		} else {
			r1, err = connCtx.GetABCIClient().BroadcastTxSync(ctx, tx)
		}
		if err == nil {
			r2 := new(ResponseSubmit)
			r2.Code = r1.Code
			r2.Data = r1.Data
			r2.Log = r1.Log
			return r2, nil
		}

		// The API call failed, let's report that and try again, we get a client to another node within the partition when available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

func submitPretend(ctx context.Context, logger log.Logger, connMgr connections.ConnectionManager, partition string, tx []byte) (*ResponseSubmit, error) {
	var r1 *core.ResultCheckTx
	errorCnt := 0
	for {
		connCtx, err := connMgr.SelectConnection(partition, false)
		if err != nil {
			return nil, err
		}

		logger.Debug("Checking a transaction", "partition", partition, "address", connCtx.GetAddress(), "base-port", connCtx.GetBasePort(), "envelope", logging.AsHex(tx))
		r1, err = connCtx.GetABCIClient().CheckTx(ctx, tx)
		if err == nil {
			r2 := new(ResponseSubmit)
			r2.Code = r1.Code
			r2.Data = r1.Data
			r2.Log = r1.Log
			r2.Info = r1.Info
			r2.MempoolError = r1.MempoolError
			return r2, nil
		}

		// The API call failed, let's report that and try again, we get a client to another node within the partition when available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

// RouterInstance sends transactions to remote nodes via RPC calls.
type RouterInstance struct {
	tree              *RouteTree
	connectionManager connections.ConnectionManager
	logger            logging.OptionalLogger
}

func NewRouter(eventBus *events.Bus, cm connections.ConnectionManager, logger log.Logger) *RouterInstance {
	r := new(RouterInstance)
	r.connectionManager = cm
	if logger != nil {
		r.logger.L = logger.With("module", "router")
	}

	events.SubscribeSync(eventBus, func(e events.WillChangeGlobals) error {
		r.logger.Debug("Loading new routing table", "table", e.New.Routing)

		tree, err := NewRouteTree(e.New.Routing)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		r.tree = tree
		return nil
	})

	return r
}

// NewStaticRouter returns a router that uses a static routing table
func NewStaticRouter(table *protocol.RoutingTable, cm connections.ConnectionManager, logger log.Logger) (*RouterInstance, error) {
	tree, err := NewRouteTree(table)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	r := new(RouterInstance)
	r.tree = tree
	r.connectionManager = cm
	if logger != nil {
		r.logger.L = logger.With("module", "router")
	}
	return r, nil
}

var _ Router = (*RouterInstance)(nil)

// RouteMessages routes the message bundle based on the routing location of its
// signatures. RouteMessages will return an error if the message bundle does not
// contain any signatures. RouteMessages will return an error if the message
// bundle has signatures with conflicting routing locations.
func RouteMessages(r Router, messages []messaging.Message) (string, error) {
	if len(messages) == 0 {
		return "", errors.BadRequest.With("nothing to route")
	}

	var route string
	for _, msg := range messages {
		err := routeMessage(r.RouteAccount, &route, msg)
		if err != nil {
			return "", errors.UnknownError.With("cannot route message(s): %w", err)
		}
	}
	if route == "" {
		return "", errors.BadRequest.With("nothing to route")
	}

	return route, nil
}

// RouteEnvelopes routes envelopes based on the routing location of their
// signatures. RouteEnvelopes will return an error if the envelopes do not
// contain any signatures. RouteEnvelopes will return an error if the message
// bundle has signatures with conflicting routing locations.
func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*messaging.Envelope) (string, error) {
	if len(envs) == 0 {
		return "", errors.BadRequest.With("nothing to route")
	}

	var route string
	for _, env := range envs {
		for _, msg := range env.Messages {
			err := routeMessage(routeAccount, &route, msg)
			if err != nil {
				return "", errors.UnknownError.With("cannot route message(s): %w", err)
			}
		}
		for _, sig := range env.Signatures {
			err := routeMessage(routeAccount, &route, &messaging.UserSignature{Signature: sig})
			if err != nil {
				return "", errors.UnknownError.With("cannot route message(s): %w", err)
			}
		}
	}
	if route == "" {
		return "", errors.BadRequest.With("nothing to route")
	}

	return route, nil
}

func routeMessage(routeAccount func(*url.URL) (string, error), route *string, msg messaging.Message) error {
	var r string
	var err error
	switch msg := msg.(type) {
	case *messaging.UserSignature:
		r, err = routeAccount(msg.Signature.RoutingLocation())

	case *messaging.SyntheticMessage:
		switch msg := msg.Message.(type) {
		case *messaging.UserTransaction:
			r, err = routeAccount(msg.Transaction.Header.Destination)

		default:
			return nil
		}

	case *messaging.UserTransaction:
		if msg.Transaction.Header.Destination == nil {
			return nil
		}
		r, err = routeAccount(msg.Transaction.Header.Destination)

	default:
		return nil
	}
	if err != nil {
		return err
	}

	if *route == "" {
		*route = r
		return nil
	}

	if *route != r {
		return errors.BadRequest.With("conflicting routes")
	}
	return nil
}

func (r *RouterInstance) RouteAccount(account *url.URL) (string, error) {
	if r.tree == nil {
		return "", errors.InternalError.With("the routing table has not been initialized")
	}
	if protocol.IsUnknown(account) {
		return "", errors.BadRequest.With("URL is unknown, cannot route")
	}

	route, err := r.tree.Route(account)
	if err != nil {
		r.logger.Debug("Failed to route", "account", account, "error", err)
		return "", errors.UnknownError.Wrap(err)
	}

	r.logger.Debug("Routing", "account", account, "to", route)
	return route, nil
}

// Route routes the account using modulo routing.
func (r *RouterInstance) Route(envs ...*messaging.Envelope) (string, error) {
	return RouteEnvelopes(r.RouteAccount, envs...)
}

func (r *RouterInstance) RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error {
	errorCnt := 0
	for {
		connCtx, err := r.connectionManager.SelectConnection(partitionId, true)
		if err != nil {
			return err
		}
		if connCtx == nil {
			return errors.InternalError.With("connCtx is nil")
		}
		client := connCtx.GetAPIClient()
		if client == nil {
			return errors.InternalError.With("connCtx.client is nil")
		}

		err = client.RequestAPIv2(ctx, method, params, result)
		if err == nil {
			return nil
		}

		// The API call failed, let's report that and try again, we get a client to another node within the partition if available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return err
		}
	}
}

// Submit submits the transaction to the specified partition. If the partition matches
// this network's ID, the transaction is broadcasted via the local client.
// Otherwise the transaction is broadcasted via an RPC client.
func (r *RouterInstance) Submit(ctx context.Context, partitionId string, tx *messaging.Envelope, pretend, async bool) (*ResponseSubmit, error) {
	raw, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if pretend {
		return submitPretend(ctx, r.logger, r.connectionManager, partitionId, raw)
	} else {
		return submit(ctx, r.logger, r.connectionManager, partitionId, raw, async)
	}
}
