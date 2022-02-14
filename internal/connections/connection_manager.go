package connections

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tendermint/tendermint/libs/log"
	rpc "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
	"net"
	neturl "net/url"
	"strings"
	"time"
)

const UnhealthyNodeCheckInterval = time.Minute * 10 // TODO Configurable in toml?

type ConnectionManager interface {
	GetLocalNodeContext() ConnectionContext
	GetLocalClient() *local.Local
	SelectConnection(subnet string) (ConnectionContext, error)
	GetBVNContextMap() map[string][]ConnectionContext
	GetDNContextList() []ConnectionContext
	GetFNContextList() []ConnectionContext
	GetAllNodeContexts() []ConnectionContext
	ResetErrors()
}

type ApiMethods interface {
	Metrics(_ context.Context, params json.RawMessage) interface{}
}

type ConnectionInitializer interface {
	InitClients(*local.Local, ApiMethods) error
}

type connectionManager struct {
	accConfig   *config.Accumulate
	bvnCtxMap   map[string][]ConnectionContext
	dnCtxList   []ConnectionContext
	fnCtxList   []ConnectionContext
	all         []ConnectionContext
	localCtx    ConnectionContext
	localClient *local.Local
	logger      log.Logger
	selfAddress string
	apiMethods  ApiMethods
}

func (cm *connectionManager) doHealthCheckOnNode(cc ConnectionContext) {
	// Try to query Tendermint with something it should not find
	qu := query.Query{}
	qd, _ := qu.MarshalBinary()
	qryRes, err := cc.GetClient().ABCIQueryWithOptions(context.Background(), "/abci_query", qd, rpc.DefaultABCIQueryOptions)
	if err != nil || qryRes.Response.Code != 19 { // FIXME code 19 will emit an error in the log
		cc.ReportError(err)
		if qryRes != nil {
			cm.logger.Info("ABCIQuery response: %v", qryRes.Response)
		}
		return
	}

	/*	TODO
		res := cm.apiMethods.Metrics(context.Background(), json.RawMessage("{Metric: \"tps\", Duration: time.Hour}"))
		cm.logger.Info("TPS response: %v", res)
	*/
	cc.GetMetrics().status = Up
}

type NodeMetrics struct {
	status   NodeStatus
	usageCnt uint64
	// TODO add metrics that can be useful for the router to determine whether it should put or should avoid putting put more load on a BVN
}

func NewConnectionManager(config *config.Config, logger log.Logger) ConnectionManager {
	cm := new(connectionManager)
	cm.accConfig = &config.Accumulate
	cm.selfAddress = cm.reformatAddress(config.Config.P2P.ListenAddress)
	cm.logger = logger
	cm.buildNodeInventory()
	return cm
}

func (cm *connectionManager) SelectConnection(subnet string) (ConnectionContext, error) {

	// When subnet is the same as the current node's subnet id, just return the local
	if strings.EqualFold(subnet, cm.accConfig.Network.ID) {
		return cm.localCtx, nil
	}

	bvnName := protocol.BvnNameFromSubnetName(subnet)
	nodeList, ok := cm.bvnCtxMap[bvnName]
	if !ok {
		dnCtx := cm.dnCtxList[0]
		if dnCtx.GetSubnetName() == subnet {
			return dnCtx, nil
		}
		return nil, fmt.Errorf("%w %q", ErrUnknownSubnet, subnet)
	}

	healthyNodes := cm.getHealthyNodes(nodeList)
	if len(healthyNodes) == 0 {
		return nil, NoHealthyNodes // None of the nodes in the subnet could be reached
	}

	// Apply simple round-robin balancing to nodes in non-local subnets
	var selCtx ConnectionContext
	selCtxCnt := ^uint64(0)
	for _, connCtx := range healthyNodes {
		usageCnt := connCtx.GetMetrics().usageCnt
		if usageCnt < selCtxCnt {
			selCtx = connCtx
			selCtxCnt = usageCnt
		}
	}
	selCtx.GetMetrics().usageCnt++
	return selCtx, nil
}

func (cm *connectionManager) getHealthyNodes(nodeList []ConnectionContext) []ConnectionContext {
	var healthyNodes = make([]ConnectionContext, 0)
	for _, connCtx := range nodeList {
		if connCtx.IsHealthy() {
			healthyNodes = append(healthyNodes, connCtx)
		}
	}

	if len(healthyNodes) == 0 { // When there is no alternative node available in the subnet, do another health check & try again
		cm.ResetErrors()
		for _, connCtx := range nodeList {
			if connCtx.IsHealthy() {
				healthyNodes = append(healthyNodes, connCtx)
			}
		}
	}
	return healthyNodes
}

func (cm *connectionManager) GetBVNContextMap() map[string][]ConnectionContext {
	return cm.bvnCtxMap
}

func (cm *connectionManager) GetDNContextList() []ConnectionContext {
	return cm.dnCtxList
}

func (cm *connectionManager) GetFNContextList() []ConnectionContext {
	return cm.fnCtxList
}

func (cm *connectionManager) GetAllNodeContexts() []ConnectionContext {
	return cm.all
}

func (cm *connectionManager) GetLocalNodeContext() ConnectionContext {
	return cm.localCtx
}

