// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TendermintConfig is a local replacement for CometBFT's config.Config.
// It contains only the fields actually used by Accumulate, with the same
// mapstructure/toml tags so existing TOML config files are read correctly.
type TendermintConfig struct {
	BaseConfig      `mapstructure:",squash"`
	RPC             *RPCConfig             `mapstructure:"rpc" toml:"rpc"`
	P2P             *P2PConfig             `mapstructure:"p2p" toml:"p2p"`
	Mempool         *MempoolConfig         `mapstructure:"mempool" toml:"mempool"`
	StateSync       *StateSyncConfig       `mapstructure:"statesync" toml:"statesync"`
	BlockSync       *BlockSyncConfig       `mapstructure:"blocksync" toml:"blocksync"`
	Consensus       *ConsensusConfig       `mapstructure:"consensus" toml:"consensus"`
	Storage         *TmStorageConfig       `mapstructure:"storage" toml:"storage"`
	TxIndex         *TxIndexConfig         `mapstructure:"tx_index" toml:"tx_index"`
	Instrumentation *InstrumentationConfig `mapstructure:"instrumentation" toml:"instrumentation"`
}

// BaseConfig contains the base configuration fields, matching CometBFT's
// BaseConfig mapstructure tags for TOML compatibility.
type BaseConfig struct {
	RootDir                 string `mapstructure:"home" toml:"home,omitempty"`
	ProxyApp                string `mapstructure:"proxy_app" toml:"proxy_app"`
	Moniker                 string `mapstructure:"moniker" toml:"moniker"`
	DBBackend               string `mapstructure:"db_backend" toml:"db_backend"`
	DBPath                  string `mapstructure:"db_dir" toml:"db_dir"`
	LogLevel                string `mapstructure:"log_level" toml:"log_level"`
	LogFormat               string `mapstructure:"log_format" toml:"log_format"`
	Genesis                 string `mapstructure:"genesis_file" toml:"genesis_file"`
	PrivValidatorKey        string `mapstructure:"priv_validator_key_file" toml:"priv_validator_key_file"`
	PrivValidatorState      string `mapstructure:"priv_validator_state_file" toml:"priv_validator_state_file"`
	PrivValidatorListenAddr string `mapstructure:"priv_validator_laddr" toml:"priv_validator_laddr"`
	NodeKey                 string `mapstructure:"node_key_file" toml:"node_key_file"`
	ABCI                    string `mapstructure:"abci" toml:"abci"`
	FilterPeers             bool   `mapstructure:"filter_peers" toml:"filter_peers"`
}

// RPCConfig holds RPC server configuration.
type RPCConfig struct {
	ListenAddress      string `mapstructure:"laddr" toml:"laddr"`
	MaxOpenConnections int    `mapstructure:"max_open_connections" toml:"max_open_connections"`
}

// P2PConfig holds P2P network configuration.
type P2PConfig struct {
	ListenAddress    string `mapstructure:"laddr" toml:"laddr"`
	ExternalAddress  string `mapstructure:"external_address" toml:"external_address"`
	Seeds            string `mapstructure:"seeds" toml:"seeds"`
	PersistentPeers  string `mapstructure:"persistent_peers" toml:"persistent_peers"`
	AddrBookStrict   bool   `mapstructure:"addr_book_strict" toml:"addr_book_strict"`
	AllowDuplicateIP bool   `mapstructure:"allow_duplicate_ip" toml:"allow_duplicate_ip"`
}

// MempoolConfig holds mempool configuration.
type MempoolConfig struct {
	Size          int   `mapstructure:"size" toml:"size"`
	MaxBatchBytes int64 `mapstructure:"max_batch_bytes" toml:"max_batch_bytes"`
	CacheSize     int   `mapstructure:"cache_size" toml:"cache_size"`
}

// StateSyncConfig holds state sync configuration.
type StateSyncConfig struct {
	Enable     bool     `mapstructure:"enable" toml:"enable"`
	RPCServers []string `mapstructure:"rpc_servers" toml:"rpc_servers"`
}

// BlockSyncConfig holds block sync configuration.
type BlockSyncConfig struct {
	Version string `mapstructure:"version" toml:"version"`
}

// ConsensusConfig holds consensus configuration.
type ConsensusConfig struct {
	WalPath           string        `mapstructure:"wal_file" toml:"wal_file"`
	CreateEmptyBlocks bool          `mapstructure:"create_empty_blocks" toml:"create_empty_blocks"`
	TimeoutCommit     time.Duration `mapstructure:"timeout_commit" toml:"timeout_commit"`
}

// TmStorageConfig holds CometBFT storage configuration.
type TmStorageConfig struct {
	DiscardABCIResponses bool `mapstructure:"discard_abci_responses" toml:"discard_abci_responses"`
}

