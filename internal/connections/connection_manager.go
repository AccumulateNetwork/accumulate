package connections

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	rpc "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
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
	globals          *core.GlobalValues
	accConfig        *config.Accumulate
	apiClientFactory func(string) (APIClient, error)
	localCtx         *connectionContext
	connections      map[[32]byte][]*connectionContext
	logger           logging.OptionalLogger
	publicKey        []byte
	statusChecker    StatusChecker

	memoize struct {
		sync.RWMutex
		byPartition map[string][]*connectionContext
	}
}

func (cm *connectionManager) connectionsForPartition(id string) []*connectionContext {
	id = strings.ToLower(id)

	// Acquire a read lock and check if memoization is done
	cm.memoize.RLock()
	if cm.memoize.byPartition != nil {
		defer cm.memoize.RUnlock()
		return cm.memoize.byPartition[id]
	}
	cm.memoize.RUnlock()

	// Acquire a write lock and verify that memoization was not done by someone else
	cm.memoize.Lock()
	defer cm.memoize.Unlock()
	if cm.memoize.byPartition != nil {
		return cm.memoize.byPartition[id]
	}

	cm.memoize.byPartition = map[string][]*connectionContext{}
	for _, cc := range cm.connections {
		for _, cc := range cc {
			id := strings.ToLower(cc.partitionId)
			cm.logger.Debug("Assigning connection to partition", "key", logging.AsHex(cc.validatorPubKey).Slice(0, 4), "address", cc.address, "partition", cc.partitionId)
			cm.memoize.byPartition[id] = append(cm.memoize.byPartition[id], cc)
		}
	}

	for _, ccs := range cm.memoize.byPartition {
		sort.Slice(ccs, func(i, j int) bool { return bytes.Compare(ccs[i].validatorPubKey, ccs[j].validatorPubKey) <= 0 })
	}
	return cm.memoize.byPartition[id]
}

