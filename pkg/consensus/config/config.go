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
	DefaultNumWorkers = 1
	// DefaultDAGGCDepth is also the round catch-up window (#4057) — see
	// consensus.DefaultDAGGCDepth. Keep the two in sync.
	DefaultDAGGCDepth       = 2_000
	DefaultCommitBufferSize = 5000

	// Batching defaults
	DefaultBatchSize        = 500
	DefaultBatchTimeout     = 100 * time.Millisecond
	DefaultMaxBatchBytes    = 500 * 1024       // 500KB
	DefaultMaxPendingSize   = 10 * 1024 * 1024 // 10MB
	DefaultMaxStoredBatches = 10000

	// Timing defaults
	//
	// Cross-partition settlement latency is priced in block intervals: the
	// proof path (source block -> source anchor -> DN block -> DN anchor ->
	// destination block) is ~6-7 intervals, measured as a uniform ~21s of
	// in-flight gap on every channel at 3s blocks (run 20260824T110727Z —
	// every channel drained at its arrival rate; the gap was pure latency).
	// Block-per-commit (#4164) made per-block overhead small enough that 1s
	// blocks are affordable at MODERATE load — but per-second overheads
	// (dispatch, anchors, block boundaries) scale with 1/interval, and at
	// 700 tx/s on 1s blocks the network saturated: BVN blocks stretched to
	// 2s+, dispatch losses opened sequence holes faster than healing filled
	// them (126k heals in 20 min), count-capped queues ballooned, and the
	// run collapsed (20260824T114552Z). 2s was tried as the balance point
	// and five consecutive runs at 250-400 tps died between t+15 and t+40 —
	// each fix (byte caps, GOMEMLIMIT, 256MB caches, capped universe, query
	// gates, record cache) moved the clock later but never past it. 3s is
	// the ONLY interval that has run clean for 40+ minutes at real load
	// (728 tx/s, run 20260824T065208Z — channels clean throughout; it died
	// of memory bugs that are all fixed now). Settlement ~29s; latency
	// improvements come back after a 12h pass exists.
	DefaultBlockInterval    = 3 * time.Second
	DefaultMinRoundInterval = 100 * time.Millisecond
	DefaultWarmupPeriod     = 8 * time.Second

	// Network defaults
	DefaultListenPort        = 9000
	DefaultMaxInbound        = 200
	DefaultMaxOutbound       = 200
	DefaultConnectionTimeout = 10 * time.Second

	// Channel buffer defaults - sized for high throughput (~30k+ TPS)
	DefaultCertificateBufferSize = 1000 // Buffer for new certificates channel
	DefaultBatchBufferSize       = 1000 // Buffer for batch gossip channel
	DefaultHeaderBufferSize      = 500  // Buffer for header gossip channel
	DefaultVoteBufferSize        = 1000 // Buffer for vote gossip channel
	DefaultCertSyncBufferSize    = 500  // Buffer for certificate sync channels
	DefaultEnvelopeBufferSize    = 500  // Buffer for inter-partition dispatch
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

	// ChannelBuffers contains buffer sizes for internal channels.
	// Larger buffers improve throughput but use more memory.
	ChannelBuffers ChannelBufferConfig `toml:"channel_buffers"`
}

// ChannelBufferConfig contains buffer sizes for consensus channels.
// These buffers help absorb bursts and prevent backpressure under high load.
type ChannelBufferConfig struct {
	// CertificateBufferSize is the buffer for the new certificates channel.
	// Used by Primary to signal certificates to Bullshark.
	CertificateBufferSize int `toml:"certificate_buffer_size"`

	// BatchBufferSize is the buffer for batch gossip channels.
	// Used by workers to receive batches from other nodes.
	BatchBufferSize int `toml:"batch_buffer_size"`

	// HeaderBufferSize is the buffer for header gossip channels.
	// Used by Primary to receive headers from other validators.
	HeaderBufferSize int `toml:"header_buffer_size"`

	// VoteBufferSize is the buffer for vote gossip channels.
	// Used by Primary to collect votes for certificate creation.
	VoteBufferSize int `toml:"vote_buffer_size"`

	// CertSyncBufferSize is the buffer for certificate sync channels.
	// Used by CertSyncer for requesting missing certificates.
	CertSyncBufferSize int `toml:"cert_sync_buffer_size"`

	// EnvelopeBufferSize is the buffer for inter-partition dispatch.
	// Used by Dispatcher for cross-partition message routing.
	EnvelopeBufferSize int `toml:"envelope_buffer_size"`
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

	// MaxInbound is the maximum number of inbound connections.
	MaxInbound int `toml:"max_inbound"`

	// MaxOutbound is the maximum number of outbound connections.
	MaxOutbound int `toml:"max_outbound"`

	// ConnectionTimeout is the timeout for establishing connections.
	ConnectionTimeout time.Duration `toml:"connection_timeout"`
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	// Level is the log level: debug, info, warn, error
	Level string `toml:"level"`

	// Format is the log format: text, json
	Format string `toml:"format"`
}

