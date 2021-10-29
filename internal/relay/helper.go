package relay

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewClients(targetList ...string) ([]Client, error) {
	clients := []Client{}
	for _, nameOrIP := range targetList {
		net := networks.Networks[nameOrIP]
		ip, err := url.Parse(nameOrIP)
		if net != nil {
			ip = &url.URL{Scheme: "tcp", Host: fmt.Sprintf("%s:%d", net.Nodes[0].IP, net.Port+node.TmRpcPortOffset)}
		} else if err != nil {
			return nil, fmt.Errorf("%q is not a URL or a named network", nameOrIP)
		} else if ip.Port() == "" {
			return nil, fmt.Errorf("missing port number: %q", nameOrIP)
		} else if _, err := strconv.ParseInt(ip.Port(), 10, 17); err != nil {
			return nil, fmt.Errorf("invalid port number: %q", nameOrIP)
		}

		client, err := rpchttp.New(ip.String())
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
