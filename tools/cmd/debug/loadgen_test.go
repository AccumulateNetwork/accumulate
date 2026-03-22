// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	config := LoadGenConfig{
		Server:         "http://localhost:26660",
		TargetTPS:      100,
		RuntimeSeconds: 300,
		RampUpSeconds:  30,
		Operations: OperationMixConfig{
			LiteToLiteTransfer: 30,
			LiteToADITransfer:  15,
			ADIToADITransfer:   15,
			KeyRotation:        10,
			AddKeyBook:         5,
			AddKeyPage:         5,
			WriteData:          15,
			CreateAccount:      5,
			UpdateKeyWeight:    5,
		},
	}

	tmpFile, err := os.CreateTemp("", "loadgen-config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	encoder := json.NewEncoder(tmpFile)
	if err := encoder.Encode(config); err != nil {
		t.Fatalf("Failed to encode config: %v", err)
	}
	tmpFile.Close()

	// Load the config
	loadedConfig, err := loadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded config
	if loadedConfig.Server != config.Server {
		t.Errorf("Expected server %s, got %s", config.Server, loadedConfig.Server)
	}

	if loadedConfig.TargetTPS != config.TargetTPS {
		t.Errorf("Expected targetTPS %d, got %d", config.TargetTPS, loadedConfig.TargetTPS)
	}

	if loadedConfig.Operations.LiteToLiteTransfer != config.Operations.LiteToLiteTransfer {
		t.Errorf("Expected liteToLiteTransfer %d, got %d",
			config.Operations.LiteToLiteTransfer, loadedConfig.Operations.LiteToLiteTransfer)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    LoadGenConfig
		shouldErr bool
	}{
		{
			name: "valid config",
			config: LoadGenConfig{
				TargetTPS: 100,
				Operations: OperationMixConfig{
					LiteToLiteTransfer: 30,
					LiteToADITransfer:  20,
					ADIToADITransfer:   20,
					KeyRotation:        10,
					AddKeyBook:         5,
					AddKeyPage:         5,
					WriteData:          5,
					CreateAccount:      5,
					UpdateKeyWeight:    0,
				},
			},
			shouldErr: false,
		},
		{
			name: "zero TPS",
			config: LoadGenConfig{
				TargetTPS: 0,
				Operations: OperationMixConfig{
					LiteToLiteTransfer: 100,
				},
			},
			shouldErr: true,
		},
		{
			name: "operations don't sum to 100",
			config: LoadGenConfig{
				TargetTPS: 100,
				Operations: OperationMixConfig{
					LiteToLiteTransfer: 50,
					LiteToADITransfer:  30,
				},
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(&tt.config)
			if (err != nil) != tt.shouldErr {
				t.Errorf("validateConfig() error = %v, shouldErr %v", err, tt.shouldErr)
			}
		})
	}
}

func TestMetrics(t *testing.T) {
	m := newMetrics()

	// Test initial state
	if m.TotalSubmitted.Load() != 0 {
		t.Errorf("Expected initial TotalSubmitted to be 0")
	}

	// Test increment
	m.TotalSubmitted.Add(1)
	if m.TotalSubmitted.Load() != 1 {
		t.Errorf("Expected TotalSubmitted to be 1")
	}

	// Test latency recording
	m.recordLatency(100)
	m.recordLatency(200)

	avgLatency := m.averageLatency()
	expectedAvg := 150.0
	if avgLatency != expectedAvg {
		t.Errorf("Expected average latency %.2f, got %.2f", expectedAvg, avgLatency)
	}

	// Test operation counts
	if _, ok := m.OperationCounts["liteToLiteTransfer"]; !ok {
		t.Errorf("Expected liteToLiteTransfer counter to exist")
	}

	m.OperationCounts["liteToLiteTransfer"].Add(5)
	if m.OperationCounts["liteToLiteTransfer"].Load() != 5 {
		t.Errorf("Expected liteToLiteTransfer count to be 5")
	}
}

func TestRandInt(t *testing.T) {
	// Test with max = 0
	if result := randInt(0); result != 0 {
		t.Errorf("Expected randInt(0) to return 0, got %d", result)
	}

	// Test with max = 10
	for i := 0; i < 100; i++ {
		result := randInt(10)
		if result < 0 || result >= 10 {
			t.Errorf("Expected randInt(10) to return value in [0,10), got %d", result)
		}
	}
}
