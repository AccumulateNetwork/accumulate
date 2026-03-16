// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package config provides configuration structures for DAG-BFT consensus.
package config

import (
	"fmt"
	"time"
)

// Default configuration values for DAG-BFT consensus.
const (
	// Consensus defaults
	DefaultNumWorkers       = 1
	DefaultDAGGCDepth       = 50
	DefaultCommitBufferSize = 5000

	// Batching defaults
	DefaultBatchSize       = 500
	DefaultBatchTimeout    = 100 * time.Millisecond
	DefaultMaxBatchBytes   = 500 * 1024 // 500KB
	DefaultMaxPendingSize  = 10 * 1024 * 1024 // 10MB
	DefaultMaxStoredBatches = 10000

	// Timing defaults
	DefaultBlockInterval    = 3 * time.Second
	DefaultMinRoundInterval = 100 * time.Millisecond
	DefaultWarmupPeriod     = 8 * time.Second

	// Network defaults
	DefaultListenPort = 9000
)

// Config is the configuration for a DAG-BFT consensus node.
type Config struct {
	// Consensus configuration
	Consensus ConsensusConfig `toml:"consensus"`

	// Batching configuration
	Batching BatchingConfig `toml:"batching"`

	// Timing configuration
	Timing TimingConfig `toml:"timing"`

	// Network configuration
	Network NetworkConfig `toml:"network"`

	// Logging configuration
	Logging LoggingConfig `toml:"logging"`
}

// ConsensusConfig contains consensus-related settings.
type ConsensusConfig struct {
	// NumWorkers is the number of batch workers to run.
	// More workers can increase throughput but use more resources.
	NumWorkers int `toml:"num_workers"`

	// DAGGCDepth is the number of rounds to keep in the DAG.
	// Older rounds are garbage collected.
	DAGGCDepth int `toml:"dag_gc_depth"`

	// CommitBufferSize is the buffer size for committed certificates.
	// Should be large enough to handle bursts of commits.
	CommitBufferSize int `toml:"commit_buffer_size"`
}

// BatchingConfig contains batch creation settings.
type BatchingConfig struct {
	// BatchSize is the maximum number of transactions per batch.
	BatchSize int `toml:"batch_size"`

	// BatchTimeout is the maximum time to wait for a full batch.
	BatchTimeout time.Duration `toml:"batch_timeout"`

	// MaxBatchBytes is the maximum size of a batch in bytes.
	MaxBatchBytes int `toml:"max_batch_bytes"`

	// MaxPendingSize is the maximum total size of pending transactions.
	// When exceeded, new transactions are rejected (backpressure).
	MaxPendingSize int `toml:"max_pending_size"`

	// MaxStoredBatches is the maximum number of batches to store.
	// When exceeded, old batches are evicted.
	MaxStoredBatches int `toml:"max_stored_batches"`
}

// TimingConfig contains timing-related settings.
type TimingConfig struct {
	// BlockInterval is the target time between blocks.
	BlockInterval time.Duration `toml:"block_interval"`

	// MinRoundInterval is the minimum time between consensus rounds.
	// Prevents consensus from racing ahead of execution.
	MinRoundInterval time.Duration `toml:"min_round_interval"`

	// WarmupPeriod is the time to wait for the gossip mesh to form
	// before starting consensus.
	WarmupPeriod time.Duration `toml:"warmup_period"`
}

