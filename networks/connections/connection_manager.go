package connections

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"github.com/ybbus/jsonrpc/v2"
	"net"
	"net/url"
	"strings"
)

type ConnectionManager interface {
	getBVNContextList() []*nodeContext
	getDNContextList() []*nodeContext
	getFNContextList() []*nodeContext
	GetLocalClient() *local.Local
}

type ConnectionInitializer interface {
	CreateClients(*local.Local) error
}

type connectionManager struct {
	accConfig           *config.Accumulate
	nodeToSubnetNameMap map[string]string
	bvnCtxList          []*nodeContext
	dnCtxList           []*nodeContext
	fnCtxList           []*nodeContext
	localClient         *local.Local
	logger              log.Logger
}

type nodeMetrics struct {
	status    NodeStatus
	latencyMS uint32
	// TODO add metrics that can be useful for the router to determine whether it should put or should avoid putting put more load on a BVN
}

func NewConnectionManager(accConfig *config.Accumulate, logger log.Logger) ConnectionManager {
	cm := new(connectionManager)
	cm.accConfig = accConfig
	cm.logger = logger
	cm.loadNodeNetworkMap()
	cm.buildNodeInventory()
	return cm
}

func (cm *connectionManager) getBVNContextList() []*nodeContext {
	return cm.bvnCtxList
}

func (cm *connectionManager) getDNContextList() []*nodeContext {
	return cm.dnCtxList
}

func (cm *connectionManager) getFNContextList() []*nodeContext {
	return cm.fnCtxList
}

func (cm *connectionManager) GetLocalClient() *local.Local {
	return cm.localClient
}

func (cm *connectionManager) loadNodeNetworkMap() {
	cm.nodeToSubnetNameMap = make(map[string]string)
	for subnetName, addresses := range cm.accConfig.Network.Addresses {
		for _, address := range addresses {
			cm.nodeToSubnetNameMap[address] = subnetName
		}
	}
}

func (cm *connectionManager) buildNodeInventory() {
	for address, subnetName := range cm.nodeToSubnetNameMap {
		ctx, err := cm.buildNodeContext(address, subnetName)
		if err != nil {
			cm.logger.Error("error building node context for node %s on net %s with type %s: %w, ignoring node...",
				ctx.address, ctx.subnetName, ctx.nodeType, err)
			continue
		}

		switch ctx.nodeType {
		case config.Validator:
			switch ctx.netType {
			case config.BlockValidator:
				cm.bvnCtxList = append(cm.bvnCtxList, ctx)
			case config.Directory:
				cm.dnCtxList = append(cm.dnCtxList, ctx)
			}
		case config.Follower:
			cm.fnCtxList = append(cm.fnCtxList, ctx)
		}
	}
}

func (cm *connectionManager) buildNodeContext(address string, subnetName string) (*nodeContext, error) {
	nodeCtx := &nodeContext{subnetName: subnetName, address: address}
	nodeCtx.networkGroup = cm.determineNetworkGroup(subnetName, address)
	nodeCtx.netType, nodeCtx.nodeType = determineTypes(address, subnetName, cm.accConfig.Network)

	var err error
	nodeCtx.resolvedIPs, err = resolveIPs(address)
	if err != nil {
		nodeCtx.ReportErrorStatus(Down, fmt.Errorf("error resolving IPs for %s: %w", address, err))
	}
	return nodeCtx, nil
}

func (cm *connectionManager) determineNetworkGroup(subnetName string, address string) NetworkGroup {
	switch {
	case (strings.EqualFold(subnetName, cm.accConfig.Network.ID) && strings.EqualFold(address, cm.accConfig.Network.SelfAddress)) ||
		strings.EqualFold(address, "local") || strings.EqualFold(address, "self"):
		return Local
	case subnetName == cm.accConfig.Network.ID:
		return SameSubnet
	default:
		return OtherSubnet
	}
}

func determineTypes(address string, subnetName string, netCfg config.Network) (config.NetworkType, config.NodeType) {
	var networkType config.NetworkType
	for _, bvnName := range netCfg.BvnNames {
		if strings.EqualFold(bvnName, subnetName) {
			networkType = config.BlockValidator
		}
	}
	if len(networkType) == 0 { // When it's not a block validator the only option is directory
		networkType = config.Directory
	}

	var nodeType config.NodeType
	nodeType = config.Validator // TODO follower support
	return networkType, nodeType
}

func (cm *connectionManager) CreateClients(lclClient *local.Local) error {
	cm.localClient = lclClient

	for _, nodeCtx := range cm.bvnCtxList {
		err := cm.createJsonRpcClient(nodeCtx)
		if err != nil {
			return err
		}
		err = cm.createAbciClients(nodeCtx)
		if err != nil {
			return err
		}
	}
	for _, nodeCtx := range cm.dnCtxList {
		err := cm.createJsonRpcClient(nodeCtx)
		if err != nil {
			return err
		}
		err = cm.createAbciClients(nodeCtx)
		if err != nil {
			return err
		}
	}
	for _, nodeCtx := range cm.fnCtxList {
		err := cm.createJsonRpcClient(nodeCtx)
		if err != nil {
			return err
		}
		err = cm.createAbciClients(nodeCtx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cm *connectionManager) createAbciClients(nodeCtx *nodeContext) error {
	switch nodeCtx.networkGroup {
	case Local:
		nodeCtx.queryClient = cm.localClient
		nodeCtx.broadcastClient = cm.localClient
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
	}
	return nil
}

func (cm *connectionManager) createJsonRpcClient(nodeCtx *nodeContext) error {
	// RPC HTTP client
	var address string
	if strings.EqualFold(nodeCtx.address, "local") || strings.EqualFold(nodeCtx.address, "self") {
		address = cm.accConfig.API.ListenAddress
	} else {
		var err error
		address, err = config.OffsetPort(nodeCtx.address, networks.AccRouterJsonPortOffset)
		if err != nil {
			return fmt.Errorf("invalid BVN address: %v", err)
		}
	}

	nodeCtx.jsonRpcClient = jsonrpc.NewClient(address)
	return nil
}

func resolveIPs(address string) ([]net.IP, error) {
	ip := net.ParseIP(address)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	var hostname string
	nodeUrl, err := url.Parse(address)
	if err == nil {
		hostname = nodeUrl.Hostname()
	} else {
		hostname = address
	}

	/* TODO
	consider using DNS resolver with DNSSEC support like go-resolver and query directly to a DNS server list that supports this, like 1.1.1.1
	*/
	ipList, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("error doing DNS lookup for %s: %w", address, err)
	}
	return ipList, nil
}