func (cm *connectionManager) doHealthCheckOnNode(connCtx *connectionContext) {
	newStatus := Down
	defer func() {
		connCtx.metrics.status = newStatus
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
	if opts.Logger != nil {
		cm.logger.L = opts.Logger.With("module", "connections")
	}
	cm.publicKey = opts.Key[32:]
	cm.connections = map[[32]byte][]*connectionContext{}

	if len(cm.accConfig.Network.Seeds) == 0 {
		cm.buildStaticNodeInventory()
	}

	events.SubscribeAsync(opts.EventBus, cm.updateNodeInventory)
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

	nodeList := cm.connectionsForPartition(strings.ToLower(partitionId))
	if nodeList == nil {
		return nil, errUnknownPartition(partitionId)
	}

	healthyNodes := cm.getHealthyNodes(nodeList, allowFollower)
	if len(healthyNodes) == 0 {
		return nil, errNoHealthyNodes(partitionId) // None of the nodes in the partition could be reached
	}

	// Apply simple round-robin balancing to nodes in non-local partitions
	var selCtx *connectionContext
	selCtxCnt := ^uint64(0)
	for _, connCtx := range healthyNodes {
		usageCnt := connCtx.metrics.usageCnt
		if usageCnt < selCtxCnt {
			selCtx = connCtx
			selCtxCnt = usageCnt
		}
	}
	selCtx.metrics.usageCnt++
	return selCtx, nil
}

func (cm *connectionManager) getHealthyNodes(nodeList []*connectionContext, allowFollower bool) []*connectionContext {
	var healthyNodes = make([]*connectionContext, 0)
	for _, connCtx := range nodeList {
		if (allowFollower || connCtx.nodeType != config.Follower) && connCtx.isHealthy() {
			healthyNodes = append(healthyNodes, connCtx)
		}
	}

	if len(healthyNodes) == 0 { // When there is no alternative node available in the partition, do another health check & try again
		cm.resetErrors()
		for _, connCtx := range nodeList {
			if connCtx.nodeType != config.Follower && connCtx.isHealthy() {
				healthyNodes = append(healthyNodes, connCtx)
			}
		}
	}
	return healthyNodes
}

func (cm *connectionManager) resetErrors() {
	for _, cc := range cm.connections {
		for _, cc := range cc {
			cc.metrics.status = Unknown
			cc.clearErrors()
		}
	}
}

func (cm *connectionManager) updateNodeInventory(e events.WillChangeGlobals) {
	cm.memoize.Lock()
	defer cm.memoize.Unlock()
	cm.memoize.byPartition = nil

	isFirst := cm.globals == nil
	cm.globals = e.New
	if isFirst {
		cm.initFromGlobals(e.New)
		return
	}

	addresses := e.Old.DiffAddressBook(e.New)
	myKeyHash := sha256.Sum256(cm.publicKey)
	for keyHash, address := range addresses {
		// Don't touch the local node context
		if keyHash == myKeyHash {
			delete(addresses, keyHash)
			continue
		}

		if len(cm.connections[keyHash]) == 0 {
			// New connection
			continue
		} else {
			// Don't re-add existing connections
			delete(addresses, keyHash)
		}

		// Remove connections if the node is down
		if address == nil {
			delete(cm.connections, keyHash)
			continue
		}

		for _, cc := range cm.connections[keyHash] {
			if strings.EqualFold(cc.partitionId, protocol.Directory) {
				cc.address = address.WithOffset(config.PortOffsetDirectory)
			} else {
				cc.address = address.WithOffset(config.PortOffsetBlockValidator)
			}
			cc.resetClient()
		}
	}

	// Add new connections
	cm.addConnections(e.New, addresses)

	if cm.statusChecker != nil {
		err := cm.initClients()
		if err != nil {
			cm.logger.Error("Failed to initialize new clients", "error", err)
		}
	}
}

func (cm *connectionManager) initFromGlobals(g *core.GlobalValues) {
	addresses := map[[32]byte]*protocol.InternetAddress{}
	for _, entry := range g.AddressBook.Entries {
		addresses[entry.PublicKeyHash] = entry.Address
	}
	cm.addConnections(g, addresses)
}

func (cm *connectionManager) addConnections(g *core.GlobalValues, addresses map[[32]byte]*protocol.InternetAddress) {
	for _, partition := range g.Network.Partitions {
		isDir := strings.EqualFold(partition.PartitionID, protocol.Directory)
		for _, validator := range partition.ValidatorKeys {
			// Skip validators we don't have an address for
			address := addresses[sha256.Sum256(validator)]
			if address == nil {
				continue
			}
			if !isDir {
				address = address.WithOffset(config.PortOffsetBlockValidator - config.PortOffsetDirectory)
			}

			connCtx := new(connectionContext)
			connCtx.partitionId = partition.PartitionID
			connCtx.validatorPubKey = validator
			connCtx.address = address
			connCtx.connMgr = cm
			connCtx.metrics = NodeMetrics{status: Unknown}
			connCtx.resetClient()
			cm.addConnection(connCtx)
		}
	}
}

func (cm *connectionManager) buildStaticNodeInventory() {
	for _, partition := range cm.accConfig.Network.Partitions {
		for _, validator := range partition.Nodes {
			connCtx := new(connectionContext)
			connCtx.partitionId = partition.Id
			connCtx.validatorPubKey = validator.PublicKey
			connCtx.address = validator.Address
			connCtx.connMgr = cm
			connCtx.metrics = NodeMetrics{status: Unknown}
			connCtx.resetClient()
			cm.addConnection(connCtx)
		}
	}
}

func (cm *connectionManager) addConnection(cc *connectionContext) {
	pubKeyHash := sha256.Sum256(cc.validatorPubKey)
	for _, entry := range cm.accConfig.Network.Ignore {
		if entry.PublicKeyHash == pubKeyHash && entry.Address.Equal(cc.address) {
			return // Ignore this connection
		}
	}

	cm.logger.Info("Add connection", "key", logging.AsHex(cc.validatorPubKey).Slice(0, 4), "address", cc.address, "partition", cc.partitionId)

	switch {
	case bytes.Equal(cc.validatorPubKey, cm.publicKey) && cc.address.Equal(cm.accConfig.Advertise):
		cc.networkGroup = Local
		cm.localCtx = cc
	case strings.EqualFold(cc.partitionId, cm.accConfig.PartitionId):
		cc.networkGroup = SamePartition
	default:
		cc.networkGroup = OtherPartition
	}

	var err error
	cc.resolvedIPs, err = resolveIPs(cc.address)
	if err != nil {
		cm.logger.Error(fmt.Sprintf("error resolving IPs for %q: %v", cc.address, err))
		cc.reportErrorStatus(Down)
	}

	cm.connections[pubKeyHash] = append(cm.connections[pubKeyHash], cc)
}

func (cm *connectionManager) InitClients(localABCI *local.Local, statusChecker StatusChecker) error {
	cm.statusChecker = statusChecker

	// TODO Support local API requests without any TCP call
	localAPI, err := cm.apiClientFactory(cm.accConfig.API.ListenAddress)
	if err != nil {
		return errCreateRPCClient(err)
	}
	cm.localCtx.setClient(localABCI, localAPI)

	return cm.initClients()
}

func (cm *connectionManager) initClients() error {
	for _, cc := range cm.connections {
		for _, cc := range cc {
			err := cm.createClient(cc, cm.statusChecker)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (cm *connectionManager) ConnectDirectly(other ConnectionManager) error {
	cm2, ok := other.(*connectionManager)
	if !ok {
		return fmt.Errorf("incompatible connection managers: want %T, got %T", cm, cm2)
	}

	cc2 := cm2.localCtx
	if cc2 == nil {
		return fmt.Errorf("local context has not been initialized")
	}

	// Replace the connection context with the other connection manager's local connection
	pubKeyHash := sha256.Sum256(cc2.validatorPubKey)
	list := cm.connections[pubKeyHash]
	if len(list) > 2 {
		panic("How!")
	}
	for i, cc := range list {
		if cc.address.Equal(cc2.address) {
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	cm.connections[pubKeyHash] = append(list, cc2)
	return nil
}

func (cm *connectionManager) createClient(cc *connectionContext, statusChecker StatusChecker) error {
	if cc.networkGroup == Local || cc.networkGroup == Direct || cc.statusChecker != nil {
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
