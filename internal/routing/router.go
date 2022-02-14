package routing

import (
	"context"
	"errors"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"strings"

	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var ErrUnknownSubnet = errors.New("unknown subnet")

// Router defines a basic interface for routing and sending transactions.
//
// TODO Route and Send should probably be handled separately.
type Router interface {
	Route(account *url.URL) (string, error)
	Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error)
	Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*ResponseSubmit, error)
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

// routeModulo routes an account using routingNumber modulo numberOfBvns to
// select a BVN.
func routeModulo(network *config.Network, account *url.URL) (string, error) {
	// Is it a DN URL?
	if protocol.BelongsToDn(account) {
		return protocol.Directory, nil
	}

	// Is it a BVN URL?
	if bvn, ok := protocol.ParseBvnUrl(account); ok {
		for _, id := range network.BvnNames {
			if strings.EqualFold(bvn, id) {
				return id, nil
			}
		}

		return "", fmt.Errorf("unknown BVN %q", bvn)
	}

	// Modulo routing
	i := account.Routing() % uint64(len(network.BvnNames))
	return network.BvnNames[i], nil
}

// submit calls the appropriate client method to submit a transaction.
func submit(ctx context.Context, connMgr connections.ConnectionManager, subnet string, tx []byte, async bool) (*ResponseSubmit, error) {
	var r1 *core.ResultBroadcastTx
	errorCnt := 0
	for {
		connCtx, err := connMgr.SelectConnection(subnet)
		if err != nil {
			return nil, err
		}

		if async {
			r1, err = connCtx.GetClient().BroadcastTxAsync(ctx, tx)
		} else {
			r1, err = connCtx.GetClient().BroadcastTxSync(ctx, tx)
		}
		if err == nil {
			r2 := new(ResponseSubmit)
			r2.Code = r1.Code
			r2.Data = r1.Data
			r2.Log = r1.Log
			r2.MempoolError = r1.MempoolError
			return r2, nil
		}

		// The API call failed, let's report that and try again, we get a client to another node within the subnet if available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

func submitPretend(ctx context.Context, connMgr connections.ConnectionManager, subnet string, tx []byte) (*ResponseSubmit, error) {
	var r1 *core.ResultCheckTx
	errorCnt := 0
	for {
		connCtx, err := connMgr.SelectConnection(subnet)
		if err != nil {
			return nil, err
		}
		r1, err = connCtx.GetClient().CheckTx(ctx, tx)
		if err == nil {
			r2 := new(ResponseSubmit)
			r2.Code = r1.Code
			r2.Data = r1.Data
			r2.Log = r1.Log
			r2.Info = r1.Info
			r2.MempoolError = r1.MempoolError
			return r2, nil
		}

		// The API call failed, let's report that and try again, we get a client to another node within the subnet if available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

// RPC sends transactions to remote nodes via RPC calls.
type RPC struct {
	*config.Network
	ConnectionManager connections.ConnectionManager
}

var _ Router = (*RPC)(nil)

// Route routes the account using modulo routing.
func (r *RPC) Route(account *url.URL) (string, error) {
	return routeModulo(r.Network, account)
}

// Query queries the specified subnet. If the subnet matches this
// network's ID, the transaction is broadcasted via the local client. Otherwise
// the transaction is broadcasted via an RPC client.
func (r *RPC) Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	errorCnt := 0
	for {
		connCtx, err := r.ConnectionManager.SelectConnection(subnet)
		if err != nil {
			return nil, err
		}
		result, err := connCtx.GetClient().ABCIQueryWithOptions(ctx, "", query, opts)
		if err == nil {
			return result, err
		}

		// The API call failed, let's report that and try again, we get a client to another node within the subnet if available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

// Submit submits the transaction to the specified subnet. If the subnet matches
// this network's ID, the transaction is broadcasted via the local client.
// Otherwise the transaction is broadcasted via an RPC client.
func (r *RPC) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*ResponseSubmit, error) {
	if pretend {
		return submitPretend(ctx, r.ConnectionManager, subnet, tx)
	} else {
		return submit(ctx, r.ConnectionManager, subnet, tx, async)
	}
}

// Direct sends transactions directly to a client.
type Direct struct {
	*config.Network
	ConnectionManager connections.ConnectionManager
}

var _ Router = (*Direct)(nil)

// Route routes the account using modulo routing.
func (r *Direct) Route(account *url.URL) (string, error) {
	return routeModulo(r.Network, account)
}

// Query sends the query to the specified subnet.
func (r *Direct) Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	errorCnt := 0
	for {
		connCtx, err := r.ConnectionManager.SelectConnection(subnet)
		if err != nil {
			return nil, err
		}

		result, err := connCtx.GetClient().ABCIQueryWithOptions(ctx, "", query, opts)
		if err == nil {
			return result, err
		}

		// The API call failed, let's report that and try again, we get a client to another node within the subnet if available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

// Submit sends the transaction to the specified subnet.
func (r *Direct) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*ResponseSubmit, error) {
	if pretend {
		return submitPretend(ctx, r.ConnectionManager, subnet, tx)
	} else {
		return submit(ctx, r.ConnectionManager, subnet, tx, async)
	}
}
