package testing

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api"
	stdlog "log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var AccuProxyAuthorityKey = acctesting.GenerateKey(nil)
var AccuProxyKey = acctesting.GenerateKey(nil)
var AccuProxy = protocol.AccountUrl("accuproxy.acme")

//LaunchAccuProcyDevNet will launch a devnet with the accuproxy.acme adi and authorized key(s)
func LaunchAccuProxyDevNet(t *testing.T) *client.Client {
	t.Helper()
	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	acctesting.RunTestNet(t, partitions, daemons)
	time.Sleep(time.Second)
	c := daemons[partitions[1]][0].Config
	n := daemons[partitions[1]][0]

	client, err := client.New(c.Accumulate.API.ListenAddress)
	require.NoError(t, err)

	//shortcut to create the account, add the key
	batch := n.DB_TESTONLY().Begin(true)
	tmAuthorityKey := tmed25519.GenPrivKeyFromSecret(AccuProxyAuthorityKey.Seed())
	tmKey := tmed25519.GenPrivKeyFromSecret(AccuProxyKey.Seed())
	require.NoError(t, acctesting.CreateADI(batch, tmAuthorityKey, "accuproxy.acme"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "accuproxy.acme/registry", tmKey.PubKey().(tmed25519.PubKey)))
	require.NoError(t, batch.Commit())

	return client
}

//make up a fake default network configuration list
var Nodes = []config.Node{{Address: "127.0.0.1", Type: config.NodeTypeValidator}}
var Partitions = []config.Partition{
	{Id: "Directory", Type: config.NetworkTypeDirectory, BasePort: 30000, Nodes: Nodes},
	{Id: "BVN0", Type: config.NetworkTypeBlockValidator, BasePort: 40000, Nodes: Nodes},
}
var Network = config.Network{Id: "AccuProxyTest", Partitions: Partitions}

