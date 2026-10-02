// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"golang.org/x/exp/slices"
)

func TestMigrateOld(t *testing.T) {
	t.Run("Dual-node", func(t *testing.T) {
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-genesis = "bvnn/config/genesis.json"
			dn-genesis = "dnn/config/genesis.json"
			listen = "/ip4/127.0.2.1/tcp/16591"
			max-envelopes-per-block = 100
			storage-type = "badger"
			type = "coreValidator"

			[configurations.validator-key]
			path = "priv_validator_key.json"
			type = "cometPrivValFile"

			[logging]

			[p2p]
			bootstrap-peers = ["/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"]
			listen = ["/ip4/127.0.2.1/tcp/16693", "/ip4/127.0.2.1/udp/16693/quic", "/ip4/127.0.2.1/tcp/16593", "/ip4/127.0.2.1/udp/16593/quic"]

			[p2p.key]
			path = "dnn/config/node_key.json"
			type = "cometNodeKeyFile"
			`,
			applyOldNodeKey(t, "bvnn", "dnn/config/node_key.json",
				`max-envelopes-per-block = 100

				[api]
				connection-limit = 500
				listen-address = "http://127.0.2.1:16695"
				read-header-timeout = "10s"
				tx-max-wait-time = "10m0s"

				[describe]
				partition-id = "Grizzly"
				type = "blockValidator"

				[describe.network]
				id = "Fozzie"

				[p2p]
				bootstrap-peers = ["/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"]
				listen = ["/ip4/127.0.2.1/tcp/16693", "/ip4/127.0.2.1/udp/16693/quic"]

				[snapshots]
				directory = "snapshots"
				retain = 10
				schedule = "0 */12 * * *"

				[storage]
				path = "data/accumulate.db"
				type = "badger"
				`, `
				priv_validator_key_file = "../priv_validator_key.json"

				[p2p]
				laddr = "tcp://127.0.2.1:16691"
				persistent_peers = "6e56f4363839e9c6a7d24a5030fd78eca6565bec@fozzie-bvn0.accumulate.defidevs.io:16691"
				`,
			),
			applyOld(t, "dnn",
				`[api]
				connection-limit = 500
				listen-address = "http://127.0.2.1:16595"
				read-header-timeout = "10s"
				tx-max-wait-time = "10m0s"

				[describe]
				partition-id = "Directory"
				type = "directory"

				[describe.network]
				id = "Fozzie"

				[p2p]
				bootstrap-peers = ["/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"]
				listen = ["/ip4/127.0.2.1/tcp/16593", "/ip4/127.0.2.1/udp/16593/quic"]

				[snapshots]
				directory = "snapshots"
				retain = 10
				schedule = "0 */12 * * *"

				[storage]
				path = "data/accumulate.db"
				type = "badger"
				`, `
				priv_validator_key_file = "../priv_validator_key.json"

				[p2p]
				laddr = "tcp://127.0.2.1:16591"
				persistent_peers = "4deb0569a3a0c3dadf2607c8c4a1d20dd5510bf8@fozzie-bvn0.accumulate.defidevs.io:16591"
				`,
			),
		)
	})

	t.Run("Minimal", func(t *testing.T) {
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-genesis = "bvnn/config/genesis.json"
			listen = "/tcp/26556"
			storage-type = "badger"
			type = "coreValidator"

			[configurations.validator-key]
			path = "bvnn/config/priv_validator_key.json"
			type = "cometPrivValFile"

			[logging]
			[p2p]
			[p2p.key]
			path = "bvnn/config/node_key.json"
			type = "cometNodeKeyFile"
			`,
			applyOld(t, "bvnn",
				`[api]
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
				`, ``,
			),
		)
	})

	t.Run("Loki", func(t *testing.T) {
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-genesis = "bvnn/config/genesis.json"
			listen = "/tcp/26556"
			storage-type = "badger"
			type = "coreValidator"

			[configurations.validator-key]
			path = "bvnn/config/priv_validator_key.json"
			type = "cometPrivValFile"

			[logging]
			[logging.loki]
			enable = true
			password = "Zm9vYmFyCg=="
			url = "https://example.net"
			username = "12345"

			[p2p]
			[p2p.key]
			path = "bvnn/config/node_key.json"
			type = "cometNodeKeyFile"
			`,
			applyOld(t, "bvnn",
				`[api]
				connection-limit = 500
				listen-address = "http://0.0.0.0:26660"
				read-header-timeout = "10s"
				tx-max-wait-time = "10m0s"

				[describe]
				partition-id = "Grizzly"
				type = "blockValidator"

				[describe.network]
				id = "Fozzie"

				[logging]
				enable-loki = true
				loki-url = "https://example.net"
				loki-username = "12345"
				loki-password = "Zm9vYmFyCg=="

				[storage]
				path = "data/accumulate.db"
				type = "badger"
				`, ``,
			),
		)
	})

	t.Run("Storage", func(t *testing.T) {
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-genesis = "bvnn/config/genesis.json"
			listen = "/tcp/26556"
			type = "coreValidator"

			[configurations.validator-key]
			path = "bvnn/config/priv_validator_key.json"
			type = "cometPrivValFile"

			[logging]
			[p2p]
			[p2p.key]
			path = "bvnn/config/node_key.json"
			type = "cometNodeKeyFile"

			[[services]]
			name = "Grizzly"
			type = "storage"
			[services.storage]
			path = "bvnn/foo/bar/accumulate.db"
			type = "badger"
			`,
			applyOld(t, "bvnn",
				`[api]
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
				path = "foo/bar/accumulate.db"
				type = "badger"
				`, ``,
			),
		)
	})

	t.Run("Snapshots", func(t *testing.T) {
		// SnapshotService has been removed from the schema, so old snapshot
		// config is silently skipped during migration
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-genesis = "bvnn/config/genesis.json"
			listen = "/tcp/26556"
			storage-type = "badger"
			type = "coreValidator"

			[configurations.validator-key]
			path = "bvnn/config/priv_validator_key.json"
			type = "cometPrivValFile"

			[logging]
			[p2p]
			[p2p.key]
			path = "bvnn/config/node_key.json"
			type = "cometNodeKeyFile"
			`,
			applyOld(t, "bvnn",
				`[api]
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

				[snapshots]
				enable = true
				directory = "snapshots"
				retain = 10
				schedule = "0 */12 * * *"
				`, ``,
			),
		)
	})
}

func testMigrateOld(t *testing.T, expect string, old ...applyFunc) {
	t.Helper()
	cfg := new(run.Config)
	cfg.P2P = new(run.P2P)
	cfg.Logging = new(run.Logging)
	cvc := new(run.CoreValidatorConfiguration)
	cfg.Configurations = []run.Configuration{cvc}

	for _, old := range old {
		old(cfg, cvc)
	}

	b, err := cfg.Marshal(run.MarshalTOML)
	require.NoError(t, err)
	require.Equal(t, trim(inlineKeys(expect)), trim(string(b)))

	// What the migration writes loads in the node.
	require.NoError(t, new(run.Config).Load(b, toml.Unmarshal))
}

// The expectations name each key by the CometBFT key file it came from, as
// the node once referenced it; the migration writes the key itself. Each key
// file the fixtures write holds testKey(path), so the expectation can be
// rewritten to the inline form the migration must produce.
var reCometKey = regexp.MustCompile(`path = "([^"]*)"\s*\n\s*type = "comet(?:PrivValFile|NodeKeyFile)"`)

