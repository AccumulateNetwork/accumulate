package connmgr

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client/http"
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

type ConnectionManager interface {
	AcquireRpcClient(url string, allowFollower bool) (*http.HTTP, error)
	AcquireQueryClient(url string) (api.ABCIQueryClient, error)
	AcquireBroadcastClient() api.ABCIBroadcastClient
}

type connectionManager struct {
	networkCfg          *config.Network
	nodeService         local.NodeService
	nodeToSubnetNameMap map[string]string
	bvNodeCtxList       []*nodeContext
	dirNodeCtxList      []*nodeContext
	followerNodeCtxList []*nodeContext
	localClient         *local.Local
	connRouter          *connectionRouter
	logger              log.Logger
}

type nodeContext struct {
	subnetName    string
	address       string
	netType       config.NetworkType
	nodeType      config.NodeType
	networkGroup  NetworkGroup
	resolvedIPs   []net.IP
	metrics       nodeMetrics
	queryClient   api.ABCIQueryClient
	rpcHttpClient *http.HTTP
}

type nodeMetrics struct {
	status    NodeStatus
	latencyMS uint32
	// TODO add metrics that can be useful for the router to determine whether it should put or should avoid putting put more load on a BVN
}

func NewConnectionManager(networkCfg *config.Network, nodeService local.NodeService, logger log.Logger) (ConnectionManager, error) {
	cm := new(connectionManager)
	cm.networkCfg = networkCfg
	cm.nodeService = nodeService
	cm.logger = logger
	cm.loadNodeNetworkMap()
	cm.buildNodeInventory()
	err := cm.createClients()
	cm.connRouter = newConnectionRouter(cm.bvNodeCtxList, cm.dirNodeCtxList, cm.followerNodeCtxList)
	return cm, err
}

func (cm *connectionManager) AcquireRpcClient(url string, allowFollower bool) (*http.HTTP, error) { // TODO Do we ever need a RPC client connection to a follower?
	ctx, err := cm.connRouter.selectNodeContext(url, allowFollower)
	if err != nil {
		return nil, errorCouldNotSelectNode(url, err)
	}
	return ctx.rpcHttpClient, nil
}

func (cm *connectionManager) AcquireQueryClient(url string) (api.ABCIQueryClient, error) {
	ctx, err := cm.connRouter.selectNodeContext(url, true)
	if err != nil {
		return nil, errorCouldNotSelectNode(url, err)
	}
	return ctx.queryClient, nil
}

func (cm *connectionManager) AcquireBroadcastClient() api.ABCIBroadcastClient {
	return cm.localClient
}

func (cm *connectionManager) loadNodeNetworkMap() {
	cm.nodeToSubnetNameMap = make(map[string]string)
	for subnetName, addresses := range cm.networkCfg.Addresses {
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
				cm.bvNodeCtxList = append(cm.bvNodeCtxList, ctx)
			case config.Directory:
				cm.dirNodeCtxList = append(cm.dirNodeCtxList, ctx)
			}
		case config.Follower:
			cm.followerNodeCtxList = append(cm.followerNodeCtxList, ctx)
		}
	}
}

func (cm *connectionManager) buildNodeContext(address string, subnetName string) (*nodeContext, error) {
	ctx := &nodeContext{subnetName: subnetName, address: address}
	ctx.networkGroup = cm.determineNetworkGroup(subnetName, address)
	ctx.netType, ctx.nodeType = determineTypes(address, subnetName, cm.networkCfg)

	var err error
	ctx.resolvedIPs, err = resolveIPs(address)
	if err != nil {
		return nil, fmt.Errorf("error resolving IPs for %s: %w", address, err)
	}
	return ctx, nil
}

func (cm *connectionManager) determineNetworkGroup(subnetName string, address string) NetworkGroup {
	switch {
	case cm.networkCfg.ID == subnetName:
		return Local
	case cm.nodeToSubnetNameMap[address] == subnetName:
		return SameSubnet
	default:
		return OtherSubnet
	}
}

func determineTypes(address string, subnetName string, netCfg *config.Network) (config.NetworkType, config.NodeType) {
	var networkType config.NetworkType
	for _, bvnName := range netCfg.BvnNames {
		if bvnName == subnetName {
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

func (cm *connectionManager) createClients() error {
	// Create a local client
	lclClient, err := local.New(cm.nodeService)
	if err != nil {
		return fmt.Errorf("failed to create local node client: %v", err)
	}
	cm.localClient = lclClient

	for _, ctx := range cm.bvNodeCtxList {
		err := cm.createRpcClient(ctx)
		if err != nil {
			return err
		}
		err = cm.createAbciClients(ctx)
		if err != nil {
			return err
		}
	}
	for _, ctx := range cm.dirNodeCtxList {
		err := cm.createRpcClient(ctx)
		if err != nil {
			return err
		}
		err = cm.createAbciClients(ctx)
		if err != nil {
			return err
		}
	}
	for _, ctx := range cm.followerNodeCtxList {
		err := cm.createRpcClient(ctx)
		if err != nil {
			return err
		}
		err = cm.createAbciClients(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cm *connectionManager) createAbciClients(ctx *nodeContext) error {
	switch ctx.networkGroup {
	case Local:
		ctx.queryClient = cm.localClient
	default:
		address := "http://" + ctx.address + ":34000" // FIXME
		offsetAddr, err := config.OffsetPort(address, networks.TmRpcPortOffset)
		if err != nil {
			return fmt.Errorf("invalid BVN address: %v", err)
		}

		ctx.queryClient, err = rpchttp.New(offsetAddr)
		if err != nil {
			return fmt.Errorf("failed to create RPC client: %v", err)
		}
	}
	return nil
}

func (cm *connectionManager) createRpcClient(ctx *nodeContext) error {
	// RPC HTTP client
	var err error
	ctx.rpcHttpClient, err = http.New(ctx.address)
	if err != nil {
		return fmt.Errorf("could not create client for block validator %q node %q: %v",
			ctx.subnetName, ctx.address, err)
	}
	return nil
}

func resolveIPs(address string) ([]net.IP, error) {
	ip := net.ParseIP(address)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	/* TODO
	consider using DNS resolver with DNSSEC support like go-resolver and query directly to a DNS server list that supports this, like 1.1.1.1
	*/
	var err error
	ipList, err := net.LookupIP(address)
	if err != nil {
		return nil, fmt.Errorf("error doing DNS lookup for %s: %w", address, err)
	}
	return ipList, nil
}
