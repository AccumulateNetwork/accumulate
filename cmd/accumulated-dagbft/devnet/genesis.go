//go:build dagbft

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package devnet

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ValidatorInfo represents a validator in the genesis document.
type ValidatorInfo struct {
	// ADI is the validator's ADI URL (e.g., acc://validator1.dagbft).
	ADI string `json:"adi"`

	// PublicKey is the validator's ed25519 public key.
	PublicKey ed25519.PublicKey `json:"publicKey"`

	// Power is the validator's voting power.
	Power int64 `json:"power"`
}

// MarshalJSON implements custom JSON marshaling for ValidatorInfo.
func (v ValidatorInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ADI       string `json:"adi"`
		PublicKey string `json:"publicKey"`
		Power     int64  `json:"power"`
	}{
		ADI:       v.ADI,
		PublicKey: hex.EncodeToString(v.PublicKey),
		Power:     v.Power,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for ValidatorInfo.
func (v *ValidatorInfo) UnmarshalJSON(data []byte) error {
	var raw struct {
		ADI       string `json:"adi"`
		PublicKey string `json:"publicKey"`
		Power     int64  `json:"power"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.ADI = raw.ADI
	v.Power = raw.Power

	pubKey, err := hex.DecodeString(raw.PublicKey)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: expected %d, got %d", ed25519.PublicKeySize, len(pubKey))
	}
	v.PublicKey = pubKey
	return nil
}

// GenesisDoc represents the genesis document for a DAG-BFT network.
type GenesisDoc struct {
	// ChainID is the network identifier.
	ChainID string `json:"chainId"`

	// GenesisTime is the timestamp when the network started.
	GenesisTime time.Time `json:"genesisTime"`

	// Validators is the initial validator set.
	Validators []ValidatorInfo `json:"validators"`

	// InitialHeight is the starting block height.
	InitialHeight int64 `json:"initialHeight"`

	// ConsensusParams holds consensus parameters.
	ConsensusParams ConsensusParams `json:"consensusParams"`
}

// ConsensusParams holds DAG-BFT consensus parameters.
type ConsensusParams struct {
	// BlockInterval is the target time between blocks.
	BlockInterval time.Duration `json:"blockInterval"`

	// NumWorkers is the default number of workers per node.
	NumWorkers int `json:"numWorkers"`

	// DAGGCDepth is the DAG garbage collection depth.
	DAGGCDepth int `json:"dagGCDepth"`

	// CommitBufferSize is the commit certificate buffer size.
	CommitBufferSize int `json:"commitBufferSize"`
}

// MarshalJSON implements custom JSON marshaling for ConsensusParams.
func (p ConsensusParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		BlockInterval    string `json:"blockInterval"`
		NumWorkers       int    `json:"numWorkers"`
		DAGGCDepth       int    `json:"dagGCDepth"`
		CommitBufferSize int    `json:"commitBufferSize"`
	}{
		BlockInterval:    p.BlockInterval.String(),
		NumWorkers:       p.NumWorkers,
		DAGGCDepth:       p.DAGGCDepth,
		CommitBufferSize: p.CommitBufferSize,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for ConsensusParams.
func (p *ConsensusParams) UnmarshalJSON(data []byte) error {
	var raw struct {
		BlockInterval    string `json:"blockInterval"`
		NumWorkers       int    `json:"numWorkers"`
		DAGGCDepth       int    `json:"dagGCDepth"`
		CommitBufferSize int    `json:"commitBufferSize"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	duration, err := time.ParseDuration(raw.BlockInterval)
	if err != nil {
		return fmt.Errorf("parse block interval: %w", err)
	}
	p.BlockInterval = duration
	p.NumWorkers = raw.NumWorkers
	p.DAGGCDepth = raw.DAGGCDepth
	p.CommitBufferSize = raw.CommitBufferSize
	return nil
}

// NewGenesisDoc creates a new genesis document from a devnet configuration.
func NewGenesisDoc(config *DevnetConfig) (*GenesisDoc, error) {
	if len(config.Nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}

	validators := make([]ValidatorInfo, len(config.Nodes))
	for i, node := range config.Nodes {
		validators[i] = ValidatorInfo{
			ADI:       node.ValidatorADI,
			PublicKey: node.ValidatorPublicKey(),
			Power:     1, // Equal voting power
		}
	}

	return &GenesisDoc{
		ChainID:       config.Network,
		GenesisTime:   time.Now().UTC(),
		Validators:    validators,
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			BlockInterval:    3 * time.Second,
			NumWorkers:       config.NumWorkers,
			DAGGCDepth:       config.DAGGCDepth,
			CommitBufferSize: config.CommitBufferSize,
		},
	}, nil
}

// Save writes the genesis document to a file.
func (g *GenesisDoc) Save(path string) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write genesis: %w", err)
	}
	return nil
}

// LoadGenesis loads a genesis document from a file.
func LoadGenesis(path string) (*GenesisDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis: %w", err)
	}
	var genesis GenesisDoc
	if err := json.Unmarshal(data, &genesis); err != nil {
		return nil, fmt.Errorf("unmarshal genesis: %w", err)
	}
	return &genesis, nil
}

// TotalPower returns the total voting power of all validators.
func (g *GenesisDoc) TotalPower() int64 {
	var total int64
	for _, v := range g.Validators {
		total += v.Power
	}
	return total
}

// QuorumPower returns the minimum power needed for a quorum (2/3 + 1).
func (g *GenesisDoc) QuorumPower() int64 {
	total := g.TotalPower()
	return (2*total)/3 + 1
}

// Validate checks that the genesis document is valid.
func (g *GenesisDoc) Validate() error {
	if g.ChainID == "" {
		return fmt.Errorf("chain ID is required")
	}
	if len(g.Validators) == 0 {
		return fmt.Errorf("at least one validator is required")
	}
	if len(g.Validators) < 3 {
		return fmt.Errorf("at least 3 validators required for BFT consensus")
	}

	seen := make(map[string]bool)
	for i, v := range g.Validators {
		if v.ADI == "" {
			return fmt.Errorf("validator %d: ADI is required", i)
		}
		if len(v.PublicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("validator %d: invalid public key size", i)
		}
		if v.Power <= 0 {
			return fmt.Errorf("validator %d: power must be positive", i)
		}
		keyHex := hex.EncodeToString(v.PublicKey)
		if seen[keyHex] {
			return fmt.Errorf("validator %d: duplicate public key", i)
		}
		seen[keyHex] = true
	}

	return nil
}
