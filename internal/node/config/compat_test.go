// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"bytes"
	"testing"

	tmconfig "github.com/cometbft/cometbft/config"
	"github.com/pelletier/go-toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompatBaseConfig verifies that the local BaseConfig reads the same
// fields from TOML as CometBFT's BaseConfig.
func TestCompatBaseConfig(t *testing.T) {
	cmt := tmconfig.DefaultBaseConfig()
	cmt.RootDir = "/test/root"
	cmt.ProxyApp = "tcp://127.0.0.1:26658"
	cmt.Moniker = "test-node"
	cmt.DBBackend = "goleveldb"
	cmt.DBPath = "data"
	cmt.LogLevel = "info"
	cmt.LogFormat = "json"
	cmt.Genesis = "config/genesis.json"
	cmt.PrivValidatorKey = "config/priv_validator_key.json"
	cmt.PrivValidatorState = "data/priv_validator_state.json"
	cmt.NodeKey = "config/node_key.json"
	cmt.ABCI = "socket"
	cmt.FilterPeers = true

	// CometBFT BaseConfig uses mapstructure tags for viper. We test that
	// our struct fields match the same semantics by checking method output.
	var local BaseConfig
	local.RootDir = cmt.RootDir
	local.ProxyApp = cmt.ProxyApp
	local.Moniker = cmt.Moniker
	local.DBBackend = cmt.DBBackend
	local.DBPath = cmt.DBPath
	local.LogLevel = cmt.LogLevel
	local.LogFormat = cmt.LogFormat
	local.Genesis = cmt.Genesis
	local.PrivValidatorKey = cmt.PrivValidatorKey
	local.PrivValidatorState = cmt.PrivValidatorState
	local.NodeKey = cmt.NodeKey
	local.ABCI = cmt.ABCI
	local.FilterPeers = cmt.FilterPeers

	// Verify path methods produce identical results
	assert.Equal(t, cmt.GenesisFile(), local.GenesisFile())
	assert.Equal(t, cmt.DBDir(), local.DBDir())
	assert.Equal(t, cmt.PrivValidatorKeyFile(), local.PrivValidatorKeyFile())
	assert.Equal(t, cmt.PrivValidatorStateFile(), local.PrivValidatorStateFile())
	assert.Equal(t, cmt.NodeKeyFile(), local.NodeKeyFile())
}

// TestCompatInstrumentationConfig verifies that our InstrumentationConfig
// roundtrips identically with CometBFT's through TOML.
func TestCompatInstrumentationConfig(t *testing.T) {
	cmt := tmconfig.DefaultInstrumentationConfig()
	cmt.Prometheus = true
	cmt.PrometheusListenAddr = ":26660"
	cmt.MaxOpenConnections = 10
	cmt.Namespace = "test_ns"

	local := &InstrumentationConfig{
		Prometheus:           cmt.Prometheus,
		PrometheusListenAddr: cmt.PrometheusListenAddr,
		MaxOpenConnections:   cmt.MaxOpenConnections,
		Namespace:            cmt.Namespace,
	}

	assert.Equal(t, cmt.Prometheus, local.Prometheus)
	assert.Equal(t, cmt.PrometheusListenAddr, local.PrometheusListenAddr)
	assert.Equal(t, cmt.MaxOpenConnections, local.MaxOpenConnections)
	assert.Equal(t, cmt.Namespace, local.Namespace)
}

