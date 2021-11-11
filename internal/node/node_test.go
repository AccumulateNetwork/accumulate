package node_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/logging"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

func TestNodeSetup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	}

	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	opts, err := acctesting.NodeInitOptsForNetwork(networks.Local["Badlands"])
	require.NoError(t, err)
	opts.WorkDir = t.TempDir()
	opts.Port = getFreePort(t)
	opts.Config[0].Accumulate.Networks[0] = fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+node.TmRpcPortOffset)

	require.NoError(t, node.Init(opts)) // Configure

	nodeDir := filepath.Join(opts.WorkDir, "Node0") //
	//disable web site
	c, err := cfg.Load(nodeDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	c.Accumulate.WebsiteEnabled = false
	cfg.Store(c)

	newLogger := func(s string) zerolog.Logger {
		return logging.NewTestZeroLogger(t, s)
	}

	node, _, _, err := acctesting.NewBVCNode(nodeDir, false, nil, newLogger, t.Cleanup) // Initialize
	require.NoError(t, err)                                                             //
	require.NoError(t, node.Start())                                                    // Start
	require.NoError(t, node.Stop())                                                     // Stop
	node.Quit()
	node.Wait() //
}

func TestNodeSetupTwiceWithPrometheus(t *testing.T) {
	t.Skip("https://github.com/tendermint/tendermint/issues/7076")

	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("Try %d", i+1), func(t *testing.T) {
			opts, err := acctesting.NodeInitOptsForNetwork(networks.Local["Badlands"])
			require.NoError(t, err)
			opts.ShardName = "accumulate"
			opts.WorkDir = t.TempDir()
			opts.Port = getFreePort(t)
			opts.Config[0].Instrumentation.Prometheus = true
			opts.Config[0].Accumulate.Networks[0] = fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+node.TmRpcPortOffset)

			newLogger := func(s string) zerolog.Logger {
				return logging.NewTestZeroLogger(t, s)
			}

			require.NoError(t, node.Init(opts))                                                 // Configure
			nodeDir := filepath.Join(opts.WorkDir, "Node0")                                     //
			node, _, _, err := acctesting.NewBVCNode(nodeDir, false, nil, newLogger, t.Cleanup) // Initialize
			require.NoError(t, err)                                                             //
			require.NoError(t, node.Start())                                                    // Start
			require.NoError(t, node.Stop())                                                     // Stop
			node.Wait()                                                                         //
		})
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()
	port, err := tmnet.GetFreePort()
	require.NoError(t, err)
	return port
}