func inlineKeys(s string) string {
	return reCometKey.ReplaceAllStringFunc(s, func(m string) string {
		path := reCometKey.FindStringSubmatch(m)[1]
		return fmt.Sprintf("address = %q\ntype = \"raw\"", address.FromED25519PrivateKey(testKey(path)).String())
	})
}

// testKey is the key the fixture writes for the key file at path, relative to
// the node directory.
func testKey(path string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(filepath.Clean(path)))
	return ed25519.NewKeyFromSeed(seed[:])
}

func writeCometKey(t *testing.T, root, path string) {
	t.Helper()
	writeCometKeyAs(t, root, path, path)
}

// writeCometKeyAs writes, at path, the key the fixture writes for keyOf.
func writeCometKeyAs(t *testing.T, root, path, keyOf string) {
	t.Helper()
	file := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
	b, err := json.Marshal(map[string]any{"priv_key": map[string]any{
		"type":  "tendermint/PrivKeyEd25519",
		"value": []byte(testKey(keyOf)),
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, b, 0600))
}

func trim(s string) string {
	p := strings.Split(s, "\n")
	for i, s := range p {
		p[i] = strings.TrimSpace(s)
	}
	p = slices.DeleteFunc(p, func(s string) bool {
		return len(s) == 0
	})
	return strings.Join(p, "\n")
}

type applyFunc = func(*run.Config, *run.CoreValidatorConfiguration)

func applyOld(t *testing.T, dir, acc, tm string) applyFunc {
	t.Helper()
	return applyOldNodeKey(t, dir, "", acc, tm)
}

// applyOldNodeKey is applyOld with the partition's node key file holding
// testKey(nodeKey) instead of its own: main's `init dual` gave both partitions
// one node key.
func applyOldNodeKey(t *testing.T, dir, nodeKey, acc, tm string) applyFunc {
	t.Helper()
	root := t.TempDir()
	part := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(part, "config"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(part, "config", "accumulate.toml"), []byte(acc), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(part, "config", "tendermint.toml"), []byte(tm), 0600))
	old, err := loadLegacy(part)
	require.NoError(t, err)
	if nodeKey == "" {
		writeCometKey(t, root, filepath.Join(dir, old.NodeKey))
	} else {
		writeCometKeyAs(t, root, filepath.Join(dir, old.NodeKey), nodeKey)
	}
	writeCometKey(t, root, filepath.Join(dir, old.PrivValidatorKey))
	return func(cfg *run.Config, cvc *run.CoreValidatorConfiguration) {
		t.Helper()
		require.NoError(t, migrateCfg(cfg, cvc, root, dir, old))
	}
}
