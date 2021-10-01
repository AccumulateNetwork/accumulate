package relay

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewWithNetworks(networkList ...string) (*Relay, error) {
	rpcClients := []*rpchttp.HTTP{}
	for _, name := range networkList {
		network := networks.Networks[name]
		if network == nil {
			return nil, fmt.Errorf("%q is not a named network", name)
		}
		lAddr := fmt.Sprintf("tcp://%s:%d", network.Nodes[0].IP, network.Port+1)
		client, err := rpchttp.New(lAddr)
		if err != nil {
			return nil, err
		}
		rpcClients = append(rpcClients, client)
	}
	txBouncer := New(rpcClients...)
	return txBouncer, nil
}
