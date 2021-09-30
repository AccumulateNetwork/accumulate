package relay

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewWithNetworks(networkList ...int) (*Relay, error) {
	rpcClients := []*rpchttp.HTTP{}
	for _, i := range networkList {
		lAddr := fmt.Sprintf("tcp://%s:%d", networks.Networks[i].Nodes[0].IP, networks.Networks[i].Port+1)
		client, err := rpchttp.New(lAddr)
		if err != nil {
			return nil, err
		}
		rpcClients = append(rpcClients, client)
	}
	txBouncer := New(rpcClients...)
	return txBouncer, nil
}
