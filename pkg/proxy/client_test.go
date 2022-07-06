package proxy

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
	stdlog "log"
	"math/big"
	"net/http"
	"os"
	"testing"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
)

//make up a fake network configuration list
var nodes = []config.Node{{Address: "127.0.0.1", Type: config.NodeTypeValidator}}
var partitions = []config.Partition{
	{Id: "Directory", Type: config.NetworkTypeDirectory, BasePort: 30000, Nodes: nodes},
	{Id: "BVN0", Type: config.NetworkTypeBlockValidator, BasePort: 40000, Nodes: nodes},
}
var network = config.Network{Id: "AccuProxyTest", Partitions: partitions}

// The RPC methods called in the JSON-RPC 2.0 specification examples.
func seedList(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	slr := SeedListRequest{}
	err := json.Unmarshal(params, &slr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := SeedListResponse{}
	snl := network.GetPartitionByID(slr.Partition)
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
	snl := network.GetPartitionByID(scr.Partition)
	resp.Count = int64(len(snl.Nodes))

	return resp
}

func getPartitions(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	scr := PartitionListRequest{}
	err := json.Unmarshal(params, &scr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := PartitionListResponse{}
	resp.Partitions = network.GetBvnNames()
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

func createProxyAccounts(node, nodeKey ed25519.PrivateKey) ed25519.PrivateKey {

	return ed25519.NewKeyFromSeed(nil)
}

func TestAccuProxyClient(t *testing.T) {
	// Create the lite addresses and one account
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	// Main identity
	accuProxy := protocol.AccountUrl("accuproxy.acme")
	accuProxyKey := acctesting.GenerateKey(nil)

	sim.CreateIdentity(accuProxy, accuProxyKey[32:])
	sim.CreateAccount(&protocol.TokenAccount{Url: accuProxy.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e9)})

	dnNetwork := protocol.DnUrl().JoinPath(protocol.Network)
	q := query.RequestByUrl{Url:dnNetwork}
	sim.Query(protocol.DnUrl(),&q, true) //don't really need to prove, but the converse is true, need to prove the dn.acme/network account


	createProxyAccounts(d.,key)

	go func() {
		// Register RPC methods.
		methods := jsonrpc2.MethodMap{
			"seed-list":  seedList,
			"seed-count": seedCount,
			"Partitions": getPartitions,
			"network":    getNetwork,
		}
		jsonrpc2.DebugMethodFunc = true
		handler := jsonrpc2.HTTPRequestHandler(methods, stdlog.New(os.Stdout, "", 0))
		require.NoError(t, http.ListenAndServe(":18888", handler))
	}()

	client, err := New(endpoint)
	require.NoError(t, err)

	ssr := PartitionListRequest{}
	ssr.Network = "AccuProxyTest"
	PartitionListResp, err := client.GetPartitionList(context.Background(), &ssr)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(PartitionListResp.Partitions), 1)

	scr := SeedCountRequest{}
	scr.Network = "AccuProxyTest"
	scr.Partition = PartitionListResp.Partitions[0]
	seedCountResp, err := client.GetSeedCount(context.Background(), &scr)
	require.NoError(t, err)

	slr := SeedListRequest{}
	slr.Network = "AccuProxyTest"
	slr.Partition = PartitionListResp.Partitions[0]
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
