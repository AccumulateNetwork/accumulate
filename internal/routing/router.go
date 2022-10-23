// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"context"

	"github.com/tendermint/tendermint/libs/log"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Router defines a basic interface for routing and sending transactions.
//
// TODO Route and Send should probably be handled separately.
type Router interface {
	RouteAccount(*url.URL) (string, error)
	Route(...*protocol.Envelope) (string, error)
	RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error
	Submit(ctx context.Context, partition string, tx *protocol.Envelope, pretend, async bool) (*ResponseSubmit, error)
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
			return errors.Wrap(errors.StatusUnknownError, err)
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
		return nil, errors.Wrap(errors.StatusUnknownError, err)
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

func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*protocol.Envelope) (string, error) {
	if len(envs) == 0 {
		return "", errors.New(errors.StatusBadRequest, "nothing to route")
	}

	var route string
	for _, env := range envs {
		if len(env.Signatures) == 0 {
			return "", errors.New(errors.StatusBadRequest, "cannot route envelope: no signatures")
		}
		for _, sig := range env.Signatures {
			sigRoute, err := routeAccount(sig.RoutingLocation())
			if err != nil {
				return "", err
			}

			if route == "" {
				route = sigRoute
				continue
			}

			if route != sigRoute {
				return "", errors.New(errors.StatusBadRequest, "cannot route envelope(s): conflicting routes")
			}
		}
	}

	return route, nil
}

func (r *RouterInstance) RouteAccount(account *url.URL) (string, error) {
	if r.tree == nil {
		return "", errors.New(errors.StatusInternalError, "the routing table has not been initialized")
	}
	if protocol.IsUnknown(account) {
		return "", errors.New(errors.StatusBadRequest, "URL is unknown, cannot route")
	}

	route, err := r.tree.Route(account)
	if err != nil {
		r.logger.Debug("Failed to route", "account", account, "error", err)
		return "", errors.Wrap(errors.StatusUnknownError, err)
	}

	r.logger.Debug("Routing", "account", account, "to", route)
	return route, nil
}

// Route routes the account using modulo routing.
func (r *RouterInstance) Route(envs ...*protocol.Envelope) (string, error) {
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
			return errors.New(errors.StatusInternalError, "connCtx is nil")
		}
		client := connCtx.GetAPIClient()
		if client == nil {
			return errors.New(errors.StatusInternalError, "connCtx.client is nil")
		}

		err = client.RequestAPIv2(ctx, method, params, result)
		if err == nil {
			return err
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
func (r *RouterInstance) Submit(ctx context.Context, partitionId string, tx *protocol.Envelope, pretend, async bool) (*ResponseSubmit, error) {
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
