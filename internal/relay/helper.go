package relay

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/networks"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func NewWith(targetList ...string) (*Relay, error) {
	rpcClients := []*rpchttp.HTTP{}
	for _, nameOrIP := range targetList {
		net := networks.Networks[nameOrIP]
		ip, err := url.Parse(nameOrIP)
		if net != nil {
			ip = &url.URL{Scheme: "tcp", Host: fmt.Sprintf("%s:%d", net.Nodes[0].IP, net.Port+node.TmRpcPortOffset)}
		} else if err != nil {
			return nil, fmt.Errorf("%q is not a URL or a named network", nameOrIP)
		} else if ip.Port() == "" {
			return nil, fmt.Errorf("missing port number: %q", nameOrIP)
		} else if _, err := strconv.ParseInt(ip.Port(), 10, 16); err != nil {
			return nil, fmt.Errorf("invalid port number: %q", nameOrIP)
		}

		client, err := rpchttp.New(ip.String())
		if err != nil {
			return nil, err
		}
		rpcClients = append(rpcClients, client)
	}
	txBouncer := New(rpcClients...)
	return txBouncer, nil
}
