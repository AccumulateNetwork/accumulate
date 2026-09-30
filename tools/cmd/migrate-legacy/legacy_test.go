// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A CometBFT-era partition directory loads whatever else its tendermint.toml
// carries — [rpc], [mempool], [statesync], [blocksync], [tx_index], proxy_app
// and the rest of CometBFT's file — keeping the fields the migration reads.
// The decoder ignores unused keys; a strict one would refuse every real file.
func TestLoadLegacyPartitionDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0777))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "tendermint.toml"), []byte(`
proxy_app = "tcp://127.0.0.1:26658"
moniker = "legacy-node"
node_key_file = "config/node_key.json"
priv_validator_key_file = "../priv_validator_key.json"

[rpc]
laddr = "tcp://127.0.0.1:26657"
max_open_connections = 900

[p2p]
laddr = "tcp://0.0.0.0:16591"
persistent_peers = "abc@1.2.3.4:26656"

[mempool]
size = 5000
cache_size = 10000

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
namespace = "tendermint"
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "accumulate.toml"), []byte(`
[describe]
partition-id = "Directory"
type = "directory"

[describe.network]
id = "Fozzie"
`), 0600))

	c, err := loadLegacy(dir)
	require.NoError(t, err)
	require.Equal(t, "config/node_key.json", c.NodeKey)
	require.Equal(t, "../priv_validator_key.json", c.PrivValidatorKey)
	require.Equal(t, "config/genesis.json", c.Genesis, "the genesis path defaults as CometBFT's did")
	require.Equal(t, "tcp://0.0.0.0:16591", c.P2P.ListenAddress)
	require.Equal(t, "abc@1.2.3.4:26656", c.P2P.PersistentPeers)
	require.Equal(t, "Fozzie", c.Accumulate.Network.Id)
	require.Equal(t, protocol.PartitionTypeDirectory, c.Accumulate.NetworkType)
}
