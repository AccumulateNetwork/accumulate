package networks

import (
	"fmt"

	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func MakeBouncer(networkList []int) *Bouncer {
	if len(networkList) > len(Networks) {
		return nil
	}

	rpcClients := []*rpchttp.HTTP{}
	for i := range networkList {
		j := networkList[i]
		lAddr := fmt.Sprintf("tcp://%s:%d", Networks[j].Ip[0], Networks[j].Port+1)
		client, err := rpchttp.New(lAddr)
		if err != nil {
			return nil
		}
		rpcClients = append(rpcClients, client)
	}
	txBouncer := NewBouncer(rpcClients)
	return txBouncer
}
