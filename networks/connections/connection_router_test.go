package connections_test

import (
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

const selfId = "BVN0"
const otherId = "BVN1"

func TestConnectionRouter(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 2, 2, 0)
	acctesting.RunTestNet(t, subnets, daemons)

	daemon0 := daemons[subnets[0]][0]
	connRouter := daemon0.ConnRouter

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