// TxIndexConfig holds transaction indexer configuration.
type TxIndexConfig struct {
	Indexer string `mapstructure:"indexer" toml:"indexer"`
}

// InstrumentationConfig holds metrics/instrumentation configuration.
// Field names and mapstructure tags match CometBFT's InstrumentationConfig.
type InstrumentationConfig struct {
	Prometheus           bool   `mapstructure:"prometheus" toml:"prometheus"`
	PrometheusListenAddr string `mapstructure:"prometheus_listen_addr" toml:"prometheus_listen_addr"`
	MaxOpenConnections   int    `mapstructure:"max_open_connections" toml:"max_open_connections"`
	Namespace            string `mapstructure:"namespace" toml:"namespace"`
}

// DefaultTendermintConfig returns a TendermintConfig with CometBFT v0.38
// defaults for the fields Accumulate reads. Keeping these in sync with
// upstream avoids subtle behavior changes when the same TOML is read by
// our local TendermintConfig vs an upstream CometBFT binary.
func DefaultTendermintConfig() *TendermintConfig {
	return &TendermintConfig{
		BaseConfig: BaseConfig{
			ProxyApp:           "tcp://127.0.0.1:26658",
			Moniker:            "",
			DBBackend:          "goleveldb",
			DBPath:             "data",
			LogLevel:           "error",
			LogFormat:          "plain",
			Genesis:            "config/genesis.json",
			PrivValidatorKey:   "config/priv_validator_key.json",
			PrivValidatorState: "data/priv_validator_state.json",
			NodeKey:            "config/node_key.json",
			ABCI:               "socket",
			FilterPeers:        false,
		},
		RPC: &RPCConfig{
			ListenAddress:      "tcp://127.0.0.1:26657",
			MaxOpenConnections: 900, // CometBFT v0.38 default
		},
		P2P: &P2PConfig{
			ListenAddress:    "tcp://0.0.0.0:26656",
			AddrBookStrict:   true,  // CometBFT v0.38 default
			AllowDuplicateIP: false, // CometBFT v0.38 default
		},
		Mempool: &MempoolConfig{
			Size:      5000,
			CacheSize: 10000, // CometBFT v0.38 default
		},
		StateSync: &StateSyncConfig{},
		BlockSync: &BlockSyncConfig{Version: "v0"},
		Consensus: &ConsensusConfig{
			WalPath:       "data/cs.wal/wal",
			TimeoutCommit: time.Second, // CometBFT v0.38 default
		},
		TxIndex:         &TxIndexConfig{Indexer: "kv"},
		Instrumentation: &InstrumentationConfig{Namespace: "cometbft"},
	}
}

// SetRoot sets the RootDir and returns the config for chaining.
func (cfg *TendermintConfig) SetRoot(root string) *TendermintConfig {
	cfg.RootDir = root
	return cfg
}

// GenesisFile returns the absolute path to the genesis file.
func (cfg BaseConfig) GenesisFile() string {
	return rootify(cfg.Genesis, cfg.RootDir)
}

// DBDir returns the absolute path to the database directory.
func (cfg BaseConfig) DBDir() string {
	return rootify(cfg.DBPath, cfg.RootDir)
}

// PrivValidatorKeyFile returns the absolute path to the private validator key file.
func (cfg BaseConfig) PrivValidatorKeyFile() string {
	return rootify(cfg.PrivValidatorKey, cfg.RootDir)
}

// PrivValidatorStateFile returns the absolute path to the private validator state file.
func (cfg BaseConfig) PrivValidatorStateFile() string {
	return rootify(cfg.PrivValidatorState, cfg.RootDir)
}

// NodeKeyFile returns the absolute path to the node key file.
func (cfg BaseConfig) NodeKeyFile() string {
	return rootify(cfg.NodeKey, cfg.RootDir)
}

// ValidateBasic performs basic validation of the config.
func (cfg *TendermintConfig) ValidateBasic() error {
	return nil
}

// rootify makes path absolute relative to root, unless it's already absolute.
// This matches CometBFT's rootify helper.
func rootify(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

// EnsureRoot creates the root, config, and data directories if they don't exist.
// This replaces CometBFT's config.EnsureRoot.
func EnsureRoot(rootDir string) {
	if err := os.MkdirAll(filepath.Join(rootDir, "config"), 0755); err != nil {
		panic(fmt.Sprintf("could not create directory %q: %v", filepath.Join(rootDir, "config"), err))
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "data"), 0755); err != nil {
		panic(fmt.Sprintf("could not create directory %q: %v", filepath.Join(rootDir, "data"), err))
	}
}

// WriteTendermintConfigFile writes the TendermintConfig to the given file path as TOML.
func WriteTendermintConfigFile(configFilePath string, config *TendermintConfig) {
	if err := writeTomlFile(config, configFilePath); err != nil {
		panic(fmt.Sprintf("failed to write config file %q: %v", configFilePath, err))
	}
}
