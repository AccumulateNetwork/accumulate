package connections_test

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"net"
	"testing"
)

const selfId = "BVN0"
const otherId = "BVN1"

func TestConnectionRouter(t *testing.T) {
	_, connRouter := mockLocalNode(t)

	localRoute, err := connRouter.GetLocalRoute()
	broadcastClient := localRoute
	t.Logf("Broadcast client: %v", broadcastClient)

	route, err := connRouter.SelectRoute(toURL(t, "acc://bvn-"+selfId+"/foo"), false)
	require.NoError(t, err)
	t.Logf("Local client: %v", route)
	assert.Equal(t, connections.Local, route.GetNetworkGroup())
	assert.NotNil(t, route.GetBroadcastClient())

	otherClient, err := connRouter.SelectRoute(toURL(t, "acc://bvn-"+otherId+"/foo"), false)
	require.NoError(t, err)
	t.Logf("Other client: %v", otherClient)
	assert.Equal(t, connections.OtherSubnet, otherClient.GetNetworkGroup())

	/*	dnClient, err := connRouter.SelectRoute("acc://dn/foo", false) // The init node does not create a DN
		require.NoError(t, err)
		t.Logf("DN client: %v", dnClient)
		assert.NotNil(t, dnClient.GetJsonRpcClient())
	*/
	queryClient1, err := connRouter.SelectRoute(toURL(t, "acc://RedWagon/foo"), true)
	require.NoError(t, err)
	t.Logf("Query client 1: %v", queryClient1)
	assert.NotNil(t, queryClient1.GetQueryClient())

	queryClient2, err := connRouter.SelectRoute(toURL(t, "acc://RedWagon2345/foo"), true)
	require.NoError(t, err)
	t.Logf("Query client 2: %v", queryClient2)

	queryClientLocalBvn, err := connRouter.SelectRoute(toURL(t, "acc://bvn-"+selfId+"/foo"), true)
	require.NoError(t, err)
	t.Logf("Query local BVN: %v", queryClientLocalBvn)
	assert.NotNil(t, queryClientLocalBvn.GetQueryClient())

	queryClientOtherBvn, err := connRouter.SelectRoute(toURL(t, "acc://bvn-"+otherId+"/foo"), true)
	require.NoError(t, err)
	t.Logf("Query other BVN: %v", queryClientOtherBvn)
	assert.NotNil(t, queryClientOtherBvn.GetQueryClient())
}

func toURL(t *testing.T, urlString string) *url.URL {
	url, err := url.Parse(urlString)
	require.NoError(t, err)
	return url
}

func mockLocalNode(t *testing.T) (*node.Node, connections.ConnectionRouter) {
	bvc0 := initNodes(t, selfId, net.ParseIP("127.0.26.1"), 3000, 3, []string{"127.0.26.1", "127.0.26.2", "127.0.26.3"})
	bvc0[0].Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) { require.NoError(t, err) })
	return bvc0[0].Node_TESTONLY(), bvc0[0].ConnRouter
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
