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

	for _, config := range opts.Config {
		//[mempool]
		//	broadcast = true
		//	cache_size = 100000
		//	max_batch_bytes = 10485760
		//	max_tx_bytes = 1048576
		//	max_txs_bytes = 1073741824
		//	recheck = true
		//	size = 50000
		//	wal_dir = ""
		//

		// config.Mempool.KeepInvalidTxsInCache = false
		// config.Mempool.MaxTxsBytes = 1073741824
		config.Mempool.MaxBatchBytes = 1048576
		config.Mempool.CacheSize = 1048576
		config.Mempool.Size = 50000
	}

	require.NoError(t, node.Init(opts))                        // Configure
	nodeDir := filepath.Join(dir, "Node0")                     //
	node, pv, err := acctesting.NewBVCNode(nodeDir, t.Cleanup) // Initialize
	require.NoError(t, err)                                    //
	require.NoError(t, node.Start())                           // Launch
	return node, pv
}
