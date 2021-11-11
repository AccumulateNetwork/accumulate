package api_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulated/config"
	. "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/logging"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/privval"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
)

func startBVC(t *testing.T, dir string) (*state.StateDB, *privval.FilePV, *Query) {
	t.Helper()

	opts, err := acctesting.NodeInitOptsForNetwork(networks.Local["Badlands"])
	require.NoError(t, err)
	opts.WorkDir = dir
	opts.Port = GetFreePort(t)
	cfg := opts.Config[0]
	cfg.Mempool.MaxBatchBytes = 1048576
	cfg.Mempool.CacheSize = 1048576
	cfg.Mempool.Size = 50000
	cfg.Accumulate.API.EnableSubscribeTX = true
	cfg.Accumulate.Networks[0] = fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+node.TmRpcPortOffset)

	newLogger := func(s string) zerolog.Logger {
		return logging.NewTestZeroLogger(t, s)
	}

	require.NoError(t, node.Init(opts))                                                    // Configure
	nodeDir := filepath.Join(dir, "Node0")                                                 //
	cfg, err = config.Load(nodeDir)                                                        // Modify configuration
	require.NoError(t, err)                                                                //
	cfg.Accumulate.WebsiteEnabled = false                                                  // Disable the website
	cfg.Instrumentation.Prometheus = false                                                 // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	require.NoError(t, config.Store(cfg))                                                  //
	node, sdb, pv, err := acctesting.NewBVCNode(nodeDir, false, nil, newLogger, t.Cleanup) // Initialize
	require.NoError(t, err)                                                                //
	require.NoError(t, node.Start())                                                       // Launch

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
