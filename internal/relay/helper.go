package relay

import (
	"github.com/AccumulateNetwork/accumulate/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewClients(local Client, targetList ...string) ([]Client, error) {
	clients := []Client{}
	for _, nameOrIP := range targetList {
		if nameOrIP == "local" || nameOrIP == "self" {
			if local == nil {
				panic("target is local but no local client was provided!")
			}
			clients = append(clients, local)
			continue
		}

		addr, err := networks.GetRpcAddr(nameOrIP)
		if err != nil {
			return nil, err
		}

		client, err := rpchttp.New(addr)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func NewWith(local Client, targetList ...string) (*Relay, error) {
	clients, err := NewClients(local, targetList...)
	if err != nil {
		return nil, err
	}

	txBouncer := New(clients...)
	return txBouncer, nil
}
