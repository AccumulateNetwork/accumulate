package api_test

import (
	"path/filepath"
	"testing"

	. "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/privval"
)

func startBVC(t *testing.T, dir string) (*node.Node, *privval.FilePV) {
	t.Helper()

	opts, err := acctesting.NodeInitOptsForNetwork("Badlands")
	require.NoError(t, err)
	opts.WorkDir = dir
	opts.Port = GetFreePort(t)

	opts.Config[0].Mempool.MaxBatchBytes = 1048576
	opts.Config[0].Mempool.CacheSize = 1048576
	opts.Config[0].Mempool.Size = 50000

	// https://github.com/tendermint/tendermint/issues/7076
	opts.Config[0].Instrumentation.Prometheus = false

	require.NoError(t, node.Init(opts))                        // Configure
	nodeDir := filepath.Join(dir, "Node0")                     //
	node, pv, err := acctesting.NewBVCNode(nodeDir, t.Cleanup) // Initialize
	require.NoError(t, err)                                    //
	require.NoError(t, node.Start())                           // Launch
	return node, pv
}
