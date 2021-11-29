package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	net2 "net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/net"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
)

type testCase func(t *testing.T, tc *testCmd)
type testMatrixTests []testCase

var testMatrix testMatrixTests

func TestCli(t *testing.T) {
	switch {
	case testing.Short():
		t.Skip("Skipping test in short mode")
	case runtime.GOOS == "windows":
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	case runtime.GOOS == "darwin" && os.Getenv("CI") == "true":
		t.Skip("This test is flaky in macOS CI")
	}

	tc := &testCmd{}
	tc.initalize(t)

	//set mnemonic for predictable addresses
	_, err := tc.execute(t, "key import mnemonic yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow")
	require.NoError(t, err)

	testMatrix.execute(t, tc)

}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (tm *testMatrixTests) addTest(tc testCase) {
	*tm = append(*tm, tc)

}

func (tm *testMatrixTests) execute(t *testing.T, tc *testCmd) {

	// Sort by testCase number
	sort.SliceStable(*tm, func(i, j int) bool {
		return GetFunctionName((*tm)[i]) < GetFunctionName((*tm)[j])
	})

	//execute the tests
	for _, f := range testMatrix {
		t.Log(GetFunctionName(f))
		f(t, tc)
	}
}

type testCmd struct {
	rootCmd        *cobra.Command
	directoryCmd   *exec.Cmd
	validatorCmd   *exec.Cmd
	defaultWorkDir string
	restPort       int
	jsonRpcPort    int
}

//NewTestBVNN creates a BVN test Node and returns the rest and jsonrpc ports
func NewTestBVNN(t *testing.T, defaultWorkDir string) (int, int) {
	t.Helper()
	opts, err := acctesting.NodeInitOptsForNetwork(acctesting.LocalBVN)
	require.NoError(t, err)
	opts.WorkDir = defaultWorkDir
	opts.Port, err = net.GetFreePort()
	require.NoError(t, err)
	cfg := opts.Config[0]
	cfg.Mempool.MaxBatchBytes = 1048576
	cfg.Mempool.CacheSize = 1048576
	cfg.Mempool.Size = 50000
	cfg.Accumulate.API.EnableSubscribeTX = false
	cfg.Accumulate.Networks[0] =
		fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+node.TmRpcPortOffset)

	newLogger := func(s string) zerolog.Logger {
		return logging.NewTestZeroLogger(t, s)
	}

	require.NoError(t, node.Init(opts))               // Configure
	nodeDir := filepath.Join(defaultWorkDir, "Node0") //
	cfg, err = config.Load(nodeDir)                   // Modify configuration
	require.NoError(t, err)                           //
	cfg.Accumulate.WebsiteEnabled = false             // Disable the website
	cfg.Instrumentation.Prometheus = false            // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	require.NoError(t, config.Store(cfg))             //

	bvnNode, _, _, err := acctesting.NewBVCNode(nodeDir, false, nil, newLogger, t.Cleanup) // Initialize
	require.NoError(t, err)                                                                //
	require.NoError(t, bvnNode.Start())                                                    // Launch
	relayTo := []string{cfg.RPC.ListenAddress}
	relay, err := relay.NewWith(relayTo...)

	t.Cleanup(func() {
		require.NoError(t, bvnNode.Stop())
		bvnNode.Wait()
		os.RemoveAll(defaultWorkDir)
	})

	// Create a local client
	lnode, ok := bvnNode.Service.(local.NodeService)
	if !ok {
		t.Fatalf("node is not a local node service!")
	}
	lclient, err := local.New(lnode)
	if err != nil {
		t.Fatalf("failed to create local node client: %v", err)
	}

	// Configure JSON-RPC
	var jrpcOpts api2.JrpcOptions
	jrpcOpts.Config = &cfg.Accumulate.API
	jrpcOpts.QueueDuration = time.Second / 4
	jrpcOpts.QueueDepth = 100
	jrpcOpts.QueryV1 = api.NewQuery(relay)
	jrpcOpts.Local = lclient

	// Build the list of remote addresses and query clients
	jrpcOpts.Remote = make([]string, len(cfg.Accumulate.Networks))
	clients := make([]api2.ABCIQueryClient, len(cfg.Accumulate.Networks))
	for i, net := range cfg.Accumulate.Networks {
		switch {
		case net == "self", net == cfg.Accumulate.Network, net == cfg.RPC.ListenAddress:
			jrpcOpts.Remote[i] = "local"
			clients[i] = lclient

		default:
			addr, err := networks.GetRpcAddr(net, node.TmRpcPortOffset)
			if err != nil {
				t.Fatalf("invalid network name or address: %v", err)
			}

			jrpcOpts.Remote[i] = addr
			clients[i], err = rpchttp.New(addr)
			if err != nil {
				t.Fatalf("failed to create RPC client: %v", err)
			}
		}
	}

	jrpcOpts.Query = api2.NewQueryDispatch(clients)

	jrpc, err := api2.NewJrpc(jrpcOpts)
	if err != nil {
		t.Fatalf("failed to start API: %v", err)
	}

	// Run JSON-RPC server
	s := &http.Server{Handler: jrpc.NewMux()}
	l, sec, err := listenHttpUrl(cfg.Accumulate.API.JSONListenAddress)
	require.NoError(t, err)
	require.Equal(t, sec, false, "jsonrpc doesn't support https")

	go func() {
		err := s.Serve(l)
		require.NoError(t, err, "JSON-RPC server", "err", err)
	}()

	time.Sleep(time.Second)
	return opts.Port + node.AccRouterJsonPortOffset, opts.Port + node.AccRouterRestPortOffset
}

func (c *testCmd) initalize(t *testing.T) {
	t.Helper()

	defaultWorkDir, err := ioutil.TempDir("", "cliTest")
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(defaultWorkDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	c.rootCmd = InitRootCmd(initDB(defaultWorkDir))
	c.rootCmd.PersistentPostRun = nil

	c.jsonRpcPort, c.restPort = NewTestBVNN(t, defaultWorkDir)
	time.Sleep(2 * time.Second)
}

func (c *testCmd) execute(t *testing.T, cmdLine string) (string, error) {
	fullCommand := fmt.Sprintf("-j -s http://127.0.0.1:%v/v1 %s",
		c.jsonRpcPort, cmdLine)
	args := strings.Split(fullCommand, " ")

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	c.rootCmd.SetErr(e)
	c.rootCmd.SetOut(b)
	c.rootCmd.SetArgs(args)
	c.rootCmd.Execute()

	errPrint, err := ioutil.ReadAll(e)
	if err != nil {
		return "", err
	} else if len(errPrint) != 0 {
		return "", fmt.Errorf("%s", string(errPrint))
	}
	ret, err := ioutil.ReadAll(b)
	return string(ret), err
}

// listenHttpUrl
// takes a string such as `http://localhost:123` and creates a TCP listener.
func listenHttpUrl(s string) (net2.Listener, bool, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false, fmt.Errorf("invalid address: %v", err)
	}

	if u.Path != "" && u.Path != "/" {
		return nil, false, fmt.Errorf("invalid address: path is not empty")
	}

	var secure bool
	switch u.Scheme {
	case "tcp", "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, false, fmt.Errorf("invalid address: unsupported scheme %q", u.Scheme)
	}

	l, err := net2.Listen("tcp", u.Host)
	if err != nil {
		return nil, false, err
	}

	return l, secure, nil
}
