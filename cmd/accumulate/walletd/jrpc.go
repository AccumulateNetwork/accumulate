// Package classification awesome.
//
// Documentation of our awesome API.
//
//     Schemes: http
//     BasePath: /
//     Version: 1.0.0
//     Host: some-url.com
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
//     Security:
//     - basic
//
//    SecurityDefinitions:
//    basic:
//      type: basic
//
// swagger:meta
package walletd

import (
	"context"
	"encoding/json"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/gommon/log"
	_ "github.com/pdrum/swagger-automation/docs" // This line is necessary for go-swagger to find your docs!
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Options struct {
	Logger        log.Logger
	TxMaxWaitTime time.Duration
	//PrometheusServer string
	//Database         *database.Database
}

type JrpcMethods struct {
	Options
	methods  jsonrpc2.MethodMap
	validate *validator.Validate
	logger   log.Logger
}

var Endpoint = "http://localhost:18888"

func NewJrpc(opts Options) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.Options = opts

	//if opts.Logger != nil {
	//	m.logger = opts.Logger.With("module", "jrpc")
	//}

	m.validate, err = protocol.NewValidator()
	if err != nil {
		return nil, err
	}

	m.populateMethodTable()
	return m, nil
}

func (m *JrpcMethods) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/wallet", jsonrpc2.HTTPRequestHandler(m.methods, stdlog.New(os.Stdout, "", 0)))
	return mux
}

func (m *JrpcMethods) jrpc2http(jrpc jsonrpc2.MethodFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			res.WriteHeader(http.StatusBadRequest)
			return
		}

		var params json.RawMessage
		mediatype, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if mediatype == "application/json" || mediatype == "text/json" {
			params = body
		}

		r := jrpc(req.Context(), params)
		res.Header().Add("Content-Type", "application/json")
		data, err := json.Marshal(r)
		if err != nil {
			m.logError("Failed to marshal status", "error", err)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = res.Write(data)
	}
}

func LaunchFakeProxy(t *testing.T) {

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

	//proxyClient, err := proxy.New(Endpoint)
	//require.NoError(t, err)

	////now spawn our little testnet and populate the fake proxy with the network info
	//accClient, dnEndpoint, bvnEndpoint := LaunchAccuProxyDevNet(t)
	//ProveNetworkToFakeProxy(t, accClient)

	//	return proxyClient, accClient, dnEndpoint, bvnEndpoint
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
