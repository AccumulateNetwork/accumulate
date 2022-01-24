package testing

import (
	"context"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"github.com/ybbus/jsonrpc/v2"
	"strings"
	"time"
)

type FakeConnectionInitializer interface {
	CreateClients(client connections.ABCIBroadcastClient) error
}

type FakeClient interface {
	// client.ABCIClient
	ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error)
	BroadcastTxAsync(context.Context, types.Tx) (*ctypes.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, types.Tx) (*ctypes.ResultBroadcastTx, error)

	// client.SignClient
	Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error)

	// client.EventsClient
	Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error)
}

type fakeConnectionManager struct {
	accConfig    *config.Accumulate
	bvnCtxMap    map[string][]*fakeNodeContext
	dnCtxList    []*fakeNodeContext
	fnCtxList    []*fakeNodeContext
	all          []*fakeNodeContext
	localNodeCtx *fakeNodeContext
	localClient  *local.Local
	logger       log.Logger
	selfAddress  string
	fakeClient   FakeClient
}

type fakeNodeMetrics struct {
	status NodeStatus
	// TODO add metrics that can be useful for the router to determine whether it should put or should avoid putting put more load on a BVN
}

func (cm *fakeConnectionManager) doHealthCheckOnNode(nc *fakeNodeContext) {
	nc.metrics.status = connections.Up
}

func NewFakeConnectionManager(accConfig *config.Accumulate) connections.ConnectionManager {
	fcm := new(fakeConnectionManager)
	fcm.accConfig = accConfig
	fcm.buildNodeInventory()
	return fcm
}

func (fcm *fakeConnectionManager) GetBVNContextMap() map[string][]*connections.nodeContext {
	return fcm.bvnCtxMap
}

func (fcm *fakeConnectionManager) GetDNContextList() []*connections.nodeContext {
	return fcm.dnCtxList
}

func (fcm *fakeConnectionManager) GetFNContextList() []*connections.nodeContext {
	return fcm.fnCtxList
}

func (fcm *fakeConnectionManager) GetAllNodeContexts() []*connections.nodeContext {
	return fcm.all
}

func (fcm *fakeConnectionManager) GetLocalNodeContext() *connections.nodeContext {
	return fcm.localNodeCtx
}

func (fcm *fakeConnectionManager) GetLocalClient() *local.Local {
	return fcm.localClient
}

func (fcm *fakeConnectionManager) ResetErrors() {
	for _, nodeCtx := range fcm.all {
		nodeCtx.metrics.status = connections.Unknown
		nodeCtx.lastError = nil
		nodeCtx.lastErrorExpiryTime = time.Now()
	}
}

func (fcm *fakeConnectionManager) buildNodeInventory() {
	fcm.bvnCtxMap = make(map[string][]*connections.nodeContext)

	for subnetName, addresses := range fcm.accConfig.Network.Addresses {
		for _, address := range addresses {
			nodeCtx, err := fcm.buildNodeContext(address, subnetName)
			if err != nil {
				fcm.logger.Error("error building node context for node %s on net %s with type %s: %w, ignoring node...",
					nodeCtx.address, nodeCtx.subnetName, nodeCtx.nodeType, err)
				continue
			}

			switch nodeCtx.nodeType {
			case config.Validator:
				switch nodeCtx.netType {
				case config.BlockValidator:
					bvnName := protocol.BvnNameFromSubnetName(subnetName)
					if bvnName == "bvn-directory" {
						panic("Directory subnet node is misconfigured as blockvalidator")
					}
					nodeList, ok := fcm.bvnCtxMap[bvnName]
					if !ok {
						nodeList := make([]*connections.nodeContext, 1)
						nodeList[0] = nodeCtx
						fcm.bvnCtxMap[bvnName] = nodeList
					} else {
						fcm.bvnCtxMap[bvnName] = append(nodeList, nodeCtx)
					}
					fcm.all = append(fcm.all, nodeCtx)
				case config.Directory:
					fcm.dnCtxList = append(fcm.dnCtxList, nodeCtx)
					fcm.all = append(fcm.all, nodeCtx)
				}
			case config.Follower:
				fcm.fnCtxList = append(fcm.fnCtxList, nodeCtx)
				fcm.all = append(fcm.all, nodeCtx)
			}
			if nodeCtx.networkGroup == connections.Local {
				fcm.localNodeCtx = nodeCtx
			}
		}
	}
}

