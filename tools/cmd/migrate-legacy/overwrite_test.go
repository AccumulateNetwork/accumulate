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
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
)

// minimalBVN is a CometBFT-era BVN partition's accumulate.toml.
const minimalBVN = `[api]
connection-limit = 500
listen-address = "http://0.0.0.0:26660"
read-header-timeout = "10s"
tx-max-wait-time = "10m0s"

[describe]
partition-id = "Grizzly"
type = "blockValidator"

[describe.network]
id = "Fozzie"

[storage]
path = "data/accumulate.db"
type = "badger"
`

// minimalDN is the same node's Directory partition.
const minimalDN = `[api]
connection-limit = 500
listen-address = "http://0.0.0.0:26660"
read-header-timeout = "10s"
tx-max-wait-time = "10m0s"

[describe]
partition-id = "Directory"
type = "directory"

[describe.network]
id = "Fozzie"

[storage]
path = "data/accumulate.db"
type = "badger"
`

// writeLegacyPartition writes root/dir as a CometBFT-era partition directory,
// its node key holding testKey(nodeKey) and its validator key the shared one.
func writeLegacyPartition(t *testing.T, root, dir, acc, nodeKey string) *legacyConfig {
	t.Helper()
	part := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(part, "config"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(part, "config", "accumulate.toml"), []byte(acc), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(part, "config", "tendermint.toml"), []byte(`priv_validator_key_file = "../priv_validator_key.json"`), 0600))
	old, err := loadLegacy(part)
	require.NoError(t, err)
	writeCometKeyAs(t, root, filepath.Join(dir, old.NodeKey), nodeKey)
	writeCometKeyAs(t, root, filepath.Join(dir, old.PrivValidatorKey), "priv_validator_key.json")
	return old
}

// The migration writes <work>/accumulate.toml; a file already there (a
// half-converted node, an operator's own) is kept in backup/ first, byte for
// byte (#4490).
func TestMigrateKeepsTheConfigItReplaces(t *testing.T) {
	work := t.TempDir()
	writeLegacyPartition(t, work, "bvnn", minimalBVN, "node_key.json")
	orig := []byte("# the operator's own file\nnetwork = \"Fozzie\"\n")
	require.NoError(t, os.WriteFile(filepath.Join(work, "accumulate.toml"), orig, 0600))

	flagMain.WorkDir = work
	migrate(nil, nil)

	b, err := os.ReadFile(filepath.Join(work, "backup", "accumulate.toml"))
	require.NoError(t, err, "the replaced configuration was not kept")
	require.Equal(t, orig, b)

	cfg := new(run.Config)
	require.NoError(t, cfg.LoadFrom(filepath.Join(work, "accumulate.toml")))
	require.Equal(t, "Fozzie", cfg.Network)

	// A second replacement does not overwrite the first backup.
	require.NoError(t, os.WriteFile(filepath.Join(work, "accumulate.toml"), []byte("second"), 0600))
	writeLegacyPartition(t, work, "bvnn", minimalBVN, "node_key.json")
	migrate(nil, nil)
	b, err = os.ReadFile(filepath.Join(work, "backup", "accumulate.toml"))
	require.NoError(t, err)
	require.Equal(t, orig, b, "the first backup was overwritten")
}

// A node is one libp2p identity. Main's `init dual` gave the DN and BVN one
// node key; a node whose dnn/ and bvnn/ node keys differ cannot be merged
// without dropping one, so the migration refuses, as it does for differing
// validator keys (#4490).
func TestMigrateRefusesDifferingNodeKeys(t *testing.T) {
	root := t.TempDir()
	bvn := writeLegacyPartition(t, root, "bvnn", minimalBVN, "bvnn/config/node_key.json")
	dn := writeLegacyPartition(t, root, "dnn", minimalDN, "dnn/config/node_key.json")

	cfg := &run.Config{P2P: new(run.P2P), Logging: new(run.Logging)}
	cvc := new(run.CoreValidatorConfiguration)
	require.NoError(t, migrateCfg(cfg, cvc, root, "bvnn", bvn))
	err := migrateCfg(cfg, cvc, root, "dnn", dn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "node key")

	// The same key in both merges.
	root = t.TempDir()
	bvn = writeLegacyPartition(t, root, "bvnn", minimalBVN, "node_key.json")
	dn = writeLegacyPartition(t, root, "dnn", minimalDN, "node_key.json")
	cfg = &run.Config{P2P: new(run.P2P), Logging: new(run.Logging)}
	cvc = new(run.CoreValidatorConfiguration)
	require.NoError(t, migrateCfg(cfg, cvc, root, "bvnn", bvn))
	require.NoError(t, migrateCfg(cfg, cvc, root, "dnn", dn))
	require.Equal(t, keyAddr("node_key.json"), cfg.P2P.Key.(*run.RawPrivateKey).Address)
}
