package connections

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/ybbus/jsonrpc/v2"
	"net"
	"time"
)

type NodeStatus int

const (
	Up           NodeStatus = iota // Healthy & ready to go
	Down                           // Not reachable
	OutOfService                   // Reachable but not ready to go (IE. still syncing up)
	Unknown                        // Not checked yet
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
	connMgr              *connectionManager
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
	lastErrorExpiryTime  time.Time
}

func (nc *nodeContext) GetSubnetName() string {
	return nc.subnetName
}

func (nc *nodeContext) GetNetworkGroup() NetworkGroup {
	return nc.networkGroup
}

func (nc *nodeContext) IsDirectoryNode() bool {
	return nc.netType == config.Directory && nc.nodeType == config.Validator
}

func (nc *nodeContext) GetJsonRpcClient() jsonrpc.RPCClient {
	return nc.jsonRpcClient
}

func (nc *nodeContext) GetQueryClient() ABCIQueryClient {
	return nc.queryClient
}

func (nc *nodeContext) GetBroadcastClient() ABCIBroadcastClient {
	return nc.broadcastClient
}

func (nc *nodeContext) GetBatchBroadcastClient() BatchABCIBroadcastClient {
	return nc.batchBroadcastClient
}

func (nc *nodeContext) IsHealthy() bool {
	switch nc.metrics.status {
	case Up:
		return true
	case Unknown:
		nc.connMgr.doHealthCheckOnNode(nc)
		if nc.metrics.status == Up {
			return true
		}
	default:
		now := time.Now()
		if now.After(nc.lastErrorExpiryTime) {
			nc.lastErrorExpiryTime = now.Add(UnhealthyNodeCheckInterval) // avoid double doHealthCheckOnNode calls
			go nc.connMgr.doHealthCheckOnNode(nc)
		}
	}
	return false
}

func (nc *nodeContext) ReportError(err error) {
	nc.metrics.status = OutOfService
	nc.lastError = err
	// TODO refine err to status, OutOfService means the node is alive & kicking, but not able to handle request (ie still loading DB or syncing up)
	nc.lastErrorExpiryTime = time.Now().Add(UnhealthyNodeCheckInterval)
}

func (nc *nodeContext) ReportErrorStatus(status NodeStatus, err error) {
	nc.metrics.status = status
	nc.lastError = err
	nc.lastErrorExpiryTime = time.Now().Add(UnhealthyNodeCheckInterval)
}
