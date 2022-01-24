package testing

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/rpc/client/http"
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

type RawClient struct {
	*http.HTTP
}

type fakeNodeContext struct {
	subnetName           string
	address              string
	connMgr              *fakeConnectionManager
	netType              config.NetworkType
	nodeType             config.NodeType
	networkGroup         NetworkGroup
	resolvedIPs          []net.IP
	metrics              fakeNodeMetrics
	queryClient          FakeClient
	broadcastClient      FakeClient
	batchBroadcastClient FakeClient
	jsonRpcClient        jsonrpc.RPCClient
	rawClient            RawClient
	service              service.Service
	lastError            error
	lastErrorExpiryTime  time.Time
}

func (nc *fakeNodeContext) GetSubnetName() string {
	return nc.subnetName
}

func (nc *fakeNodeContext) GetNetworkGroup() NetworkGroup {
	return nc.networkGroup
}

func (nc *fakeNodeContext) IsDirectoryNode() bool {
	return nc.netType == config.Directory && nc.nodeType == config.Validator
}

func (nc *fakeNodeContext) GetJsonRpcClient() jsonrpc.RPCClient {
	return nc.jsonRpcClient
}

func (nc *fakeNodeContext) GetQueryClient() connections.ABCIQueryClient {
	return nc.queryClient
}

func (nc *fakeNodeContext) GetRawClient() RawClient {
	return nc.rawClient
}

func (nc *fakeNodeContext) GetBroadcastClient() connections.ABCIBroadcastClient {
	return nc.broadcastClient
}

func (nc *fakeNodeContext) GetBatchBroadcastClient() connections.BatchABCIBroadcastClient {
	return nc.batchBroadcastClient
}

func (nc *fakeNodeContext) IsHealthy() bool {
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
			nc.lastErrorExpiryTime = now.Add(connections.UnhealthyNodeCheckInterval) // avoid double doHealthCheckOnNode calls
			go nc.connMgr.doHealthCheckOnNode(nc)
		}
	}
	return false
}

func (nc *fakeNodeContext) ReportError(err error) {
	nc.metrics.status = OutOfService
	nc.lastError = err
	// TODO refine err to status, OutOfService means the node is alive & kicking, but not able to handle request (ie still loading DB or syncing up)
	nc.lastErrorExpiryTime = time.Now().Add(connections.UnhealthyNodeCheckInterval)
}

func (nc *fakeNodeContext) ReportErrorStatus(status NodeStatus, err error) {
	nc.metrics.status = status
	nc.lastError = err
	nc.lastErrorExpiryTime = time.Now().Add(connections.UnhealthyNodeCheckInterval)
}

func (nc *fakeNodeContext) GetService() service.Service {
	return nc.service
}
