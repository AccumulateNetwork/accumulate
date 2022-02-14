package connections

import (
	"context"
	"fmt"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/core/types"
	tm "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
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

// Client is a subset of from TM/rpc/client.ABCIClient.
type Client interface {
	ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error)
	CheckTx(ctx context.Context, tx tm.Tx) (*core.ResultCheckTx, error)
	BroadcastTxAsync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
}

type ConnectionContext interface {
	GetSubnetName() string
	GetNodeUrl() string
	GetNetworkGroup() NetworkGroup
	GetNodeType() config.NodeType
	GetMetrics() *NodeMetrics
	GetLastError() error
	GetAddress() string
	SetNodeUrl(addr string)
	GetClient() Client
	IsDirectoryNode() bool
	IsHealthy() bool
	ReportError(err error)
	ReportErrorStatus(status NodeStatus, err error)
	ClearErrors()
}

type connectionContext struct {
	subnetName          string
	address             string
	nodeUrl             string
	client              Client
	connMgr             *connectionManager
	netType             config.NetworkType
	nodeType            config.NodeType
	networkGroup        NetworkGroup
	resolvedIPs         []net.IP
	metrics             NodeMetrics
	lastError           error
	lastErrorExpiryTime time.Time
}

func (cc *connectionContext) GetClient() Client {
	if cc.client != nil {
		return cc.client
	}

	// Client not there yet? Wait for it.
	for i := 0; i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
		if cc.client != nil {
			return cc.client
		}
	}
	panic(fmt.Sprintf("Could not obtain a client for node %s  ", cc.nodeUrl))
}

func (cc *connectionContext) GetAddress() string {
	return cc.address
}

func (cc *connectionContext) GetNodeType() config.NodeType {
	return cc.nodeType
}

func (cc *connectionContext) SetNodeUrl(addr string) {
	cc.nodeUrl = addr
}

func (cc *connectionContext) GetLastError() error {
	return cc.lastError
}

func (cc *connectionContext) GetNodeUrl() string {
	return cc.nodeUrl
}

func (cc *connectionContext) GetSubnetName() string {
	return cc.subnetName
}

func (cc *connectionContext) GetNetworkGroup() NetworkGroup {
	return cc.networkGroup
}

func (cc *connectionContext) IsDirectoryNode() bool {
	return cc.netType == config.Directory && cc.nodeType == config.Validator
}

func (cc *connectionContext) IsHealthy() bool {
	switch cc.metrics.status {
	case Up:
		return true
	case Unknown:
		cc.connMgr.doHealthCheckOnNode(cc)
		if cc.metrics.status == Up {
			return true
		}
	default:
		now := time.Now()
		if now.After(cc.lastErrorExpiryTime) {
			cc.lastErrorExpiryTime = now.Add(UnhealthyNodeCheckInterval) // avoid double doHealthCheckOnNode calls
			go cc.connMgr.doHealthCheckOnNode(cc)
		}
	}
	return false
}

func (cc *connectionContext) ClearErrors() {
	cc.lastError = nil
	cc.lastErrorExpiryTime = time.Now()
}

func (cc *connectionContext) GetMetrics() *NodeMetrics {
	return &cc.metrics
}

func (cc *connectionContext) ReportError(err error) {
	// TODO Maybe we need to filter out certain errors, those should not mark the node as being out of service
	cc.metrics.status = OutOfService
	// TODO refine err to status, OutOfService means the node is alive & kicking, but not able to handle request (ie still loading DB or syncing up)
	cc.lastError = err
	cc.lastErrorExpiryTime = time.Now().Add(UnhealthyNodeCheckInterval)
}

func (cc *connectionContext) ReportErrorStatus(status NodeStatus, err error) {
	cc.metrics.status = status
	cc.lastError = err
	cc.lastErrorExpiryTime = time.Now().Add(UnhealthyNodeCheckInterval)
}
