// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package metrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServer_StartStop(t *testing.T) {
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0", // Use port 0 to get a random available port
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestServer_DoubleStart(t *testing.T) {
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}()

	// Second start should fail
	if err := server.Start(); err == nil {
		t.Error("Expected error on double start, got nil")
	}
}

func TestServer_MetricsEndpoint(t *testing.T) {
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:19090",
		Path:    "/metrics",
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test metrics endpoint
	resp, err := http.Get("http://127.0.0.1:19090/metrics")
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	// Verify we get Prometheus format output
	if !strings.Contains(string(body), "# HELP") {
		t.Error("Expected Prometheus format output with # HELP")
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:19091",
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get("http://127.0.0.1:19091/health")
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("Expected 'OK', got '%s'", string(body))
	}
}

func TestServerConfig_Defaults(t *testing.T) {
	server := NewServer(ServerConfig{})

	if server.Address() != ":9090" {
		t.Errorf("Expected address :9090, got %s", server.Address())
	}

	if server.Path() != "/metrics" {
		t.Errorf("Expected path /metrics, got %s", server.Path())
	}
}
