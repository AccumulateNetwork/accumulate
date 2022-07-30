package testing

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var AccuProxyAuthorityKey ed25519.PrivateKey
var AccuProxyKey ed25519.PrivateKey
var AccuProxy = protocol.AccountUrl("accuproxy.acme")

//LaunchAccuProcyDevNet will launch a devnet with the accuproxy.acme adi and authorized key(s)
func LaunchAccuProxyDevNet(t *testing.T) (*client.Client, *url.URL, *url.URL) {
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

	require.NoError(t, acctesting.CreateADI(batch, []byte(AccuProxyAuthorityKey), "accuproxy.acme"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "accuproxy.acme/registry", []byte(AccuProxyKey.Public().(ed25519.PublicKey))))
	require.NoError(t, batch.Commit())

	accApiUrl, err := url.Parse(c.Accumulate.API.ListenAddress)
	require.NoError(t, err)

	port, err := strconv.ParseInt(accApiUrl.Port(), 10, 16)
	require.NoError(t, err, "invalid port number")

	dnEndpoint := fmt.Sprintf("%s://%s:%d", accApiUrl.Scheme, accApiUrl.Hostname(), port-config.PortOffsetBlockValidator)
	bvnEndpoint := fmt.Sprintf("%s://%s:%d", accApiUrl.Scheme, accApiUrl.Hostname(), port)
	dnEndpointUrl, err := url.Parse(dnEndpoint)
	require.NoError(t, err)
	bvnEndpointUrl, err := url.Parse(bvnEndpoint)
	require.NoError(t, err)

	return client, dnEndpointUrl, bvnEndpointUrl
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
		return jsonrpc2.ErrorInvalidParams("invalid SeedListRequest parameters")
	}

	resp := proxy.SeedListResponse{}
	snl := Network.GetPartitionByID(slr.Partition)
	if snl == nil {
		return jsonrpc2.ErrorInvalidParams(fmt.Sprintf("cannot find partition %s", slr.Partition))
	}
	resp.Type = snl.Type
	resp.BasePort = uint64(snl.BasePort)

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
	if snl == nil {
		return jsonrpc2.ErrorInvalidParams(fmt.Sprintf("canont find partition %s", scr.Partition))
	}

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
	resp.NetworkState.Network = Network
	resp.NetworkState.Version = accumulate.Version
	resp.NetworkState.VersionIsKnown = accumulate.IsVersionKnown()
	resp.NetworkState.Commit = accumulate.Commit
	resp.NetworkState.IsTestNet = true

	if ncr.Sign {
		s, err := resp.NetworkState.MarshalBinary()
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

func LaunchFakeProxy(t *testing.T) (*proxy.Client, *client.Client, *url.URL, *url.URL) {
	t.Helper()

	pk, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	AccuProxyAuthorityKey = ed25519.NewKeyFromSeed(pk)
	pk, _, err = ed25519.GenerateKey(nil)
	require.NoError(t, err)
	AccuProxyKey = ed25519.NewKeyFromSeed(pk)

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
	accClient, dnEndpoint, bvnEndpoint := LaunchAccuProxyDevNet(t)
	ProveNetworkToFakeProxy(t, accClient)

	return proxyClient, accClient, dnEndpoint, bvnEndpoint
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

	//now query the acc://dn.acme/network account with prove flag enabled to get the network names and anchor hash
	networkUrl := protocol.DnUrl().JoinPath("network")
	gq := api.GeneralQuery{}
	gq.Prove = true
	gq.Url = networkUrl
	gq.Expand = true
	networkDefinition := protocol.NetworkDefinition{}
	//sends {"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme/network","prove":true},"id":2282}
	res, err = client.Query(context.Background(), &gq)
	require.NoError(t, err)
	out, err := json.Marshal(res)
	require.NoError(t, err)

	qr := new(api.ChainQueryResponse)
	require.NoError(t, json.Unmarshal(out, qr))

	localRecipt := qr.Receipt.Proof
	_ = localRecipt
	//example receipt, where "start" is the current account state hash, and "anchor" is the dn anchor hash
	//running the merkle dag proof using "start" via the entries will give you anchor, and anchor is independently
	//published so the anchor can also be verified as belonging to the network
	//see https://gitlab.com/accumulatenetwork/sdk/anchor-solidity for more information and pseudocode
	//for proving account states
	/*
	 "receipt": {
	    "localBlock": 14583,
	    "proof": {
	      "start": "a5ad1114262b7cf565429a4268a4bd05263427580665d6236538f19b335c5fed",
	      "endIndex": 2,
	      "anchor": "329ca5b0e274fdca20f1508bbf5d8c4740a39503ea54df09489531eadb2340c5",
	      "entries": [
	        {
	          "right": true,
	          "hash": "2f137f695bc642abfdc186c89db86da9e1aa4c966abb8ca844851ea9125caee8"
	        },
	        {
	          "right": true,
	          "hash": "0000000000000000000000000000000000000000000000000000000000000000"
	        },
	        {
	          "hash": "e1490ae8bdba2a94875001e121b6d4cec880585ccdff038f385f8fdb10b1d24b"
	        },
	        {
	          "right": true,
	          "hash": "1e0198860a741b5471c547b4c25ec1028aca137d2d3d5af43ab6eaa388d078aa"
	        },
	        {
	          "right": true,
	          "hash": "c2ca76496bb6c93b426ccf940300c1a8e2a6aa09a73cc3959fa94e064cc59b7c"
	        }
	      ]
	    },
	*/
	out, err = json.Marshal(qr.Data)

	da := protocol.DataAccount{}
	require.NoError(t, err)
	err = da.UnmarshalJSON(out)
	require.NoError(t, err)

	require.Greater(t, len(da.Entry.GetData()), 0)
	err = networkDefinition.UnmarshalBinary(da.Entry.GetData()[0])
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

func remove(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func ensurePartitions(networkDefinition *protocol.NetworkDefinition, describe *config.Network) bool {
	parts := []string{}
	for _, p := range describe.Partitions {
		parts = append(parts, p.Id)
	}
	for _, p := range networkDefinition.Partitions {
		for i, v := range parts {
			if v == p.PartitionID {
				parts = remove(parts, i)
				break
			}
		}
	}
	return len(parts) == 0
}
