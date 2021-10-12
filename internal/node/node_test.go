package node_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/stretchr/testify/require"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

func TestNodeSetup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	}

	opts, err := acctesting.NodeInitOptsForNetwork("Badlands")
	require.NoError(t, err)
	opts.WorkDir = t.TempDir()
	opts.Port = getFreePort(t)

	require.NoError(t, node.Init(opts)) // Configure

	nodeDir := filepath.Join(opts.WorkDir, "Node0") //
	//disable web site
	c, err := cfg.Load(nodeDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	c.Accumulate.WebsiteEnabled = false
	cfg.Store(c)

	node, _, err := acctesting.NewBVCNode(nodeDir, t.Cleanup) // Initialize
	require.NoError(t, err)                                   //
	require.NoError(t, node.Start())                          // Start
	require.NoError(t, node.Stop())                           // Stop
	node.Wait()                                               //
}

func TestNodeSetupTwiceWithPrometheus(t *testing.T) {
	t.Skip("https://github.com/tendermint/tendermint/issues/7076")

	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("Try %d", i+1), func(t *testing.T) {
			opts, err := acctesting.NodeInitOptsForNetwork("Badlands")
			require.NoError(t, err)
			opts.ShardName = "accumulate"
			opts.WorkDir = t.TempDir()
			opts.Port = getFreePort(t)
			opts.Config[0].Instrumentation.Prometheus = true

			require.NoError(t, node.Init(opts))                       // Configure
			nodeDir := filepath.Join(opts.WorkDir, "Node0")           //
			node, _, err := acctesting.NewBVCNode(nodeDir, t.Cleanup) // Initialize
			require.NoError(t, err)                                   //
			require.NoError(t, node.Start())                          // Start
			require.NoError(t, node.Stop())                           // Stop
			node.Wait()                                               //
		})
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()
	port, err := tmnet.GetFreePort()
	require.NoError(t, err)
	return port
}
