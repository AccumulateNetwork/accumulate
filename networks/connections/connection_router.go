package connections

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/rpc/client/local"
	"github.com/ybbus/jsonrpc/v2"
	"log"
	"strings"
	"sync/atomic"
)
import "github.com/AccumulateNetwork/accumulate/internal/url"

var dnNameMap = map[string]bool{
	"dn":   true,
	"acme": true,
}

type ConnectionRouter interface {
	SelectRoute(url *url.URL, allowFollower bool) (Route, error)
	GetLocalRoute() (Route, error)
	GetAll() ([]Route, error)
	GetAllBVNs() ([]Route, error)
}

type connectionRouter struct {
	bvnNames     []string
	bvnGroupMap  map[string]nodeGroup
	dnGroup      nodeGroup
	flGroup      nodeGroup
	localNodeCtx *nodeContext
	localClient  *local.Local
	isTest       bool
}

// Node group is just a list of nodeContext items and an int field for round-robin routing
type nodeGroup struct {
	nodes   []*nodeContext
	next    uint32
	nodeUrl *url.URL
}

type LocalRoute interface {
	GetSubnetName() string
	GetJsonRpcClient() jsonrpc.RPCClient
	GetQueryClient() ABCIQueryClient
	GetBroadcastClient() ABCIBroadcastClient
	IsDirectoryNode() bool
}

type Route interface {
	LocalRoute
	GetNetworkGroup() NetworkGroup
	GetBatchBroadcastClient() BatchABCIBroadcastClient
	ReportError(err error)
	ReportErrorStatus(status NodeStatus, err error)
}

func NewConnectionRouter(connMgr ConnectionManager, test bool) ConnectionRouter {
	bvnGroupMap := createBvnGroupMap(connMgr.getBVNContextMap())
	cr := &connectionRouter{
		bvnNames:     createKeyList(bvnGroupMap),
		bvnGroupMap:  bvnGroupMap,
		dnGroup:      nodeGroup{nodes: connMgr.getDNContextList()},
		flGroup:      nodeGroup{nodes: connMgr.getFNContextList()},
		localNodeCtx: connMgr.GetLocalNodeContext(),
		localClient:  connMgr.GetLocalClient(),
		isTest:       test,
	}
	return cr
}

func (cr *connectionRouter) SelectRoute(adiUrl *url.URL, allowFollower bool) (Route, error) {
	if cr.isTest && protocol.IsDnUrl(adiUrl) { // TODO remove hacks to accommodate testing code
		return cr.GetLocalRoute()
	}

	nodeCtx, err := cr.selectNodeContext(adiUrl, allowFollower)
	if err != nil {
		return nil, errorCouldNotSelectNode(adiUrl, err)
	}
	return nodeCtx, err
}

func (cr *connectionRouter) GetLocalRoute() (Route, error) {
	if cr.localNodeCtx == nil {
		return nil, LocaNodeNotFound
	}
	if !cr.localNodeCtx.IsHealthy() {
		return nil, LocaNodeNotHealthy
	}
	return cr.localNodeCtx, nil
}

// GetAll is not currently in use, but could be used to verify the health of a seed list in the future, otherwise this can be pruned
func (cr *connectionRouter) GetAll() ([]Route, error) {
	routes, _ := cr.GetAllBVNs()
	for _, route := range cr.dnGroup.nodes {
		if route.IsHealthy() {
			routes = append(routes, route)
		}
	}
	for _, route := range cr.flGroup.nodes {
		if route.IsHealthy() {
			routes = append(routes, route)
		}
	}

	if len(routes) == 0 {
		return nil, NoHealthyNodes
	}
	return routes, nil
}

func (cr *connectionRouter) GetAllBVNs() ([]Route, error) {
	routes := make([]Route, 0)
	for _, group := range cr.bvnGroupMap {
		bvn, err := cr.selectNodeFromGroup(group, false)
		if err != nil {
			return nil, err
		}
		routes = append(routes, bvn)
	}
	if len(routes) == 0 {
		return nil, NoHealthyNodes
	}
	return routes, nil
}

func (cr *connectionRouter) selectNodeContext(adiUrl *url.URL, allowFollower bool) (*nodeContext, error) {
	switch {
	case protocol.IsBvnUrl(adiUrl):
		return cr.lookupBvnNode(adiUrl)
	case protocol.IsDnUrl(adiUrl):
		return cr.selectDirNode()
	default:
		return cr.selectBvnNode(adiUrl, allowFollower)
	}
}

