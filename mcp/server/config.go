package server

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds the MCP server configuration
type Config struct {
	// WalletDir is the path to the wallet directory
	WalletDir string

	// Network is the network to use (mainnet, testnet, devnet, or custom URL)
	Network string

	// Server is the API server URL (derived from Network if not custom)
	Server string

	// DockerImage is the Docker image to use for follower deployment
	DockerImage string

	// Timeout configuration
	APITimeout      time.Duration // Timeout for API calls (default: 30s)
	BuildTimeout    time.Duration // Timeout for build operations (default: 10m)
	DatabaseTimeout time.Duration // Timeout for database operations (default: 60s)
	LogAnalysisTimeout time.Duration // Timeout for log analysis (default: 30s)
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		WalletDir:          filepath.Join(home, ".accumulate", "wallet"),
		Network:            "mainnet",
		Server:             "https://mainnet.accumulatenetwork.io/v3",
		DockerImage:        "registry.gitlab.com/accumulatenetwork/accumulate:v1.4.0",
		APITimeout:         30 * time.Second,
		BuildTimeout:       10 * time.Minute,
		DatabaseTimeout:    60 * time.Second,
		LogAnalysisTimeout: 30 * time.Second,
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

	if dockerImage := os.Getenv("ACCUMULATE_DOCKER_IMAGE"); dockerImage != "" {
		cfg.DockerImage = dockerImage
	}

	// Timeout configuration (in seconds)
	if timeout := os.Getenv("MCP_API_TIMEOUT"); timeout != "" {
		if secs, err := strconv.Atoi(timeout); err == nil && secs > 0 {
			cfg.APITimeout = time.Duration(secs) * time.Second
		}
	}

	if timeout := os.Getenv("MCP_BUILD_TIMEOUT"); timeout != "" {
		if secs, err := strconv.Atoi(timeout); err == nil && secs > 0 {
			cfg.BuildTimeout = time.Duration(secs) * time.Second
		}
	}

	if timeout := os.Getenv("MCP_DATABASE_TIMEOUT"); timeout != "" {
		if secs, err := strconv.Atoi(timeout); err == nil && secs > 0 {
			cfg.DatabaseTimeout = time.Duration(secs) * time.Second
		}
	}

	if timeout := os.Getenv("MCP_LOG_ANALYSIS_TIMEOUT"); timeout != "" {
		if secs, err := strconv.Atoi(timeout); err == nil && secs > 0 {
			cfg.LogAnalysisTimeout = time.Duration(secs) * time.Second
		}
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
