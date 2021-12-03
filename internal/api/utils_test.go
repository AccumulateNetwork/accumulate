package api_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	. "github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/privval"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
)

func startBVC(t *testing.T, dir string) (*state.StateDB, *privval.FilePV, *Query) {
	t.Helper()

	opts, err := acctesting.NodeInitOptsForNetwork(acctesting.LocalBVN)
	require.NoError(t, err)
	opts.WorkDir = dir
	opts.Port = GetFreePort(t)
	cfg := opts.Config[0]
	cfg.Mempool.MaxBatchBytes = 1048576
	cfg.Mempool.CacheSize = 1048576
	cfg.Mempool.Size = 50000
	cfg.Accumulate.API.EnableSubscribeTX = true
	cfg.Accumulate.Networks[0] = fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+networks.TmRpcPortOffset)

	opts2 := acctesting.BVNNOptions{
		Dir:       filepath.Join(dir, "Node0"),
		LogWriter: logging.TestLogWriter(t),
	}

	require.NoError(t, node.Init(opts))                        // Configure
	cfg, err = config.Load(opts2.Dir)                          // Modify configuration
	require.NoError(t, err)                                    //
	cfg.Accumulate.WebsiteEnabled = false                      // Disable the website
	cfg.Instrumentation.Prometheus = false                     // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	cfg.Consensus.TimeoutCommit = time.Second / 10             // ~10 blocks per second
	require.NoError(t, config.Store(cfg))                      //
	node, sdb, pv, err := acctesting.NewBVNN(opts2, t.Cleanup) // Initialize
	require.NoError(t, err)                                    //
	require.NoError(t, node.Start())                           // Launch

	t.Cleanup(func() { require.NoError(t, node.Stop()) })

	rpc, err := rpc.New(node.Config.RPC.ListenAddress)
	require.NoError(t, err)

	relay := relay.New(rpc)
	if cfg.Accumulate.API.EnableSubscribeTX {
		require.NoError(t, relay.Start())
		t.Cleanup(func() { require.NoError(t, relay.Stop()) })
	}

	query := NewQuery(relay)
	return sdb, pv, query
}