// DefaultChannelBufferConfig returns a ChannelBufferConfig with default values.
func DefaultChannelBufferConfig() ChannelBufferConfig {
	return ChannelBufferConfig{
		CertificateBufferSize: DefaultCertificateBufferSize,
		BatchBufferSize:       DefaultBatchBufferSize,
		HeaderBufferSize:      DefaultHeaderBufferSize,
		VoteBufferSize:        DefaultVoteBufferSize,
		CertSyncBufferSize:    DefaultCertSyncBufferSize,
		EnvelopeBufferSize:    DefaultEnvelopeBufferSize,
	}
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Consensus: ConsensusConfig{
			NumWorkers:       DefaultNumWorkers,
			DAGGCDepth:       DefaultDAGGCDepth,
			CommitBufferSize: DefaultCommitBufferSize,
			ChannelBuffers:   DefaultChannelBufferConfig(),
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
			ListenAddr:        fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", DefaultListenPort),
			MaxInbound:        DefaultMaxInbound,
			MaxOutbound:       DefaultMaxOutbound,
			ConnectionTimeout: DefaultConnectionTimeout,
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

	// Validate channel buffer sizes
	if err := c.Consensus.ChannelBuffers.Validate(); err != nil {
		return fmt.Errorf("channel_buffers: %w", err)
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

// Validate validates channel buffer configuration.
func (c *ChannelBufferConfig) Validate() error {
	if c.CertificateBufferSize < 10 {
		return fmt.Errorf("certificate_buffer_size must be at least 10")
	}
	if c.BatchBufferSize < 10 {
		return fmt.Errorf("batch_buffer_size must be at least 10")
	}
	if c.HeaderBufferSize < 10 {
		return fmt.Errorf("header_buffer_size must be at least 10")
	}
	if c.VoteBufferSize < 10 {
		return fmt.Errorf("vote_buffer_size must be at least 10")
	}
	if c.CertSyncBufferSize < 10 {
		return fmt.Errorf("cert_sync_buffer_size must be at least 10")
	}
	if c.EnvelopeBufferSize < 10 {
		return fmt.Errorf("envelope_buffer_size must be at least 10")
	}
	return nil
}

// ApplyDefaults fills in default values for any unset channel buffer fields.
func (c *ChannelBufferConfig) ApplyDefaults() {
	if c.CertificateBufferSize <= 0 {
		c.CertificateBufferSize = DefaultCertificateBufferSize
	}
	if c.BatchBufferSize <= 0 {
		c.BatchBufferSize = DefaultBatchBufferSize
	}
	if c.HeaderBufferSize <= 0 {
		c.HeaderBufferSize = DefaultHeaderBufferSize
	}
	if c.VoteBufferSize <= 0 {
		c.VoteBufferSize = DefaultVoteBufferSize
	}
	if c.CertSyncBufferSize <= 0 {
		c.CertSyncBufferSize = DefaultCertSyncBufferSize
	}
	if c.EnvelopeBufferSize <= 0 {
		c.EnvelopeBufferSize = DefaultEnvelopeBufferSize
	}
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

	// Channel buffer defaults
	c.Consensus.ChannelBuffers.ApplyDefaults()

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
	if c.Network.MaxInbound <= 0 {
		c.Network.MaxInbound = defaults.Network.MaxInbound
	}
	if c.Network.MaxOutbound <= 0 {
		c.Network.MaxOutbound = defaults.Network.MaxOutbound
	}
	if c.Network.ConnectionTimeout <= 0 {
		c.Network.ConnectionTimeout = defaults.Network.ConnectionTimeout
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = defaults.Logging.Level
	}
	if c.Logging.Format == "" {
		c.Logging.Format = defaults.Logging.Format
	}
}
