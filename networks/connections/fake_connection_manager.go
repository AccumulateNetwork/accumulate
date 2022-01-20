package connections

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"github.com/ybbus/jsonrpc/v2"
	"strings"
	"time"
)

type FakeConnectionInitializer interface {
	CreateClients(client ABCIBroadcastClient) error
}

type fakeConnectionManager struct {
	accConfig    *config.Accumulate
	bvnCtxMap    map[string][]*nodeContext
	dnCtxList    []*nodeContext
	fnCtxList    []*nodeContext
	all          []*nodeContext
	localNodeCtx *nodeContext
	localClient  *local.Local
	logger       log.Logger
	selfAddress  string
	fakeClient   ABCIBroadcastClient
}

func (cm *fakeConnectionManager) doHealthCheckOnNode(nc *nodeContext) {
	nc.metrics.status = Up
}

func NewFakeConnectionManager(accConfig *config.Accumulate) ConnectionManager {
	fcm := new(fakeConnectionManager)
	fcm.accConfig = accConfig
	fcm.buildNodeInventory()
	return fcm
}

func (fcm *fakeConnectionManager) GetBVNContextMap() map[string][]*nodeContext {
	return fcm.bvnCtxMap
}

func (fcm *fakeConnectionManager) GetDNContextList() []*nodeContext {
	return fcm.dnCtxList
}

func (fcm *fakeConnectionManager) GetFNContextList() []*nodeContext {
	return fcm.fnCtxList
}

func (fcm *fakeConnectionManager) GetAllNodeContexts() []*nodeContext {
	return fcm.all
}

func (fcm *fakeConnectionManager) GetLocalNodeContext() *nodeContext {
	return fcm.localNodeCtx
}

func (fcm *fakeConnectionManager) GetLocalClient() *local.Local {
	return fcm.localClient
}

func (fcm *fakeConnectionManager) ResetErrors() {
	for _, nodeCtx := range fcm.all {
		nodeCtx.metrics.status = Unknown
		nodeCtx.lastError = nil
		nodeCtx.lastErrorExpiryTime = time.Now()
	}
}

func (fcm *fakeConnectionManager) buildNodeInventory() {
	fcm.bvnCtxMap = make(map[string][]*nodeContext)

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
						nodeList := make([]*nodeContext, 1)
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
			if nodeCtx.networkGroup == Local {
				fcm.localNodeCtx = nodeCtx
			}
		}
	}
}

func (fcm *fakeConnectionManager) buildNodeContext(address string, subnetName string) (*nodeContext, error) {
	nodeCtx := &nodeContext{subnetName: subnetName,
		address: address,
		metrics: nodeMetrics{status: Unknown}}
	nodeCtx.networkGroup = fcm.determineNetworkGroup(subnetName, address)
	nodeCtx.netType, nodeCtx.nodeType = determineTypes(subnetName, fcm.accConfig.Network)

	if address != "local" && address != "self" {
		var err error
		nodeCtx.resolvedIPs, err = resolveIPs(address)
		if err != nil {
			nodeCtx.ReportErrorStatus(Down, fmt.Errorf("error resolving IPs for %s: %w", address, err))
		}
	}
	return nodeCtx, nil
}

func (fcm *fakeConnectionManager) determineNetworkGroup(subnetName string, address string) NetworkGroup {
	switch {
	case (strings.EqualFold(subnetName, fcm.accConfig.Network.ID) && strings.EqualFold(fcm.reformatAddress(address), fcm.selfAddress)):
		return Local
	case strings.EqualFold(subnetName, fcm.accConfig.Network.ID):
		return SameSubnet
	default:
		return OtherSubnet
	}
}

func (fcm *fakeConnectionManager) CreateClients(client ABCIBroadcastClient) error {
	fcm.fakeClient = client
	/*
		for _, nodeCtxList := range fcm.bvnCtxMap {
			for _, nodeCtx := range nodeCtxList {
				err := fcm.createJsonRpcClient(nodeCtx)
				if err != nil {
					return err
				}
				err = fcm.createAbciClients(nodeCtx)
				if err != nil {
					return err
				}
			}
		}
		for _, nodeCtx := range fcm.dnCtxList {
			err := fcm.createJsonRpcClient(nodeCtx)
			if err != nil {
				return err
			}
			err = fcm.createAbciClients(nodeCtx)
			if err != nil {
				return err
			}
		}
		for _, nodeCtx := range fcm.fnCtxList {
			err := fcm.createJsonRpcClient(nodeCtx)
			if err != nil {
				return err
			}
			err = fcm.createAbciClients(nodeCtx)
			if err != nil {
				return err
			}
		}*/
	return nil
}

func (fcm *fakeConnectionManager) reformatAddress(address string) string {
	if address == "local" || address == "self" {
		return fcm.selfAddress
	}
	return strings.Split(address, "//")[1]
}

func (fcm *fakeConnectionManager) createAbciClients(nodeCtx *nodeContext) error {
	switch nodeCtx.networkGroup {
	case Local:
		nodeCtx.queryClient = fcm.localClient
		nodeCtx.broadcastClient = fcm.localClient
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
		nodeCtx.rawClient = RawClient{client}
		nodeCtx.service = nodeCtx.rawClient
	}
	return nil
}

func (fcm *fakeConnectionManager) createJsonRpcClient(nodeCtx *nodeContext) error {
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
