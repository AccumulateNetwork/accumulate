package connections

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/tendermint/tendermint/rpc/client/local"
	"github.com/ybbus/jsonrpc/v2"
	"strings"
	"sync/atomic"
)
import "github.com/AccumulateNetwork/accumulate/internal/url"

var dnNameMap = map[string]bool{
	"dn":   true,
	"acme": true,
}

type ConnectionRouter interface {
	SelectRoute(url string, allowFollower bool) (Route, error) // TODO: Check if we should take url.URL instead of string
	GetLocalRoute() (Route, error)
	GetAll() ([]Route, error)
	GetAllBVNs() ([]Route, error)
}

type connectionRouter struct {
	bvnGroup    nodeGroup
	dnGroup     nodeGroup
	flGroup     nodeGroup
	otherGroup  nodeGroup
	bvnNameMap  map[string]bool
	localClient *local.Local
}

type nodeGroup struct {
	nodes []*nodeContext
	next  uint32
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
}

func NewConnectionRouter(connMgr ConnectionManager) ConnectionRouter {
	cr := &connectionRouter{
		bvnNameMap:  createBvnNameMap(connMgr.getBVNContextList()),
		bvnGroup:    nodeGroup{nodes: connMgr.getBVNContextList()},
		dnGroup:     nodeGroup{nodes: connMgr.getDNContextList()},
		flGroup:     nodeGroup{nodes: connMgr.getFNContextList()},
		localClient: connMgr.GetLocalClient(),
	}
	otherNodes := append(append(cr.bvnGroup.nodes, cr.flGroup.nodes...)) // TODO Allow also DNs?
	cr.otherGroup = nodeGroup{nodes: otherNodes}
	return cr
}

func (cr *connectionRouter) SelectRoute(adiUrl string, allowFollower bool) (Route, error) {
	nodeCtx, err := cr.selectNodeContext(adiUrl, allowFollower)
	if err != nil {
		return nil, errorCouldNotSelectNode(adiUrl, err)
	}
	return nodeCtx, err
}

func (cr *connectionRouter) GetLocalRoute() (Route, error) {
	for _, nodeCtx := range cr.bvnGroup.nodes {
		if nodeCtx.networkGroup == Local {
			return nodeCtx, nil
		}
	}
	for _, nodeCtx := range cr.dnGroup.nodes {
		if nodeCtx.networkGroup == Local {
			return nodeCtx, nil
		}
	}
	for _, nodeCtx := range cr.flGroup.nodes {
		if nodeCtx.networkGroup == Local {
			return nodeCtx, nil
		}
	}
	return nil, LocaNodeNotFound
}

func (cr *connectionRouter) GetAll() ([]Route, error) {
	routes := make([]Route, 0)
	for _, route := range cr.otherGroup.nodes {
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
	for _, route := range cr.bvnGroup.nodes {
		if route.IsHealthy() {
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		return nil, NoHealthyNodes
	}
	return routes, nil
}

func (cr *connectionRouter) selectNodeContext(adiUrl string, allowFollower bool) (*nodeContext, error) {
	url, err := url.Parse(adiUrl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}
	hostnameLower := strings.ToLower(url.Hostname())
	switch {
	case cr.isBvnUrl(hostnameLower):
		return cr.lookupBvnNode(hostnameLower, cr.bvnGroup)
	case cr.isDnUrl(hostnameLower):
		return cr.lookupDirNode(cr.dnGroup)
	case allowFollower:
		return cr.selectNode(cr.otherGroup)
	default:
		return cr.selectNode(cr.bvnGroup)
	}
}

func (cr *connectionRouter) isBvnUrl(hostname string) bool {
	return cr.bvnNameMap[hostname]
}

func (cr *connectionRouter) isDnUrl(hostname string) bool {
	return dnNameMap[hostname]
}

func (cr *connectionRouter) lookupBvnNode(hostname string, group nodeGroup) (*nodeContext, error) {
	for _, nodeCtx := range group.nodes {
		if strings.HasPrefix(hostname, "bvn-") && strings.EqualFold(hostname[4:], nodeCtx.subnetName) ||
			strings.EqualFold(hostname, nodeCtx.subnetName) {
			if !nodeCtx.IsHealthy() {
				return nil, bvnNotHealthy(nodeCtx.address, nodeCtx.lastError)
			}
			return nodeCtx, nil
		}
	}

	// This state is not yet possible because we collected the names, but perhaps when the code starts changing and nodes are disabled/off-boarded
	return nil, bvnNotFound(hostname)
}

func (cr *connectionRouter) lookupDirNode(group nodeGroup) (*nodeContext, error) {
	if len(group.nodes) > 0 {
		nodeCtx := group.nodes[0]
		if !nodeCtx.IsHealthy() {
			return nil, dnNotHealthy(nodeCtx.address, nodeCtx.lastError)
		}
		return nodeCtx, nil
	}

	return nil, dnNotFound()
}

func (cr *connectionRouter) selectNode(group nodeGroup) (*nodeContext, error) {
	// If we only have one node we don't have to route
	if len(group.nodes) == 1 {
		nodeCtx := group.nodes[0]
		if !nodeCtx.IsHealthy() {
			return nil, errorNodeNotHealthy(nodeCtx.subnetName, nodeCtx.address, nodeCtx.lastError)
		}
		return nodeCtx, nil
	}

	// Loop in case we get one or more unhealthy nodes
	for i := 0; i < len(group.nodes); i++ {

		/* Apply round-robin on the nodes within the group
		This part is going to be smarter in the future to filter out unhealthy nodes and maybe determine the nodes load
		*/
		next := atomic.AddUint32(&group.next, 1)
		nodeCtx := group.nodes[int(next-1)%len(group.nodes)]
		if nodeCtx.IsHealthy() {
			return nodeCtx, nil
		}
	}
	return nil, NoHealthyNodes
}

func createBvnNameMap(nodes []*nodeContext) map[string]bool {
	bvnMap := make(map[string]bool)
	for _, node := range nodes {
		if node.netType == config.BlockValidator && node.nodeType == config.Validator {
			bvnName := strings.ToLower(node.subnetName)
			if !strings.HasPrefix(bvnName, "bvn-") {
				bvnName = "bvn-" + bvnName
			}
			bvnMap[bvnName] = true
		}
	}
	return bvnMap
}
