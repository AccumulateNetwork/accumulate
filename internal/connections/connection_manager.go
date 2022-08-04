package connections

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	rpc "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const UnhealthyNodeCheckInterval = time.Minute * 10 // TODO Configurable in toml?

type ConnectionManager interface {
	SelectConnection(partitionId string, allowFollower bool) (ConnectionContext, error)
}

type ConnectionInitializer interface {
	ConnectionManager
	InitClients(*local.Local, StatusChecker) error
	ConnectDirectly(other ConnectionManager) error
}

type connectionManager struct {
	accConfig *config.Accumulate
	bvnCtxMap map[string][]ConnectionContext
	dnCtxList []ConnectionContext
	fnCtxList []ConnectionContext
	all       []ConnectionContext
	localCtx  *connectionContext
	logger    logging.OptionalLogger
	publicKey []byte

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

type Options struct {
	Config           *config.Config
	Logger           log.Logger
	EventBus         *events.Bus
	Key              []byte
	ApiClientFactory func(string) (APIClient, error)
}

func NewConnectionManager(opts Options) ConnectionInitializer {
	cm := new(connectionManager)
	cm.accConfig = &opts.Config.Accumulate
	cm.apiClientFactory = opts.ApiClientFactory
	cm.logger.L = opts.Logger
	cm.publicKey = opts.Key[32:]
	events.SubscribeAsync(opts.EventBus, cm.buildNodeInventory)
	return cm
}

func (cm *connectionManager) SelectConnection(partitionId string, allowFollower bool) (ConnectionContext, error) {
	// When partitionId is the same as the current node's partition id, just return the local
	if strings.EqualFold(partitionId, cm.accConfig.PartitionId) {
		if cm.localCtx == nil {
			return nil, errNoLocalClient(partitionId)
		}
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

func (cm *connectionManager) buildNodeInventory(g events.WillChangeGlobals) {
	addresses := map[[32]byte]*protocol.InternetAddress{}
	for _, entry := range g.New.AddressBook.Entries {
		addresses[entry.PublicKeyHash] = entry.Address
	}

	for _, partition := range g.New.Network.Partitions {
		for _, validator := range partition.Validators {
			// Skip validators we don't have an address for
			h := sha256.Sum256(validator.PublicKey)
			if addresses[h] == nil {
				continue
			}

			connCtx := new(connectionContext)
			connCtx.partition = partition
			connCtx.validator = validator
			connCtx.address = addresses[h]
			connCtx.connMgr = cm
			connCtx.metrics = NodeMetrics{status: Unknown}
			connCtx.hasClient = make(chan struct{})

			switch {
			case bytes.Equal(validator.PublicKey, cm.publicKey):
				connCtx.networkGroup = Local
			case strings.EqualFold(partition.ID, cm.accConfig.PartitionId):
				connCtx.networkGroup = SamePartition
			default:
				connCtx.networkGroup = OtherPartition
			}

			cm.addConnection(connCtx)
		}
	}
}

func (cm *connectionManager) addConnection(cc *connectionContext) {
	if cc.networkGroup == Local {
		cm.localCtx = cc
	}

	var err error
	cc.resolvedIPs, err = resolveIPs(cc.address)
	if err != nil {
		cm.logger.Error(fmt.Sprintf("error resolving IPs for %q: %v", cc.address, err))
		cc.ReportErrorStatus(Down)
	}

	if !cc.validator.Active {
		cm.fnCtxList = append(cm.fnCtxList, cc)
		cm.all = append(cm.all, cc)
		return
	}

	switch cc.partition.Type {
	case protocol.PartitionTypeDirectory:
		cm.dnCtxList = append(cm.dnCtxList, cc)
		cm.all = append(cm.all, cc)

	case protocol.PartitionTypeBlockValidator:
		bvnName := protocol.BvnNameFromPartitionId(cc.partition.ID)
		if cc.partition.ID == protocol.Directory {
			panic("Directory partition node is misconfigured as blockvalidator")
		}
		nodeList, ok := cm.bvnCtxMap[bvnName]
		if ok {
			cm.bvnCtxMap[bvnName] = append(nodeList, cc)
		} else {
			nodeList := make([]ConnectionContext, 1)
			nodeList[0] = cc
			cm.bvnCtxMap[bvnName] = nodeList
		}
		cm.all = append(cm.all, cc)
	}
}

func (cm *connectionManager) InitClients(localABCI *local.Local, statusChecker StatusChecker) error {
	// TODO Support local API requests without any TCP call
	localAPI, err := cm.apiClientFactory(cm.accConfig.API.ListenAddress)
	if err != nil {
		return errCreateRPCClient(err)
	}
	cm.localCtx.setClient(localABCI, localAPI)

	for _, connCtxList := range cm.bvnCtxMap {
		for _, cc := range connCtxList {
			err := cm.createClient(cc.(*connectionContext), statusChecker)
			if err != nil {
				return err
			}
		}
	}
	for _, cc := range cm.dnCtxList {
		err := cm.createClient(cc.(*connectionContext), statusChecker)
		if err != nil {
			return err
		}
	}
	for _, cc := range cm.fnCtxList {
		err := cm.createClient(cc.(*connectionContext), statusChecker)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cm *connectionManager) ConnectDirectly(other ConnectionManager) error {
	cm2, ok := other.(*connectionManager)
	if !ok {
		return fmt.Errorf("incompatible connection managers: want %T, got %T", cm, cm2)
	}
	if cm2.localCtx == nil {
		return fmt.Errorf("other connection manager: local context has not been initialized")
	}
	if cm2.localCtx.abciClient == nil {
		return fmt.Errorf("other connection manager: local context's clients have not been initialized")
	}

	for _, connCtx := range cm.all {
		cc := connCtx.(*connectionContext)
		if !cc.address.Equal(cm2.accConfig.Network.Advertise) {
			continue
		}

		cc.setClient(cm2.localCtx.abciClient, cm2.localCtx.apiClient)
		return nil
	}

	return fmt.Errorf("cannot find %s node %s", cm2.accConfig.PartitionId, cm2.accConfig.Network.Advertise)
}

func (cm *connectionManager) createClient(cc *connectionContext, statusChecker StatusChecker) error {
	if cc == cm.localCtx {
		return nil
	}

	abci, err := http.New(cc.address.WithOffset(int(config.PortOffsetTendermintRpc)).String())
	if err != nil {
		return errCreateRPCClient(err)
	}
	api, err := cm.apiClientFactory(cc.address.WithOffset(int(config.PortOffsetAccumulateApi)).String())
	if err != nil {
		return errCreateRPCClient(err)
	}
	cc.statusChecker = statusChecker
	cc.setClient(abci, api)
	return nil
}

func resolveIPs(address *protocol.InternetAddress) ([]net.IP, error) {
	hostname := address.Hostname()
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
