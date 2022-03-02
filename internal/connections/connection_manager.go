package connections

import (
	"context"
	"fmt"
	"net"
	"net/url"
	neturl "net/url"
	"strings"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	rpc "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

const UnhealthyNodeCheckInterval = time.Minute * 10 // TODO Configurable in toml?

type ConnectionManager interface {
	SelectConnection(subnetId string, allowFollower bool) (ConnectionContext, error)
}

type ConnectionInitializer interface {
	ConnectionManager
	InitClients(*local.Local, StatusChecker) error
	ConnectDirectly(other ConnectionManager) error
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
	localHost   string
}

func (cm *connectionManager) doHealthCheckOnNode(connCtx *connectionContext) {
	newStatus := Down
	defer func() {
		connCtx.GetMetrics().status = newStatus
	}()

	// Try to query Tendermint with something it should not find
	qu := query.Query{}
	qd, _ := qu.MarshalBinary()
	qryRes, err := connCtx.GetClient().ABCIQueryWithOptions(context.Background(), "/abci_query", qd, rpc.DefaultABCIQueryOptions)
	if err != nil || protocol.ErrorCode(qryRes.Response.Code) != protocol.ErrorCodeInvalidQueryType {
		// FIXME code ErrorCodeInvalidQueryType will emit an error in the log, maybe there is a nicer option to probe the abci API
		connCtx.ReportError(err)
		if qryRes != nil {
			cm.logger.Info("ABCIQuery response: %v", qryRes.Response)
			newStatus = OutOfService
		}
		return
	}

	newStatus = Up

	/*	TODO The status check is not passing in the "validate docker" CI pipeline check. This means that the V2 API is not up while the Tendermint part is.
		    TODO Since we only need status info when we start to do advanced routing I've disabled this for now.
			res := connCtx.statusChecker.IsStatusOk(connCtx)
			if res {
				newStatus = Up
			} else {
				newStatus = OutOfService
			}
	*/
}

type NodeMetrics struct {
	status   NodeStatus
	usageCnt uint64
	// TODO add metrics that can be useful for the router to determine whether it should put or should avoid putting put more load on a BVN
}

func NewConnectionManager(config *config.Config, logger log.Logger) ConnectionInitializer {
	cm := new(connectionManager)
	cm.accConfig = &config.Accumulate
	cm.localHost = cm.reformatAddress(cm.accConfig.Network.LocalAddress)
	cm.logger = logger
	cm.buildNodeInventory()
	return cm
}

func (cm *connectionManager) SelectConnection(subnetId string, allowFollower bool) (ConnectionContext, error) {
	// When subnet is the same as the current node's subnet id, just return the local
	if strings.EqualFold(subnetId, cm.accConfig.Network.LocalSubnetID) {
		if cm.localCtx == nil {
			return nil, errNoLocalClient(subnetId)
		}
		return cm.localCtx, nil
	}

	bvnName := protocol.BvnNameFromSubnetId(subnetId)
	nodeList, ok := cm.bvnCtxMap[bvnName]
	if !ok {
		if strings.EqualFold(subnetId, "directory") {
			nodeList = cm.dnCtxList
		} else {
			return nil, errUnknownSubnet(subnetId)
		}
	}

	healthyNodes := cm.getHealthyNodes(nodeList, allowFollower)
	if len(healthyNodes) == 0 {
		return nil, errNoHealthyNodes(subnetId) // None of the nodes in the subnet could be reached
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

func (cm *connectionManager) getHealthyNodes(nodeList []ConnectionContext, allowFollower bool) []ConnectionContext {
	var healthyNodes = make([]ConnectionContext, 0)
	for _, connCtx := range nodeList {
		if allowFollower || connCtx.GetNodeType() != config.Follower && connCtx.IsHealthy() {
			healthyNodes = append(healthyNodes, connCtx)
		}
	}

	if len(healthyNodes) == 0 { // When there is no alternative node available in the subnet, do another health check & try again
		cm.ResetErrors()
		for _, connCtx := range nodeList {
			if connCtx.GetNodeType() != config.Follower && connCtx.IsHealthy() {
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

	for _, subnet := range cm.accConfig.Network.Subnets {
		for _, node := range subnet.Nodes {
			connCtx, err := cm.buildNodeContext(node, subnet)
			if err != nil {
				cm.logger.Error("error building node context for node %s on net %s with type %s: %w, ignoring node...",
					connCtx.GetAddress(), connCtx.subnetId, connCtx.nodeConfig.Type, err)
				continue
			}

			switch connCtx.nodeConfig.Type {
			case config.Validator:
				switch connCtx.subnet.Type {
				case config.BlockValidator:
					bvnName := protocol.BvnNameFromSubnetId(subnet.ID)
					if bvnName == "bvn-directory" {
						panic("Directory subnet node is misconfigured as blockvalidator")
					}
					nodeList, ok := cm.bvnCtxMap[bvnName]
					if ok {
						cm.bvnCtxMap[bvnName] = append(nodeList, connCtx)
					} else {
						nodeList := make([]ConnectionContext, 1)
						nodeList[0] = connCtx
						cm.bvnCtxMap[bvnName] = nodeList
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

func (cm *connectionManager) buildNodeContext(node config.Node, subnet config.Subnet) (*connectionContext, error) {
	connCtx := &connectionContext{subnetId: subnet.ID,
		subnet:     subnet,
		nodeConfig: node,
		connMgr:    cm,
		metrics:    NodeMetrics{status: Unknown},
		hasClient:  make(chan struct{}),
	}
	connCtx.networkGroup = cm.determineNetworkGroup(subnet.ID, node.Address)

	if node.Address != "local" && node.Address != "self" {
		var err error
		connCtx.resolvedIPs, err = resolveIPs(node.Address)
		if err != nil {
			cm.logger.Error("error resolving IPs for %q: %v", node.Address, err)
			connCtx.ReportErrorStatus(Down)
		}
	}
	return connCtx, nil
}

func (cm *connectionManager) determineNetworkGroup(subnetId string, address string) NetworkGroup {
	ownSubnet := cm.accConfig.Network.LocalSubnetID
	fmtAddr := cm.reformatAddress(address)
	switch {
	case strings.EqualFold(subnetId, ownSubnet) && strings.EqualFold(fmtAddr, cm.localHost):
		return Local
	case strings.EqualFold(subnetId, ownSubnet):
		return SameSubnet
	default:
		return OtherSubnet
	}
}

func (cm *connectionManager) reformatAddress(address string) string {
	if address == "local" || address == "self" {
		return cm.localHost
	}
	if !strings.Contains(address, "//") {
		return address
	}

	url, err := neturl.Parse(address)
	if err != nil {
		return address
	}
	return url.Host
}

func (cm *connectionManager) InitClients(lclClient *local.Local, statusChecker StatusChecker) error {
	cm.localClient = lclClient

	for _, connCtxList := range cm.bvnCtxMap {
		for _, connCtx := range connCtxList {
			cc := connCtx.(*connectionContext)
			err := cm.createAbciClient(cc)
			if err != nil {
				return err
			}
			cc.statusChecker = statusChecker
		}
	}
	for _, connCtx := range cm.dnCtxList {
		cc := connCtx.(*connectionContext)
		err := cm.createAbciClient(cc)
		if err != nil {
			return err
		}
		cc.statusChecker = statusChecker
	}
	for _, connCtx := range cm.fnCtxList {
		cc := connCtx.(*connectionContext)
		err := cm.createAbciClient(cc)
		if err != nil {
			return err
		}
		cc.statusChecker = statusChecker
	}
	return nil
}

func (cm *connectionManager) ConnectDirectly(other ConnectionManager) error {
	cm2, ok := other.(*connectionManager)
	if !ok {
		return fmt.Errorf("incompatible connection managers: want %T, got %T", cm, cm2)
	}

	var list []ConnectionContext
	if cm2.accConfig.Network.LocalSubnetID == protocol.Directory {
		list = cm.dnCtxList
	} else if list, ok = cm.bvnCtxMap[protocol.BvnNameFromSubnetId(cm2.accConfig.Network.LocalSubnetID)]; !ok {
		return fmt.Errorf("unknown subnet %q", cm2.accConfig.Network.LocalSubnetID)
	}

	for _, connCtx := range list {
		cc := connCtx.(*connectionContext)
		url, err := url.Parse(cc.nodeConfig.Address)
		if err != nil {
			continue
		}

		if url.Host != cm2.accConfig.Network.LocalAddress {
			continue
		}

		cc.setClient(cm2.localClient)
		return nil
	}

	return fmt.Errorf("cannot find %s node %s", cm2.accConfig.Network.LocalSubnetID, cm2.accConfig.Network.LocalAddress)
}

func (cm *connectionManager) createAbciClient(connCtx *connectionContext) error {
	switch connCtx.GetNetworkGroup() {
	case Local:
		connCtx.setClient(cm.localClient)
	default:
		offsetAddr, err := config.OffsetPort(connCtx.GetAddress(), networks.TmRpcPortOffset)
		if err != nil {
			return errInvalidAddress(err)
		}
		client, err := http.New(offsetAddr.String())
		if err != nil {
			return errCreateRPCClient(err)
		}
		connCtx.setClient(client)
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
	For mainnet, consider using DNS resolver with DNSSEC support for increased security, like go-resolver and query directly to a DNS server list that supports this, like 1.1.1.1
	*/
	ipList, err := net.LookupIP(hostname)
	if err != nil {
		return nil, errDNSLookup(hostname, err)
	}
	return ipList, nil
}
