package node_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/genesis"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	apitypes "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

func TestEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("This test consistently fails in CI")
	}

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) *api.Query {
		// Restart the nodes for every test
		nodes := initNodes(s.T(), s.T().TempDir(), s.T().Name(), net.ParseIP("127.0.25.1"), 3000, 3, "error", nil)
		query := startNodes(s.T(), nodes)
		return query
	}))
}

func TestFaucetMultiNetwork(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	bvc0 := initNodes(t, t.TempDir(), "BVC0", net.ParseIP("127.0.26.1"), 3000, 1, "error", []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc1 := initNodes(t, t.TempDir(), "BVC1", net.ParseIP("127.0.27.1"), 3000, 1, "error", []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc2 := initNodes(t, t.TempDir(), "BVC2", net.ParseIP("127.0.28.1"), 3000, 1, "error", []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	rpcAddrs := make([]string, 0, 3)
	wg := new(sync.WaitGroup)
	for _, bvc := range [][]*node.Node{bvc0, bvc1, bvc2} {
		rpcAddrs = append(rpcAddrs, bvc[0].Config.RPC.ListenAddress)
		for _, n := range bvc {
			n := n
			wg.Add(1)
			go func() {
				defer wg.Done()
				require.NoError(t, n.Start())
				t.Cleanup(func() {
					n.Stop()
					n.Wait()
				})
			}()
		}
	}
	wg.Wait()

	relay, err := relay.NewWith(rpcAddrs...)
	require.NoError(t, err)
	if bvc0[0].Config.Accumulate.API.EnableSubscribeTX {
		require.NoError(t, relay.Start())
		t.Cleanup(func() { require.NoError(t, relay.Stop()) })
	}
	query := api.NewQuery(relay)

	lite, err := url.Parse("acc://b5d4ac455c08bedc04a56d8147e9e9c9494c99eb81e9d8c3/ACME")
	require.NoError(t, err)
	require.NotEqual(t, lite.Routing()%3, genesis.FaucetUrl.Routing()%3, "The point of this test is to ensure synthetic transactions are routed correctly. That doesn't work if both URLs route to the same place.")

	req := new(apitypes.APIRequestURL)
	req.Wait = true
	req.URL = types.String(lite.String())

	params, err := json.Marshal(&req)
	require.NoError(t, err)

	port, err := tmnet.GetFreePort()
	require.NoError(t, err)
	jsonapi, err := api.StartAPI(&config.API{
		JSONListenAddress: fmt.Sprintf("tcp://localhost:%d", port),
		RESTListenAddress: fmt.Sprintf("tcp://localhost:%d", port+1),
	}, query)
	require.NoError(t, err)
	res := jsonapi.Faucet(context.Background(), params)
	switch r := res.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *apitypes.APIDataResponse:
		// OK
	default:
		require.IsType(t, apitypes.APIDataResponse{}, r)
	}

	// Wait for synthetic TX to settle
	time.Sleep(3 * time.Second)

	obj := new(state.Object)
	chain := new(state.ChainHeader)
	account := new(protocol.AnonTokenAccount)
	qres, err := query.QueryByUrl(lite.String())
	require.NoError(t, err)
	require.Zero(t, qres.Response.Code, "Failed, log=%q, info=%q", qres.Response.Log, qres.Response.Info)
	require.NoError(t, obj.UnmarshalBinary(qres.Response.Value))
	require.NoError(t, obj.As(chain))
	require.Equal(t, types.ChainTypeAnonTokenAccount, chain.Type)
	require.NoError(t, obj.As(account))
}

func TestAC489(t *testing.T) {
	workDir := t.TempDir()
	nodes := initNodes(t, workDir, t.Name(), net.ParseIP("127.0.29.1"), 3000, 2, "info", nil)
	query := startNodes(t, nodes[:1])
	jsonapi, err := api.New(new(config.API), query)
	require.NoError(t, err)

	cmd := exec.Command("go", "run", "../../cmd/accumulated", "run", "-w", workDir, "-n", "1")
	require.NoError(t, cmd.Start())

	lite, err := url.Parse("acc://b5d4ac455c08bedc04a56d8147e9e9c9494c99eb81e9d8c3/ACME")
	require.NoError(t, err)
	require.NotEqual(t, lite.Routing()%3, genesis.FaucetUrl.Routing()%3, "The point of this test is to ensure synthetic transactions are routed correctly. That doesn't work if both URLs route to the same place.")

	req := new(apitypes.APIRequestURL)
	req.Wait = true
	req.URL = types.String(lite.String())
	params, err := json.Marshal(&req)
	require.NoError(t, err)

	// Faucet request 1
	res := jsonapi.Faucet(context.Background(), params)
	switch r := res.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *apitypes.APIDataResponse:
		// OK
	default:
		require.IsType(t, apitypes.APIDataResponse{}, r)
	}

	// Wait for synthetic TX to settle
	time.Sleep(3 * time.Second)

	// Stop node 1
	require.NoError(t, syscall.Kill(cmd.Process.Pid, syscall.SIGINT))
	require.NoError(t, cmd.Wait())

	// Faucet request 2
	res = jsonapi.Faucet(context.Background(), params)
	switch r := res.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *apitypes.APIDataResponse:
		// OK
	default:
		require.IsType(t, apitypes.APIDataResponse{}, r)
	}

	// // Restart node 1
	// cmd = exec.Command("go", "run", "../../cmd/accumulated", "run", "-w", workDir, "-n", "1")
	// require.NoError(t, cmd.Start())

	// Wait for synthetic TX to settle
	time.Sleep(10 * time.Second)

	obj := new(state.Object)
	chain := new(state.ChainHeader)
	account := new(protocol.AnonTokenAccount)
	qres, err := query.QueryByUrl(lite.String())
	require.NoError(t, err)
	require.Zero(t, qres.Response.Code, "Failed, log=%q, info=%q", qres.Response.Log, qres.Response.Info)
	require.NoError(t, obj.UnmarshalBinary(qres.Response.Value))
	require.NoError(t, obj.As(chain))
	require.Equal(t, types.ChainTypeAnonTokenAccount, chain.Type)
	require.NoError(t, obj.As(account))
	require.Equal(t, int64(20*protocol.AcmePrecision), account.Balance.Int64())

	// // Stop node 1
	// require.NoError(t, syscall.Kill(cmd.Process.Pid, syscall.SIGINT))
	// require.NoError(t, cmd.Wait())
}
