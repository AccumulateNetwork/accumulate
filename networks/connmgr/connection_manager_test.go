package connmgr_test

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/networks/connmgr"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client/local"
	"net"
	"testing"
)

func TestConnectionManager(t *testing.T) {
	networkCfg := mockNetworkCfg()
	connMgr, err := connmgr.NewConnectionManager(networkCfg, mockLocalNode(t), makeLogger(t))
	require.NoError(t, err)

	broadcastClient := connMgr.AcquireBroadcastClient()
	t.Logf("Broadcast client: %v", broadcastClient)

	lclClient, err := connMgr.AcquireRpcClient("acc://"+networkCfg.ID+"/foo", false)
	require.NoError(t, err)
	t.Logf("Local client: %v", lclClient)

	otherBvn := networkCfg.Addresses[networkCfg.BvnNames[1]][0]
	otherClient, err := connMgr.AcquireRpcClient("acc://"+otherBvn+"/foo", false)
	require.NoError(t, err)
	t.Logf("Other client: %v", otherClient)

	dnClient, err := connMgr.AcquireRpcClient("acc://dn/foo", false)
	require.NoError(t, err)
	t.Logf("DN client: %v", dnClient)

	queryClient1, err := connMgr.AcquireQueryClient("acc://RedWagon/foo")
	require.NoError(t, err)
	t.Logf("Query client 1: %v", queryClient1)

	queryClient2, err := connMgr.AcquireQueryClient("acc://RedWagon/foo")
	require.NoError(t, err)
	t.Logf("Query client 2: %v", queryClient2)

	queryClientLocalBvn, err := connMgr.AcquireQueryClient("acc://" + networkCfg.ID + "/foo")
	require.NoError(t, err)
	t.Logf("Query local BVN: %v", queryClientLocalBvn)

	queryClientOtherBvn, err := connMgr.AcquireQueryClient("acc://" + otherBvn + "/foo")
	require.NoError(t, err)
	t.Logf("Query other BVN: %v", queryClientOtherBvn)
}

func mockLocalNode(t *testing.T) local.NodeService {
	bvc0 := initNodes(t, "BVC0", net.ParseIP("127.0.26.1"), 3000, 1, []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc0[0].Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) { require.NoError(t, err) })
	return bvc0[0].Node_TESTONLY().Service.(local.NodeService)
}

func mockNetworkCfg() *config.Network {
	networkCfg := new(config.Network)
	networkCfg.Addresses = make(map[string][]string)
	for _, subnet := range networks.SubnetsForNetwork("DevNet") {
		switch subnet.Type {
		case config.Directory:
			networkCfg.Addresses[protocol.Directory] = getAddresses(subnet.Nodes, config.Validator)
		case config.BlockValidator:
			if len(networkCfg.ID) == 0 {
				networkCfg.ID = subnet.Name
				networkCfg.Type = subnet.Type
			}
			networkCfg.BvnNames = append(networkCfg.BvnNames, subnet.Name)
			networkCfg.Addresses[subnet.Name] = getAddresses(subnet.Nodes, config.Validator)
		}
	}
	return networkCfg
}

func getAddresses(nodes []networks.Node, nodeType config.NodeType) []string {
	ret := make([]string, 0)
	for _, node := range nodes {
		if node.Type == nodeType {
			ret = append(ret, node.IP)
		}
	}
	return ret
}

func makeLogger(t *testing.T) tmlog.Logger {
	w, _ := logging.TestLogWriter(t)("plain")
	zl := zerolog.New(w)
	tm, err := logging.NewTendermintLogger(zl, "error", false)
	require.NoError(t, err)
	return tm
}