func (cm *connectionManager) GetLocalClient() *local.Local {
	return cm.localClient
}

func (cm *connectionManager) ResetErrors() {
	for _, nodeCtx := range cm.all {
		nodeCtx.GetMetrics().status = Unknown
		nodeCtx.ClearErrors()
	}
}

func (cm *connectionManager) buildNodeInventory() {
	cm.bvnCtxMap = make(map[string][]ConnectionContext)

	for subnetName, addresses := range cm.accConfig.Network.Addresses {
		for _, address := range addresses {
			connCtx, err := cm.buildNodeContext(address, subnetName)
			if err != nil {
				cm.logger.Error("error building node context for node %s on net %s with type %s: %w, ignoring node...",
					connCtx.address, connCtx.subnetName, connCtx.nodeType, err)
				continue
			}

			switch connCtx.nodeType {
			case config.Validator:
				switch connCtx.netType {
				case config.BlockValidator:
					bvnName := protocol.BvnNameFromSubnetName(subnetName)
					if bvnName == "bvn-directory" {
						panic("Directory subnet node is misconfigured as blockvalidator")
					}
					nodeList, ok := cm.bvnCtxMap[bvnName]
					if !ok {
						nodeList := make([]ConnectionContext, 1)
						nodeList[0] = connCtx
						cm.bvnCtxMap[bvnName] = nodeList
					} else {
						cm.bvnCtxMap[bvnName] = append(nodeList, connCtx)
					}
					cm.all = append(cm.all, connCtx)
				case config.Directory:
					cm.dnCtxList = append(cm.dnCtxList, connCtx)
					cm.all = append(cm.all, connCtx)
				}
			case config.Follower:
				cm.fnCtxList = append(cm.fnCtxList, connCtx)
				cm.all = append(cm.all, connCtx)
			}
			if connCtx.networkGroup == Local {
				cm.localCtx = connCtx
			}
		}
	}
}

func (cm *connectionManager) buildNodeContext(address string, subnetName string) (*connectionContext, error) {
	nodeCtx := &connectionContext{subnetName: subnetName,
		address: address,
		connMgr: cm,
		metrics: NodeMetrics{status: Unknown}}
	nodeCtx.networkGroup = cm.determineNetworkGroup(subnetName, address)
	nodeCtx.netType, nodeCtx.nodeType = determineTypes(subnetName, cm.accConfig.Network)

	if address != "local" && address != "self" {
		var err error
		nodeCtx.resolvedIPs, err = resolveIPs(address)
		if err != nil {
			nodeCtx.ReportErrorStatus(Down, fmt.Errorf("error resolving IPs for %s: %w", address, err))
		}
	}
	return nodeCtx, nil
}

func (cm *connectionManager) determineNetworkGroup(subnetName string, address string) NetworkGroup {
	switch {
	case (strings.EqualFold(subnetName, cm.accConfig.Network.ID) && strings.EqualFold(cm.reformatAddress(address), cm.selfAddress)):
		return Local
	case strings.EqualFold(subnetName, cm.accConfig.Network.ID):
		return SameSubnet
	default:
		return OtherSubnet
	}
}

func (cm *connectionManager) reformatAddress(address string) string {
	if address == "local" || address == "self" {
		return cm.selfAddress
	}
	return strings.Split(address, "//")[1]
}

func determineTypes(subnetName string, netCfg config.Network) (config.NetworkType, config.NodeType) {
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

func (cm *connectionManager) InitClients(lclClient *local.Local, apiMethods ApiMethods) error {
	cm.localClient = lclClient
	cm.apiMethods = apiMethods

	for _, connCtxList := range cm.bvnCtxMap {
		for _, connCtx := range connCtxList {
			err := cm.createAbciClients(connCtx.(*connectionContext))
			if err != nil {
				return err
			}
		}
	}
	for _, nodeCtx := range cm.dnCtxList {
		err := cm.createAbciClients(nodeCtx.(*connectionContext))
		if err != nil {
			return err
		}
	}
	for _, nodeCtx := range cm.fnCtxList {
		err := cm.createAbciClients(nodeCtx.(*connectionContext))
		if err != nil {
			return err
		}
	}
	return nil
}

func (cm *connectionManager) createAbciClients(connCtx *connectionContext) error {
	switch connCtx.GetNetworkGroup() {
	case Local:
		connCtx.client = cm.localClient
	default:
		offsetAddr, err := config.OffsetPort(connCtx.GetAddress(), networks.TmRpcPortOffset)
		if err != nil {
			return fmt.Errorf("invalid BVN address: %v", err)
		}
		client, err := http.New(offsetAddr)
		if err != nil {
			return fmt.Errorf("failed to create RPC client: %v", err)
		}
		connCtx.client = client
		connCtx.SetNodeUrl(offsetAddr)
	}
	return nil
}

func resolveIPs(address string) ([]net.IP, error) {
	var hostname string
	nodeUrl, err := neturl.Parse(address)
	if err == nil {
		hostname = nodeUrl.Hostname()
	} else {
		hostname = address
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		return []net.IP{ip}, nil
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

func (m *NodeMetrics) SetStatus(status NodeStatus) {
	m.status = status
}
