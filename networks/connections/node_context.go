package connections

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/ybbus/jsonrpc/v2"
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
	subnetName           string
	address              string
	netType              config.NetworkType
	nodeType             config.NodeType
	networkGroup         NetworkGroup
	resolvedIPs          []net.IP
	metrics              nodeMetrics
	queryClient          ABCIQueryClient
	broadcastClient      ABCIBroadcastClient
	batchBroadcastClient BatchABCIBroadcastClient
	jsonRpcClient        jsonrpc.RPCClient
	lastError            error
}

func (n nodeContext) GetSubnetName() string {
	return n.subnetName
}

func (n nodeContext) GetNetworkGroup() NetworkGroup {
	return n.networkGroup
}

func (n nodeContext) IsDirectoryNode() bool {
	return n.netType == config.Directory && n.nodeType == config.Validator
}

func (n nodeContext) GetJsonRpcClient() jsonrpc.RPCClient {
	return n.jsonRpcClient
}

func (n nodeContext) GetQueryClient() ABCIQueryClient {
	return n.queryClient
}

func (n nodeContext) GetBroadcastClient() ABCIBroadcastClient {
	return n.broadcastClient
}

func (n nodeContext) GetBatchBroadcastClient() BatchABCIBroadcastClient {
	return n.batchBroadcastClient
}

func (n nodeContext) IsHealthy() bool {
	return n.metrics.status == Up
}

func (n nodeContext) ReportError(err error) {
	n.metrics.status = OutOfService // TODO refine err to status
	n.lastError = err
}

func (n nodeContext) ReportErrorStatus(status NodeStatus, err error) {
	n.metrics.status = status
	n.lastError = err
}