// NetworkConfig contains network-related settings.
type NetworkConfig struct {
	// ListenAddr is the multiaddr to listen on.
	// Example: /ip4/0.0.0.0/tcp/9000
	ListenAddr string `toml:"listen_addr"`

	// BootstrapPeers is a list of peer multiaddrs to connect to at startup.
	BootstrapPeers []string `toml:"bootstrap_peers"`

	// ExternalAddr is the external address to advertise to peers.
	// If empty, the listen address is used.
	ExternalAddr string `toml:"external_addr"`
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	// Level is the log level: debug, info, warn, error
	Level string `toml:"level"`

	// Format is the log format: text, json
	Format string `toml:"format"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Consensus: ConsensusConfig{
			NumWorkers:       DefaultNumWorkers,
			DAGGCDepth:       DefaultDAGGCDepth,
			CommitBufferSize: DefaultCommitBufferSize,
		},
		Batching: BatchingConfig{
			BatchSize:        DefaultBatchSize,
			BatchTimeout:     DefaultBatchTimeout,
			MaxBatchBytes:    DefaultMaxBatchBytes,
			MaxPendingSize:   DefaultMaxPendingSize,
			MaxStoredBatches: DefaultMaxStoredBatches,
		},
		Timing: TimingConfig{
			BlockInterval:    DefaultBlockInterval,
			MinRoundInterval: DefaultMinRoundInterval,
			WarmupPeriod:     DefaultWarmupPeriod,
		},
		Network: NetworkConfig{
			ListenAddr: fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", DefaultListenPort),
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.Consensus.NumWorkers < 1 {
		return fmt.Errorf("num_workers must be at least 1")
	}
	if c.Consensus.DAGGCDepth < 10 {
		return fmt.Errorf("dag_gc_depth must be at least 10")
	}
	if c.Consensus.CommitBufferSize < 100 {
		return fmt.Errorf("commit_buffer_size must be at least 100")
	}

	if c.Batching.BatchSize < 1 {
		return fmt.Errorf("batch_size must be at least 1")
	}
	if c.Batching.BatchTimeout < time.Millisecond {
		return fmt.Errorf("batch_timeout must be at least 1ms")
	}
	if c.Batching.MaxBatchBytes < 1024 {
		return fmt.Errorf("max_batch_bytes must be at least 1KB")
	}

	if c.Timing.BlockInterval < time.Second {
		return fmt.Errorf("block_interval must be at least 1s")
	}
	if c.Timing.MinRoundInterval < 10*time.Millisecond {
		return fmt.Errorf("min_round_interval must be at least 10ms")
	}

	if c.Network.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}

	return nil
}

// ApplyDefaults fills in default values for any unset fields.
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()

	// Consensus defaults
	if c.Consensus.NumWorkers <= 0 {
		c.Consensus.NumWorkers = defaults.Consensus.NumWorkers
	}
	if c.Consensus.DAGGCDepth <= 0 {
		c.Consensus.DAGGCDepth = defaults.Consensus.DAGGCDepth
	}
	if c.Consensus.CommitBufferSize <= 0 {
		c.Consensus.CommitBufferSize = defaults.Consensus.CommitBufferSize
	}

	// Batching defaults
	if c.Batching.BatchSize <= 0 {
		c.Batching.BatchSize = defaults.Batching.BatchSize
	}
	if c.Batching.BatchTimeout <= 0 {
		c.Batching.BatchTimeout = defaults.Batching.BatchTimeout
	}
	if c.Batching.MaxBatchBytes <= 0 {
		c.Batching.MaxBatchBytes = defaults.Batching.MaxBatchBytes
	}
	if c.Batching.MaxPendingSize <= 0 {
		c.Batching.MaxPendingSize = defaults.Batching.MaxPendingSize
	}
	if c.Batching.MaxStoredBatches <= 0 {
		c.Batching.MaxStoredBatches = defaults.Batching.MaxStoredBatches
	}

	// Timing defaults
	if c.Timing.BlockInterval <= 0 {
		c.Timing.BlockInterval = defaults.Timing.BlockInterval
	}
	if c.Timing.MinRoundInterval <= 0 {
		c.Timing.MinRoundInterval = defaults.Timing.MinRoundInterval
	}
	if c.Timing.WarmupPeriod <= 0 {
		c.Timing.WarmupPeriod = defaults.Timing.WarmupPeriod
	}

	// Network defaults
	if c.Network.ListenAddr == "" {
		c.Network.ListenAddr = defaults.Network.ListenAddr
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = defaults.Logging.Level
	}
	if c.Logging.Format == "" {
		c.Logging.Format = defaults.Logging.Format
	}
}
