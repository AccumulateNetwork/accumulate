package run

import (
	"log/slog"
	"os"
	"testing"

	"github.com/pelletier/go-toml"
	"github.com/multiformats/go-multiaddr"
)

func TestGenerateCyclopsConfig(t *testing.T) {
	// Create the configuration structure
	listenAddr := multiaddr.StringCast("/ip4/0.0.0.0/tcp/26656")
	enableSnapshots := false
	
	cfg := &Config{
		Network: "cyclops",
		Services: []Service{},
		Configurations: []Configuration{
			&CoreValidatorConfiguration{
				Mode:            CoreValidatorModeDual,
				Listen:          Multiaddr(listenAddr),
				BVN:             "BVN0",
				EnableSnapshots: &enableSnapshots,
			},
		},
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
			},
		},
		P2P: &P2P{
			Key: &CometNodeKeyFile{
				Path: "config/node_key.json",
			},
		},
	}

	// Marshal to TOML
	data, err := toml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Error marshaling config: %v", err)
	}

	// Add the old-style API configuration to satisfy the daemon system
	// This is needed because the daemon still tries to start its own API server
	apiConfig := `
[api]
listen-address = "http://0.0.0.0:26660"
`
	data = append(data, []byte(apiConfig)...)

	// Re-parse to validate
	_, err = toml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Error marshaling config: %v", err)
	}

	// Write to file
	err = os.WriteFile("/tmp/cyclops/node/artifacts/accumulate_generated.toml", data, 0644)
	if err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}

	t.Logf("Generated configuration file: /tmp/cyclops/node/artifacts/accumulate_generated.toml")
	t.Logf("Configuration:\n%s", string(data))
}
