// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)

	// Verify defaults are set
	assert.Equal(t, DefaultNumWorkers, cfg.Consensus.NumWorkers)
	assert.Equal(t, DefaultDAGGCDepth, cfg.Consensus.DAGGCDepth)
	assert.Equal(t, DefaultCommitBufferSize, cfg.Consensus.CommitBufferSize)

	assert.Equal(t, DefaultBatchSize, cfg.Batching.BatchSize)
	assert.Equal(t, DefaultBatchTimeout, cfg.Batching.BatchTimeout)
	assert.Equal(t, DefaultMaxBatchBytes, cfg.Batching.MaxBatchBytes)

	assert.Equal(t, DefaultBlockInterval, cfg.Timing.BlockInterval)
	assert.Equal(t, DefaultMinRoundInterval, cfg.Timing.MinRoundInterval)
	assert.Equal(t, DefaultWarmupPeriod, cfg.Timing.WarmupPeriod)

	assert.Contains(t, cfg.Network.ListenAddr, "9000")
	assert.Equal(t, "info", cfg.Logging.Level)

	// Verify channel buffer defaults
	assert.Equal(t, DefaultCertificateBufferSize, cfg.Consensus.ChannelBuffers.CertificateBufferSize)
	assert.Equal(t, DefaultBatchBufferSize, cfg.Consensus.ChannelBuffers.BatchBufferSize)
	assert.Equal(t, DefaultHeaderBufferSize, cfg.Consensus.ChannelBuffers.HeaderBufferSize)
	assert.Equal(t, DefaultVoteBufferSize, cfg.Consensus.ChannelBuffers.VoteBufferSize)
	assert.Equal(t, DefaultCertSyncBufferSize, cfg.Consensus.ChannelBuffers.CertSyncBufferSize)
	assert.Equal(t, DefaultEnvelopeBufferSize, cfg.Consensus.ChannelBuffers.EnvelopeBufferSize)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "default config is valid",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "zero workers invalid",
			modify:  func(c *Config) { c.Consensus.NumWorkers = 0 },
			wantErr: true,
		},
		{
			name:    "low gc depth invalid",
			modify:  func(c *Config) { c.Consensus.DAGGCDepth = 5 },
			wantErr: true,
		},
		{
			name:    "low commit buffer invalid",
			modify:  func(c *Config) { c.Consensus.CommitBufferSize = 50 },
			wantErr: true,
		},
		{
			name:    "zero batch size invalid",
			modify:  func(c *Config) { c.Batching.BatchSize = 0 },
			wantErr: true,
		},
		{
			name:    "too short batch timeout invalid",
			modify:  func(c *Config) { c.Batching.BatchTimeout = 100 * time.Microsecond },
			wantErr: true,
		},
		{
			name:    "too small max batch bytes invalid",
			modify:  func(c *Config) { c.Batching.MaxBatchBytes = 512 },
			wantErr: true,
		},
		{
			name:    "too short block interval invalid",
			modify:  func(c *Config) { c.Timing.BlockInterval = 500 * time.Millisecond },
			wantErr: true,
		},
		{
			name:    "too short min round interval invalid",
			modify:  func(c *Config) { c.Timing.MinRoundInterval = 5 * time.Millisecond },
			wantErr: true,
		},
		{
			name:    "empty listen addr invalid",
			modify:  func(c *Config) { c.Network.ListenAddr = "" },
			wantErr: true,
		},
		{
			name:    "low certificate buffer size invalid",
			modify:  func(c *Config) { c.Consensus.ChannelBuffers.CertificateBufferSize = 5 },
			wantErr: true,
		},
		{
			name:    "low batch buffer size invalid",
			modify:  func(c *Config) { c.Consensus.ChannelBuffers.BatchBufferSize = 5 },
			wantErr: true,
		},
		{
			name:    "low header buffer size invalid",
			modify:  func(c *Config) { c.Consensus.ChannelBuffers.HeaderBufferSize = 5 },
			wantErr: true,
		},
		{
			name:    "low vote buffer size invalid",
			modify:  func(c *Config) { c.Consensus.ChannelBuffers.VoteBufferSize = 5 },
			wantErr: true,
		},
		{
			name:    "low cert sync buffer size invalid",
			modify:  func(c *Config) { c.Consensus.ChannelBuffers.CertSyncBufferSize = 5 },
			wantErr: true,
		},
		{
			name:    "low envelope buffer size invalid",
			modify:  func(c *Config) { c.Consensus.ChannelBuffers.EnvelopeBufferSize = 5 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	// Start with empty config
	cfg := &Config{}
	cfg.ApplyDefaults()

	// Should have all defaults
	assert.Equal(t, DefaultNumWorkers, cfg.Consensus.NumWorkers)
	assert.Equal(t, DefaultDAGGCDepth, cfg.Consensus.DAGGCDepth)
	assert.Equal(t, DefaultBatchSize, cfg.Batching.BatchSize)
	assert.Equal(t, DefaultBlockInterval, cfg.Timing.BlockInterval)
	assert.Equal(t, "info", cfg.Logging.Level)
}

func TestConfigApplyDefaultsPreservesExisting(t *testing.T) {
	cfg := &Config{
		Consensus: ConsensusConfig{
			NumWorkers: 4,
		},
		Batching: BatchingConfig{
			BatchSize: 1000,
		},
	}
	cfg.ApplyDefaults()

	// Should preserve existing values
	assert.Equal(t, 4, cfg.Consensus.NumWorkers)
	assert.Equal(t, 1000, cfg.Batching.BatchSize)

	// Should fill in defaults for unset values
	assert.Equal(t, DefaultDAGGCDepth, cfg.Consensus.DAGGCDepth)
	assert.Equal(t, DefaultBlockInterval, cfg.Timing.BlockInterval)

	// Should fill in channel buffer defaults
	assert.Equal(t, DefaultCertificateBufferSize, cfg.Consensus.ChannelBuffers.CertificateBufferSize)
	assert.Equal(t, DefaultBatchBufferSize, cfg.Consensus.ChannelBuffers.BatchBufferSize)
}

func TestChannelBufferConfigDefaults(t *testing.T) {
	cfg := DefaultChannelBufferConfig()

	assert.Equal(t, DefaultCertificateBufferSize, cfg.CertificateBufferSize)
	assert.Equal(t, DefaultBatchBufferSize, cfg.BatchBufferSize)
	assert.Equal(t, DefaultHeaderBufferSize, cfg.HeaderBufferSize)
	assert.Equal(t, DefaultVoteBufferSize, cfg.VoteBufferSize)
	assert.Equal(t, DefaultCertSyncBufferSize, cfg.CertSyncBufferSize)
	assert.Equal(t, DefaultEnvelopeBufferSize, cfg.EnvelopeBufferSize)
}

func TestChannelBufferConfigApplyDefaults(t *testing.T) {
	cfg := &ChannelBufferConfig{
		CertificateBufferSize: 2000, // Custom value
	}
	cfg.ApplyDefaults()

	// Should preserve custom value
	assert.Equal(t, 2000, cfg.CertificateBufferSize)

	// Should apply defaults to unset values
	assert.Equal(t, DefaultBatchBufferSize, cfg.BatchBufferSize)
	assert.Equal(t, DefaultHeaderBufferSize, cfg.HeaderBufferSize)
	assert.Equal(t, DefaultVoteBufferSize, cfg.VoteBufferSize)
	assert.Equal(t, DefaultCertSyncBufferSize, cfg.CertSyncBufferSize)
	assert.Equal(t, DefaultEnvelopeBufferSize, cfg.EnvelopeBufferSize)
}

func TestChannelBufferConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ChannelBufferConfig
		wantErr bool
	}{
		{
			name:    "default config is valid",
			cfg:     DefaultChannelBufferConfig(),
			wantErr: false,
		},
		{
			name: "custom high values are valid",
			cfg: ChannelBufferConfig{
				CertificateBufferSize: 5000,
				BatchBufferSize:       5000,
				HeaderBufferSize:      2000,
				VoteBufferSize:        5000,
				CertSyncBufferSize:    2000,
				EnvelopeBufferSize:    2000,
			},
			wantErr: false,
		},
		{
			name: "low certificate buffer invalid",
			cfg: ChannelBufferConfig{
				CertificateBufferSize: 5,
				BatchBufferSize:       100,
				HeaderBufferSize:      100,
				VoteBufferSize:        100,
				CertSyncBufferSize:    100,
				EnvelopeBufferSize:    100,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
