package connections_test

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client/local"
	"net"
	"testing"
)

func TestConnectionRouter(t *testing.T) {
	networkCfg := mockNetworkCfg()
	thisAddress := networkCfg.Addresses[networkCfg.ID]
	connMgr, err := connections.NewConnectionManager(networkCfg, mockLocalNode(t, networkCfg.ID, thisAddress), makeLogger(t))
	require.NoError(t, err)

	connRouter := connections.NewConnectionRouter(connMgr)

	broadcastClient := connRouter.AcquireBroadcastClient()
	t.Logf("Broadcast client: %v", broadcastClient)

	route, err := connRouter.AcquireRoute("acc://bvn-"+networkCfg.ID+"/foo", false)
	require.NoError(t, err)
	t.Logf("Local client: %v", route)
	assert.Equal(t, connections.Local, route.GetNetworkGroup())
	assert.NotNil(t, route.GetBroadcastClient())

	otherBvn := networkCfg.BvnNames[1]
	otherClient, err := connRouter.AcquireRoute("acc://bvn-"+otherBvn+"/foo", false)
	require.NoError(t, err)
	t.Logf("Other client: %v", otherClient)
	assert.Equal(t, connections.OtherSubnet, otherClient.GetNetworkGroup())

	dnClient, err := connRouter.AcquireRoute("acc://dn/foo", false)
	require.NoError(t, err)
	t.Logf("DN client: %v", dnClient)
	assert.NotNil(t, dnClient.GetRpcHttpClient())

	queryClient1, err := connRouter.AcquireRoute("acc://RedWagon/foo", true)
	require.NoError(t, err)
	t.Logf("Query client 1: %v", queryClient1)
	assert.NotNil(t, queryClient1.GetQueryClient())

	queryClient2, err := connRouter.AcquireRoute("acc://RedWagon/foo", true)
	require.NoError(t, err)
	t.Logf("Query client 2: %v", queryClient2)

	queryClientLocalBvn, err := connRouter.AcquireRoute("acc://bvn-"+networkCfg.ID+"/foo", true)
	require.NoError(t, err)
	t.Logf("Query local BVN: %v", queryClientLocalBvn)
	assert.NotNil(t, queryClientLocalBvn.GetQueryClient())

	queryClientOtherBvn, err := connRouter.AcquireRoute("acc://bvn-"+otherBvn+"/foo", true)
	require.NoError(t, err)
	t.Logf("Query other BVN: %v", queryClientOtherBvn)
	assert.NotNil(t, queryClientOtherBvn.GetQueryClient())
}

func mockLocalNode(t *testing.T, id string, address []string) local.NodeService {
	bvc0 := initNodes(t, id, net.ParseIP("127.0.26.1"), 3000, 1, []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc0[0].Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) { require.NoError(t, err) })
	return bvc0[0].Node_TESTONLY().Service.(local.NodeService)
}

func mockNetworkCfg() *config.Network {
	networkCfg := new(config.Network)
	networkCfg.Addresses = make(map[string][]string)
	for _, subnet := range networks.SubnetsForNetwork("DevNet") {
		addresses := getAddresses(subnet.Nodes, subnet.Port, config.Validator)
		switch subnet.Type {
		case config.Directory:
			networkCfg.Addresses[protocol.Directory] = addresses
		case config.BlockValidator:
			networkCfg.BvnNames = append(networkCfg.BvnNames, subnet.Name)
			networkCfg.Addresses[subnet.Name] = addresses
			if len(networkCfg.ID) == 0 {
				networkCfg.ID = subnet.Name
				networkCfg.SelfAddress = addresses[0]
				networkCfg.Type = subnet.Type
			}
		}
	}
	return networkCfg
}

func getAddresses(nodes []networks.Node, port int, nodeType config.NodeType) []string {
	ret := make([]string, 0)
	for _, node := range nodes {
		if node.Type == nodeType {
			ret = append(ret, buildUrl(node.IP, port))
		}
	}
	return ret
}

func buildUrl(address string, port int) string {
	return fmt.Sprintf("http://%s:%d", address, port)
}

func makeLogger(t *testing.T) tmlog.Logger {
	w, _ := logging.TestLogWriter(t)("plain")
	zl := zerolog.New(w)
	tm, err := logging.NewTendermintLogger(zl, "error", false)
	require.NoError(t, err)
	return tm
}
