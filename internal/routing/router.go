package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Router defines a basic interface for routing and sending transactions.
//
// TODO Route and Send should probably be handled separately.
type Router interface {
	RouteAccount(*url.URL) (string, error)
	Route(...*protocol.Envelope) (string, error)
	Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error)
	Submit(ctx context.Context, subnet string, tx *protocol.Envelope, pretend, async bool) (*ResponseSubmit, error)
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
	bvnNames := network.GetBvnNames()
	if bvn, ok := protocol.ParseBvnUrl(account); ok {
		for _, id := range bvnNames {
			if strings.EqualFold(bvn, id) {
				return id, nil
			}
		}

		return "", fmt.Errorf("unknown BVN %q", bvn)
	}

	// Modulo routing
	i := account.Routing() % uint64(len(bvnNames))
	return bvnNames[i], nil
}

// submit calls the appropriate client method to submit a transaction.
func submit(ctx context.Context, connMgr connections.ConnectionManager, subnetId string, tx []byte, async bool) (*ResponseSubmit, error) {
	var r1 *core.ResultBroadcastTx
	errorCnt := 0
	for {
		connCtx, err := connMgr.SelectConnection(subnetId, false)
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

		// The API call failed, let's report that and try again, we get a client to another node within the subnet when available
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
		connCtx, err := connMgr.SelectConnection(subnet, false)
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

		// The API call failed, let's report that and try again, we get a client to another node within the subnet when available
		connCtx.ReportError(err)
		errorCnt++
		if errorCnt > 1 {
			return nil, err
		}
	}
}

// RouterInstance sends transactions to remote nodes via RPC calls.
type RouterInstance struct {
	*config.Network
	ConnectionManager connections.ConnectionManager
}

var _ Router = (*RouterInstance)(nil)

func RouteAccount(net *config.Network, account *url.URL) (string, error) {
	return routeModulo(net, account)
}

func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*protocol.Envelope) (string, error) {
	if len(envs) == 0 {
		return "", errors.New("nothing to route")
	}

	var route string
	for _, env := range envs {
		if len(env.Signatures) == 0 {
			return "", errors.New("cannot route envelope: no signatures")
		}
		for _, sig := range env.Signatures {
			sigRoute, err := routeAccount(sig.GetSigner())
			if err != nil {
				return "", err
			}

			if route == "" {
				route = sigRoute
				continue
			}

			if route != sigRoute {
				return "", errors.New("cannot route envelope(s): conflicting routes")
			}
		}
	}

	return route, nil
}

func (r *RouterInstance) RouteAccount(account *url.URL) (string, error) {
	return RouteAccount(r.Network, account)
}

// Route routes the account using modulo routing.
func (r *RouterInstance) Route(envs ...*protocol.Envelope) (string, error) {
	return RouteEnvelopes(r.RouteAccount, envs...)
}

// Query queries the specified subnet. If the subnet matches this
// network's ID, the transaction is broadcasted via the local client. Otherwise
// the transaction is broadcasted via an RPC client.
func (r *RouterInstance) Query(ctx context.Context, subnetId string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	errorCnt := 0
	for {
		connCtx, err := r.ConnectionManager.SelectConnection(subnetId, true)
		if err != nil {
			return nil, err
		}
		if connCtx == nil {
			return nil, errors.New("connCtx is nil")
		}
		client := connCtx.GetClient()
		if client == nil {
			return nil, errors.New("connCtx.client is nil")
		}

		result, err := client.ABCIQueryWithOptions(ctx, "", query, opts)
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
func (r *RouterInstance) Submit(ctx context.Context, subnetId string, tx *protocol.Envelope, pretend, async bool) (*ResponseSubmit, error) {
	raw, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if pretend {
		return submitPretend(ctx, r.ConnectionManager, subnetId, raw)
	} else {
		return submit(ctx, r.ConnectionManager, subnetId, raw, async)
	}
}
