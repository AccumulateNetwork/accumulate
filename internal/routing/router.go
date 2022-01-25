package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/rpc/client/http"
)

// Router defines a basic interface for routing and sending transactions.
//
// TODO Route and Send should probably be handled separately.
type Router interface {
	Route(account *url.URL) string
	Send(ctx context.Context, subnet string, tx []byte) error
}

// routeModulo routes an account using routingNumber modulo numberOfBvns to
// select a BVN.
func routeModulo(network *config.Network, account *url.URL) string {
	// Is it a DN URL?
	if protocol.IsDnUrl(account) {
		return protocol.Directory
	}

	// Is it a BVN URL?
	if bvn, ok := protocol.ParseBvnUrl(account); ok {
		for _, id := range network.BvnNames {
			if strings.EqualFold(bvn, id) {
				return id
			}
		}

		// Is it OK to just route unknown BVNs normally?
	}

	// Modulo routing
	i := account.Routing() % uint64(len(network.BvnNames))
	return network.BvnNames[i]
}

// RPC sends transactions to remote nodes via RPC calls.
type RPC struct {
	*config.Network
	Local api.ABCIBroadcastClient
}

var _ Router = (*RPC)(nil)

// Route routes the account using modulo routing.
func (r *RPC) Route(account *url.URL) string {
	return routeModulo(r.Network, account)
}

// Send broadcasts the transaction to the specified subnet. If the subnet
// matches this network's ID, the transaction is broadcasted via the local
// client. Otherwise the transaction is broadcasted via an RPC client.
func (r *RPC) Send(ctx context.Context, subnet string, tx []byte) error {
	if strings.EqualFold(r.ID, subnet) {
		_, err := r.Local.BroadcastTxAsync(ctx, tx)
		return err
	}

	// Viper always lower-cases map keys
	subnet = strings.ToLower(subnet)

	if len(r.Addresses[subnet]) == 0 {
		return fmt.Errorf("unknown subnet %q", subnet)
	}

	addr := r.Network.AddressWithPortOffset(subnet, networks.TmRpcPortOffset)
	client, err := http.New(addr)
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}

	_, err = client.BroadcastTxAsync(ctx, tx)
	return err
}

// Direct sends transactions directly to a client.
type Direct struct {
	*config.Network
	Clients map[string]api.ABCIBroadcastClient
}

var _ Router = (*Direct)(nil)

// Route routes the account using modulo routing.
func (r *Direct) Route(account *url.URL) string {
	return routeModulo(r.Network, account)
}

// Send sends the transaction to the specified subnet.
func (r *Direct) Send(ctx context.Context, subnet string, tx []byte) error {
	client, ok := r.Clients[subnet]
	if !ok {
		return fmt.Errorf("unknown subnet %q", subnet)
	}

	_, err := client.BroadcastTxAsync(ctx, tx)
	return err
}