// The RPC methods called in the JSON-RPC 2.0 specification examples.
func seedList(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	slr := proxy.SeedListRequest{}
	err := json.Unmarshal(params, &slr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := proxy.SeedListResponse{}
	snl := Network.GetPartitionByID(slr.Partition)
	for _, n := range snl.Nodes {
		resp.Addresses = append(resp.Addresses, n.Address)
	}

	if slr.Sign {
		s, err := resp.SeedList.MarshalBinary()
		if err != nil {
			return jsonrpc2.ErrorInvalidParams("cannot marshal payload")
		}
		sig := protocol.ED25519Signature{}
		sig.Signer = AccuProxy
		sig.PublicKey = AccuProxyKey.Public().(ed25519.PublicKey)
		sig.Timestamp = uint64(time.Now().UnixNano())
		txnHash := sha256.Sum256(s)
		protocol.SignED25519(&sig, AccuProxyKey, txnHash[:])
		resp.Signature = &sig
	}
	return resp
}

// The RPC methods called in the JSON-RPC 2.0 specification examples.
func seedCount(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	scr := proxy.SeedCountRequest{}
	err := json.Unmarshal(params, &scr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := proxy.SeedCountResponse{}

	snl := Network.GetPartitionByID(scr.Partition)
	resp.Count = int64(len(snl.Nodes))

	if scr.Sign {
		s, err := resp.SeedCount.MarshalBinary()
		if err != nil {
			return jsonrpc2.ErrorInvalidParams("cannot marshal payload")
		}
		sig := protocol.ED25519Signature{}
		sig.Signer = AccuProxy
		sig.PublicKey = AccuProxyKey.Public().(ed25519.PublicKey)
		sig.Timestamp = uint64(time.Now().UnixNano())
		txnHash := sha256.Sum256(s)
		protocol.SignED25519(&sig, AccuProxyKey, txnHash[:])
		resp.Signature = &sig
	}

	return resp
}

func getPartitions(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	scr := proxy.PartitionListRequest{}
	err := json.Unmarshal(params, &scr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := proxy.PartitionListResponse{}
	resp.Partitions = Network.GetBvnNames()

	if scr.Sign {
		s, err := resp.PartitionList.MarshalBinary()
		if err != nil {
			return jsonrpc2.ErrorInvalidParams("cannot marshal payload")
		}
		sig := protocol.ED25519Signature{}
		sig.Signer = AccuProxy
		sig.PublicKey = AccuProxyKey.Public().(ed25519.PublicKey)
		sig.Timestamp = uint64(time.Now().UnixNano())
		txnHash := sha256.Sum256(s)
		protocol.SignED25519(&sig, AccuProxyKey, txnHash[:])
		resp.Signature = &sig
	}

	return resp
}

func getNetwork(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	ncr := proxy.NetworkConfigRequest{}
	err := json.Unmarshal(params, &ncr)
	if err != nil {
		return jsonrpc2.ErrorInvalidParams("Invalid SeedListRequest parameters")
	}

	resp := proxy.NetworkConfigResponse{}
	resp.Network = Network

	if ncr.Sign {
		s, err := resp.Network.MarshalBinary()
		if err != nil {
			return jsonrpc2.ErrorInvalidParams("cannot marshal payload")
		}
		sig := protocol.ED25519Signature{}
		sig.Signer = AccuProxy
		sig.PublicKey = AccuProxyKey.Public().(ed25519.PublicKey)
		sig.Timestamp = uint64(time.Now().UnixNano())
		txnHash := sha256.Sum256(s)
		protocol.SignED25519(&sig, AccuProxyKey, txnHash[:])
		resp.Signature = &sig
	}

	return resp
}

var Endpoint = "http://localhost:18888"

func LaunchFakeProxy(t *testing.T) (*proxy.Client, *client.Client) {
	t.Helper()

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

	proxyClient, err := proxy.New(Endpoint)
	require.NoError(t, err)

	//now spawn our little testnet and populate the fake proxy with the network info
	accClient := LaunchAccuProxyDevNet(t)
	ProveNetworkToFakeProxy(t, accClient)

	return proxyClient, accClient
}

func ProveNetworkToFakeProxy(t *testing.T, client *client.Client) {
	//this is provided as an example for the proxy to prove the contents of the "Describe" api call.
	var res interface{}

	err := client.RequestAPIv2(context.Background(), "describe", nil, &res)
	if err != nil {
		require.NoError(t, err)
	}

	str, err := json.Marshal(res)
	require.NoError(t, err)

	d := config.Describe{}
	require.NoError(t, d.UnmarshalJSON([]byte(str)))

	//now query the dn/network with prove capability to get the network names
	networkUrl := protocol.DnUrl().JoinPath("network")
	gq := api.GeneralQuery{}
	gq.Prove = true
	gq.Url = networkUrl
	gq.Expand = true
	networkDefinition := protocol.NetworkDefinition{}
	res, err = client.Query(context.Background(), &gq)
	require.NoError(t, err)
	out, err := json.Marshal(res)
	require.NoError(t, err)
	qr := new(api2.AccountRecord)
	require.NoError(t, json.Unmarshal(out, qr))

	localRecipt := qr.Proof
	_ = localRecipt
	err = networkDefinition.UnmarshalJSON([]byte(str))
	require.NoError(t, err)

	//TODO: what is needed is the operator registration will contain an ADI in that adi we need a data account that will contain
	//	H(validator_pubkey@serverIP:Port).  This will allow the user of describe to verify the node.

	//example to show the network id
	require.Equal(t, networkDefinition.NetworkName, d.Network.Id)

	//example to show what we have queried is how the network partitions are defined
	require.True(t, ensurePartitions(&networkDefinition, &d.Network))

	//override the network defaults with our queries network config
	Network = d.Network
}

func remove(s []config.Partition, i int) []config.Partition {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func ensurePartitions(networkDefinition *protocol.NetworkDefinition, describe *config.Network) bool {
	parts := describe.Partitions
	for _, p := range networkDefinition.Partitions {
		for i, v := range parts {
			if v.Id == p.PartitionID {
				parts = remove(parts, i)
				break
			}
		}
	}
	return len(parts) == 0
}
