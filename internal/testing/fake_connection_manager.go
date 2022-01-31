package testing

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client/local"
	"reflect"
	"strings"
)

type fakeConnectionManager struct {
	accConfig    *config.Accumulate
	bvnCtxMap    map[string][]connections.NodeContext
	clientMap    map[string][]FakeTendermint
	dnCtxList    []connections.NodeContext
	fnCtxList    []connections.NodeContext
	all          []connections.NodeContext
	localNodeCtx connections.NodeContext
	localClient  *local.Local
	logger       log.Logger
	selfAddress  string
}

func NewFakeConnectionManager(config *config.Config, logger log.Logger) connections.ConnectionManager {
	cm := new(fakeConnectionManager)
	cm.accConfig = &config.Accumulate
	cm.selfAddress = cm.reformatAddress(config.Config.P2P.ListenAddress)
	cm.logger = logger
	cm.buildNodeInventory()
	return cm
}

func (cm *fakeConnectionManager) AssignFakeClients(allNodes interface{}) error {
	cm.clientMap = make(map[string][]FakeTendermint)
	mapIter := reflect.ValueOf(allNodes).MapRange()
	for mapIter.Next() {
		netName := mapIter.Key().String()
		nodeCnt := mapIter.Value().Len()
		clients := make([]FakeTendermint, nodeCnt)
		for i := 0; i < nodeCnt; i++ {
			fn := mapIter.Value().Index(i).Interface()
			v := reflect.ValueOf(fn)
			client := v.MethodByName("GetClient").Call([]reflect.Value{})[0]
			ft := client.Interface().(*FakeTendermint)
			clients = append(clients, *ft)
		}
		cm.clientMap[netName] = clients
	}
	return nil
}

func (cm *fakeConnectionManager) GetLocalNodeContext() connections.NodeContext {
	return cm.localNodeCtx
}

func (cm *fakeConnectionManager) GetLocalClient() *local.Local {
	return cm.localClient
}

func (cm *fakeConnectionManager) GetBVNContextMap() map[string][]connections.NodeContext {
	return cm.bvnCtxMap
}

func (cm *fakeConnectionManager) GetDNContextList() []connections.NodeContext {
	return cm.dnCtxList
}

func (cm *fakeConnectionManager) GetFNContextList() []connections.NodeContext {
	return cm.fnCtxList
}

func (cm *fakeConnectionManager) GetAllNodeContexts() []connections.NodeContext {
	return cm.all
}

func (cm *fakeConnectionManager) ResetErrors() {
}

func (cm *fakeConnectionManager) reformatAddress(address string) string {
	if address == "local" || address == "self" {
		return cm.selfAddress
	}
	return strings.Split(address, "//")[1]
}

func (cm *fakeConnectionManager) buildNodeInventory() {
	cm.bvnCtxMap = make(map[string][]connections.NodeContext)

	for subnetName, addresses := range cm.accConfig.Network.Addresses {
		for _, address := range addresses {
			nodeCtx, err := cm.buildNodeContext(address, subnetName)
			if err != nil {
				cm.logger.Error("error building node context for node %s on net %s with type %s: %w, ignoring node...",
					nodeCtx.address, nodeCtx.subnetName, nodeCtx.nodeType, err)
				continue
			}

			switch nodeCtx.nodeType {
			case config.Validator:
				switch nodeCtx.netType {
				case config.BlockValidator:
					bvnName := protocol.BvnNameFromSubnetName(subnetName)
					if bvnName == "bvn-directory" {
						panic("Directory subnet node is misconfigured as blockvalidator")
					}
					nodeList, ok := cm.bvnCtxMap[bvnName]
					if !ok {
						nodeList := make([]connections.NodeContext, 1)
						nodeList[0] = nodeCtx
						cm.bvnCtxMap[bvnName] = nodeList
					} else {
						cm.bvnCtxMap[bvnName] = append(nodeList, nodeCtx)
					}
					cm.all = append(cm.all, nodeCtx)
				case config.Directory:
					cm.dnCtxList = append(cm.dnCtxList, nodeCtx)
					cm.all = append(cm.all, nodeCtx)
				}
			case config.Follower:
				cm.fnCtxList = append(cm.fnCtxList, nodeCtx)
				cm.all = append(cm.all, nodeCtx)
			}
			if nodeCtx.networkGroup == connections.Local {
				cm.localNodeCtx = nodeCtx
			}
		}
	}
}

func (cm *fakeConnectionManager) buildNodeContext(address string, subnetName string) (*fakeNodeContext, error) {
	nodeCtx := &fakeNodeContext{subnetName: subnetName,
		address: address,
		connMgr: cm,
	}
	nodeCtx.networkGroup = cm.determineNetworkGroup(subnetName, address)
	nodeCtx.netType, nodeCtx.nodeType = determineTypes(subnetName, cm.accConfig.Network)
	return nodeCtx, nil
}

func (cm *fakeConnectionManager) determineNetworkGroup(subnetName string, address string) connections.NetworkGroup {
	switch {
	case (strings.EqualFold(subnetName, cm.accConfig.Network.ID) && strings.EqualFold(cm.reformatAddress(address), cm.selfAddress)):
		return connections.Local
	case strings.EqualFold(subnetName, cm.accConfig.Network.ID):
		return connections.SameSubnet
	default:
		return connections.OtherSubnet
	}
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