func (fcm *fakeConnectionManager) buildNodeContext(address string, subnetName string) (*connections.nodeContext, error) {
	nodeCtx := &connections.nodeContext{subnetName: subnetName,
		address: address,
		metrics: connections.nodeMetrics{status: connections.Unknown}}
	nodeCtx.networkGroup = fcm.determineNetworkGroup(subnetName, address)
	nodeCtx.netType, nodeCtx.nodeType = connections.determineTypes(subnetName, fcm.accConfig.Network)

	if address != "local" && address != "self" {
		var err error
		nodeCtx.resolvedIPs, err = connections.resolveIPs(address)
		if err != nil {
			nodeCtx.ReportErrorStatus(connections.Down, fmt.Errorf("error resolving IPs for %s: %w", address, err))
		}
	}
	return nodeCtx, nil
}

func (fcm *fakeConnectionManager) determineNetworkGroup(subnetName string, address string) connections.NetworkGroup {
	switch {
	case (strings.EqualFold(subnetName, fcm.accConfig.Network.ID) && strings.EqualFold(fcm.reformatAddress(address), fcm.selfAddress)):
		return connections.Local
	case strings.EqualFold(subnetName, fcm.accConfig.Network.ID):
		return connections.SameSubnet
	default:
		return connections.OtherSubnet
	}
}

func (fcm *fakeConnectionManager) CreateClients(client testing.FakeTendermint) error {
	fcm.fakeClient = client
	for _, nodeCtxList := range fcm.bvnCtxMap {
		for _, nodeCtx := range nodeCtxList {
			err := fcm.createAbciClients(nodeCtx)
			if err != nil {
				return err
			}
		}
	}
	for _, nodeCtx := range fcm.dnCtxList {
		err := fcm.createAbciClients(nodeCtx)
		if err != nil {
			return err
		}
	}
	for _, nodeCtx := range fcm.fnCtxList {
		err := fcm.createAbciClients(nodeCtx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fcm *fakeConnectionManager) reformatAddress(address string) string {
	if address == "local" || address == "self" {
		return fcm.selfAddress
	}
	return strings.Split(address, "//")[1]
}

func (fcm *fakeConnectionManager) createAbciClients(nodeCtx *connections.nodeContext) error {
	switch nodeCtx.networkGroup {
	case connections.Local:
		nodeCtx.queryClient = &fcm.fakeClient
		nodeCtx.broadcastClient = &fcm.fakeClient
		nodeCtx.service = fcm.localClient
	default:
		offsetAddr, err := config.OffsetPort(nodeCtx.address, networks.TmRpcPortOffset)
		if err != nil {
			return fmt.Errorf("invalid BVN address: %v", err)
		}
		client, err := rpchttp.New(offsetAddr)
		if err != nil {
			return fmt.Errorf("failed to create RPC client: %v", err)
		}

		nodeCtx.queryClient = client
		nodeCtx.broadcastClient = client
		nodeCtx.batchBroadcastClient = client
		nodeCtx.rawClient = connections.RawClient{client}
		nodeCtx.service = nodeCtx.rawClient
	}
	return nil
}

func (fcm *fakeConnectionManager) createJsonRpcClient(nodeCtx *connections.nodeContext) error {
	// RPC HTTP client
	var address string
	if strings.EqualFold(nodeCtx.address, "local") || strings.EqualFold(nodeCtx.address, "self") {
		address = fcm.accConfig.API.ListenAddress
	} else {
		var err error
		address, err = config.OffsetPort(nodeCtx.address, networks.AccRouterJsonPortOffset)
		if err != nil {
			return fmt.Errorf("invalid BVN address: %v", err)
		}
	}

	nodeCtx.jsonRpcClient = jsonrpc.NewClient(address + "/v2")
	return nil
}
