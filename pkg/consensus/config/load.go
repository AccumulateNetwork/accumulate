// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// LoadFile loads a Config from a TOML file.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	return Load(data)
}

// Load parses a Config from TOML data.
func Load(data []byte) (*Config, error) {
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// SaveFile saves a Config to a TOML file.
func SaveFile(cfg *Config, path string) error {
	data, err := Save(cfg)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// Save serializes a Config to TOML.
func Save(cfg *Config) ([]byte, error) {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	return data, nil
}

// ExampleConfig returns an example TOML configuration as a string.
func ExampleConfig() string {
	return `# DAG-BFT Consensus Configuration
#
# This file configures the DAG-BFT (Bullshark-variant) consensus node.

[consensus]
# Number of batch workers (default: 1)
num_workers = 1

# Rounds to keep in DAG before garbage collection (default: 50)
dag_gc_depth = 50

# Buffer size for committed certificates (default: 5000)
commit_buffer_size = 5000

[batching]
# Maximum transactions per batch (default: 500)
batch_size = 500

# Maximum time to wait for a full batch (default: 100ms)
batch_timeout = "100ms"

# Maximum batch size in bytes (default: 500KB)
max_batch_bytes = 512000

# Maximum total size of pending transactions (default: 10MB)
max_pending_size = 10485760

# Maximum number of stored batches (default: 10000)
max_stored_batches = 10000

[timing]
# Target time between blocks (default: 3s)
block_interval = "3s"

# Minimum time between consensus rounds (default: 100ms)
min_round_interval = "100ms"

# Warmup period for gossip mesh formation (default: 8s)
warmup_period = "8s"

[network]
# Multiaddr to listen on
listen_addr = "/ip4/0.0.0.0/tcp/9000"

# Bootstrap peers to connect to at startup
# bootstrap_peers = [
#   "/ip4/10.0.0.1/tcp/9000/p2p/12D3KooW...",
#   "/ip4/10.0.0.2/tcp/9000/p2p/12D3KooW...",
# ]

# External address to advertise (optional)
# external_addr = "/ip4/1.2.3.4/tcp/9000"

[logging]
# Log level: debug, info, warn, error (default: info)
level = "info"

# Log format: text, json (default: text)
format = "text"
`
}
