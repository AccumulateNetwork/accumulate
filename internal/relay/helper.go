package relay

import (
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewClients(targetList ...string) ([]Client, error) {
	clients := []Client{}
	for _, nameOrIP := range targetList {
		addr, err := networks.GetRpcAddr(nameOrIP, node.TmRpcPortOffset)
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

func NewWith(targetList ...string) (*Relay, error) {
	clients, err := NewClients(targetList...)
	if err != nil {
		return nil, err
	}

	txBouncer := New(clients...)
	return txBouncer, nil
}
