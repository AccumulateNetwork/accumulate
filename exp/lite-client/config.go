// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"time"
)

// Config holds configuration settings for the lite client
type Config struct {
	// Network configuration
	Network NetworkConfig `json:"network"`
	
	// Cache configuration
	Cache CacheConfig `json:"cache"`
	
	// API configuration
	API APIConfig `json:"api"`
	
	// Debug settings
	Debug DebugConfig `json:"debug"`
}

// NetworkConfig contains network endpoint and connection settings
type NetworkConfig struct {
	// ServerURL is the base URL for the Accumulate API server
	ServerURL string `json:"serverUrl"`
	
	// Network name (mainnet, testnet, devnet, etc.)
	NetworkName string `json:"networkName"`
	
	// Connection timeout for API requests
	Timeout time.Duration `json:"timeout"`
	
	// Maximum number of retry attempts for failed requests
	MaxRetries int `json:"maxRetries"`
	
	// Delay between retry attempts
	RetryDelay time.Duration `json:"retryDelay"`
}

// CacheConfig contains caching behavior settings
type CacheConfig struct {
	// Default TTL for cached data
	DefaultTTL time.Duration `json:"defaultTtl"`
	
	// TTL for account data specifically
	AccountDataTTL time.Duration `json:"accountDataTtl"`
	
	// TTL for transaction data
	TransactionTTL time.Duration `json:"transactionTtl"`
	
	// TTL for proof data
	ProofTTL time.Duration `json:"proofTtl"`
	
	// Maximum number of entries in cache (0 = unlimited)
	MaxEntries int `json:"maxEntries"`
	
	// Enable/disable persistent caching to disk
	PersistentCache bool `json:"persistentCache"`
	
	// Directory for persistent cache files
	CacheDirectory string `json:"cacheDirectory"`
}

// APIConfig contains API behavior settings
type APIConfig struct {
	// Maximum number of concurrent API requests
	MaxConcurrentRequests int `json:"maxConcurrentRequests"`
	
	// Rate limit: requests per second (0 = no limit)
	RateLimit int `json:"rateLimit"`
	
	// Enable/disable automatic proof validation
	AutoValidateProofs bool `json:"autoValidateProofs"`
	
	// Batch size for bulk operations
	BatchSize int `json:"batchSize"`
}

// DebugConfig contains debugging and logging settings
type DebugConfig struct {
	// Enable debug logging
	EnableDebug bool `json:"enableDebug"`
	
	// Log API requests and responses
	LogAPIRequests bool `json:"logApiRequests"`
	
	// Log cache operations
	LogCacheOperations bool `json:"logCacheOperations"`
	
	// Log proof generation details
	LogProofGeneration bool `json:"logProofGeneration"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Network: NetworkConfig{
			ServerURL:   "https://mainnet.accumulatenetwork.io/v2",
			NetworkName: "mainnet",
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			RetryDelay:  1 * time.Second,
		},
		Cache: CacheConfig{
			DefaultTTL:      5 * time.Minute,
			AccountDataTTL:  10 * time.Minute,
			TransactionTTL:  30 * time.Minute,
			ProofTTL:        15 * time.Minute,
			MaxEntries:      10000,
			PersistentCache: false,
			CacheDirectory:  "./cache",
		},
		API: APIConfig{
			MaxConcurrentRequests: 10,
			RateLimit:            0, // No rate limit by default
			AutoValidateProofs:   true,
			BatchSize:           50,
		},
		Debug: DebugConfig{
			EnableDebug:        false,
			LogAPIRequests:     false,
			LogCacheOperations: false,
			LogProofGeneration: false,
		},
	}
}

// TestnetConfig returns configuration for Accumulate testnet
func TestnetConfig() *Config {
	config := DefaultConfig()
	config.Network.ServerURL = "https://testnet.accumulatenetwork.io/v2"
	config.Network.NetworkName = "testnet"
	return config
}

// DevnetConfig returns configuration for local development
func DevnetConfig() *Config {
	config := DefaultConfig()
	config.Network.ServerURL = "http://localhost:26660/v2"
	config.Network.NetworkName = "devnet"
	config.Debug.EnableDebug = true
	config.Debug.LogAPIRequests = true
	return config
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Network.ServerURL == "" {
		return ErrInvalidConfig{Field: "network.serverUrl", Reason: "cannot be empty"}
	}
	
	if c.Network.Timeout <= 0 {
		return ErrInvalidConfig{Field: "network.timeout", Reason: "must be positive"}
	}
	
	if c.Cache.DefaultTTL <= 0 {
		return ErrInvalidConfig{Field: "cache.defaultTtl", Reason: "must be positive"}
	}
	
	if c.API.MaxConcurrentRequests <= 0 {
		return ErrInvalidConfig{Field: "api.maxConcurrentRequests", Reason: "must be positive"}
	}
	
	return nil
}

// ErrInvalidConfig represents a configuration validation error
type ErrInvalidConfig struct {
	Field  string
	Reason string
}

func (e ErrInvalidConfig) Error() string {
	return "invalid config field " + e.Field + ": " + e.Reason
}
