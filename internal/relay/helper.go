package relay

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewWithNetworks(networkList ...int) *Relay {
	if len(networkList) > len(networks.Networks) {
		return nil
	}

	rpcClients := []*rpchttp.HTTP{}
	for i := range networkList {
		j := networkList[i]
		lAddr := fmt.Sprintf("tcp://%s:%d", networks.Networks[j].Ip[0], networks.Networks[j].Port+1)
		client, err := rpchttp.New(lAddr, "/websocket")
		if err != nil {
			return nil
		}
		rpcClients = append(rpcClients, client)
	}
	txBouncer := New(rpcClients...)
	return txBouncer
}