func (cr *connectionRouter) isBvnExists(hostname string) bool {
	_, ok := cr.bvnGroupMap[hostname]
	return ok
}

func (cr *connectionRouter) isDnExists(hostname string) bool {
	return dnNameMap[strings.ToLower(hostname)]
}

func (cr *connectionRouter) lookupBvnNode(adiUrl *url.URL) (*nodeContext, error) {
	bvnName := strings.ToLower(adiUrl.Hostname())
	bvnGroup, ok := cr.bvnGroupMap[bvnName]
	if !ok {
		return nil, bvnNotFound(adiUrl.String())
	}
	for _, nodeCtx := range bvnGroup.nodes {
		if nodeCtx.IsHealthy() {
			return nodeCtx, nil
		}
	}

	return nil, NoHealthyNodes
}

func (cr *connectionRouter) selectDirNode() (*nodeContext, error) {
	if cr.isDnExists(protocol.DnUrl().Hostname()) && len(cr.dnGroup.nodes) > 0 {
		nodeCtx := cr.dnGroup.nodes[0] // TODO follower support?
		if !nodeCtx.IsHealthy() {
			return nil, dnNotHealthy(nodeCtx.address, nodeCtx.lastError)
		}
		return nodeCtx, nil
	}
	return nil, dnNotFound()
}

func (cr *connectionRouter) selectBvnNode(adiUrl *url.URL, allowFollower bool) (*nodeContext, error) {
	bvnGroup := cr.selectBvnGroup(adiUrl)

	return cr.selectNodeFromGroup(bvnGroup, allowFollower)
}

func (cr *connectionRouter) selectNodeFromGroup(bvnGroup nodeGroup, allowFollower bool) (*nodeContext, error) {
	nodeCnt := len(bvnGroup.nodes)
	// If we only have one node we don't have to route
	if nodeCnt == 1 {
		nodeCtx := bvnGroup.nodes[0]
		if !nodeCtx.IsHealthy() {
			return nil, errorNodeNotHealthy(nodeCtx.subnetName, nodeCtx.address, nodeCtx.lastError)
		}
		return nodeCtx, nil
	}

	// Loop in case we get one or more unhealthy nodes
	for i := 0; i < nodeCnt; i++ {
		// Apply round-robin on the nodes within the group
		next := atomic.AddUint32(&bvnGroup.next, 1)
		nodeCtx := bvnGroup.nodes[int(next-1)%nodeCnt]
		if nodeCtx.IsHealthy() && (allowFollower || nodeCtx.nodeType == config.Validator) { // TODO Can BVN subnets also contain followers? (In networks.go on the DN has that)
			log.Println("   ==> selected address " + nodeCtx.address) // TODO remove after debug
			return nodeCtx, nil
		}
	}
	return nil, NoHealthyNodes
}

func (cr *connectionRouter) selectBvnGroup(adiUrl *url.URL) nodeGroup {
	// Create a fixed route which is based on the subnet name (rather than the order of them in the configuration)
	adiRoutingNr := adiUrl.Routing()
	var bvnGroup nodeGroup
	highestRoutingNr := uint64(0)
	for _, bvn := range cr.bvnGroupMap {
		// XOR every node url with the ADI URL, the node with the highest value wins.
		mergedRoutingNr := bvn.nodeUrl.Routing() ^ adiRoutingNr
		if mergedRoutingNr > highestRoutingNr {
			bvnGroup = bvn
			highestRoutingNr = mergedRoutingNr
		}
	}
	log.Printf("=====> selected %s for ADI %s\n", bvnGroup.nodeUrl.String(), adiUrl.String()) // TODO remove after debug
	return bvnGroup
}

func createBvnGroupMap(nodes map[string][]*nodeContext) map[string]nodeGroup {
	bvnMap := make(map[string]nodeGroup)
	for bvnName, nodeCtxList := range nodes {
		nodeUrl, _ := protocol.BuildNodeUrl(bvnName)
		bvnMap[bvnName] = nodeGroup{
			nodes:   nodeCtxList,
			next:    0,
			nodeUrl: nodeUrl,
		}
	}
	return bvnMap

}

func createKeyList(groupMap map[string]nodeGroup) []string {
	keys := make([]string, 0, len(groupMap))
	for k := range groupMap {
		keys = append(keys, k)
	}
	return keys
}
