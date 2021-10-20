package api_test

import (
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulated/config"
	. "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/privval"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
)

func startBVC(t *testing.T, dir string) (*node.Node, *privval.FilePV, *Query) {
	t.Helper()

	opts, err := acctesting.NodeInitOptsForNetwork("Badlands")
	require.NoError(t, err)
	opts.WorkDir = dir
	opts.Port = GetFreePort(t)
	cfg := opts.Config[0]
	cfg.Mempool.MaxBatchBytes = 1048576
	cfg.Mempool.CacheSize = 1048576
	cfg.Mempool.Size = 50000

	require.NoError(t, node.Init(opts))                               // Configure
	nodeDir := filepath.Join(dir, "Node0")                            //
	cfg, err = config.Load(nodeDir)                                   // Modify configuration
	require.NoError(t, err)                                           //
	cfg.Accumulate.WebsiteEnabled = false                             // Disable the website
	cfg.Instrumentation.Prometheus = false                            // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	require.NoError(t, config.Store(cfg))                             //
	node, pv, err := acctesting.NewBVCNode(nodeDir, false, t.Cleanup) // Initialize
	require.NoError(t, err)                                           //
	require.NoError(t, node.Start())                                  // Launch

	t.Cleanup(func() { require.NoError(t, node.Stop()) })

	rpc, err := rpc.New(node.Config.RPC.ListenAddress)
	require.NoError(t, err)

	relay := relay.New(rpc)
	require.NoError(t, relay.Start())
	t.Cleanup(func() { require.NoError(t, relay.Stop()) })

	query := NewQuery(relay)
	return node, pv, query
}
