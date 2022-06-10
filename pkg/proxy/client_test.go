package proxy

import (
	"context"
	"encoding/json"
	stdlog "log"
	"net/http"
	"os"
	"testing"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
)

//make up a fake network configuration list
var nodes = []config.Node{{Address: "127.0.0.1", Type: config.NodeTypeValidator}}
var subnets = []config.Subnet{
	{Id: "Directory", Type: config.NetworkTypeDirectory, BasePort: 30000, Nodes: nodes},
	{Id: "BVN0", Type: config.NetworkTypeBlockValidator, BasePort: 40000, Nodes: nodes},
}
var network = config.Network{Id: "AccuProxyTest", Subnets: subnets}

// The RPC methods called in the JSON-RPC 2.0 specification examples.
func seedList(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	slr := SeedListRequest{}
	err := json.Unmarshal(params, &slr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := SeedListResponse{}
	snl := network.GetSubnetByID(slr.Subnet)
	for _, n := range snl.Nodes {
		resp.Addresses = append(resp.Addresses, n.Address)
	}
	return resp
}

// The RPC methods called in the JSON-RPC 2.0 specification examples.
func seedCount(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	scr := SeedCountRequest{}
	err := json.Unmarshal(params, &scr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := SeedCountResponse{}
	snl := network.GetSubnetByID(scr.Subnet)
	resp.Count = int64(len(snl.Nodes))

	return resp
}

func getSubnets(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	scr := SubnetListRequest{}
	err := json.Unmarshal(params, &scr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := SubnetListResponse{}
	resp.Subnets = network.GetBvnNames()
	return resp
}

func getNetwork(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	ncr := NetworkConfigRequest{}
	err := json.Unmarshal(params, &ncr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := NetworkConfigResponse{}
	resp.Network = network
	return resp
}

var endpoint = "http://localhost:18888"

func TestAccuProxyClient(t *testing.T) {

	go func() {
		// Register RPC methods.
		methods := jsonrpc2.MethodMap{
			"seed-list":  seedList,
			"seed-count": seedCount,
			"subnets":    getSubnets,
			"network":    getNetwork,
		}
		jsonrpc2.DebugMethodFunc = true
		handler := jsonrpc2.HTTPRequestHandler(methods, stdlog.New(os.Stdout, "", 0))
		require.NoError(t, http.ListenAndServe(":18888", handler))
	}()

	client, err := New(endpoint)
	require.NoError(t, err)

	ssr := SubnetListRequest{}
	ssr.Network = "AccuProxyTest"
	subnetListResp, err := client.GetSubnetList(context.Background(), &ssr)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(subnetListResp.Subnets), 1)

	scr := SeedCountRequest{}
	scr.Network = "AccuProxyTest"
	scr.Subnet = subnetListResp.Subnets[0]
	seedCountResp, err := client.GetSeedCount(context.Background(), &scr)
	require.NoError(t, err)

	slr := SeedListRequest{}
	slr.Network = "AccuProxyTest"
	slr.Subnet = subnetListResp.Subnets[0]
	slr.Count = seedCountResp.Count
	seedListResp, err := client.GetSeedList(context.Background(), &slr)
	require.NoError(t, err)

	require.Equal(t, len(seedListResp.Addresses), len(nodes))

	ncr := NetworkConfigRequest{}
	ncr.Network = "AccuProxyTest"
	ncResp, err := client.GetNetworkConfig(context.Background(), &ncr)
	require.NoError(t, err)
	require.True(t, ncResp.Network.Equal(&network))

}
