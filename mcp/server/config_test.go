package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Should have default wallet dir
	if cfg.WalletDir == "" {
		t.Error("default WalletDir is empty")
	}

	// Should contain .accumulate/wallet
	if !filepath.IsAbs(cfg.WalletDir) {
		t.Error("default WalletDir should be absolute path")
	}

	// Should default to mainnet
	if cfg.Network != "mainnet" {
		t.Errorf("expected default Network 'mainnet', got '%s'", cfg.Network)
	}

	// Should have mainnet server URL
	if cfg.Server != "https://mainnet.accumulatenetwork.io/v3" {
		t.Errorf("expected default Server 'https://mainnet.accumulatenetwork.io/v3', got '%s'", cfg.Server)
	}
}

func TestGetServerURL_Mainnet(t *testing.T) {
	url := getServerURL("mainnet")
	expected := "https://mainnet.accumulatenetwork.io/v3"

	if url != expected {
		t.Errorf("expected '%s', got '%s'", expected, url)
	}
}

func TestGetServerURL_Testnet(t *testing.T) {
	url := getServerURL("testnet")
	expected := "https://testnet.accumulatenetwork.io/v3"

	if url != expected {
		t.Errorf("expected '%s', got '%s'", expected, url)
	}
}

func TestGetServerURL_Devnet(t *testing.T) {
	url := getServerURL("devnet")
	expected := "http://127.0.0.1:26660/v2"

	if url != expected {
		t.Errorf("expected '%s', got '%s'", expected, url)
	}
}

func TestGetServerURL_Custom(t *testing.T) {
	customURL := "http://custom-server.example.com:8080/v3"
	url := getServerURL(customURL)

	// Should return the custom URL as-is
	if url != customURL {
		t.Errorf("expected '%s', got '%s'", customURL, url)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("ACCUMULATE_WALLET_DIR")
	os.Unsetenv("ACCUMULATE_NETWORK")
	os.Unsetenv("ACCUMULATE_SERVER")

	cfg := LoadConfig()

	// Should have defaults
	if cfg.Network != "mainnet" {
		t.Errorf("expected default network 'mainnet', got '%s'", cfg.Network)
	}

	if cfg.Server != "https://mainnet.accumulatenetwork.io/v3" {
		t.Errorf("expected mainnet server URL, got '%s'", cfg.Server)
	}
}

func TestLoadConfig_WalletDirEnv(t *testing.T) {
	testDir := "/test/wallet/dir"
	os.Setenv("ACCUMULATE_WALLET_DIR", testDir)
	defer os.Unsetenv("ACCUMULATE_WALLET_DIR")

	cfg := LoadConfig()

	if cfg.WalletDir != testDir {
		t.Errorf("expected WalletDir '%s', got '%s'", testDir, cfg.WalletDir)
	}
}

func TestLoadConfig_NetworkEnv(t *testing.T) {
	os.Setenv("ACCUMULATE_NETWORK", "testnet")
	defer os.Unsetenv("ACCUMULATE_NETWORK")

	cfg := LoadConfig()

	if cfg.Network != "testnet" {
		t.Errorf("expected Network 'testnet', got '%s'", cfg.Network)
	}

	if cfg.Server != "https://testnet.accumulatenetwork.io/v3" {
		t.Errorf("expected testnet server URL, got '%s'", cfg.Server)
	}
}

func TestLoadConfig_ServerEnv(t *testing.T) {
	customServer := "http://custom.example.com:9090/v3"
	os.Setenv("ACCUMULATE_SERVER", customServer)
	defer os.Unsetenv("ACCUMULATE_SERVER")

	// Also set network to verify server overrides it
	os.Setenv("ACCUMULATE_NETWORK", "mainnet")
	defer os.Unsetenv("ACCUMULATE_NETWORK")

	cfg := LoadConfig()

	// Server env var should override network
	if cfg.Server != customServer {
		t.Errorf("expected Server '%s', got '%s'", customServer, cfg.Server)
	}

	// Network should be set to "custom"
	if cfg.Network != "custom" {
		t.Errorf("expected Network 'custom' when server is explicitly set, got '%s'", cfg.Network)
	}
}

func TestLoadConfig_DevnetNetwork(t *testing.T) {
	os.Setenv("ACCUMULATE_NETWORK", "devnet")
	defer os.Unsetenv("ACCUMULATE_NETWORK")

	cfg := LoadConfig()

	if cfg.Network != "devnet" {
		t.Errorf("expected Network 'devnet', got '%s'", cfg.Network)
	}

	if cfg.Server != "http://127.0.0.1:26660/v2" {
		t.Errorf("expected devnet server URL, got '%s'", cfg.Server)
	}
}

func TestSetNetwork_Mainnet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetNetwork("mainnet")

	if cfg.Network != "mainnet" {
		t.Errorf("expected Network 'mainnet', got '%s'", cfg.Network)
	}

	if cfg.Server != "https://mainnet.accumulatenetwork.io/v3" {
		t.Errorf("expected mainnet server URL, got '%s'", cfg.Server)
	}
}

func TestSetNetwork_Testnet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetNetwork("testnet")

	if cfg.Network != "testnet" {
		t.Errorf("expected Network 'testnet', got '%s'", cfg.Network)
	}

	if cfg.Server != "https://testnet.accumulatenetwork.io/v3" {
		t.Errorf("expected testnet server URL, got '%s'", cfg.Server)
	}
}

func TestSetNetwork_Devnet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetNetwork("devnet")

	if cfg.Network != "devnet" {
		t.Errorf("expected Network 'devnet', got '%s'", cfg.Network)
	}

	if cfg.Server != "http://127.0.0.1:26660/v2" {
		t.Errorf("expected devnet server URL, got '%s'", cfg.Server)
	}
}

func TestSetNetwork_Custom(t *testing.T) {
	cfg := DefaultConfig()
	customURL := "http://my-node.example.com/v3"
	cfg.SetNetwork(customURL)

	if cfg.Network != customURL {
		t.Errorf("expected Network '%s', got '%s'", customURL, cfg.Network)
	}

	if cfg.Server != customURL {
		t.Errorf("expected Server '%s', got '%s'", customURL, cfg.Server)
	}
}

func TestLoadConfig_AllEnvVars(t *testing.T) {
	// Set all environment variables
	testWalletDir := "/test/wallet"
	testNetwork := "devnet"

	os.Setenv("ACCUMULATE_WALLET_DIR", testWalletDir)
	os.Setenv("ACCUMULATE_NETWORK", testNetwork)
	defer func() {
		os.Unsetenv("ACCUMULATE_WALLET_DIR")
		os.Unsetenv("ACCUMULATE_NETWORK")
	}()

	cfg := LoadConfig()

	if cfg.WalletDir != testWalletDir {
		t.Errorf("expected WalletDir '%s', got '%s'", testWalletDir, cfg.WalletDir)
	}

	if cfg.Network != testNetwork {
		t.Errorf("expected Network '%s', got '%s'", testNetwork, cfg.Network)
	}

	if cfg.Server != "http://127.0.0.1:26660/v2" {
		t.Errorf("expected devnet server URL, got '%s'", cfg.Server)
	}
}