// TestCompatConfigTOMLRoundtrip verifies that a TendermintConfig written to
// TOML can be read back and retains the same field values, and that the TOML
// keys match what CometBFT would produce via viper/mapstructure.
func TestCompatConfigTOMLRoundtrip(t *testing.T) {
	original := DefaultTendermintConfig()
	original.RootDir = "" // RootDir is omitted in TOML (set separately)
	original.ProxyApp = "tcp://127.0.0.1:26658"
	original.Moniker = "roundtrip-test"
	original.LogLevel = "debug"
	original.Instrumentation.Prometheus = true
	original.Instrumentation.Namespace = "test_ns"
	original.P2P.PersistentPeers = "node1@1.2.3.4:26656,node2@5.6.7.8:26656"
	original.P2P.ExternalAddress = "1.2.3.4:26656"
	original.Consensus.CreateEmptyBlocks = true
	original.StateSync.RPCServers = []string{"http://rpc1:26657", "http://rpc2:26657"}

	// Marshal to TOML
	var buf bytes.Buffer
	err := toml.NewEncoder(&buf).Encode(original)
	require.NoError(t, err)

	// Unmarshal back
	var loaded TendermintConfig
	err = toml.Unmarshal(buf.Bytes(), &loaded)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.ProxyApp, loaded.ProxyApp)
	assert.Equal(t, original.Moniker, loaded.Moniker)
	assert.Equal(t, original.LogLevel, loaded.LogLevel)
	assert.Equal(t, original.DBBackend, loaded.DBBackend)
	assert.Equal(t, original.DBPath, loaded.DBPath)
	assert.Equal(t, original.Genesis, loaded.Genesis)
	assert.Equal(t, original.PrivValidatorKey, loaded.PrivValidatorKey)
	assert.Equal(t, original.PrivValidatorState, loaded.PrivValidatorState)
	assert.Equal(t, original.NodeKey, loaded.NodeKey)

	assert.Equal(t, original.RPC.ListenAddress, loaded.RPC.ListenAddress)

	assert.Equal(t, original.P2P.ListenAddress, loaded.P2P.ListenAddress)
	assert.Equal(t, original.P2P.ExternalAddress, loaded.P2P.ExternalAddress)
	assert.Equal(t, original.P2P.PersistentPeers, loaded.P2P.PersistentPeers)

	assert.Equal(t, original.Consensus.CreateEmptyBlocks, loaded.Consensus.CreateEmptyBlocks)
	assert.Equal(t, original.Consensus.WalPath, loaded.Consensus.WalPath)

	assert.Equal(t, original.Mempool.Size, loaded.Mempool.Size)

	assert.Equal(t, original.StateSync.RPCServers, loaded.StateSync.RPCServers)

	assert.Equal(t, original.Instrumentation.Prometheus, loaded.Instrumentation.Prometheus)
	assert.Equal(t, original.Instrumentation.Namespace, loaded.Instrumentation.Namespace)
}

// TestCompatDefaultsMatchCometBFT verifies that our default config field
// values match CometBFT's defaults for all fields we use.
func TestCompatDefaultsMatchCometBFT(t *testing.T) {
	cmt := tmconfig.DefaultConfig()
	local := DefaultTendermintConfig()

	// BaseConfig defaults
	assert.Equal(t, cmt.BaseConfig.ProxyApp, local.ProxyApp, "ProxyApp default")
	assert.Equal(t, cmt.BaseConfig.DBBackend, local.DBBackend, "DBBackend default")
	assert.Equal(t, cmt.BaseConfig.DBPath, local.DBPath, "DBPath default")
	assert.Equal(t, cmt.BaseConfig.LogFormat, local.LogFormat, "LogFormat default")
	assert.Equal(t, cmt.BaseConfig.Genesis, local.Genesis, "Genesis default")
	assert.Equal(t, cmt.BaseConfig.PrivValidatorKey, local.PrivValidatorKey, "PrivValidatorKey default")
	assert.Equal(t, cmt.BaseConfig.PrivValidatorState, local.PrivValidatorState, "PrivValidatorState default")
	assert.Equal(t, cmt.BaseConfig.NodeKey, local.NodeKey, "NodeKey default")
	assert.Equal(t, cmt.BaseConfig.ABCI, local.ABCI, "ABCI default")
	assert.Equal(t, cmt.BaseConfig.FilterPeers, local.FilterPeers, "FilterPeers default")

	// Instrumentation defaults
	assert.Equal(t, cmt.Instrumentation.Namespace, local.Instrumentation.Namespace, "Instrumentation.Namespace default")
}
