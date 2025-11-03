package server

import (
	"os"
	"path/filepath"
)

// Config holds the MCP server configuration
type Config struct {
	// WalletDir is the path to the wallet directory
	WalletDir string

	// Network is the network to use (mainnet, testnet, devnet, or custom URL)
	Network string

	// Server is the API server URL (derived from Network if not custom)
	Server string
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		WalletDir: filepath.Join(home, ".accumulate", "wallet"),
		Network:   "mainnet",
		Server:    "https://mainnet.accumulatenetwork.io/v3",
	}
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// Allow override via environment variables
	if walletDir := os.Getenv("ACCUMULATE_WALLET_DIR"); walletDir != "" {
		cfg.WalletDir = walletDir
	}

	if network := os.Getenv("ACCUMULATE_NETWORK"); network != "" {
		cfg.Network = network
		cfg.Server = getServerURL(network)
	}

	if server := os.Getenv("ACCUMULATE_SERVER"); server != "" {
		cfg.Server = server
		cfg.Network = "custom"
	}

	return cfg
}

// getServerURL returns the API server URL for a given network name
func getServerURL(network string) string {
	switch network {
	case "mainnet":
		return "https://mainnet.accumulatenetwork.io/v3"
	case "testnet":
		return "https://testnet.accumulatenetwork.io/v3"
	case "devnet":
		return "http://127.0.0.1:26660/v2"
	default:
		// Assume it's a custom URL
		return network
	}
}

// SetNetwork updates the network and server URL
func (c *Config) SetNetwork(network string) {
	c.Network = network
	c.Server = getServerURL(network)
}
