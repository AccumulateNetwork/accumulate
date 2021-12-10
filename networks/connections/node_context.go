package connections

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"net"
)

type NodeStatus int

const (
	Up           NodeStatus = iota // Healthy & ready to go
	Down                           // Not reachable
	OutOfService                   // Reachable but not ready to go (IE. still syncing up)
)

type NetworkGroup int

const (
	Local NetworkGroup = iota
	SameSubnet
	OtherSubnet
)

type nodeContext struct {
	subnetName    string
	address       string
	netType       config.NetworkType
	nodeType      config.NodeType
	networkGroup  NetworkGroup
	resolvedIPs   []net.IP
	metrics       nodeMetrics
	queryClient   api.ABCIQueryClient
	rpcHttpClient *rpchttp.HTTP
	localClient   *local.Local
}

func (n nodeContext) GetSubnetName() string {
	return n.subnetName
}

func (n nodeContext) GetNetworkGroup() NetworkGroup {
	return n.networkGroup
}

func (n nodeContext) GetRpcHttpClient() *rpchttp.HTTP {
	return n.rpcHttpClient
}

func (n nodeContext) GetQueryClient() api.ABCIQueryClient {
	return n.queryClient
}

func (n nodeContext) GetBroadcastClient() api.ABCIBroadcastClient {
	return n.localClient
}
