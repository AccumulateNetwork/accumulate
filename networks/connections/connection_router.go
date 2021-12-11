package connections

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"strings"
	"sync/atomic"
)
import "github.com/AccumulateNetwork/accumulate/internal/url"

var dnNameMap = map[string]bool{
	"dn":   true,
	"acme": true,
}

type ConnectionRouter interface {
	AcquireRoute(url string, allowFollower bool) (Route, error)
	AcquireBroadcastClient() api.ABCIBroadcastClient
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

type Route interface {
	GetSubnetName() string
	GetNetworkGroup() NetworkGroup
	GetRpcHttpClient() *http.HTTP
	GetQueryClient() api.ABCIQueryClient
	GetBroadcastClient() api.ABCIBroadcastClient
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

func (cr *connectionRouter) AcquireRoute(adiUrl string, allowFollower bool) (Route, error) {
	nodeCtx, err := cr.selectNodeContext(adiUrl, allowFollower)
	if err != nil {
		return nil, errorCouldNotSelectNode(adiUrl, err)
	}
	return nodeCtx, err
}

func (cr *connectionRouter) AcquireBroadcastClient() api.ABCIBroadcastClient {
	return cr.localClient
}

func (cr *connectionRouter) selectNodeContext(adiUrl string, allowFollower bool) (*nodeContext, error) {
	url, err := url.Parse(adiUrl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	switch {
	case cr.isBvnUrl(url.Hostname()):
		return cr.lookupBvnNode(url.Hostname(), cr.bvnGroup)
	case cr.isDnUrl(url.Hostname()):
		return cr.lookupDirNode(cr.dnGroup)
	case allowFollower:
		return cr.selectNode(cr.otherGroup), nil
	default:
		return cr.selectNode(cr.bvnGroup), nil
	}
}

func (cr *connectionRouter) isBvnUrl(hostname string) bool {
	return cr.bvnNameMap[strings.ToLower(hostname)]
}

func (cr *connectionRouter) isDnUrl(hostname string) bool {
	hostnameLower := strings.ToLower(hostname)
	return dnNameMap[hostnameLower]
}

func (cr *connectionRouter) lookupBvnNode(hostname string, group nodeGroup) (*nodeContext, error) {
	for _, node := range group.nodes {
		if strings.HasPrefix(hostname, "bvn-") && strings.EqualFold(hostname[4:], node.subnetName) ||
			strings.EqualFold("bvn-"+hostname, node.subnetName) {
			return node, nil
		}
	}

	// This state is not yet possible because we collected the names, but perhaps when the code starts changing and nodes are disabled/off-boarded
	return nil, bvnNotFound(hostname)
}

func (cr *connectionRouter) lookupDirNode(group nodeGroup) (*nodeContext, error) {
	if len(group.nodes) > 0 {
		return group.nodes[0], nil
	}

	return nil, dnNotFound()
}

func (cr *connectionRouter) selectNode(group nodeGroup) *nodeContext {
	// If we only have one node we don't have to route
	if len(group.nodes) == 1 {
		return group.nodes[0]
	}

	/* Apply round-robin on the nodes within the group
	This part is going to be smarter in the future to filter out unhealthy nodes and maybe determine the nodes load
	*/
	next := atomic.AddUint32(&group.next, 1)
	return group.nodes[int(next-1)%len(group.nodes)]

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
