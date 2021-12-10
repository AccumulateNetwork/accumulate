package connmgr

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"strings"
	"sync/atomic"
)
import "github.com/AccumulateNetwork/accumulate/internal/url"

var dnNameMap = map[string]bool{
	"dn":   true,
	"acme": true,
}

type connectionRouter struct {
	bvnGroup      nodeGroup
	dirGroup      nodeGroup
	followerGroup nodeGroup
	otherGroup    nodeGroup
	bvnNameMap    map[string]bool
}

type nodeGroup struct {
	nodes []*nodeContext
	next  uint32
}

func newConnectionRouter(bvNodes []*nodeContext, dirNodes []*nodeContext, followerNodes []*nodeContext) *connectionRouter {
	cr := new(connectionRouter)
	cr.bvnGroup = nodeGroup{nodes: bvNodes}
	cr.dirGroup = nodeGroup{nodes: dirNodes}
	cr.followerGroup = nodeGroup{nodes: followerNodes}
	otherNodes := append(append(cr.bvnGroup.nodes, cr.followerGroup.nodes...)) // TODO Allow also DNs?
	cr.otherGroup = nodeGroup{nodes: otherNodes}
	cr.bvnNameMap = createBvnNameMap(bvNodes)
	return cr
}

func (cr *connectionRouter) selectNodeContext(adiUrl string, allowFollower bool) (*nodeContext, error) {
	url, err := url.Parse(adiUrl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	switch {
	case cr.isBvnUrl(url.Hostname()):
		return cr.lookupNode(url.Hostname(), cr.bvnGroup)
	case cr.isDnUrl(url.Hostname()):
		return cr.lookupNode(url.Hostname(), cr.dirGroup)
	case allowFollower:
		return cr.selectNode(cr.otherGroup), nil
	default:
		return cr.selectNode(cr.bvnGroup), nil
	}
}

func (cr *connectionRouter) isBvnUrl(hostname string) bool {
	// TODO check if hostname is already lowercase
	return cr.bvnNameMap[strings.ToLower(hostname)]
}

func (cr *connectionRouter) isDnUrl(hostname string) bool {
	hostnameLower := strings.ToLower(hostname)
	return dnNameMap[hostnameLower]
}

func (cr *connectionRouter) lookupNode(hostname string, group nodeGroup) (*nodeContext, error) {
	for _, node := range group.nodes {
		if strings.EqualFold(hostname, node.subnetName) {
			return node, nil
		}
	}

	// This state is not yet possible because we collected the names, but perhaps when the code starts changing and nodes are disabled/off-boarded
	return nil, bvnNotFound(hostname)
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
			bvnMap[strings.ToLower(node.subnetName)] = true
		}
	}
	return bvnMap
}
