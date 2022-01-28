package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	core "github.com/tendermint/tendermint/rpc/core/types"
	tm "github.com/tendermint/tendermint/types"
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

// Client is a subset of from TM/rpc/client.ABCIClient.
type Client interface {
	ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error)
	CheckTx(ctx context.Context, tx tm.Tx) (*core.ResultCheckTx, error)
	BroadcastTxAsync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
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
	if protocol.IsDnUrl(account) {
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
func submit(client Client, ctx context.Context, tx []byte, pretend, async bool) (*ResponseSubmit, error) {
	if pretend {
		r1, err := client.CheckTx(ctx, tx)
		if err != nil {
			return nil, err
		}

		r2 := new(ResponseSubmit)
		r2.Code = r1.Code
		r2.Data = r1.Data
		r2.Log = r1.Log
		r2.Info = r1.Info
		r2.MempoolError = r1.MempoolError
		return r2, nil
	}

	var r1 *core.ResultBroadcastTx
	var err error
	if async {
		r1, err = client.BroadcastTxAsync(ctx, tx)
	} else {
		r1, err = client.BroadcastTxSync(ctx, tx)
	}
	if err != nil {
		return nil, err
	}

	r2 := new(ResponseSubmit)
	r2.Code = r1.Code
	r2.Data = r1.Data
	r2.Log = r1.Log
	r2.MempoolError = r1.MempoolError
	return r2, nil
}

// RPC sends transactions to remote nodes via RPC calls.
type RPC struct {
	*config.Network
	Local Client
}

var _ Router = (*RPC)(nil)

// Route routes the account using modulo routing.
func (r *RPC) Route(account *url.URL) (string, error) {
	return routeModulo(r.Network, account)
}

func (r *RPC) getClient(subnet string) (Client, error) {
	if strings.EqualFold(r.ID, subnet) {
		return r.Local, nil
	}

	// Viper always lower-cases map keys
	subnet = strings.ToLower(subnet)

	if len(r.Addresses[subnet]) == 0 {
		return nil, fmt.Errorf("%w %q", ErrUnknownSubnet, subnet)
	}

	addr := r.Network.AddressWithPortOffset(subnet, networks.TmRpcPortOffset)
	client, err := http.New(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return client, nil
}

// Query queries the specified subnet. If the subnet matches this
// network's ID, the transaction is broadcasted via the local client. Otherwise
// the transaction is broadcasted via an RPC client.
func (r *RPC) Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	client, err := r.getClient(subnet)
	if err != nil {
		return nil, err
	}

	return client.ABCIQueryWithOptions(ctx, "", query, opts)
}

// Submit submits the transaction to the specified subnet. If the subnet matches
// this network's ID, the transaction is broadcasted via the local client.
// Otherwise the transaction is broadcasted via an RPC client.
func (r *RPC) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*ResponseSubmit, error) {
	client, err := r.getClient(subnet)
	if err != nil {
		return nil, err
	}

	return submit(client, ctx, tx, pretend, async)
}

// Direct sends transactions directly to a client.
type Direct struct {
	*config.Network
	Clients map[string]Client
}

var _ Router = (*Direct)(nil)

// Route routes the account using modulo routing.
func (r *Direct) Route(account *url.URL) (string, error) {
	return routeModulo(r.Network, account)
}

// Query sends the query to the specified subnet.
func (r *Direct) Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	client, ok := r.Clients[subnet]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownSubnet, subnet)
	}

	return client.ABCIQueryWithOptions(ctx, "", query, opts)
}

// Submit sends the transaction to the specified subnet.
func (r *Direct) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*ResponseSubmit, error) {
	client, ok := r.Clients[subnet]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownSubnet, subnet)
	}

	return submit(client, ctx, tx, pretend, async)
}
