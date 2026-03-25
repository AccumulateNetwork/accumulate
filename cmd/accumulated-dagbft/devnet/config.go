//go:build dagbft

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package devnet provides configuration and management for DAG-BFT devnet deployments.
package devnet

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Default configuration values.
const (
	DefaultNetwork          = "dagbft-devnet"
	DefaultNumNodes         = 7
	DefaultBasePort         = 9000
	DefaultNumWorkers       = 4
	DefaultDAGGCDepth       = 50
	DefaultCommitBufferSize = 5000
)

// DevnetConfig holds the configuration for a DAG-BFT devnet.
type DevnetConfig struct {
	// Network is the network ID (e.g., "dagbft-devnet").
	Network string `json:"network"`

	// NumNodes is the number of validator nodes (3-13).
	NumNodes int `json:"numNodes"`

	// BasePort is the starting port (default: 9000).
	BasePort int `json:"basePort"`

	// DataDir is the base data directory.
	DataDir string `json:"dataDir"`

	// Nodes holds individual node configurations.
	Nodes []NodeConfig `json:"nodes"`

	// DAG-BFT specific settings
	NumWorkers       int `json:"numWorkers"`       // Workers per node (default: 4)
	DAGGCDepth       int `json:"dagGCDepth"`       // DAG garbage collection depth
	CommitBufferSize int `json:"commitBufferSize"` // Commit buffer size

	// Seed for deterministic key generation.
	Seed [32]byte `json:"seed"`
}

// NewDevnetConfig creates a new DevnetConfig with default values.
func NewDevnetConfig() *DevnetConfig {
	return &DevnetConfig{
		Network:          DefaultNetwork,
		NumNodes:         DefaultNumNodes,
		BasePort:         DefaultBasePort,
		NumWorkers:       DefaultNumWorkers,
		DAGGCDepth:       DefaultDAGGCDepth,
		CommitBufferSize: DefaultCommitBufferSize,
	}
}

// ValidNodeCounts returns the valid number of nodes for BFT consensus.
// BFT requires n = 3f+1 nodes to tolerate f Byzantine faults.
func ValidNodeCounts() []int {
	return []int{3, 4, 5, 7, 10, 13}
}

// IsValidNodeCount returns true if n is a valid node count for BFT.
func IsValidNodeCount(n int) bool {
	for _, valid := range ValidNodeCounts() {
		if n == valid {
			return true
		}
	}
	return false
}

// Validate checks that the configuration is valid.
func (c *DevnetConfig) Validate() error {
	if c.Network == "" {
		return fmt.Errorf("network is required")
	}
	if c.NumNodes < 3 {
		return fmt.Errorf("numNodes must be at least 3")
	}
	if c.NumNodes > 13 {
		return fmt.Errorf("numNodes must be at most 13")
	}
	if c.BasePort < 1024 || c.BasePort > 65000 {
		return fmt.Errorf("basePort must be between 1024 and 65000")
	}
	if c.DataDir == "" {
		return fmt.Errorf("dataDir is required")
	}
	if c.NumWorkers < 1 {
		return fmt.Errorf("numWorkers must be at least 1")
	}
	if c.DAGGCDepth < 10 {
		return fmt.Errorf("dagGCDepth must be at least 10")
	}
	if c.CommitBufferSize < 100 {
		return fmt.Errorf("commitBufferSize must be at least 100")
	}
	return nil
}

// ApplyDefaults fills in default values for unset fields.
func (c *DevnetConfig) ApplyDefaults() {
	if c.Network == "" {
		c.Network = DefaultNetwork
	}
	if c.NumNodes == 0 {
		c.NumNodes = DefaultNumNodes
	}
	if c.BasePort == 0 {
		c.BasePort = DefaultBasePort
	}
	if c.NumWorkers == 0 {
		c.NumWorkers = DefaultNumWorkers
	}
	if c.DAGGCDepth == 0 {
		c.DAGGCDepth = DefaultDAGGCDepth
	}
	if c.CommitBufferSize == 0 {
		c.CommitBufferSize = DefaultCommitBufferSize
	}
}

// GenerateNodeConfigs generates NodeConfig entries for all nodes.
func (c *DevnetConfig) GenerateNodeConfigs() error {
	c.Nodes = make([]NodeConfig, c.NumNodes)

	for i := 0; i < c.NumNodes; i++ {
		nodeID := fmt.Sprintf("node%d", i+1)
		validatorADI := fmt.Sprintf("acc://validator%d.%s", i+1, c.Network)

		// Generate deterministic keys from seed
		privKey := c.generateKey(nodeID, "validator")
		p2pKey := c.generateKey(nodeID, "p2p")

		c.Nodes[i] = NodeConfig{
			ID:             nodeID,
			ValidatorADI:   validatorADI,
			P2PPort:        c.BasePort + (i * 10),
			APIPort:        c.BasePort + (i * 10) + 1,
			DataDir:        filepath.Join(c.DataDir, nodeID),
			ValidatorKey:   privKey,
			P2PKey:         p2pKey,
			NumWorkers:     c.NumWorkers,
			DAGGCDepth:     c.DAGGCDepth,
			CommitBufferSize: c.CommitBufferSize,
		}
	}

	return nil
}

// generateKey generates a deterministic key from the seed and path components.
func (c *DevnetConfig) generateKey(components ...string) ed25519.PrivateKey {
	keyPath := record.KeyFromHash(c.Seed)
	for _, comp := range components {
		keyPath = keyPath.Append(comp)
	}
	seed := keyPath.Hash()
	return ed25519.NewKeyFromSeed(seed[:])
}

// Save saves the configuration to a file.
func (c *DevnetConfig) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// Load loads a configuration from a file.
func Load(path string) (*DevnetConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var config DevnetConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &config, nil
}

// ConfigPath returns the path to the devnet config file.
func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "devnet.json")
}

// GetPeerAddresses returns the P2P addresses of all nodes except the given one.
func (c *DevnetConfig) GetPeerAddresses(excludeNodeID string) []string {
	var peers []string
	for _, node := range c.Nodes {
		if node.ID != excludeNodeID {
			peers = append(peers, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", node.P2PPort))
		}
	}
	return peers
}

// GetAllValidatorKeys returns all validator public keys in order.
func (c *DevnetConfig) GetAllValidatorKeys() []ed25519.PublicKey {
	keys := make([]ed25519.PublicKey, len(c.Nodes))
	for i, node := range c.Nodes {
		keys[i] = node.ValidatorKey.Public().(ed25519.PublicKey)
	}
	return keys
}
