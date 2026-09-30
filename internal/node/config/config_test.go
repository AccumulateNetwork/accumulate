// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0777))

	// Create
	cfg := Default("unittest", protocol.PartitionTypeBlockValidator, Follower, t.Name())
	cfg.SetRoot(dir)
	cfg.Accumulate.API.ListenAddress = "api-listen"

	// Slice values are unmarshalled as empty. This avoids issues with empty
	// slice != nil.
	cfg.Accumulate.P2P.Listen = []multiaddr.Multiaddr{}
	cfg.Accumulate.P2P.BootstrapPeers = []multiaddr.Multiaddr{}

	// Store
	require.NoError(t, Store(cfg))

	// Load
	lcfg, err := Load(dir)
	require.NoError(t, err)

	// Should be equal
	require.Equal(t, cfg, lcfg)
}

// TestLoadLegacyTendermintToml loads a tendermint.toml written by a CometBFT-era
// node, with the [rpc], [mempool], [statesync], [blocksync] and [tx_index]
// sections and the proxy_app key the local config no longer carries (#3926).
// The load must succeed and keep the fields that are still read. Load itself
// falls back to defaults on any error, so this calls loadTendermint to see it.
func TestLoadLegacyTendermintToml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0777))
	file := filepath.Join(dir, "config", "tendermint.toml")
	require.NoError(t, os.WriteFile(file, []byte(`
proxy_app = "tcp://127.0.0.1:26658"
moniker = "legacy-node"
node_key_file = "config/node_key.json"

[rpc]
laddr = "tcp://127.0.0.1:26657"
max_open_connections = 900

[p2p]
laddr = "tcp://0.0.0.0:26656"
persistent_peers = "abc@1.2.3.4:26656"

[mempool]
size = 5000
cache_size = 10000
max_batch_bytes = 1048576

[statesync]
enable = true
rpc_servers = "a,b"

[blocksync]
version = "v0"

[consensus]
timeout_commit = "1s"

[tx_index]
indexer = "kv"

[instrumentation]
prometheus = true
prometheus_listen_addr = ":26660"
namespace = "legacy"
`), 0600))

	cfg, err := loadTendermint(dir, file)
	require.NoError(t, err)
	require.Equal(t, "legacy-node", cfg.Moniker)
	require.Equal(t, "tcp://0.0.0.0:26656", cfg.P2P.ListenAddress)
	require.Equal(t, "abc@1.2.3.4:26656", cfg.P2P.PersistentPeers)
	require.Equal(t, time.Second, cfg.Consensus.TimeoutCommit)
	require.Equal(t, ":26660", cfg.Instrumentation.PrometheusListenAddr)
	require.Equal(t, "legacy", cfg.Instrumentation.Namespace)
	require.Equal(t, filepath.Join(dir, "config", "node_key.json"), cfg.NodeKeyFile())
}
