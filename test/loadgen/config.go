// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package loadgen

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Config represents the load generator configuration.
type Config struct {
	// Network configuration
	Nodes       []string      `json:"nodes"`
	TPS         int           `json:"tps"`
	Duration    time.Duration `json:"duration"`
	Concurrency int           `json:"concurrency"`

	// Account configuration
	WalletPath string `json:"wallet_path"`
	UseWallet  bool   `json:"use_wallet"`

	// Transaction mix configuration
	TransactionMix []TransactionType `json:"transaction_mix"`

	// Monitoring
	MetricsInterval time.Duration `json:"metrics_interval"`
	OutputFile      string        `json:"output_file"`
}

// TransactionType defines a type of transaction and its weight in the mix.
type TransactionType struct {
	Type   string  `json:"type"`   // "send_tokens", "write_data", "burn_tokens", "add_credits", "create_token_account", "create_data_account"
	Weight float64 `json:"weight"` // Relative weight (0.0-1.0)
	Config any     `json:"config"` // Type-specific configuration
}

// SendTokensConfig configures SendTokens transactions.
type SendTokensConfig struct {
	MinAmount uint64 `json:"min_amount"` // Minimum amount in atomic units
	MaxAmount uint64 `json:"max_amount"` // Maximum amount in atomic units
	SamePartition bool `json:"same_partition"` // Force same partition for testing
	CrossPartition bool `json:"cross_partition"` // Force cross-partition for testing
}

// WriteDataConfig configures WriteData transactions.
type WriteDataConfig struct {
	MinSize int  `json:"min_size"` // Minimum data size in bytes
	MaxSize int  `json:"max_size"` // Maximum data size in bytes
	Scratch bool `json:"scratch"`  // Write to scratch space
}

// BurnTokensConfig configures BurnTokens transactions.
type BurnTokensConfig struct {
	MinAmount uint64 `json:"min_amount"`
	MaxAmount uint64 `json:"max_amount"`
}

// AddCreditsConfig configures AddCredits transactions.
type AddCreditsConfig struct {
	MinCredits uint64  `json:"min_credits"`
	MaxCredits uint64  `json:"max_credits"`
	OraclePrice float64 `json:"oracle_price"` // ACME per credit
}

// CreateAccountConfig configures account creation transactions.
type CreateAccountConfig struct {
	InitialFunds uint64 `json:"initial_funds"` // For token accounts
}

// LoadConfig loads configuration from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if len(c.Nodes) == 0 {
		return fmt.Errorf("no nodes specified")
	}

	if c.TPS <= 0 {
		return fmt.Errorf("TPS must be positive")
	}

	if c.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	if c.Concurrency <= 0 {
		c.Concurrency = c.TPS // Default to TPS
	}

	// Validate transaction mix
	if len(c.TransactionMix) == 0 {
		return fmt.Errorf("no transaction types specified")
	}

	totalWeight := 0.0
	for _, tx := range c.TransactionMix {
		if tx.Weight < 0 {
			return fmt.Errorf("transaction weight cannot be negative: %s", tx.Type)
		}
		totalWeight += tx.Weight
	}

	if totalWeight == 0 {
		return fmt.Errorf("total transaction weight is zero")
	}

	// Normalize weights to sum to 1.0
	for i := range c.TransactionMix {
		c.TransactionMix[i].Weight /= totalWeight
	}

	return nil
}

// Save saves the configuration to a JSON file.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Nodes:       []string{"http://localhost:8080/v3"},
		TPS:         1000,
		Duration:    30 * time.Minute,
		Concurrency: 1000,
		WalletPath:  os.ExpandEnv("$HOME/.accumulate/test-wallet.json"),
		UseWallet:   true,
		TransactionMix: []TransactionType{
			{
				Type:   "send_tokens",
				Weight: 0.7,
				Config: SendTokensConfig{
					MinAmount: 1,
					MaxAmount: 100 * protocol.AcmePrecision,
				},
			},
			{
				Type:   "write_data",
				Weight: 0.2,
				Config: WriteDataConfig{
					MinSize: 100,
					MaxSize: 1000,
				},
			},
			{
				Type:   "burn_tokens",
				Weight: 0.1,
				Config: BurnTokensConfig{
					MinAmount: 1,
					MaxAmount: 10 * protocol.AcmePrecision,
				},
			},
		},
		MetricsInterval: 10 * time.Second,
		OutputFile:      "/tmp/loadgen-results.json",
	}
}

// ExampleConfigs returns a map of example configurations.
func ExampleConfigs() map[string]*Config {
	return map[string]*Config{
		"token-heavy": {
			Nodes:    []string{"http://localhost:8080/v3"},
			TPS:      1000,
			Duration: 30 * time.Minute,
			TransactionMix: []TransactionType{
				{Type: "send_tokens", Weight: 1.0, Config: SendTokensConfig{MinAmount: 1, MaxAmount: 1000}},
			},
		},
		"data-heavy": {
			Nodes:    []string{"http://localhost:8080/v3"},
			TPS:      500,
			Duration: 30 * time.Minute,
			TransactionMix: []TransactionType{
				{Type: "write_data", Weight: 1.0, Config: WriteDataConfig{MinSize: 1000, MaxSize: 10000}},
			},
		},
		"mixed": {
			Nodes:    []string{"http://localhost:8080/v3"},
			TPS:      1000,
			Duration: 30 * time.Minute,
			TransactionMix: []TransactionType{
				{Type: "send_tokens", Weight: 0.5, Config: SendTokensConfig{MinAmount: 1, MaxAmount: 100}},
				{Type: "write_data", Weight: 0.3, Config: WriteDataConfig{MinSize: 100, MaxSize: 1000}},
				{Type: "add_credits", Weight: 0.1, Config: AddCreditsConfig{MinCredits: 100, MaxCredits: 1000, OraclePrice: 0.05}},
				{Type: "burn_tokens", Weight: 0.1, Config: BurnTokensConfig{MinAmount: 1, MaxAmount: 10}},
			},
		},
		"cross-partition": {
			Nodes:    []string{"http://localhost:8080/v3", "http://localhost:8081/v3", "http://localhost:8082/v3"},
			TPS:      1000,
			Duration: 30 * time.Minute,
			TransactionMix: []TransactionType{
				{Type: "send_tokens", Weight: 1.0, Config: SendTokensConfig{MinAmount: 1, MaxAmount: 100, CrossPartition: true}},
			},
		},
	}
}
