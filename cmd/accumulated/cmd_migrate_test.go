// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"golang.org/x/exp/slices"
)

func TestMigrateOld(t *testing.T) {
	t.Run("Dual-node", func(t *testing.T) {
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-bootstrap-peers = ["/dns/fozzie-bvn0.accumulate.defidevs.io/tcp/16691/p2p/QmVmNTZgZDEfS4NAi4bMKj9BnBLb7R1YdKmfmBH7nPwBKM"]
			bvn-genesis = "bvnn/config/genesis.json"
			dn-bootstrap-peers = ["/dns/fozzie-bvn0.accumulate.defidevs.io/tcp/16591/p2p/QmTaoxQm8iW3kunoJzQvDNUmeKBqz8khYKCbR38cqMb3Dq"]
			dn-genesis = "dnn/config/genesis.json"
			enable-healing = false
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
			applyOld(t, "bvnn",
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
			enable-healing = false
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
			enable-healing = false
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
			enable-healing = false
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
		testMigrateOld(t,
			`network = "Fozzie"

			[[configurations]]
			bvn = "Grizzly"
			bvn-genesis = "bvnn/config/genesis.json"
			enable-healing = false
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

			[[services]]
			directory = "snapshots"
			enable-indexing = false
			partition = "Grizzly"
			retain-count = 10
			schedule = "0 */12 * * *"
			type = "snapshot"
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
	require.Equal(t, trim(expect), trim(string(b)))
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
	tmp := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tmp, "config"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "config", "accumulate.toml"), []byte(acc), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "config", "tendermint.toml"), []byte(tm), 0600))
	old, err := config.Load(tmp)
	require.NoError(t, err)
	return func(cfg *run.Config, cvc *run.CoreValidatorConfiguration) {
		t.Helper()
		require.NoError(t, migrateCfg(cfg, cvc, dir, old))
	}
}
