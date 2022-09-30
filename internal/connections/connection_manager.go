// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package connections

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client"
	rpc "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const UnhealthyNodeCheckInterval = time.Minute * 10 // TODO Configurable in toml?

type ConnectionManager interface {
	SelectConnection(partitionId string, allowFollower bool) (ConnectionContext, error)
}

type ConnectionInitializer interface {
	ConnectionManager
	InitClients(client.Client, StatusChecker) error
	ConnectDirectly(other ConnectionManager) error
}

type connectionManager struct {
	accConfig   *config.Accumulate
	bvnCtxMap   map[string][]ConnectionContext
	dnCtxList   []ConnectionContext
	fnCtxList   []ConnectionContext
	all         []ConnectionContext
	localCtx    ConnectionContext
	localClient client.Client
	logger      logging.OptionalLogger
	localHost   string

	apiClientFactory func(string) (APIClient, error)
}

func (cm *connectionManager) doHealthCheckOnNode(connCtx *connectionContext) {
	newStatus := Down
	defer func() {
		connCtx.GetMetrics().status = newStatus
	}()

	// Try to query Tendermint with something it should not find
	qu := new(query.UnknownRequest)
	qd, _ := qu.MarshalBinary()
	qryRes, err := connCtx.GetABCIClient().ABCIQueryWithOptions(context.Background(), "/up", qd, rpc.DefaultABCIQueryOptions)
	if err != nil || protocol.ErrorCode(qryRes.Response.Code) != protocol.ErrorCodeOK {
		// FIXME code ErrorCodeInvalidQueryType will emit an error in the log, maybe there is a nicer option to probe the abci API
		connCtx.ReportError(err)
		if qryRes != nil {
			cm.logger.Info("ABCIQuery", "response", qryRes.Response)
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

func NewConnectionManager(config *config.Config, logger log.Logger, apiClientFactory func(string) (APIClient, error)) ConnectionInitializer {
	cm := new(connectionManager)
	cm.accConfig = &config.Accumulate
	cm.apiClientFactory = apiClientFactory
	cm.localHost = cm.reformatAddress(cm.accConfig.LocalAddress)
	if logger != nil {
		cm.logger.L = logger.With("module", "connection-manager")
	}
	cm.buildNodeInventory()
	return cm
}

func (cm *connectionManager) SelectConnection(partitionId string, allowFollower bool) (ConnectionContext, error) {
	// When partitionId is the same as the current node's partition id, just return the local
	if strings.EqualFold(partitionId, cm.accConfig.PartitionId) {
		if cm.localCtx == nil {
			return nil, errNoLocalClient(partitionId)
		}
		cm.logger.Debug("Selected connection", "partition", partitionId, "address", "self")
		return cm.localCtx, nil
	}

	bvnName := protocol.BvnNameFromPartitionId(partitionId)
	nodeList, ok := cm.bvnCtxMap[bvnName]
	if !ok {
		if strings.EqualFold(partitionId, "directory") {
			nodeList = cm.dnCtxList
		} else {
			return nil, errUnknownPartition(partitionId)
		}
	}

	healthyNodes := cm.getHealthyNodes(nodeList, allowFollower)
	if len(healthyNodes) == 0 {
		return nil, errNoHealthyNodes(partitionId) // None of the nodes in the partition could be reached
	}

	// Apply simple round-robin balancing to nodes in non-local partitions
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
	cm.logger.Debug("Selected connection", "partition", partitionId, "address", selCtx.GetAddress())
	return selCtx, nil
}

func (cm *connectionManager) getHealthyNodes(nodeList []ConnectionContext, allowFollower bool) []ConnectionContext {
	var healthyNodes = make([]ConnectionContext, 0)
	for _, connCtx := range nodeList {
		if (allowFollower || connCtx.GetNodeType() != config.Follower) && connCtx.IsHealthy() {
			healthyNodes = append(healthyNodes, connCtx)
		}
	}

	if len(healthyNodes) == 0 { // When there is no alternative node available in the partition, do another health check & try again
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

func (cm *connectionManager) ResetErrors() {
	for _, nodeCtx := range cm.all {
		nodeCtx.GetMetrics().status = Unknown
		nodeCtx.ClearErrors()
	}
}

func (cm *connectionManager) buildNodeInventory() {
	cm.bvnCtxMap = make(map[string][]ConnectionContext)

	for _, partition := range cm.accConfig.Network.Partitions {
		for _, node := range partition.Nodes {
			connCtx, err := cm.buildNodeContext(node, partition)
			if err != nil {
				cm.logger.Error("error building node context for node %s on net %s with type %s: %w, ignoring node...",
					connCtx.GetAddress(), connCtx.partitionId, connCtx.nodeConfig.Type, err)
				continue
			}

			switch connCtx.nodeConfig.Type {
			case config.Validator:
				switch connCtx.partition.Type {
				case config.BlockValidator:
					bvnName := protocol.BvnNameFromPartitionId(partition.Id)
					if partition.Id == protocol.Directory {
						panic("Directory partition node is misconfigured as blockvalidator")
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

func (cm *connectionManager) buildNodeContext(node config.Node, partition config.Partition) (*connectionContext, error) {
	connCtx := &connectionContext{partitionId: partition.Id,
		partition:  partition,
		nodeConfig: node,
		connMgr:    cm,
		metrics:    NodeMetrics{status: Unknown},
		hasClient:  make(chan struct{}),
	}
	connCtx.networkGroup = cm.determineNetworkGroup(partition.Id, node.Address)

	if node.Address != "local" && node.Address != "self" {
		var err error
		connCtx.resolvedIPs, err = resolveIPs(node.Address)
		if err != nil {
			cm.logger.Error(fmt.Sprintf("error resolving IPs for %q: %v", node.Address, err))
			connCtx.ReportErrorStatus(Down)
		}
	}
	return connCtx, nil
}

func (cm *connectionManager) determineNetworkGroup(partitionId string, address string) NetworkGroup {
	ownpartition := cm.accConfig.PartitionId
	fmtAddr := cm.reformatAddress(address)
	switch {
	case strings.EqualFold(partitionId, ownpartition) && strings.EqualFold(fmtAddr, cm.localHost):
		return Local
	case strings.EqualFold(partitionId, ownpartition):
		return SamePartition
	default:
		return OtherPartition
	}
}

func (cm *connectionManager) reformatAddress(address string) string {
	if address == "local" || address == "self" {
		return cm.localHost
	}
	if !strings.Contains(address, "//") {
		return address
	}

	url, err := url.Parse(address)
	if err != nil {
		return address
	}
	return url.Host
}

func (cm *connectionManager) InitClients(lclClient client.Client, statusChecker StatusChecker) error {
	cm.logger.Debug("Initializing clients")
	cm.localClient = lclClient

	for _, connCtxList := range cm.bvnCtxMap {
		for _, connCtx := range connCtxList {
			cc := connCtx.(*connectionContext)
			err := cm.createClient(cc)
			if err != nil {
				return err
			}
			cc.statusChecker = statusChecker
		}
	}
	for _, connCtx := range cm.dnCtxList {
		cc := connCtx.(*connectionContext)
		err := cm.createClient(cc)
		if err != nil {
			return err
		}
		cc.statusChecker = statusChecker
	}
	for _, connCtx := range cm.fnCtxList {
		cc := connCtx.(*connectionContext)
		err := cm.createClient(cc)
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

	for _, connCtx := range cm.all {
		cc := connCtx.(*connectionContext)
		url, err := url.Parse(cc.nodeConfig.Address)
		if err != nil {
			continue
		}

		if url.Host != cm2.accConfig.LocalAddress {
			continue
		}

		api, err := cm.apiClientFactory(cm2.accConfig.API.ListenAddress)
		if err != nil {
			return errCreateRPCClient(err)
		}
		cc.setClient(cm2.localClient, api)
		return nil
	}

	return fmt.Errorf("cannot find %s node %s", cm2.accConfig.PartitionId, cm2.accConfig.LocalAddress)
}

func (cm *connectionManager) createClient(connCtx *connectionContext) error {
	switch connCtx.GetNetworkGroup() {
	case Local:
		// TODO Support local API requests without any TCP call
		api, err := cm.apiClientFactory(cm.accConfig.API.ListenAddress)
		if err != nil {
			return errCreateRPCClient(err)
		}
		connCtx.setClient(cm.localClient, api)
	default:
		abciAddr, err := config.OffsetPort(connCtx.GetAddress(), connCtx.GetBasePort(), int(config.PortOffsetTendermintRpc))
		if err != nil {
			return errInvalidAddress(err)
		}
		abci, err := http.New(abciAddr.String())
		if err != nil {
			return errCreateRPCClient(err)
		}
		apiAddr, err := config.OffsetPort(connCtx.GetAddress(), connCtx.GetBasePort(), int(config.PortOffsetAccumulateApi))
		if err != nil {
			return errInvalidAddress(err)
		}
		api, err := cm.apiClientFactory(apiAddr.String())
		if err != nil {
			return errCreateRPCClient(err)
		}
		connCtx.setClient(abci, api)
		connCtx.SetNodeUrl(abciAddr)
	}
	return nil
}

func resolveIPs(address string) ([]net.IP, error) {
	var hostname string
	nodeUrl, err := url.Parse(address)
	if err == nil {
		hostname = nodeUrl.Hostname()
	} else {
		hostname = address
	}

	if hostname == "" {
		//no hostname, but url is valid? could happen if scheme is missing.
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
