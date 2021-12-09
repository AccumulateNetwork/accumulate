package connmgr

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client/http"
	"net"
)

type ConnectionManagerOpts struct {
	NetworkName   string
	selfAddresses []string
	logger        log.Logger
}

type NodeStatus int

const (
	Up           NodeStatus = iota // Healthy & ready to go
	Down                           // Not reachable
	OutOfService                   // Reachable but not ready to go (IE. still syncing up)
)

type NetworkGroup int

const (
	Self NetworkGroup = iota
	SameSubnet
	OtherSubnet
)

type nodeMetrics struct {
	status    NodeStatus
	latencyMS uint32
	// TODO add metrics that can be useful for the router to determine whether it should put or should avoid putting put more load on a BVN
}

type nodeContext struct {
	node            networks.Node
	subnet          *networks.Subnet
	resolvedIPs     []net.IP
	metrics         nodeMetrics
	networkGroup    NetworkGroup
	queryClient     api.ABCIQueryClient
	broadcastClient api.ABCIBroadcastClient
	rpcHttpClient   *http.HTTP
}

type connectionManager struct {
	network             networks.Network
	opts                ConnectionManagerOpts
	selfRemotesMap      map[string]bool
	selfSubnet          *networks.Subnet
	nodeToSubnetMap     map[string]*networks.Subnet
	bvNodeCtxList       []*nodeContext
	dirNodeCtxList      []*nodeContext
	followerNodeCtxList []*nodeContext
	logger              log.Logger
}

func newConnectionManager(opts ConnectionManagerOpts) (*connectionManager, error) {
	cm := new(connectionManager)
	cm.opts = opts
	cm.network = networks.Networks[opts.NetworkName]
	cm.selfRemotesMap = buildSelfRemotesMap(cm.opts.selfAddresses)
	cm.loadNodeSubnetMap()
	for _, subnet := range cm.network {
		cm.collectNodes(subnet)
	}
	cm.createBvClients()
	cm.createDnClients()
	return cm, nil
}

func (cm connectionManager) loadNodeSubnetMap() {
	for _, subnet := range cm.network {
		for _, node := range subnet.Nodes {
			cm.nodeToSubnetMap[node.IP] = subnet
			if cm.selfRemotesMap[node.IP] {
				cm.selfSubnet = subnet
			}
		}
	}
}

func buildSelfRemotesMap(selfAddresses []string) map[string]bool {
	ret := make(map[string]bool)
	for _, selfAddress := range selfAddresses {
		ret[selfAddress] = true
	}
	/* TODO
	May need refining, we need somehow exclude ourselves from the network list so we don't self-route over the network.
	An option could be that we collect an ADI URL of every node in the network and use that, when possible that is.
	*/
	return ret
}

func (cm connectionManager) collectNodes(subnet *networks.Subnet) {
	for _, node := range subnet.Nodes {
		context, err := cm.buildNodeContext(subnet, node)
		if err != nil {
			cm.logger.Error("error building node context for %s %s: %w, ignoring node...", node.Type, node.IP, err)
			continue
		}

		switch node.Type {
		case config.Validator:
			switch subnet.Type {
			case config.BlockValidator:
				cm.bvNodeCtxList = append(cm.bvNodeCtxList, context)
			case config.Directory:
				cm.dirNodeCtxList = append(cm.dirNodeCtxList, context)
			}
		case config.Follower:
			cm.followerNodeCtxList = append(cm.followerNodeCtxList, context)
		}
	}
}

func (cm connectionManager) buildNodeContext(subnet *networks.Subnet, node networks.Node) (*nodeContext, error) {
	ctx := &nodeContext{node: node, subnet: subnet}
	ctx.networkGroup = cm.determineNetworkGroup(node)

	var err error
	ctx.resolvedIPs, err = resolveIPs(node.IP)
	if err != nil {
		return nil, fmt.Errorf("error resolving IPs for %s: %w", node.IP, err)
	}
	return ctx, nil

}

func (cm connectionManager) determineNetworkGroup(node networks.Node) NetworkGroup {
	switch {
	case cm.selfRemotesMap[node.IP]:
		return Self
	case cm.nodeToSubnetMap[node.IP] == cm.selfSubnet:
		return SameSubnet
	default:
		return OtherSubnet
	}
}

func (cm connectionManager) createBvClients() {
	// TODO
}

func (cm connectionManager) createDnClients() {
	// TODO
}

func resolveIPs(remote string) ([]net.IP, error) {
	ip := net.ParseIP(remote)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	/* TODO
	consider using DNS resolver with DNSSEC support like go-resolver and query directly to a DNS server list that supports this, like 1.1.1.1
	*/
	var err error
	ipList, err := net.LookupIP(remote)
	if err != nil {
		return nil, fmt.Errorf("error doing DNS lookup for %s: %w", remote, err)
	}
	return ipList, nil
}
